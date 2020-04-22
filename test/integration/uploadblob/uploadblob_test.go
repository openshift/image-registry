package integration

import (
	"context"
	"net/url"
	"testing"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	authorizationapiv1 "github.com/openshift/api/authorization/v1"
	authorizationv1 "github.com/openshift/client-go/authorization/clientset/versioned/typed/authorization/v1"

	"github.com/openshift/image-registry/pkg/testframework"
	"github.com/openshift/image-registry/pkg/testutil"
)

func TestUploadBlobCancel(t *testing.T) {
	master := testframework.NewMaster(t)
	defer master.Close()
	registry := master.StartRegistry(t)
	defer registry.Close()

	ctx := context.Background()
	testuser := master.CreateUser("testuser", "testp@ssw0rd")
	testproject := master.CreateProject("image-registry-test-upload-blob-cancel", testuser.Name)
	imageStreamName := "test-upload-blob-cancel"
	repo := registry.Repository(testproject.Name, imageStreamName, testuser)

	w, err := repo.Blobs(ctx).Create(ctx)
	if err != nil {
		t.Fatalf("unable to initiate upload: %s", err)
	}
	if err := w.Cancel(ctx); err != nil {
		t.Fatalf("unable to cancel upload: %s", err)
	}
}

func TestPutManifestToANonExistentNamespace(t *testing.T) {
	master := testframework.NewMaster(t)
	defer master.Close()

	registry := master.StartRegistry(t)
	defer registry.Close()

	testuser := master.CreateUser("cleveruser", "!cleverpass")
	authorizationClient := authorizationv1.NewForConfigOrDie(master.AdminKubeConfig())

	_, err := authorizationClient.ClusterRoleBindings().Create(context.Background(), &authorizationapiv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cleveruser",
		},
		UserNames: []string{"cleveruser"},
		RoleRef: corev1.ObjectReference{
			Kind: "ClusterRole",
			Name: "admin",
		},
	}, metav1.CreateOptions{})
	if err != nil && !kerrors.IsAlreadyExists(err) {
		t.Fatal(err)
	}

	regURL, err := url.Parse(registry.BaseURL())
	if err != nil {
		t.Fatal(err)
	}

	_, _, _, _, err = testutil.CreateAndUploadTestManifest(context.Background(), testutil.ManifestSchema1, 1, regURL, testutil.NewBasicCredentialStore(testuser.Name, testuser.Token), "doesnotexist/img", "latest")
	if err == nil {
		t.Errorf("able to put a manifest to a non-existent namespace")
	}

	if err.Error() != "failed to upload manifest to doesnotexist/img: denied" {
		t.Errorf("unexpected error: %q", err.Error())
	}
}
