package integration

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/docker/distribution"
	"github.com/docker/distribution/configuration"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"
	kapi "k8s.io/kubernetes/pkg/api/v1"

	authorizationapiv1 "github.com/openshift/origin/pkg/authorization/apis/authorization/v1"
	authorizationv1 "github.com/openshift/origin/pkg/authorization/generated/clientset/typed/authorization/v1"
	"github.com/openshift/origin/pkg/cmd/util/tokencmd"
	imageapi "github.com/openshift/origin/pkg/image/apis/image"
	imagev1 "github.com/openshift/origin/pkg/image/generated/clientset/typed/image/v1"
	projectapiv1 "github.com/openshift/origin/pkg/project/apis/project/v1"
	projectv1 "github.com/openshift/origin/pkg/project/generated/clientset/typed/project/v1"

	"github.com/openshift/image-registry/pkg/cmd/dockerregistry"
	registryconfig "github.com/openshift/image-registry/pkg/dockerregistry/server/configuration"
	registrytest "github.com/openshift/image-registry/pkg/dockerregistry/testutil"
)

// FindFreeLocalPort returns the number of an available port number on
// the loopback interface.  Useful for determining the port to launch
// a server on.  Error handling required - there is a non-zero chance
// that the returned port number will be bound by another process
// after this function returns.
//
// k8s.io/kubernetes/test/integration/framework.FindFreeLocalPort
func FindFreeLocalPort() (int, error) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	_, portStr, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		return 0, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, err
	}
	return port, nil
}

func waitTCP(addr string) error {
	var lastErr error
	err := wait.Poll(500*time.Millisecond, wait.ForeverTestTimeout, func() (done bool, err error) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			lastErr = err
			return false, nil
		}
		_ = conn.Close()
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("wait for %s: %v", addr, lastErr)
	}
	return nil
}

func waitHTTP(rt http.RoundTripper, url string) error {
	var lastErr error
	httpClient := &http.Client{
		Transport: rt,
	}
	err := wait.Poll(500*time.Millisecond, wait.ForeverTestTimeout, func() (done bool, err error) {
		resp, err := httpClient.Get(url)
		if err != nil {
			lastErr = err
			return false, nil
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("%s", resp.Status)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("wait for %s: %v", url, lastErr)
	}
	return nil
}

func StartTestRegistry(t *testing.T, kubeConfigPath string) (string, error) {
	port, err := FindFreeLocalPort()
	if err != nil {
		return "", fmt.Errorf("unable to find a free local port for the registry: %v", err)
	}

	// FIXME(dmage): the port can be claimed by someone else before the registry is started.
	registryAddr := fmt.Sprintf("127.0.0.1:%d", port)

	dockerConfig := &configuration.Configuration{
		Version: "0.1",
		Storage: configuration.Storage{
			"inmemory": configuration.Parameters{},
		},
		Auth: configuration.Auth{
			"openshift": configuration.Parameters{},
		},
		Middleware: map[string][]configuration.Middleware{
			"registry": {{
				Name: "openshift",
			}},
			"repository": {{
				Name: "openshift",
				Options: configuration.Parameters{
					"dockerregistryurl":      registryAddr,
					"acceptschema2":          true,
					"pullthrough":            true,
					"enforcequota":           false,
					"projectcachettl":        "1m",
					"blobrepositorycachettl": "10m",
				},
			}},
			"storage": {{
				Name: "openshift",
			}},
		},
	}
	dockerConfig.Log.Level = "debug"
	dockerConfig.HTTP.Addr = registryAddr

	extraConfig := &registryconfig.Configuration{
		KubeConfig: kubeConfigPath,
	}

	go func() {
		ctx := context.Background()
		ctx = registrytest.WithTestLogger(ctx, t)
		err := dockerregistry.Start(ctx, dockerConfig, extraConfig)
		// We cannot call t.Fatal here, because it's a different goroutine.
		panic(fmt.Errorf("failed to start the image registry: %v", err))
	}()

	return registryAddr, waitTCP(registryAddr)
}

// uploadImageWithSchema2Manifest creates a random image with a schema 2
// manifest and uploads it to the repository.
func uploadImageWithSchema2Manifest(ctx context.Context, repo distribution.Repository, tag string) error {
	layers := make([]distribution.Descriptor, 3)
	for i := range layers {
		content, desc, err := registrytest.MakeRandomLayer()
		if err != nil {
			return fmt.Errorf("make random layer: %v", err)
		}

		if err := registrytest.UploadBlob(ctx, repo, desc, content); err != nil {
			return fmt.Errorf("upload random blob: %v", err)
		}

		layers[i] = desc
	}

	cfg := map[string]interface{}{
		"rootfs": map[string]interface{}{
			"diff_ids": make([]string, len(layers)),
		},
		"history": make([]struct{}, len(layers)),
	}

	configContent, err := json.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("marshal image config: %v", err)
	}

	config := distribution.Descriptor{
		Digest: digest.FromBytes(configContent),
		Size:   int64(len(configContent)),
	}

	if err := registrytest.UploadBlob(ctx, repo, config, configContent); err != nil {
		return fmt.Errorf("upload image config: %v", err)
	}

	manifest, err := registrytest.MakeSchema2Manifest(config, layers)
	if err != nil {
		return fmt.Errorf("make schema 2 manifest: %v", err)
	}

	if err := registrytest.UploadManifest(ctx, repo, tag, manifest); err != nil {
		return fmt.Errorf("upload schema 2 manifest: %v", err)
	}

	return nil
}

// getSchema1Manifest simulates a client which supports only schema 1
// manifests, fetches a manifest from a registry and returns it.
func getSchema1Manifest(transport http.RoundTripper, baseURL, repoName, tag string) (distribution.Manifest, error) {
	c := &http.Client{
		Transport: transport,
	}

	resp, err := c.Get(baseURL + "/v2/" + repoName + "/manifests/" + tag)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read manifest %s:%s: %v", repoName, tag, err)
	}

	m, _, err := distribution.UnmarshalManifest(resp.Header.Get("Content-Type"), body)
	return m, err
}

