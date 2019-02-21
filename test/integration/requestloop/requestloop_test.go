package integration

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreclientv1 "k8s.io/client-go/kubernetes/typed/core/v1"

	imageapiv1 "github.com/openshift/api/image/v1"
	imageclientv1 "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"

	"github.com/openshift/image-registry/pkg/testframework"
	"github.com/openshift/image-registry/pkg/testutil"
)

func TestRequestLoop(t *testing.T) {
	master := testframework.NewMaster(t)
	defer master.Close()

	imageClient := imageclientv1.NewForConfigOrDie(master.AdminKubeConfig())
	coreClient := coreclientv1.NewForConfigOrDie(master.AdminKubeConfig())

	username := "testuser"
	password := "testp@ssw0rd"

	testuser := master.CreateUser(username, password)
	testproject := master.CreateProject("test-image-pullthrough-loop", testuser.Name)
	teststreamName := "pullthrough"
	ctx := context.Background()

	registry := master.StartRegistry(t, testframework.DisableMirroring{})
	defer registry.Close()

	repo := registry.Repository(testproject.Name, teststreamName, testuser)

	_, err := testutil.UploadSchema2Image(ctx, repo, "v1")
	if err != nil {
		t.Fatal(err)
	}

	registryIP, registryPort, err := net.SplitHostPort(registry.Addr())
	if err != nil {
		t.Fatal(err)
	}

	dockerjson := fmt.Sprintf("{\"auths\": {\"%s.nip.io:%s\": {\"auth\": \"%s\"}}}", registryIP, registryPort, base64.StdEncoding.EncodeToString([]byte(testuser.Name+":"+testuser.Token)))

	_, err = coreClient.Secrets(testproject.Name).Create(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: teststreamName,
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: []byte(dockerjson),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = imageClient.ImageStreamImports(testproject.Name).Create(&imageapiv1.ImageStreamImport{
		ObjectMeta: metav1.ObjectMeta{
			Name: teststreamName,
		},
		Spec: imageapiv1.ImageStreamImportSpec{
			Import: true,
			Images: []imageapiv1.ImageImportSpec{
				{
					From: corev1.ObjectReference{
						Kind: "DockerImage",
						Name: fmt.Sprintf("%s.nip.io:%s/%s/%s:v1", registryIP, registryPort, testproject.Name, teststreamName),
					},
					To: &corev1.LocalObjectReference{
						Name: "v2",
					},
					ImportPolicy: imageapiv1.TagImportPolicy{
						Insecure: true,
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest("HEAD", fmt.Sprintf(
		"http://%s:%s/v2/%s/%s/blobs/sha256:0000000000000000000000000000000000000000000000000000000000000000",
		registryIP,
		registryPort,
		testproject.Name,
		teststreamName,
	), nil)
	if err != nil {
		t.Fatal(err)
	}

	req.SetBasicAuth(testuser.Name, testuser.Token)

	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:       1,
			IdleConnTimeout:    5 * time.Second,
			DisableCompression: true,
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != 404 {
		t.Fatalf("response: %#+v", resp)
	}
}
