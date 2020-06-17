package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	imageapiv1 "github.com/openshift/api/image/v1"
	imageclientv1 "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"

	"github.com/openshift/image-registry/pkg/testframework"
	"github.com/openshift/image-registry/pkg/testutil/counter"
)

func TestPullthroughCopy(t *testing.T) {
	imageData, err := testframework.NewSchema2ImageData()
	if err != nil {
		t.Fatal(err)
	}

	master := testframework.NewMaster(t)
	defer master.Close()

	registry := master.StartRegistry(t, testframework.DisableMirroring{})
	defer registry.Close()

	testuser := master.CreateUser("testuser", "testp@ssw0rd")
	projectImageFromExternalRegistry := master.CreateProject("test-image-pullthrough-copy-external", testuser.Name)
	projectImageFromInternalRegistry := master.CreateProject("test-image-pullthrough-copy-internal", testuser.Name)

	requestCounter := counter.New()
	ts := testframework.NewHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := fmt.Sprintf("%s %s", r.Method, r.URL.Path)

		t.Logf("remote registry: %s", req)
		requestCounter.Add(req, 1)

		w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")

		if testframework.ServeV2(w, r) ||
			testframework.ServeImage(w, r, "myapp", imageData, []string{"latest"}) {
			return
		}

		t.Errorf("error: remote registry got unexpected request %s: %#+v", req, r)
		http.Error(w, "unable to handle the request", http.StatusInternalServerError)
	}))
	defer ts.Close()

	imageClient := imageclientv1.NewForConfigOrDie(master.AdminKubeConfig())

	ctx := context.Background()

	isi, err := imageClient.ImageStreamImports(projectImageFromExternalRegistry.Name).Create(ctx, &imageapiv1.ImageStreamImport{
		ObjectMeta: metav1.ObjectMeta{
			Name: "external-myapp",
		},
		Spec: imageapiv1.ImageStreamImportSpec{
			Import: true,
			Images: []imageapiv1.ImageImportSpec{
				{
					From: corev1.ObjectReference{
						Kind: "DockerImage",
						Name: fmt.Sprintf("%s/myapp:latest", ts.URL.Host),
					},
					ImportPolicy: imageapiv1.TagImportPolicy{
						Insecure: true,
					},
				},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(isi.Status.Import.Status.Tags) == 0 {
		t.Fatalf("failed to import image: %#+v", isi)
	}

	isi, err = imageClient.ImageStreamImports(projectImageFromInternalRegistry.Name).Create(ctx, &imageapiv1.ImageStreamImport{
		ObjectMeta: metav1.ObjectMeta{
			Name: "internal-myapp",
		},
		Spec: imageapiv1.ImageStreamImportSpec{
			Import: true,
			Images: []imageapiv1.ImageImportSpec{
				{
					From: corev1.ObjectReference{
						Kind: "DockerImage",
						Name: fmt.Sprintf("%s/%s/external-myapp:latest", registry.Addr(), projectImageFromInternalRegistry.Name),
					},
					ImportPolicy: imageapiv1.TagImportPolicy{
						Insecure: true,
					},
				},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(isi.Status.Import.Status.Tags) == 0 {
		t.Fatalf("failed to import image: %#+v", isi)
	}

	// Reset counter
	requestCounter = counter.New()

	repo := registry.Repository(projectImageFromInternalRegistry.Name, "internal-myapp", testuser)

	ms, err := repo.Manifests(ctx)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ms.Get(ctx, imageData.ManifestDigest)
	if err != nil {
		t.Fatal(err)
	}

	data, err := repo.Blobs(ctx).Get(ctx, imageData.LayerDigest)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != string(imageData.Layer) {
		t.Fatalf("got %q, want %q", string(data), string(imageData.Layer))
	}

	// TODO(dmage): remove the HEAD request
	if diff := requestCounter.Diff(counter.M{
		"GET /v2/": 1,
		"HEAD /v2/remoteimage/blobs/" + imageData.LayerDigest.String(): 1,
		"GET /v2/remoteimage/blobs/" + imageData.LayerDigest.String():  1,
	}); diff != nil {
		t.Fatalf("unexpected number of requests: %q", diff)
	}
}
