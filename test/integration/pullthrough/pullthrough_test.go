package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/docker/distribution/manifest/schema1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	imageapiv1 "github.com/openshift/api/image/v1"
	imageclientv1 "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"

	imageapi "github.com/openshift/image-registry/pkg/origin-common/image/apis/image"
	"github.com/openshift/image-registry/pkg/testframework"
	"github.com/openshift/image-registry/pkg/testutil"
)

func testPullThroughGetManifest(baseURL string, stream *imageapiv1.ImageStreamImport, user, token, urlPart string) error {
	url := fmt.Sprintf("%s/v2/%s/%s/manifests/%s", baseURL, stream.Namespace, stream.Name, urlPart)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.SetBasicAuth(user, token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("error retrieving manifest from registry: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading manifest: %v", err)
	}

	var retrievedManifest schema1.Manifest

	if err := json.Unmarshal(body, &retrievedManifest); err != nil {
		return fmt.Errorf("error unmarshaling retrieved manifest")
	}

	return nil
}

func testPullThroughStatBlob(baseURL string, stream *imageapiv1.ImageStreamImport, user, token, digest string) error {
	url := fmt.Sprintf("%s/v2/%s/%s/blobs/%s", baseURL, stream.Namespace, stream.Name, digest)

	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.SetBasicAuth(user, token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("error retrieving manifest from registry: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	if resp.Header.Get("Docker-Content-Digest") != digest {
		return fmt.Errorf("unexpected blob digest: %s (expected %s)", resp.Header.Get("Docker-Content-Digest"), digest)
	}

	return nil
}

func TestPullThroughInsecure(t *testing.T) {
	imageData, err := testframework.NewSchema2ImageData()
	if err != nil {
		t.Fatal(err)
	}

	master := testframework.NewMaster(t)
	defer master.Close()

	namespace := "image-registry-test-integration"
	testuser := master.CreateUser("testuser", "testp@ssw0rd")
	master.CreateProject(namespace, testuser.Name)

	// start regular HTTP server
	reponame := "testrepo"
	repotag := "testtag"
	isname := "test/" + reponame

	descriptors := map[string]int64{
		string(imageData.ConfigDigest): int64(len(imageData.Config)),
		string(imageData.LayerDigest):  int64(len(imageData.Layer)),
	}
	imageSize := int64(0)
	for _, size := range descriptors {
		imageSize += size
	}

	remoteRegistryAddr, _, _ := testframework.CreateEphemeralRegistry(t, master.AdminKubeConfig(), namespace, nil)

	remoteRepo, err := testutil.NewInsecureRepository(remoteRegistryAddr+"/"+isname, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = testframework.PushSchema2ImageData(context.TODO(), remoteRepo, repotag, imageData)
	if err != nil {
		t.Fatal(err)
	}

	stream := imageapiv1.ImageStreamImport{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "myimagestream",
			Annotations: map[string]string{
				imageapi.InsecureRepositoryAnnotation: "true",
			},
		},
		Spec: imageapiv1.ImageStreamImportSpec{
			Import: true,
			Images: []imageapiv1.ImageImportSpec{
				{
					From: corev1.ObjectReference{
						Kind: "DockerImage",
						Name: remoteRegistryAddr + "/" + isname + ":" + repotag,
					},
					ImportPolicy: imageapiv1.TagImportPolicy{Insecure: true},
				},
			},
		},
	}

	adminImageClient := imageclientv1.NewForConfigOrDie(testuser.KubeConfig())

	isi, err := adminImageClient.ImageStreamImports(namespace).Create(context.Background(), &stream, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if len(isi.Status.Images) != 1 {
		t.Fatalf("imported unexpected number of images (%d != 1)", len(isi.Status.Images))
	}
	for i, image := range isi.Status.Images {
		if image.Status.Status != metav1.StatusSuccess {
			t.Fatalf("unexpected status %d: %#v", i, image.Status)
		}

		if image.Image == nil {
			t.Fatalf("unexpected empty image %d", i)
		}

		// the image name is always the sha256, and size is calculated
		if image.Image.Name != imageData.ManifestDigest.String() {
			t.Fatalf("unexpected image %d: %#v (expect %q)", i, image.Image.Name, imageData.ManifestDigest.String())
		}
	}

	istream, err := adminImageClient.ImageStreams(stream.Namespace).Get(context.Background(), stream.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if istream.Annotations == nil {
		istream.Annotations = make(map[string]string)
	}
	istream.Annotations[imageapi.InsecureRepositoryAnnotation] = "true"

	_, err = adminImageClient.ImageStreams(istream.Namespace).Update(context.Background(), istream, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Run registry...")
	registry := master.StartRegistry(t, testframework.DisableMirroring{})
	defer registry.Close()

	t.Logf("Run testPullThroughGetManifest with tag...")
	if err := testPullThroughGetManifest(registry.BaseURL(), &stream, testuser.Name, testuser.Token, repotag); err != nil {
		t.Fatal(err)
	}

	t.Logf("Run testPullThroughGetManifest with digest...")
	if err := testPullThroughGetManifest(registry.BaseURL(), &stream, testuser.Name, testuser.Token, imageData.ManifestDigest.String()); err != nil {
		t.Fatal(err)
	}

	t.Logf("Run testPullThroughStatBlob (%s == true, spec.tags[%q].importPolicy.insecure == true)...", imageapi.InsecureRepositoryAnnotation, repotag)
	for digest := range descriptors {
		if err := testPullThroughStatBlob(registry.BaseURL(), &stream, testuser.Name, testuser.Token, digest); err != nil {
			t.Fatal(err)
		}
	}

	istream, err = adminImageClient.ImageStreams(stream.Namespace).Get(context.Background(), stream.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	istream.Annotations[imageapi.InsecureRepositoryAnnotation] = "false"

	_, err = adminImageClient.ImageStreams(istream.Namespace).Update(context.Background(), istream, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Run testPullThroughStatBlob (%s == false, spec.tags[%q].importPolicy.insecure == true)...", imageapi.InsecureRepositoryAnnotation, repotag)
	for digest := range descriptors {
		if err := testPullThroughStatBlob(registry.BaseURL(), &stream, testuser.Name, testuser.Token, digest); err != nil {
			t.Fatal(err)
		}
	}

	istream, err = adminImageClient.ImageStreams(stream.Namespace).Get(context.Background(), stream.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for i, tag := range istream.Spec.Tags {
		if tag.Name == repotag {
			istream.Spec.Tags[i].ImportPolicy.Insecure = false
			break
		}
	}
	_, err = adminImageClient.ImageStreams(istream.Namespace).Update(context.Background(), istream, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Run testPullThroughStatBlob (%s == false, spec.tags[%q].importPolicy.insecure == false)...", imageapi.InsecureRepositoryAnnotation, repotag)
	for digest := range descriptors {
		if err := testPullThroughStatBlob(registry.BaseURL(), &stream, testuser.Name, testuser.Token, digest); err == nil {
			t.Fatal("unexpexted access to insecure blobs")
		} else {
			t.Logf("%#+v", err)
		}
	}
}
