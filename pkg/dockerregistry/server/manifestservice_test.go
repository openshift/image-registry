package server

import (
	"context"
	"fmt"
	"testing"

	"github.com/docker/distribution"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/opencontainers/go-digest"

	registryclient "github.com/openshift/image-registry/pkg/dockerregistry/server/client"
	"github.com/openshift/image-registry/pkg/imagestream"
	"github.com/openshift/image-registry/pkg/testutil"
)

func TestManifestServiceExists(t *testing.T) {
	ctx := context.Background()
	ctx = testutil.WithTestLogger(ctx, t)

	namespace := "user"
	repo := "app"
	tag := "latest"

	fos, imageClient := testutil.NewFakeOpenShiftWithClient(ctx)
	testImage := testutil.AddRandomImage(t, fos, namespace, repo, tag)

	imageStream := imagestream.New(namespace, repo, registryclient.NewFakeRegistryAPIClient(nil, imageClient))

	ms := &manifestService{
		imageStream:   imageStream,
		acceptSchema2: true,
	}

	ok, err := ms.Exists(ctx, digest.Digest(testImage.Name))
	if err != nil {
		t.Errorf("ms.Exists(ctx, %q): %s", testImage.Name, err)
	} else if !ok {
		t.Errorf("ms.Exists(ctx, %q): got false, want true", testImage.Name)
	}

	_, err = ms.Exists(ctx, unknownBlobDigest)
	if err == nil {
		t.Errorf("ms.Exists(ctx, %q): got success, want error", unknownBlobDigest)
	}
}

func TestManifestServicePut(t *testing.T) {
	ctx := context.Background()
	ctx = testutil.WithTestLogger(ctx, t)

	namespace := "user"
	repo := "app"
	repoName := fmt.Sprintf("%s/%s", namespace, repo)

	_, imageClient := testutil.NewFakeOpenShiftWithClient(ctx)

	bs := newTestBlobStore(nil, blobContents{
		"test:1": []byte("{}"),
	})

	tms := newTestManifestService(repoName, nil)

	imageStream := imagestream.New(namespace, repo, registryclient.NewFakeRegistryAPIClient(nil, imageClient))

	ms := &manifestService{
		manifests:     tms,
		blobStore:     bs,
		imageStream:   imageStream,
		acceptSchema2: true,
	}

	manifest := &schema2.DeserializedManifest{
		Manifest: schema2.Manifest{
			Config: distribution.Descriptor{
				Digest: "test:1",
				Size:   2,
			},
		},
	}

	osclient, err := registryclient.NewFakeRegistryClient(imageClient).Client()
	if err != nil {
		t.Fatal(err)
	}

	putCtx := withAuthPerformed(ctx)
	putCtx = withUserClient(putCtx, osclient)
	dgst, err := ms.Put(putCtx, manifest)
	if err != nil {
		t.Fatalf("ms.Put(ctx, manifest): %s", err)
	}

	// recreate objects to reset cached image streams
	imageStream = imagestream.New(namespace, repo, registryclient.NewFakeRegistryAPIClient(nil, imageClient))

	ms = &manifestService{
		manifests:     tms,
		imageStream:   imageStream,
		acceptSchema2: true,
	}

	_, err = ms.Get(ctx, dgst)
	if err != nil {
		t.Errorf("ms.Get(ctx, %q): %s", dgst, err)
	}
}
