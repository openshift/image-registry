package integration

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/docker/distribution/manifest/schema1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kapi "k8s.io/kubernetes/pkg/api/v1"

	"github.com/openshift/origin/pkg/cmd/util/tokencmd"
	imageapi "github.com/openshift/origin/pkg/image/apis/image"
	imageapiv1 "github.com/openshift/origin/pkg/image/apis/image/v1"
	imagev1 "github.com/openshift/origin/pkg/image/generated/clientset/typed/image/v1"

	"github.com/openshift/image-registry/pkg/testframework"
)

// gzippedEmptyTar is a gzip-compressed version of an empty tar file
// (1024 NULL bytes)
var gzippedEmptyTar = []byte{
	31, 139, 8, 0, 0, 9, 110, 136, 0, 255, 98, 24, 5, 163, 96, 20, 140, 88,
	0, 8, 0, 0, 255, 255, 46, 175, 181, 239, 0, 4, 0, 0,
}

func testPullThroughGetManifest(registryAddr string, stream *imageapiv1.ImageStreamImport, user, token, urlPart string) error {
	url := fmt.Sprintf("http://%s/v2/%s/%s/manifests/%s", registryAddr, stream.Namespace, stream.Name, urlPart)

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

func testPullThroughStatBlob(registryAddr string, stream *imageapiv1.ImageStreamImport, user, token, digest string) error {
	url := fmt.Sprintf("http://%s/v2/%s/%s/blobs/%s", registryAddr, stream.Namespace, stream.Name, digest)

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
	tmpDir, err := ioutil.TempDir("", "image-registry-test-integration-")
	if err != nil {
		t.Fatalf("failed to create temporary directory: %s", err)
	}
	defer os.RemoveAll(tmpDir)

	configDir := path.Join(tmpDir, "config")
	adminKubeConfigPath := path.Join(configDir, "master", "admin.kubeconfig")

	masterContainer, err := testframework.StartMasterContainer(configDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := masterContainer.Stop(); err != nil {
			t.Log(err)
		}
	}()

	clusterAdminClientConfig, err := testframework.ConfigFromFile(adminKubeConfigPath)
	if err != nil {
		t.Fatal(err)
	}

	namespace := "image-registry-test-integration"
	//imageStreamName := "test-imagelayers"
	user := "testuser"
	password := "testp@ssw0rd"

	if err := testframework.CreateProject(clusterAdminClientConfig, namespace, user); err != nil {
		t.Fatal(err)
	}

	token, err := tokencmd.RequestToken(clusterAdminClientConfig, nil, user, password)
	if err != nil {
		t.Fatalf("error requesting token: %v", err)
	}

	// start regular HTTP server
	reponame := "testrepo"
	repotag := "testtag"
	isname := "test/" + reponame
	countStat := 0

	descriptors := map[string]int64{
		"sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4": 3000,
		"sha256:86e0e091d0da6bde2456dbb48306f3956bbeb2eae1b5b9a43045843f69fe4aaa": 200,
		"sha256:b4ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4": 10,
	}
	imageSize := int64(0)
	for _, size := range descriptors {
		imageSize += size
	}

	localIPv4, err := testframework.DefaultLocalIP4()
	if err != nil {
		t.Fatal(err)
	}

	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	_, portStr, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		http.Serve(l, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Logf("External registry got %s %s", r.Method, r.URL.Path)

			w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")

			switch r.URL.Path {
			case "/v2/":
				w.Write([]byte(`{}`))
			case "/v2/" + isname + "/tags/list":
				w.Write([]byte("{\"name\": \"" + isname + "\", \"tags\": [\"latest\", \"" + repotag + "\"]}"))
			case "/v2/" + isname + "/manifests/latest", "/v2/" + isname + "/manifests/" + repotag, "/v2/" + isname + "/manifests/" + etcdDigest:
				if r.Method == "HEAD" {
					w.Header().Set("Content-Length", fmt.Sprintf("%d", len(etcdManifest)))
					w.Header().Set("Docker-Content-Digest", etcdDigest)
					w.WriteHeader(http.StatusOK)
				} else {
					w.Write([]byte(etcdManifest))
				}
			default:
				if strings.HasPrefix(r.URL.Path, "/v2/"+isname+"/blobs/") {
					for dgst, size := range descriptors {
						if r.URL.Path != "/v2/"+isname+"/blobs/"+dgst {
							continue
						}
						if r.Method == "HEAD" {
							w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
							w.Header().Set("Docker-Content-Digest", dgst)
							w.WriteHeader(http.StatusOK)
							countStat++
							return
						}
						w.Write(gzippedEmptyTar)
						return
					}
				}
				t.Fatalf("unexpected request %s: %#v", r.URL.Path, r)
			}
		}))
	}()
	addr := net.JoinHostPort(localIPv4.String(), portStr)
	srvurl, _ := url.Parse("http://" + addr)

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
					From: kapi.ObjectReference{
						Kind: "DockerImage",
						Name: srvurl.Host + "/" + isname + ":" + repotag,
					},
					ImportPolicy: imageapiv1.TagImportPolicy{Insecure: true},
				},
			},
		},
	}

	userClientConfig := testframework.UserClientConfig(clusterAdminClientConfig, token)

	adminImageClient := imagev1.NewForConfigOrDie(userClientConfig)

	isi, err := adminImageClient.ImageStreamImports(namespace).Create(&stream)
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
		if image.Image.Name != etcdDigest {
			t.Fatalf("unexpected image %d: %#v (expect %q)", i, image.Image.Name, etcdDigest)
		}
	}

	istream, err := adminImageClient.ImageStreams(stream.Namespace).Get(stream.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if istream.Annotations == nil {
		istream.Annotations = make(map[string]string)
	}
	istream.Annotations[imageapi.InsecureRepositoryAnnotation] = "true"

	_, err = adminImageClient.ImageStreams(istream.Namespace).Update(istream)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Run registry...")
	registryAddr, err := testframework.StartTestRegistry(t, adminKubeConfigPath)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Run testPullThroughGetManifest with tag...")
	if err := testPullThroughGetManifest(registryAddr, &stream, user, token, repotag); err != nil {
		t.Fatal(err)
	}

	t.Logf("Run testPullThroughGetManifest with digest...")
	if err := testPullThroughGetManifest(registryAddr, &stream, user, token, etcdDigest); err != nil {
		t.Fatal(err)
	}

	t.Logf("Run testPullThroughStatBlob (%s == true, spec.tags[%q].importPolicy.insecure == true)...", imageapi.InsecureRepositoryAnnotation, repotag)
	for digest := range descriptors {
		if err := testPullThroughStatBlob(registryAddr, &stream, user, token, digest); err != nil {
			t.Fatal(err)
		}
	}

	istream, err = adminImageClient.ImageStreams(stream.Namespace).Get(stream.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	istream.Annotations[imageapi.InsecureRepositoryAnnotation] = "false"

	_, err = adminImageClient.ImageStreams(istream.Namespace).Update(istream)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Run testPullThroughStatBlob (%s == false, spec.tags[%q].importPolicy.insecure == true)...", imageapi.InsecureRepositoryAnnotation, repotag)
	for digest := range descriptors {
		if err := testPullThroughStatBlob(registryAddr, &stream, user, token, digest); err != nil {
			t.Fatal(err)
		}
	}

	istream, err = adminImageClient.ImageStreams(stream.Namespace).Get(stream.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for i, tag := range istream.Spec.Tags {
		if tag.Name == repotag {
			istream.Spec.Tags[i].ImportPolicy.Insecure = false
			break
		}
	}
	_, err = adminImageClient.ImageStreams(istream.Namespace).Update(istream)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Run testPullThroughStatBlob (%s == false, spec.tags[%q].importPolicy.insecure == false)...", imageapi.InsecureRepositoryAnnotation, repotag)
	for digest := range descriptors {
		if err := testPullThroughStatBlob(registryAddr, &stream, user, token, digest); err == nil {
			t.Fatal("unexpexted access to insecure blobs")
		} else {
			t.Logf("%#+v", err)
		}
	}
}
