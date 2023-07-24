package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/distribution/distribution/v3/manifest/schema1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	imageapiv1 "github.com/openshift/api/image/v1"
	operatorapiv1alpha1 "github.com/openshift/api/operator/v1alpha1"
	imageclientv1 "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"
	operatorclientv1alpha1 "github.com/openshift/client-go/operator/clientset/versioned/typed/operator/v1alpha1"

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

	submanifestData, err := testframework.NewSchema2ImageData()
	if err != nil {
		t.Fatal(err)
	}

	imageIndexData, err := testframework.NewImageIndexData(submanifestData)
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
	imageindextag := "manifestlist"
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

	_, err = testframework.PushSchema2ImageData(context.TODO(), remoteRepo, "redundant-tag-to-make-testutil-happy", submanifestData)
	if err != nil {
		t.Fatal(err)
	}

	_, err = testframework.PushImageIndexData(context.TODO(), remoteRepo, imageindextag, imageIndexData)
	if err != nil {
		t.Fatal(err)
	}

	stream := imageapiv1.ImageStreamImport{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "myimagestream",
			Annotations: map[string]string{
				imageapiv1.InsecureRepositoryAnnotation: "true",
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
				{
					From: corev1.ObjectReference{
						Kind: "DockerImage",
						Name: remoteRegistryAddr + "/" + isname + ":" + imageindextag,
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

	if len(isi.Status.Images) != 2 {
		t.Fatalf("imported unexpected number of images (%d != 2)", len(isi.Status.Images))
	}
	for i, image := range isi.Status.Images {
		if image.Status.Status != metav1.StatusSuccess {
			t.Fatalf("unexpected status %d: %#v", i, image.Status)
		}

		if image.Image == nil {
			t.Fatalf("unexpected empty image %d", i)
		}
	}

	istream, err := adminImageClient.ImageStreams(stream.Namespace).Get(context.Background(), stream.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if istream.Annotations == nil {
		istream.Annotations = make(map[string]string)
	}
	istream.Annotations[imageapiv1.InsecureRepositoryAnnotation] = "true"

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

	t.Logf("Run testPullThroughGetManifest with submanifest digest...")
	if err := testPullThroughGetManifest(registry.BaseURL(), &stream, testuser.Name, testuser.Token, submanifestData.ManifestDigest.String()); err != nil {
		t.Fatal(err)
	}

	t.Logf("Run testPullThroughStatBlob (%s == true, spec.tags[%q].importPolicy.insecure == true)...", imageapiv1.InsecureRepositoryAnnotation, repotag)
	for digest := range descriptors {
		if err := testPullThroughStatBlob(registry.BaseURL(), &stream, testuser.Name, testuser.Token, digest); err != nil {
			t.Fatal(err)
		}
	}

	istream, err = adminImageClient.ImageStreams(stream.Namespace).Get(context.Background(), stream.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	istream.Annotations[imageapiv1.InsecureRepositoryAnnotation] = "false"

	_, err = adminImageClient.ImageStreams(istream.Namespace).Update(context.Background(), istream, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Run testPullThroughStatBlob (%s == false, spec.tags[%q].importPolicy.insecure == true)...", imageapiv1.InsecureRepositoryAnnotation, repotag)
	for digest := range descriptors {
		if err := testPullThroughStatBlob(registry.BaseURL(), &stream, testuser.Name, testuser.Token, digest); err != nil {
			t.Fatal(err)
		}
	}

	istream, err = adminImageClient.ImageStreams(stream.Namespace).Get(context.Background(), stream.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for i := range istream.Spec.Tags {
		// Ideally we need to set insecure to false only for repotag.
		// But if there is at least one tag with insecure set to true,
		// its upstream registry is allowed to be accessed insecurely
		// for all tags, so we set it to false for all tags to forbid
		// insecure access.
		istream.Spec.Tags[i].ImportPolicy.Insecure = false
	}
	_, err = adminImageClient.ImageStreams(istream.Namespace).Update(context.Background(), istream, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Run testPullThroughStatBlob (%s == false, spec.tags[%q].importPolicy.insecure == false)...", imageapiv1.InsecureRepositoryAnnotation, repotag)
	for digest := range descriptors {
		if err := testPullThroughStatBlob(registry.BaseURL(), &stream, testuser.Name, testuser.Token, digest); err == nil {
			t.Fatal("unexpexted access to insecure blobs")
		} else {
			t.Logf("%#+v", err)
		}
	}
}

func TestPullThroughICSP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	namespace := "image-registry-test-integration-icsp"
	reponame := "testrepo"
	repotag := "testtag"
	isname := "test/" + reponame

	master := testframework.NewMaster(t)
	defer master.Close()

	testuser := master.CreateUser("testuser", "testp@ssw0rd")
	master.CreateProject(namespace, testuser.Name)

	opclient := operatorclientv1alpha1.NewForConfigOrDie(master.AdminKubeConfig())
	imgclient := imageclientv1.NewForConfigOrDie(testuser.KubeConfig())

	imageData, err := testframework.NewSchema2ImageData()
	if err != nil {
		t.Fatal(err)
	}
	imgdgst := imageData.ManifestDigest.String()

	descriptors := map[string]int64{
		string(imageData.ConfigDigest): int64(len(imageData.Config)),
		string(imageData.LayerDigest):  int64(len(imageData.Layer)),
	}

	remoteRegistryAddr, _, _ := testframework.CreateEphemeralRegistry(
		t, master.AdminKubeConfig(), namespace, nil,
	)

	remoteRepo, err := testutil.NewInsecureRepository(remoteRegistryAddr+"/"+isname, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := testframework.PushSchema2ImageData(
		ctx, remoteRepo, repotag, imageData,
	); err != nil {
		t.Fatal(err)
	}

	icspRule, err := opclient.ImageContentSourcePolicies().Create(
		ctx,
		&operatorapiv1alpha1.ImageContentSourcePolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name: "image-registry-icsp-testing",
			},
			Spec: operatorapiv1alpha1.ImageContentSourcePolicySpec{
				RepositoryDigestMirrors: []operatorapiv1alpha1.RepositoryDigestMirrors{
					{
						Source: "does.not.exist/repo/image",
						Mirrors: []string{
							remoteRegistryAddr + "/" + isname,
						},
					},
				},
			},
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		t.Fatalf("error creating ICSP rule: %s", err)
	}
	defer func() {
		if err := opclient.ImageContentSourcePolicies().Delete(
			ctx, icspRule.Name, metav1.DeleteOptions{},
		); err != nil {
			t.Errorf("error deleting ICSP rule: %s", err)
		}
	}()

	imgsrc := fmt.Sprintf("does.not.exist/repo/image@%s", imgdgst)
	isi := &imageapiv1.ImageStreamImport{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "myimagestream",
			Annotations: map[string]string{
				imageapiv1.InsecureRepositoryAnnotation: "true",
			},
		},
		Spec: imageapiv1.ImageStreamImportSpec{
			Import: true,
			Images: []imageapiv1.ImageImportSpec{
				{
					From: corev1.ObjectReference{
						Kind: "DockerImage",
						Name: imgsrc,
					},
					To: &corev1.LocalObjectReference{
						Name: repotag,
					},
					ImportPolicy: imageapiv1.TagImportPolicy{
						Insecure: true,
					},
				},
			},
		},
	}

	if isi, err = imgclient.ImageStreamImports(namespace).Create(
		ctx, isi, metav1.CreateOptions{},
	); err != nil {
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

		if image.Image.Name != imgdgst {
			t.Fatalf(
				"unexpected image %d: %#v (expect %q)",
				i, image.Image.Name, imgdgst,
			)
		}
	}

	istream, err := imgclient.ImageStreams(isi.Namespace).Get(
		ctx, isi.Name, metav1.GetOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if istream.Annotations == nil {
		istream.Annotations = make(map[string]string)
	}
	istream.Annotations[imageapiv1.InsecureRepositoryAnnotation] = "true"

	istream, err = imgclient.ImageStreams(istream.Namespace).Update(
		ctx, istream, metav1.UpdateOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Run registry...")
	registry := master.StartRegistry(t, testframework.DisableMirroring{})
	defer registry.Close()

	t.Logf("Pulling manifest by tag...")
	if err := testPullThroughGetManifest(
		registry.BaseURL(), isi, testuser.Name, testuser.Token, repotag,
	); err != nil {
		t.Fatal(err)
	}

	t.Logf("Pulling manifest by digest...")
	if err := testPullThroughGetManifest(
		registry.BaseURL(), isi, testuser.Name, testuser.Token, imgdgst,
	); err != nil {
		t.Fatal(err)
	}

	t.Logf("Pulling image blobs...")
	for digest := range descriptors {
		if err := testPullThroughStatBlob(
			registry.BaseURL(), isi, testuser.Name, testuser.Token, digest,
		); err != nil {
			t.Fatal(err)
		}
	}
}
