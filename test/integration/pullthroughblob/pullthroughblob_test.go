package integration

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"regexp"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"

	imageapiv1 "github.com/openshift/api/image/v1"
	imageclientv1 "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"

	"github.com/openshift/image-registry/pkg/testframework"
	"github.com/openshift/image-registry/pkg/testutil"
	"github.com/openshift/image-registry/pkg/testutil/counter"
)

var accessLogLine = regexp.MustCompile(`^[^ ]+ - - \[[^]]*\] "([A-Z]+ [^"]+) HTTP/[0-9.]+"`)

func TestPullthroughBlob(t *testing.T) {
	imageData, err := testframework.NewSchema2ImageData()
	if err != nil {
		t.Fatal(err)
	}

	master := testframework.NewMaster(t)
	defer master.Close()

	testuser := master.CreateUser("testuser", "testp@ssw0rd")
	testproject := master.CreateProject("test-image-pullthrough-blob", testuser.Name)
	teststreamName := "pullthrough"

	kubeClient, err := kubeclient.NewForConfig(master.AdminKubeConfig())
	if err != nil {
		t.Fatalf("failed to create Kubernetes client: %s", err)
	}

	remoteRegistryAddr, remoteRegistryPodName, _ := testframework.CreateEphemeralRegistry(t, master.AdminKubeConfig(), testproject.Name, nil)

	requestCounter := counter.New()

	go func() {
		log, err := kubeClient.CoreV1().Pods(testproject.Name).GetLogs(remoteRegistryPodName, &corev1.PodLogOptions{
			Container: "registry",
			Follow:    true,
		}).Stream(context.Background())
		if err != nil {
			t.Logf("failed to get logs for pod %s: %s", remoteRegistryPodName, err)
			return
		}
		r := bufio.NewReader(log)
		for {
			line, readErr := r.ReadString('\n')
			if len(line) > 0 || readErr == nil {
				match := accessLogLine.FindStringSubmatch(line)
				// If it's an HTTP request and it's not a health check, count it
				if match != nil && match[1] != "GET /" {
					requestCounter.Add(match[1], 1)
				}
			}
			if readErr == io.EOF {
				break
			} else if readErr != nil {
				t.Errorf("failed to read log from pod %s: %s", remoteRegistryPodName, readErr)
				return
			}
		}
	}()

	remoteRepo, err := testutil.NewInsecureRepository(remoteRegistryAddr+"/remoteimage", nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = testframework.PushSchema2ImageData(context.TODO(), remoteRepo, "latest", imageData)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(1 * time.Second) // give logs some time to settle down

	// Step 1: Import an image
	requestCounter.Reset()

	imageClient := imageclientv1.NewForConfigOrDie(master.AdminKubeConfig())

	ctx := context.Background()

	isi, err := imageClient.ImageStreamImports(testproject.Name).Create(ctx, &imageapiv1.ImageStreamImport{
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

	teststream, err := imageClient.ImageStreams(testproject.Name).Get(ctx, teststreamName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if len(teststream.Status.Tags) == 0 {
		t.Fatalf("failed to import image: %#+v %#+v", isi, teststream)
	}
	t.Logf("failed to import image: %#+v %#+v", isi, teststream)

	time.Sleep(1 * time.Second) // give logs some time to settle down

	if diff := requestCounter.Diff(counter.M{
		"GET /v2/":                             1,
		"GET /v2/remoteimage/manifests/latest": 1,
		"GET /v2/remoteimage/blobs/" + imageData.ConfigDigest.String(): 1,
	}); diff != nil {
		t.Fatalf("unexpected number of requests: %q", diff)
	}

	// Step 2: Pull a blob
	requestCounter.Reset()

	registry := master.StartRegistry(t, testframework.DisableMirroring{})
	defer registry.Close()

	repo := registry.Repository(testproject.Name, teststream.Name, testuser)

	data, err := repo.Blobs(ctx).Get(ctx, imageData.LayerDigest)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != string(imageData.Layer) {
		t.Fatalf("got %q, want %q", string(data), string(imageData.Layer))
	}

	time.Sleep(1 * time.Second) // give logs some time to settle down

	// TODO(dmage): remove the HEAD request
	if diff := requestCounter.Diff(counter.M{
		"GET /v2/": 1,
		"HEAD /v2/remoteimage/blobs/" + imageData.LayerDigest.String(): 2,
		"GET /v2/remoteimage/blobs/" + imageData.LayerDigest.String():  1,
	}); diff != nil {
		t.Fatalf("unexpected number of requests: %q", diff)
	}
}
