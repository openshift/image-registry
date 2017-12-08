package integration

import (
	"fmt"
	"testing"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/reference"
	"github.com/docker/distribution/registry/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	imagev1 "github.com/openshift/origin/pkg/image/generated/clientset/typed/image/v1"

	registrytest "github.com/openshift/image-registry/pkg/dockerregistry/testutil"
	"github.com/openshift/image-registry/pkg/testframework"
)

type errRegistryWantsContent struct {
	src reference.Canonical
	dst reference.Named
}

func (e errRegistryWantsContent) Error() string {
	return fmt.Sprintf("the registry cannot mount %s to %s and wants the content of the blob", e.src, e.dst)
}

func crossMountImage(ctx context.Context, destRepo distribution.Repository, tag string, srcRepoNamed reference.Named, manifest distribution.Manifest) error {
	destBlobs := destRepo.Blobs(ctx)
	for _, desc := range manifest.References() {
		canonicalRef, _ := reference.WithDigest(srcRepoNamed, desc.Digest)
		bw, err := destBlobs.Create(ctx, client.WithMountFrom(canonicalRef))
		if _, ok := err.(distribution.ErrBlobMounted); ok {
			continue
		}
		if err != nil {
			return fmt.Errorf("unable to mount blob %s to %s: %v", canonicalRef, destRepo.Named(), err)
		}
		bw.Cancel(ctx)
		bw.Close()
		return errRegistryWantsContent{
			src: canonicalRef,
			dst: destRepo.Named(),
		}
	}
	if err := registrytest.UploadManifest(ctx, destRepo, tag, manifest); err != nil {
		return fmt.Errorf("failed to upload the manifest after cross-mounting blobs: %v", err)
	}
	return nil
}

func copyISTag(imageClient imagev1.ImageV1Interface, destNamespace, destISTag, sourceNamespace, sourceISTag string) error {
	istag, err := imageClient.ImageStreamTags(sourceNamespace).Get(sourceISTag, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("copy istag %s/%s to %s/%s: get source: %v", sourceNamespace, sourceISTag, destNamespace, destISTag, err)
	}
	istag.Name = destISTag
	_, err = imageClient.ImageStreamTags(destNamespace).Create(istag)
	if err != nil {
		return fmt.Errorf("copy istag %s/%s to %s/%s: create destination: %v", sourceNamespace, sourceISTag, destNamespace, destISTag, err)
	}
	return nil
}

func TestCrossMount(t *testing.T) {
	master := testframework.NewMaster(t)
	defer master.Close()

	alice := master.CreateUser("alice", "qwerty")
	bob := master.CreateUser("bob", "123456")
	aliceproject := master.CreateProject("aliceproject", alice.Name)
	bobproject := master.CreateProject("bobproject", bob.Name)

	wantCrossMountError := func(err error) error {
		if _, ok := err.(errRegistryWantsContent); !ok {
			return fmt.Errorf("want a cross-mount error, got %v", err)
		}
		return nil
	}
	wantSuccess := func(err error) error {
		if err != nil {
			return fmt.Errorf("failed to cross-mount image: %v", err)
		}
		return nil
	}

	for _, test := range []struct {
		name                                       string
		actor                                      *testframework.User
		destinationProject, destinationImageStream string
		sourceProject, sourceImageStream           string
		check                                      func(error) error
	}{
		{
			"alice_from_foo",
			alice, aliceproject.Name, "mounted-foo", aliceproject.Name, "foo",
			wantSuccess,
		},
		{
			"bob_from_foo",
			bob, bobproject.Name, "from-foo", aliceproject.Name, "foo",
			wantCrossMountError,
		},
		{
			"bob_from_foo-copy",
			bob, bobproject.Name, "from-foo-copy", aliceproject.Name, "foo-copy",
			wantCrossMountError,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			registry := master.StartRegistry(t)
			defer registry.Close()

			// Upload a random image to aliceproject/foo:latest
			aliceFooRepo := registry.Repository(aliceproject.Name, "foo", alice)
			manifest, err := registrytest.UploadSchema2Image(context.Background(), aliceFooRepo, "latest")
			if err != nil {
				t.Fatal(err)
			}

			// Copy image stream tag aliceproject/foo:latest to aliceproject/foo-copy:latest
			aliceImageClient := imagev1.NewForConfigOrDie(alice.KubeConfig())
			if err := copyISTag(aliceImageClient, aliceproject.Name, "foo-copy:latest", aliceproject.Name, "foo:latest"); err != nil {
				t.Fatal(err)
			}

			// Cross-mount image
			repo := registry.Repository(test.destinationProject, test.destinationImageStream, test.actor)
			sourceNamed, _ := reference.WithName(fmt.Sprintf("%s/%s", test.sourceProject, test.sourceImageStream))
			err = test.check(crossMountImage(context.Background(), repo, "latest", sourceNamed, manifest))
			if err != nil {
				t.Error(err)
			}
		})
	}
}
