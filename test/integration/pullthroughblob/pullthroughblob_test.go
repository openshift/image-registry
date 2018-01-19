package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	digest "github.com/docker/distribution/digest"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	imageapiv1 "github.com/openshift/api/image/v1"
	imageclientv1 "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"

	"github.com/openshift/image-registry/pkg/testframework"
	"github.com/openshift/image-registry/pkg/testutil/counter"
)

func TestPullthroughBlob(t *testing.T) {
	config := "{}"
	configDigest := digest.FromBytes([]byte(config))

	foo := "foo"
	fooDigest := digest.FromBytes([]byte(foo))

	master := testframework.NewMaster(t)
	defer master.Close()

	testuser := master.CreateUser("testuser", "testp@ssw0rd")
	testproject := master.CreateProject("testproject", testuser.Name)
	teststreamName := "pullthrough"

	requestCounter := counter.New()
	ts := testframework.NewHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := fmt.Sprintf("%s %s", r.Method, r.URL.Path)

		t.Logf("remote registry: %s", req)
		requestCounter.Add(req, 1)

		w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")

		switch req {
		case "GET /v2/":
			w.Write([]byte(`{}`))
		case "GET /v2/remoteimage/manifests/latest":
			mediaType := "application/vnd.docker.distribution.manifest.v2+json"
			w.Header().Set("Content-Type", mediaType)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"schemaVersion": 2,
				"mediaType":     mediaType,
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
		case "GET /v2/remoteimage/blobs/" + configDigest.String():
			w.Write([]byte(config))
		case "HEAD /v2/remoteimage/blobs/" + fooDigest.String():
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(foo)))
			w.WriteHeader(http.StatusOK)
		case "GET /v2/remoteimage/blobs/" + fooDigest.String():
			w.Write([]byte(foo))
		default:
			t.Errorf("error: remote registry got unexpected request %s: %#+v", req, r)
		}
	}))
	defer ts.Close()

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

	if diff := requestCounter.Diff(counter.M{
		"GET /v2/":                                           1,
		"GET /v2/remoteimage/manifests/latest":               1,
		"GET /v2/remoteimage/blobs/" + configDigest.String(): 1,
	}); diff != nil {
		t.Fatalf("unexpected count of requests: %q", diff)
	}

	// Reset counter
	requestCounter = counter.New()

	registry := master.StartRegistry(t, testframework.DisableMirroring{})
	defer registry.Close()

	repo := registry.Repository(testproject.Name, teststream.Name, testuser)

	ctx := context.Background()

	data, err := repo.Blobs(ctx).Get(ctx, fooDigest)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != foo {
		t.Fatalf("got %q, want %q", string(data), foo)
	}

	// TODO(dmage): reduce number of requests
	if diff := requestCounter.Diff(counter.M{
		"GET /v2/": 2,
		"HEAD /v2/remoteimage/blobs/" + fooDigest.String(): 2,
		"GET /v2/remoteimage/blobs/" + fooDigest.String():  1,
	}); diff != nil {
		t.Fatalf("unexpected count of requests: %q", diff)
	}
}
