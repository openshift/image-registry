package integration

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	imageclientv1 "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/image-registry/pkg/testframework"
	"github.com/openshift/image-registry/pkg/testutil"
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

	dgst, _, _, _, err := testutil.CreateAndUploadTestManifest(
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

	imgcli := imageclientv1.NewForConfigOrDie(testuser.KubeConfig())
	is, err := imgcli.ImageStreams(namespace).Get(
		ctx, isname, metav1.GetOptions{})
	if err != nil {
		t.Errorf("error getting image stream: %s", err)
	}
	if len(is.Status.Tags) != 1 {
		t.Fatal("no tag found in image stream")
	}

	if is.Status.Tags[0].Items[0].Image != dgst.String() {
		t.Errorf(
			"expecting image digest %s, received %s instead",
			dgst,
			is.Status.Tags[0].Items[0].Image,
		)
	}
}
