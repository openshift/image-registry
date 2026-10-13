package integration

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dockerapiv10 "github.com/openshift/api/image/docker10"
	imageclientv1 "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"

	"github.com/distribution/distribution/v3/manifest/ocischema"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/openshift/image-registry/pkg/testframework"
	"github.com/openshift/image-registry/pkg/testutil"
	"github.com/openshift/library-go/pkg/image/imageutil"
)

func TestOCIPush(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	master := testframework.NewMaster(t)
	defer master.Close()
	registry := master.StartRegistry(t)
	defer registry.Close()

	namespace := "oci-integration-test"
	isname := "imagestream"
	testuser := master.CreateUser("testuser", "testp@ssw0rd")
	proj := master.CreateProject(namespace, testuser.Name)

	regURL, err := url.Parse(registry.BaseURL())
	if err != nil {
		t.Fatal(err)
	}

	dgst, _, _, man, err := testutil.CreateAndUploadTestManifest(
		ctx,
		testutil.ManifestSchemaOCI,
		5,
		regURL,
		testutil.NewBasicCredentialStore(testuser.Name, testuser.Token),
		fmt.Sprintf("%s/%s", proj.Name, isname),
		"latest",
	)
	if err != nil {
		t.Errorf("error uploading manifest: %s", err)
	}

	imgcli := imageclientv1.NewForConfigOrDie(master.AdminKubeConfig())
	is, err := imgcli.ImageStreams(namespace).Get(
		ctx, isname, metav1.GetOptions{},
	)
	if err != nil {
		t.Errorf("error getting image stream: %s", err)
	}
	if len(is.Status.Tags) != 1 {
		t.Fatalf("expected one tag, found: %v", is.Status.Tags)
	}

	imgname := is.Status.Tags[0].Items[0].Image
	if imgname != dgst.String() {
		t.Errorf(
			"expecting image digest %s, received %s instead",
			dgst,
			is.Status.Tags[0].Items[0].Image,
		)
	}

	img, err := imgcli.Images().Get(ctx, imgname, metav1.GetOptions{})
	if err != nil {
		t.Errorf("unexpected error retrieving image: %s", err)
	}

	if img.DockerImageManifestMediaType != imgspecv1.MediaTypeImageManifest {
		t.Errorf(
			"invalid manifest media type: %s, expecting %s",
			img.DockerImageManifestMediaType,
			imgspecv1.MediaTypeImageManifest,
		)
	}

	for i, l := range img.DockerImageLayers {
		if l.MediaType != imgspecv1.MediaTypeImageLayer {
			t.Errorf("invalid type for layer %d: %s", i, l.MediaType)
		}
	}

	if err := imageutil.ImageWithMetadata(img); err != nil {
		t.Fatalf("unable to parse image metadata: %s", err)
	}

	meta, ok := img.DockerImageMetadata.Object.(*dockerapiv10.DockerImage)
	if !ok {
		t.Error("error casting image metadata object to DockerImage")
	}

	ds, ok := man.(*ocischema.DeserializedManifest)
	if !ok {
		t.Errorf("error casting to deserialized manifest")
	}

	if meta.ID != ds.Config.Digest.String() {
		t.Errorf(
			"config digest mismatch: %s, %s",
			meta.ID,
			ds.Config.Digest.String(),
		)
	}
}