type InspectResult struct {
	State struct {
		Status string
	}
	NetworkSettings struct {
		IPAddress string
	}
}

func Inspect(containerID string) (*InspectResult, error) {
	cmd := exec.Command("docker", "inspect", containerID)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var v []*InspectResult
	if err := json.NewDecoder(stdout).Decode(&v); err != nil {
		return nil, err
	}

	return v[0], cmd.Wait()
}

// TestImageLayers tests that the integrated registry handles schema 1
// manifests and schema 2 manifests consistently and it produces similar Image
// resources for them.
//
// The test relies on ability of the registry to downconvert manifests.
func TestImageLayers(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "image-registry-test-integration-")
	if err != nil {
		t.Fatalf("failed to create temporary directory: %s", err)
	}
	defer os.RemoveAll(tmpDir)

	configDir := path.Join(tmpDir, "config")
	caBundlePath := path.Join(configDir, "master", "ca-bundle.crt")
	adminKubeConfigPath := path.Join(configDir, "master", "admin.kubeconfig")

	masterContainerIDRaw, err := exec.Command(
		"docker", "run", "-d",
		"docker.io/openshift/origin", "start", "master",
	).Output()
	if err != nil {
		t.Fatalf("failed to run the origin container: %v", err)
	}
	masterContainerID := strings.TrimSpace(string(masterContainerIDRaw))
	defer func() {
		if err := exec.Command("docker", "rm", "-f", "-v", masterContainerID).Run(); err != nil {
			t.Logf("failed to remove the master container %s: %v", masterContainerID, err)
		}
	}()

	masterContainer, err := Inspect(masterContainerID)
	if err != nil {
		t.Fatal(err)
	}

	if err := waitTCP(masterContainer.NetworkSettings.IPAddress + ":8443"); err != nil {
		t.Fatal(err)
	}

	if err := exec.Command("docker", "cp", fmt.Sprintf("%s:%s", masterContainerID, "/var/lib/origin/openshift.local.config"), configDir).Run(); err != nil {
		t.Fatalf("failed to get configs from the master container %s: %v", masterContainerID, err)
	}

	caBundle, err := ioutil.ReadFile(caBundlePath)
	if err != nil {
		t.Fatalf("failed to read CA bundle: %v", err)
	}

	rootCAs := x509.NewCertPool()
	rootCAs.AppendCertsFromPEM(caBundle)
	tlsConfig := &tls.Config{
		RootCAs: rootCAs,
	}
	rt := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       tlsConfig,
	}

	if err := waitHTTP(rt, fmt.Sprintf("https://%s:8443/healthz", masterContainer.NetworkSettings.IPAddress)); err != nil {
		t.Fatal(err)
	}

	adminConfig, err := clientcmd.LoadFromFile(adminKubeConfigPath)
	if err != nil {
		t.Fatal(err)
	}

	adminClientConfig, err := clientcmd.NewDefaultClientConfig(*adminConfig, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		t.Fatal(err)
	}

	imageClient := imagev1.NewForConfigOrDie(adminClientConfig)

	namespace := "image-registry-test-integration"
	imageStreamName := "test-imagelayers"
	user := "testuser"
	password := "testp@ssw0rd"

	projectClient := projectv1.NewForConfigOrDie(adminClientConfig)
	_, err = projectClient.ProjectRequests().Create(&projectapiv1.ProjectRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	authorizationClient := authorizationv1.NewForConfigOrDie(adminClientConfig)
	_, err = authorizationClient.RoleBindings(namespace).Update(&authorizationapiv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "admin",
		},
		UserNames: []string{user},
		RoleRef: kapi.ObjectReference{
			Name: "admin",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	token, err := tokencmd.RequestToken(adminClientConfig, nil, user, password)
	if err != nil {
		t.Fatalf("error requesting token: %v", err)
	}

	registryAddr, err := StartTestRegistry(t, adminKubeConfigPath)
	if err != nil {
		t.Fatalf("start registry: %v", err)
	}

	creds := registrytest.NewBasicCredentialStore(user, token)

	baseURL := "http://" + registryAddr
	repoName := fmt.Sprintf("%s/%s", namespace, imageStreamName)

	schema1Tag := "schema1"
	schema2Tag := "schema2"

	transport, err := registrytest.NewTransport(baseURL, repoName, creds)
	if err != nil {
		t.Fatalf("get transport: %v", err)
	}

	ctx := context.Background()

	repo, err := registrytest.NewRepository(ctx, repoName, baseURL, transport)
	if err != nil {
		t.Fatalf("get repository: %v", err)
	}

	if err := uploadImageWithSchema2Manifest(ctx, repo, schema2Tag); err != nil {
		t.Fatalf("upload image with schema 2 manifest: %v", err)
	}

	// get the schema2 image's manifest downconverted to a schema 1 manifest
	schema1Manifest, err := getSchema1Manifest(transport, baseURL, repoName, schema2Tag)
	if err != nil {
		t.Fatalf("get schema 1 manifest for image schema2: %v", err)
	}

	if err := registrytest.UploadManifest(ctx, repo, schema1Tag, schema1Manifest); err != nil {
		t.Fatalf("upload schema 1 manifest: %v", err)
	}

	schema1ISTag, err := imageClient.ImageStreamTags(namespace).Get(imageStreamName+":"+schema1Tag, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get image stream tag %s:%s: %v", imageStreamName, schema1Tag, err)
	}

	schema2ISTag, err := imageClient.ImageStreamTags(namespace).Get(imageStreamName+":"+schema2Tag, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get image stream tag %s:%s: %v", imageStreamName, schema1Tag, err)
	}

	if schema1ISTag.Image.DockerImageManifestMediaType == schema2ISTag.Image.DockerImageManifestMediaType {
		t.Errorf("expected different media types, but got %q", schema1ISTag.Image.DockerImageManifestMediaType)
	}

	image1LayerOrder := schema1ISTag.Image.Annotations[imageapi.DockerImageLayersOrderAnnotation]
	image2LayerOrder := schema2ISTag.Image.Annotations[imageapi.DockerImageLayersOrderAnnotation]
	if image1LayerOrder != image2LayerOrder {
		t.Errorf("the layer order annotations are different: schema1=%q, schema2=%q", image1LayerOrder, image2LayerOrder)
	} else if image1LayerOrder == "" {
		t.Errorf("the layer order annotation is empty or not present")
	}

	image1Layers := schema1ISTag.Image.DockerImageLayers
	image2Layers := schema2ISTag.Image.DockerImageLayers
	if len(image1Layers) != len(image2Layers) {
		t.Errorf("layers are different: schema1=%#+v, schema2=%#+v", image1Layers, image2Layers)
	} else {
		for i := range image1Layers {
			if image1Layers[i].Name != image2Layers[i].Name {
				t.Errorf("different names for the layer #%d: schema1=%#+v, schema2=%#+v", i, image1Layers[i], image2Layers[i])
			}
			if image1Layers[i].LayerSize != image2Layers[i].LayerSize {
				t.Errorf("different sizes for the layer #%d: schema1=%#+v, schema2=%#+v", i, image1Layers[i], image2Layers[i])
			} else if image1Layers[i].LayerSize <= 0 {
				t.Errorf("unexpected size for the layer #%d: %d", i, image1Layers[i].LayerSize)
			}
		}
	}
}
