package integration

import (
	"fmt"
	"testing"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/reference"
	"github.com/docker/distribution/registry/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kapiv1 "k8s.io/kubernetes/pkg/api/v1"

	imageapiv1 "github.com/openshift/origin/pkg/image/apis/image/v1"
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

func copyISTag(t *testing.T, imageClient imagev1.ImageV1Interface, destNamespace, destISTag, sourceNamespace, sourceISTag string) error {
	istag := &imageapiv1.ImageStreamTag{
		ObjectMeta: metav1.ObjectMeta{
			Name: destISTag,
		},
		Tag: &imageapiv1.TagReference{
			From: &kapiv1.ObjectReference{
				Kind:      "ImageStreamTag",
				Name:      sourceISTag,
				Namespace: sourceNamespace,
			},
		},
	}
	_, err := imageClient.ImageStreamTags(destNamespace).Create(istag)
	if err != nil {
		return fmt.Errorf("copy istag %s/%s to %s/%s: %v", sourceNamespace, sourceISTag, destNamespace, destISTag, err)
	}
	return nil
}

func TestCrossMount(t *testing.T) {
	master := testframework.NewMaster(t)
	defer master.Close()

	alice := master.CreateUser("alice", "qwerty")
	bob := master.CreateUser("bob", "123456")

	const aliceprojectName = "aliceproject"
	const bobprojectName = "bobproject"

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
		prefix                                     string
		actor                                      *testframework.User
		destinationProject, destinationImageStream string
		sourceProject, sourceImageStream           string
		check                                      func(error) error
	}{
		{
			"alice_from_foo", "a1-",
			alice, aliceprojectName, "mounted-foo", aliceprojectName, "foo",
			wantSuccess,
		},
		{
			"alice_from_foo-copy", "a2-",
			alice, aliceprojectName, "mounted-foo-copy", aliceprojectName, "foo-copy",
			wantSuccess,
		},
		{
			"bob_from_foo", "b1-",
			bob, bobprojectName, "from-foo", aliceprojectName, "foo",
			wantCrossMountError,
		},
		{
			"bob_from_foo-copy", "b2-",
			bob, bobprojectName, "from-foo-copy", aliceprojectName, "foo-copy",
			wantCrossMountError,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			aliceproject := testframework.CreateProject(t, master.AdminKubeConfig(), test.prefix+aliceprojectName, alice.Name)
			defer testframework.DeleteProject(t, master.AdminKubeConfig(), aliceproject.Name)
			bobproject := testframework.CreateProject(t, master.AdminKubeConfig(), test.prefix+bobprojectName, bob.Name)
			defer testframework.DeleteProject(t, master.AdminKubeConfig(), bobproject.Name)

			registry := master.StartRegistry(t)
			defer registry.Close()

			// Upload a random image to aliceproject/foo:latest
			aliceFooRepo := registry.Repository(aliceproject.Name, "foo", alice)
			manifest, err := registrytest.UploadSchema2Image(context.Background(), aliceFooRepo, "latest")
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("uploaded manifest %s: %v", aliceFooRepo.Named(), manifest)

			// Copy image stream tag aliceproject/foo:latest to aliceproject/foo-copy:latest
			aliceImageClient := imagev1.NewForConfigOrDie(alice.KubeConfig())
			if err := copyISTag(t, aliceImageClient, aliceproject.Name, "foo-copy:latest", aliceproject.Name, "foo:latest"); err != nil {
				t.Fatal(err)
			}

			// Cross-mount image
			repo := registry.Repository(test.prefix+test.destinationProject, test.destinationImageStream, test.actor)
			sourceNamed, _ := reference.WithName(fmt.Sprintf("%s/%s", test.prefix+test.sourceProject, test.sourceImageStream))
			err = test.check(crossMountImage(context.Background(), repo, "latest", sourceNamed, manifest))
			if err != nil {
				t.Error(err)
			}
		})
	}
}
