package integration

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"testing"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/origin/pkg/cmd/util/tokencmd"
	imageapi "github.com/openshift/origin/pkg/image/apis/image"
	imagev1 "github.com/openshift/origin/pkg/image/generated/clientset/typed/image/v1"

	registrytest "github.com/openshift/image-registry/pkg/dockerregistry/testutil"
	"github.com/openshift/image-registry/pkg/testframework"
)

// uploadImageWithSchema2Manifest creates a random image with a schema 2
// manifest and uploads it to the repository.
func uploadImageWithSchema2Manifest(ctx context.Context, repo distribution.Repository, tag string) error {
	layers := make([]distribution.Descriptor, 3)
	for i := range layers {
		content, desc, err := registrytest.MakeRandomLayer()
		if err != nil {
			return fmt.Errorf("make random layer: %v", err)
		}

		if err := registrytest.UploadBlob(ctx, repo, desc, content); err != nil {
			return fmt.Errorf("upload random blob: %v", err)
		}

		layers[i] = desc
	}

	cfg := map[string]interface{}{
		"rootfs": map[string]interface{}{
			"diff_ids": make([]string, len(layers)),
		},
		"history": make([]struct{}, len(layers)),
	}

	configContent, err := json.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("marshal image config: %v", err)
	}

	config := distribution.Descriptor{
		Digest: digest.FromBytes(configContent),
		Size:   int64(len(configContent)),
	}

	if err := registrytest.UploadBlob(ctx, repo, config, configContent); err != nil {
		return fmt.Errorf("upload image config: %v", err)
	}

	manifest, err := registrytest.MakeSchema2Manifest(config, layers)
	if err != nil {
		return fmt.Errorf("make schema 2 manifest: %v", err)
	}

	if err := registrytest.UploadManifest(ctx, repo, tag, manifest); err != nil {
		return fmt.Errorf("upload schema 2 manifest: %v", err)
	}

	return nil
}

// getSchema1Manifest simulates a client which supports only schema 1
// manifests, fetches a manifest from a registry and returns it.
func getSchema1Manifest(transport http.RoundTripper, baseURL, repoName, tag string) (distribution.Manifest, error) {
	c := &http.Client{
		Transport: transport,
	}

	resp, err := c.Get(baseURL + "/v2/" + repoName + "/manifests/" + tag)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read manifest %s:%s: %v", repoName, tag, err)
	}

	m, _, err := distribution.UnmarshalManifest(resp.Header.Get("Content-Type"), body)
	return m, err
}

// TestImageLayers tests that the integrated registry handles schema 1
// manifests and schema 2 manifests consistently and it produces similar Image
// resources for them.
//
// The test relies on ability of the registry to downconvert manifests.
func TestImageLayers(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "image-registry-test-integration-")
	if err != nil {
		t.Fatalf("failed to create temporary directory: %s", err)
	}
	defer os.RemoveAll(tmpDir)

	configDir := path.Join(tmpDir, "config")
	adminKubeConfigPath := path.Join(configDir, "master", "admin.kubeconfig")

	masterContainer, err := testframework.StartMasterContainer(configDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := masterContainer.Stop(); err != nil {
			t.Log(err)
		}
	}()

	clusterAdminClientConfig, err := testframework.ConfigFromFile(adminKubeConfigPath)
	if err != nil {
		t.Fatal(err)
	}

	namespace := "image-registry-test-integration"
	imageStreamName := "test-imagelayers"
	user := "testuser"
	password := "testp@ssw0rd"

	if err := testframework.CreateProject(clusterAdminClientConfig, namespace, user); err != nil {
		t.Fatal(err)
	}

	token, err := tokencmd.RequestToken(clusterAdminClientConfig, nil, user, password)
	if err != nil {
		t.Fatalf("error requesting token: %v", err)
	}

	registryAddr, err := testframework.StartTestRegistry(t, adminKubeConfigPath)
	if err != nil {
		t.Fatalf("start registry: %v", err)
	}

	creds := registrytest.NewBasicCredentialStore(user, token)

	baseURL := "http://" + registryAddr
	repoName := fmt.Sprintf("%s/%s", namespace, imageStreamName)

	schema1Tag := "schema1"
	schema2Tag := "schema2"

	transport, err := registrytest.NewTransport(baseURL, repoName, creds)
	if err != nil {
		t.Fatalf("get transport: %v", err)
	}

	ctx := context.Background()

	repo, err := registrytest.NewRepository(ctx, repoName, baseURL, transport)
	if err != nil {
		t.Fatalf("get repository: %v", err)
	}

	if err := uploadImageWithSchema2Manifest(ctx, repo, schema2Tag); err != nil {
		t.Fatalf("upload image with schema 2 manifest: %v", err)
	}

	// get the schema2 image's manifest downconverted to a schema 1 manifest
	schema1Manifest, err := getSchema1Manifest(transport, baseURL, repoName, schema2Tag)
	if err != nil {
		t.Fatalf("get schema 1 manifest for image schema2: %v", err)
	}

	if err := registrytest.UploadManifest(ctx, repo, schema1Tag, schema1Manifest); err != nil {
		t.Fatalf("upload schema 1 manifest: %v", err)
	}

	imageClient := imagev1.NewForConfigOrDie(clusterAdminClientConfig)

	schema1ISTag, err := imageClient.ImageStreamTags(namespace).Get(imageStreamName+":"+schema1Tag, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get image stream tag %s:%s: %v", imageStreamName, schema1Tag, err)
	}

	schema2ISTag, err := imageClient.ImageStreamTags(namespace).Get(imageStreamName+":"+schema2Tag, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get image stream tag %s:%s: %v", imageStreamName, schema1Tag, err)
	}

	if schema1ISTag.Image.DockerImageManifestMediaType == schema2ISTag.Image.DockerImageManifestMediaType {
		t.Errorf("expected different media types, but got %q", schema1ISTag.Image.DockerImageManifestMediaType)
	}

	image1LayerOrder := schema1ISTag.Image.Annotations[imageapi.DockerImageLayersOrderAnnotation]
	image2LayerOrder := schema2ISTag.Image.Annotations[imageapi.DockerImageLayersOrderAnnotation]
	if image1LayerOrder != image2LayerOrder {
		t.Errorf("the layer order annotations are different: schema1=%q, schema2=%q", image1LayerOrder, image2LayerOrder)
	} else if image1LayerOrder == "" {
		t.Errorf("the layer order annotation is empty or not present")
	}

	image1Layers := schema1ISTag.Image.DockerImageLayers
	image2Layers := schema2ISTag.Image.DockerImageLayers
	if len(image1Layers) != len(image2Layers) {
		t.Errorf("layers are different: schema1=%#+v, schema2=%#+v", image1Layers, image2Layers)
	} else {
		for i := range image1Layers {
			if image1Layers[i].Name != image2Layers[i].Name {
				t.Errorf("different names for the layer #%d: schema1=%#+v, schema2=%#+v", i, image1Layers[i], image2Layers[i])
			}
			if image1Layers[i].LayerSize != image2Layers[i].LayerSize {
				t.Errorf("different sizes for the layer #%d: schema1=%#+v, schema2=%#+v", i, image1Layers[i], image2Layers[i])
			} else if image1Layers[i].LayerSize <= 0 {
				t.Errorf("unexpected size for the layer #%d: %d", i, image1Layers[i].LayerSize)
			}
		}
	}
}
