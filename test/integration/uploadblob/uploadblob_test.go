package integration

import (
	"context"
	"testing"

	"github.com/openshift/image-registry/pkg/testframework"
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
