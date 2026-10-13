package integration

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/distribution/distribution/v3"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"

	imageapiv1 "github.com/openshift/api/image/v1"
	imageclient "github.com/openshift/client-go/image/clientset/versioned"

	"github.com/openshift/image-registry/pkg/testframework"
	"github.com/openshift/image-registry/pkg/testutil"
)

func TestMetricsPullthroughBlob(t *testing.T) {
	imageData, err := testframework.NewSchema2ImageData()
	if err != nil {
		t.Fatal(err)
	}

	master := testframework.NewMaster(t)
	defer master.Close()

	testuser := master.CreateUser("testuser", "testp@ssw0rd")
	testproject := master.CreateProject("testproject", testuser.Name)
	teststreamName := "pullthrough"

	ctx := context.Background()
	kubeClient := kubeclient.NewForConfigOrDie(master.AdminKubeConfig())
	imageClient := imageclient.NewForConfigOrDie(master.AdminKubeConfig())

	remoteRegistryAddr, _, _ := testframework.CreateEphemeralRegistry(t, master.AdminKubeConfig(), testproject.Name, map[string]string{
		"remoteuser": "remotepass",
	})

	remoteRepo, err := testutil.NewInsecureRepository(remoteRegistryAddr+"/remoteimage", testutil.NewBasicCredentialStore("remoteuser", "remotepass"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = testframework.PushSchema2ImageData(context.TODO(), remoteRepo, "latest", imageData)
	if err != nil {
		t.Fatal(err)
	}

	remoteRegistrySecret, err := testframework.MakeDockerConfigSecret("remote-registry", &testframework.DockerConfig{
		Auths: map[string]testframework.AuthConfig{
			remoteRegistryAddr: {
				Username: "remoteuser",
				Password: "remotepass",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = kubeClient.CoreV1().Secrets(testproject.Name).Create(ctx, remoteRegistrySecret, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	isi, err := imageClient.ImageV1().ImageStreamImports(testproject.Name).Create(ctx, &imageapiv1.ImageStreamImport{
		ObjectMeta: metav1.ObjectMeta{
			Name: teststreamName,
		},
		Spec: imageapiv1.ImageStreamImportSpec{
			Import: true,
			Images: []imageapiv1.ImageImportSpec{
				{
					From: corev1.ObjectReference{
						Kind: "DockerImage",
						Name: fmt.Sprintf("%s/remoteimage:latest", remoteRegistryAddr),
					},
					ImportPolicy: imageapiv1.TagImportPolicy{
						Insecure: true,
					},
				},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	teststream, err := imageClient.ImageV1().ImageStreams(testproject.Name).Get(ctx, teststreamName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(teststream.Status.Tags) == 0 {
		t.Fatalf("failed to import image: %#+v %#+v", isi, teststream)
	}
	for _, tag := range teststream.Status.Tags {
		for _, cond := range tag.Conditions {
			if cond.Type == "ImportSuccess" {
				if cond.Status != corev1.ConditionTrue {
					t.Fatalf("failed to import image: %s", cond.Message)
				}
			}
		}
	}

	err = kubeClient.CoreV1().Secrets(testproject.Name).Delete(ctx, remoteRegistrySecret.Name, metav1.DeleteOptions{})
	if err != nil {
		t.Fatal(err)
	}

	registry := master.StartRegistry(t, testframework.DisableMirroring{}, testframework.EnableMetrics{Secret: "MetricsSecret"})
	defer registry.Close()

	repo := registry.Repository(testproject.Name, teststream.Name, testuser)

	_, err = repo.Blobs(ctx).Get(ctx, imageData.LayerDigest)
	if err != distribution.ErrBlobUnknown {
		t.Fatal(err)
	}

	req, err := http.NewRequest("GET", registry.BaseURL()+"/extensions/v2/metrics", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer MetricsSecret")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	metrics := []struct {
		name   string
		values []string
	}{
		{
			name:   "imageregistry_storage_duration_seconds",
			values: []string{`operation="StorageDriver.Stat"`},
		},
		{
			name:   "imageregistry_storage_errors_total",
			values: []string{`operation="StorageDriver.Stat"`, `code="PATH_NOT_FOUND"`},
		},
		{
			name:   "imageregistry_pullthrough_repository_errors_total",
			values: []string{`operation="BlobStore.Stat"`, `code="UNAUTHORIZED"`},
		},
		{
			name:   "imageregistry_pullthrough_blobstore_cache_requests_total",
			values: []string{`type="Miss"`},
		},
		{
			name:   "imageregistry_pullthrough_repository_duration_seconds",
			values: []string{`operation="Init"`},
		},
	}

	r := bufio.NewReader(resp.Body)
	for {
		line, err := r.ReadString('\n')
		line = strings.TrimRight(line, "\n")
		t.Log(line)

	metric:
		for i, m := range metrics {
			if !strings.HasPrefix(line, m.name+"{") {
				continue
			}
			for _, v := range m.values {
				if !strings.Contains(line, v) {
					continue metric
				}
			}

			// metric found, delete it
			metrics[i] = metrics[len(metrics)-1]
			metrics = metrics[:len(metrics)-1]
			break
		}

		if err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
	}
	if len(metrics) != 0 {
		t.Fatalf("unable to find metrics: %v", metrics)
	}
}
