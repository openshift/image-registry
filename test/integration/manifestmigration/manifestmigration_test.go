package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/docker/distribution/registry/storage/driver/inmemory"
	"github.com/opencontainers/go-digest"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	imageapiv1 "github.com/openshift/api/image/v1"
	imageclientv1 "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"

	imageapi "github.com/openshift/image-registry/pkg/origin-common/image/apis/image"
	"github.com/openshift/image-registry/pkg/testframework"
	"github.com/openshift/image-registry/pkg/testutil"
	"github.com/openshift/image-registry/test/internal/storage"
	"github.com/openshift/image-registry/test/internal/storagepath"
)

func TestManifestMigration(t *testing.T) {
	config := []byte("{}")
	configDigest := digest.FromBytes(config)

	foo := []byte("foo-manifest-migration")
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

	master := testframework.NewMaster(t)
	defer master.Close()

	testuser := master.CreateUser("testuser", "testp@ssw0rd")
	testproject := master.CreateProject("test-manifest-migration", testuser.Name)
	teststreamName := "manifestmigration"

	imageClient := imageclientv1.NewForConfigOrDie(master.AdminKubeConfig())

	_, err = imageClient.ImageStreams(testproject.Name).Create(&imageapiv1.ImageStream{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testproject.Name,
			Name:      teststreamName,
		},
	})
	if err != nil && !kerrors.IsAlreadyExists(err) {
		t.Fatal(err)
	}

	err = imageClient.Images().Delete(string(manifestDigest), &metav1.DeleteOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		t.Fatalf("failed to delete an old instance of the image: %v", err)
	}

	_, err = imageClient.ImageStreamMappings(testproject.Name).Create(&imageapiv1.ImageStreamMapping{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testproject.Name,
			Name:      teststreamName,
		},
		Image: imageapiv1.Image{
			ObjectMeta: metav1.ObjectMeta{
				Name: string(manifestDigest),
				Annotations: map[string]string{
					imageapi.ManagedByOpenShiftAnnotation: "true",
				},
			},
			DockerImageReference:         "shouldnt-be-resolved.example.com/this-is-a-fake-image",
			DockerImageManifestMediaType: manifestMediaType,
			DockerImageManifest:          string(manifest),
			DockerImageConfig:            string(config),
		},
		Tag: "latest",
	})
	if err != nil {
		t.Fatalf("failed to create image stream mapping: %v", err)
	}

	driver := storage.NewWaitableDriver(inmemory.New())
	registry := master.StartRegistry(t, storage.WithDriver(driver))
	defer registry.Close()

	repo := registry.Repository(testproject.Name, teststreamName, testuser)

	ctx := context.Background()
	ctx = testutil.WithTestLogger(ctx, t)

	ms, err := repo.Manifests(ctx)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ms.Get(ctx, digest.Digest(manifestDigest))
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("waiting for migration to finish...")

	if err := driver.WaitFor(ctx, storagepath.Blob(manifestDigest)); err != nil {
		t.Fatal(err)
	}

	t.Logf("manifest is migrated, checking results...")

	manifestOnStorage, err := driver.GetContent(ctx, storagepath.Blob(manifestDigest))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(manifestOnStorage, manifest) {
		t.Errorf("migration has changed the manifest: got %q, want %q", manifestOnStorage, manifest)
	}

	w, err := imageClient.Images().Watch(metav1.ListOptions{
		Watch: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = watch.Until(30*time.Second, w, func(event watch.Event) (bool, error) {
		if event.Type != "MODIFIED" {
			return false, nil
		}
		image, ok := event.Object.(*imageapiv1.Image)
		if !ok {
			return false, nil
		}
		if image.Name != string(manifestDigest) || image.DockerImageManifest != "" && image.DockerImageConfig != "" {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("waiting for the manifest and the config to be removed from the image: %v", err)
	}
}
