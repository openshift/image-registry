package prune

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest/schema1"
	"github.com/distribution/distribution/v3/reference"
	"github.com/distribution/distribution/v3/registry/storage"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/opencontainers/go-digest"
	imageapiv1 "github.com/openshift/api/image/v1"
	registryclient "github.com/openshift/image-registry/pkg/dockerregistry/server/client"
	"github.com/openshift/image-registry/pkg/testutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func makeRepoRef(t *testing.T, namespace, name string) reference.Named {
	refName := fmt.Sprintf("%s/%s", namespace, name)
	parsed, err := reference.Parse(refName)
	if err != nil {
		t.Fatalf("failed to parse reference %q: %v", refName, err)
	}
	ref, ok := parsed.(reference.Named)
	if !ok {
		t.Fatalf("expected Named reference, not %T", parsed)
	}
	return ref
}

func makeNamedTaggedRef(t *testing.T, namespace, name, tag string) reference.NamedTagged {
	refName := fmt.Sprintf("%s/%s:%s", namespace, name, tag)
	parsed, err := reference.Parse(refName)
	if err != nil {
		t.Fatalf("failed to parse reference %q: %v", refName, err)
	}
	namedTagged, ok := parsed.(reference.NamedTagged)
	if !ok {
		t.Fatalf("expected NamedTagged reference, not %T", parsed)
	}
	return namedTagged
}

func createBlob(
	ctx context.Context,
	t *testing.T,
	reg distribution.Namespace,
	namespace, name, tag string,
) distribution.Descriptor {
	namedTagged := makeNamedTaggedRef(t, namespace, name, tag)
	repo, err := reg.Repository(ctx, namedTagged)
	if err != nil {
		t.Fatalf("unexpected error getting repo %q: %v", namedTagged.Name(), err)
	}
	payload, blobDesc, err := testutil.MakeRandomLayer()
	wr, err := repo.Blobs(ctx).Create(ctx)
	if err != nil {
		t.Fatalf("unexpected error creating test upload: %v", err)
	}
	defer wr.Close()
	_, err = io.Copy(wr, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("unexpected error copying to upload: %v", err)
	}
	dgst := digest.FromBytes(payload)
	if _, err := wr.Commit(ctx, distribution.Descriptor{Digest: dgst, MediaType: schema1.MediaTypeManifestLayer}); err != nil {
		t.Fatalf("unexpected error finishing upload: %v", err)
	}
	return blobDesc
}

func populateRegistry(
	ctx context.Context,
	t *testing.T,
	fos *testutil.FakeOpenShift,
	reg distribution.Namespace,
	namespace, name, tag string,
) *imageapiv1.Image {
	blobDesc := createBlob(ctx, t, reg, namespace, name, tag)

	shasum := "sha256:83c0e63f5efb64cba26be647e93bf036b8d88b774f0726936c1b956424b1abf6"
	remoteRef := "registry.redhat.io/ubi8/ubi"
	is := &imageapiv1.ImageStream{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: imageapiv1.ImageStreamSpec{
			LookupPolicy: imageapiv1.ImageLookupPolicy{Local: false},
			Tags: []imageapiv1.TagReference{
				{
					Name: tag,
					From: &corev1.ObjectReference{
						Kind: "DockerImage",
						Name: fmt.Sprintf("%s:%s", remoteRef, tag),
					},
				},
			},
		},
		Status: imageapiv1.ImageStreamStatus{
			Tags: []imageapiv1.NamedTagEventList{
				{
					Tag: tag,
					Items: []imageapiv1.TagEvent{
						{
							Created:              metav1.NewTime(time.Now()),
							DockerImageReference: fmt.Sprintf("%s@%s", remoteRef, shasum),
							Image:                shasum,
						},
					},
				},
			},
		},
	}

	is, err := fos.CreateImageStream(namespace, is)
	if err != nil {
		t.Fatalf("Could not create image stream: %v", err)
	}

	image := &imageapiv1.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name: shasum,
		},
		DockerImageLayers: []imageapiv1.ImageLayer{
			{
				Name:      blobDesc.Digest.String(),
				LayerSize: blobDesc.Size,
				MediaType: blobDesc.MediaType,
			},
		},
	}

	image, err = fos.CreateImage(image)
	if err != nil {
		t.Fatalf("Could not create image: %v", err)
	}

	return image
}

func TestPrune(t *testing.T) {
	ctx := context.Background()
	ctx = testutil.WithTestLogger(ctx, t)
	storageDriver := inmemory.New()
	fos, imageClient := testutil.NewFakeOpenShiftWithClient(ctx)
	reg, err := storage.NewRegistry(ctx, storageDriver)
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}

	image1 := populateRegistry(ctx, t, fos, reg, "ns-test", "is-test", "latest")
	danglingBlob := createBlob(ctx, t, reg, "ns-test", "this-is-has-been-deleted", "latest")

	pruner := &RegistryPruner{StorageDriver: storageDriver}
	_, err = Prune(ctx, reg, registryclient.NewFakeRegistryClient(imageClient), pruner)
	if err != nil {
		t.Fatalf("error calling Prune: %s", err)
	}

	statter := reg.BlobStatter()
	for _, blob := range image1.DockerImageLayers {
		_, err := statter.Stat(ctx, digest.Digest(blob.Name))
		if err != nil {
			t.Errorf("error retrieving blob %q: %#v", blob.Name, err)
		}
	}

	_, err = statter.Stat(ctx, danglingBlob.Digest)
	if err != distribution.ErrBlobUnknown {
		t.Errorf("expected error to be distribution.ErrBlobUnknown, got %#v", err)
	}
}
