package supermiddleware

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/Sirupsen/logrus"

	"github.com/docker/distribution"
	"github.com/docker/distribution/configuration"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/reference"
	"github.com/docker/distribution/registry/auth"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	_ "github.com/docker/distribution/registry/storage/driver/inmemory"
)

func init() {
	// FIXME(dmage): update logrus and remove next line.
	logrus.WithField("init formatter", "suppress data race error").String()
}

type log struct {
	records []string
}

func (l *log) Reset() {
	l.records = nil
}

func (l *log) Record(format string, v ...interface{}) {
	l.records = append(l.records, fmt.Sprintf(format, v...))
}

func (l *log) Compare(expected []string) error {
	if len(l.records) == 0 && len(expected) == 0 {
		return nil
	}
	if !reflect.DeepEqual(l.records, expected) {
		return fmt.Errorf("got %v, want %v", l.records, expected)
	}
	return nil
}

type testAccessController struct {
	log *log
}

func (ac *testAccessController) Authorized(ctx context.Context, access ...auth.Access) (context.Context, error) {
	var args []string
	for _, a := range access {
		args = append(args, fmt.Sprintf("%s:%s:%s:%s", a.Type, a.Class, a.Name, a.Action))
	}
	ac.log.Record("AccessController(%s)", strings.Join(args, ", "))
	return ctx, nil
}

type testRegistry struct {
	distribution.Namespace
	log *log
}

func (reg *testRegistry) Repository(ctx context.Context, named reference.Named) (distribution.Repository, error) {
	reg.log.Record("%s: enter Registry.Repository", named)
	defer reg.log.Record("%s: leave Registry.Repository", named)
	return reg.Namespace.Repository(ctx, named)
}

type testBlobDesctiptorService struct {
	distribution.BlobDescriptorService
	log  *log
	repo string
	name string
}

func (bds *testBlobDesctiptorService) Stat(ctx context.Context, dgst digest.Digest) (distribution.Descriptor, error) {
	bds.log.Record("%s: enter BlobDescriptorService(%s).Stat", bds.repo, bds.name)
	defer bds.log.Record("%s: leave BlobDescriptorService(%s).Stat", bds.repo, bds.name)
	return bds.BlobDescriptorService.Stat(ctx, dgst)
}

type testApp struct {
	log *log
}

func (app *testApp) Auth(options map[string]interface{}) (auth.AccessController, error) {
	return &testAccessController{
		log: app.log,
	}, nil
}

func (app *testApp) Storage(driver storagedriver.StorageDriver, options map[string]interface{}) (storagedriver.StorageDriver, error) {
	return driver, nil
}

func (app *testApp) Registry(registry distribution.Namespace, options map[string]interface{}) (distribution.Namespace, error) {
	return &testRegistry{
		Namespace: registry,
		log:       app.log,
	}, nil
}

func (app *testApp) Repository(ctx context.Context, repo distribution.Repository, crossmount bool) (distribution.Repository, distribution.BlobDescriptorServiceFactory, error) {
	name := "regular"
	if crossmount {
		name = "crossmount"
	}

	bdsf := blobDescriptorServiceFactoryFunc(func(svc distribution.BlobDescriptorService) distribution.BlobDescriptorService {
		return &testBlobDesctiptorService{
			BlobDescriptorService: svc,
			log:  app.log,
			repo: repo.Named().String(),
			name: name,
		}
	})

	return repo, bdsf, nil
}

func TestApp(t *testing.T) {
	log := &log{}

	ctx := context.Background()
	app := &testApp{log: log}
	config := &configuration.Configuration{
		Auth: configuration.Auth{
			Name: nil,
		},
		Storage: configuration.Storage{
			"inmemory": nil,
		},
		Middleware: map[string][]configuration.Middleware{
			"registry":   {{Name: Name}},
			"repository": {{Name: Name}},
			"storage":    {{Name: Name}},
		},
	}
	handler := NewApp(ctx, config, app)

	server := httptest.NewServer(handler)
	defer server.Close()

	fooDigest := "sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"
	fooContent := []byte("foo")

	var location string
	serverURL := func(s string) func() string { return func() string { return server.URL + s } }
	lastLocation := func(s string) func() string { return func() string { return location + s } }

	for _, test := range []struct {
		name         string
		method       string
		url          func() string
		body         io.Reader
		expectStatus int
		expectLog    []string
	}{
		{
			name:         "foo_get_blob",
			method:       "HEAD",
			url:          serverURL("/v2/foo/blobs/" + fooDigest + "/"),
			expectStatus: http.StatusNotFound,
			expectLog: []string{
				"AccessController(repository::foo:pull)",
				"foo: enter Registry.Repository",
				"foo: leave Registry.Repository",
				"foo: enter BlobDescriptorService(regular).Stat",
				"foo: leave BlobDescriptorService(regular).Stat",
			},
		},
		{
			name:         "foo_start_blob_upload",
			method:       "POST",
			url:          serverURL("/v2/foo/blobs/uploads/"),
			expectStatus: http.StatusAccepted,
			expectLog: []string{
				"AccessController(repository::foo:pull, repository::foo:push)",
				"foo: enter Registry.Repository",
				"foo: leave Registry.Repository",
			},
		},
		{
			name:         "foo_put_blob_foo",
			method:       "PUT",
			url:          lastLocation("&digest=" + fooDigest),
			body:         bytes.NewReader(fooContent),
			expectStatus: http.StatusCreated,
			expectLog: []string{
				"AccessController(repository::foo:pull, repository::foo:push)",
				"foo: enter Registry.Repository",
				"foo: leave Registry.Repository",
			},
		},
		{
			name:         "bar_mount_blob",
			method:       "POST",
			url:          serverURL("/v2/bar/blobs/uploads/?mount=" + fooDigest + "&from=foo"),
			expectStatus: http.StatusCreated,
			expectLog: []string{
				"AccessController(repository::bar:pull, repository::bar:push, repository::foo:pull)",
				"bar: enter Registry.Repository",
				"bar: leave Registry.Repository",
				"foo: enter BlobDescriptorService(crossmount).Stat",
				"foo: leave BlobDescriptorService(crossmount).Stat",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			req, err := http.NewRequest(test.method, test.url(), test.body)
			if err != nil {
				t.Fatal(err)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			location = resp.Header.Get("Location")

			if resp.StatusCode != test.expectStatus {
				t.Errorf("got status %d (%s), want %d", resp.StatusCode, resp.Status, test.expectStatus)
			}

			if err := log.Compare(test.expectLog); err != nil {
				t.Fatal(err)
			}
			log.Reset()
		})
	}
}
