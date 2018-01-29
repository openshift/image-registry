package integration

import (
	gocontext "context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/docker/distribution/context"
	digest "github.com/docker/distribution/digest"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/inmemory"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	imageapiv1 "github.com/openshift/api/image/v1"
	imageclientv1 "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"

	"github.com/openshift/image-registry/pkg/testframework"
	"github.com/openshift/image-registry/pkg/testutil"
	"github.com/openshift/image-registry/test/internal/storage"
	"github.com/openshift/image-registry/test/internal/storagepath"
)

type driver struct {
	storagedriver.StorageDriver

	mu      sync.Mutex
	demands map[string]chan struct{}
}

var _ storagedriver.StorageDriver = &driver{}

func newDriver() *driver {
	return &driver{
		StorageDriver: inmemory.New(),
		demands:       make(map[string]chan struct{}),
	}
}

func (d *driver) WaitFor(ctx context.Context, paths ...string) error {
	type pending struct {
		path string
		c    <-chan struct{}
	}
	var queue []pending

	d.mu.Lock()
	for _, path := range paths {
		if _, err := d.Stat(ctx, path); err != nil {
			if _, ok := err.(storagedriver.PathNotFoundError); ok {
				c, ok := d.demands[path]
				if !ok {
					c = make(chan struct{})
					d.demands[path] = c
				}
				queue = append(queue, pending{path: path, c: c})
			} else {
				d.mu.Unlock()
				return fmt.Errorf("stat %s: %v", path, err)
			}
		}
	}
	d.mu.Unlock()

	for _, p := range queue {
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for %s: %v", p.path, ctx.Err())
		case <-p.c:
		}
	}
	return nil
}

func (d *driver) PutContent(ctx context.Context, path string, content []byte) error {
	err := d.StorageDriver.PutContent(ctx, path, content)
	if err == nil {
		d.mu.Lock()
		c, ok := d.demands[path]
		if ok {
			close(c)
			delete(d.demands, path)
		}
		d.mu.Unlock()
	}
	return err
}

func TestPullthroughBlob(t *testing.T) {
	config := []byte("{}")
	configDigest := digest.FromBytes(config)

	foo := []byte("foo")
	fooDigest := digest.FromBytes(foo)

	manifestMediaType := "application/vnd.docker.distribution.manifest.v2+json"
	manifest, err := json.Marshal(map[string]interface{}{
		"schemaVersion": 2,
		"mediaType":     manifestMediaType,
		"config": map[string]interface{}{
			"mediaType": "application/vnd.docker.container.image.v1+json",
			"size":      len(config),
			"digest":    configDigest.String(),
		},
		"layers": []map[string]interface{}{
			{
				"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
				"size":      len(foo),
				"digest":    fooDigest.String(),
			},
		},
	})
	if err != nil {
		t.Fatalf("unable to marshal manifest: %v", err)
	}
	manifestDigest := digest.FromBytes(manifest)

	remoteRegistryUnavailable := false
	ts := testframework.NewHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := fmt.Sprintf("%s %s", r.Method, r.URL.Path)

		t.Logf("remote registry: %s", req)

		w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")

		if remoteRegistryUnavailable {
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}

		switch req {
		case "GET /v2/":
			w.Write([]byte(`{}`))
		case "GET /v2/remoteimage/manifests/latest", "GET /v2/remoteimage/manifests/" + manifestDigest.String():
			w.Header().Set("Content-Type", manifestMediaType)
			w.Write(manifest)
		case "HEAD /v2/remoteimage/blobs/" + configDigest.String():
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(config)))
			w.WriteHeader(http.StatusOK)
		case "GET /v2/remoteimage/blobs/" + configDigest.String():
			w.Write(config)
		case "HEAD /v2/remoteimage/blobs/" + fooDigest.String():
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(foo)))
			w.WriteHeader(http.StatusOK)
		case "GET /v2/remoteimage/blobs/" + fooDigest.String():
			w.Write(foo)
		default:
			t.Errorf("error: remote registry got unexpected request %s: %#+v", req, r)
			http.Error(w, "unable to handle the request", http.StatusInternalServerError)
		}
	}))
	defer ts.Close()

	master := testframework.NewMaster(t)
	defer master.Close()

	testuser := master.CreateUser("testuser", "testp@ssw0rd")
	testproject := master.CreateProject("testproject", testuser.Name)
	teststreamName := "pullthrough"

	t.Log("=== import image")

	imageClient := imageclientv1.NewForConfigOrDie(master.AdminKubeConfig())

	isi, err := imageClient.ImageStreamImports(testproject.Name).Create(&imageapiv1.ImageStreamImport{
		ObjectMeta: metav1.ObjectMeta{
			Name: teststreamName,
		},
		Spec: imageapiv1.ImageStreamImportSpec{
			Import: true,
			Images: []imageapiv1.ImageImportSpec{
				{
					From: corev1.ObjectReference{
						Kind: "DockerImage",
						Name: fmt.Sprintf("%s/remoteimage:latest", ts.URL.Host),
					},
					ImportPolicy: imageapiv1.TagImportPolicy{
						Insecure: true,
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	teststream, err := imageClient.ImageStreams(testproject.Name).Get(teststreamName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if len(teststream.Status.Tags) == 0 {
		t.Fatalf("failed to import image: %#+v %#+v", isi, teststream)
	}

	t.Log("=== mirror image")

	driver := newDriver()
	registry := master.StartRegistry(t, storage.WithDriver(driver))
	defer registry.Close()

	repo := registry.Repository(testproject.Name, teststream.Name, testuser)

	ctx := context.Background()
	ctx = testutil.WithTestLogger(ctx, t)

	/* Pull the image to start mirroring */
	mediatype, dgst, err := testutil.VerifyRemoteImage(ctx, repo, "latest")
	if err != nil {
		t.Fatal(err)
	}
	if mediatype != manifestMediaType {
		t.Fatalf("manifest mediatype: got %q, want %q", mediatype, manifestMediaType)
	}
	if dgst != manifestDigest {
		t.Fatalf("manifest digest: got %q, want %q", dgst, manifestDigest)
	}

	/* Wait for mirroring to complete */
	timeoutContext, timeoutCancel := gocontext.WithTimeout(ctx, 10*time.Second)
	if err := driver.WaitFor(
		timeoutContext,
		storagepath.Layer(testproject.Name+"/"+teststream.Name, configDigest),
		storagepath.Layer(testproject.Name+"/"+teststream.Name, fooDigest),
		storagepath.Manifest(testproject.Name+"/"+teststream.Name, manifestDigest),
	); err != nil {
		t.Fatal(err)
	}
	timeoutCancel()

	t.Log("=== check if image pullable without remote registry")

	remoteRegistryUnavailable = true

	mediatype, dgst, err = testutil.VerifyRemoteImage(ctx, repo, "latest")
	if err != nil {
		t.Fatal(err)
	}
	if mediatype != manifestMediaType {
		t.Fatalf("manifest mediatype: got %q, want %q", mediatype, manifestMediaType)
	}
	if dgst != manifestDigest {
		t.Fatalf("manifest digest: got %q, want %q", dgst, manifestDigest)
	}
}
