package testframework

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"testing"
	"time"

	"github.com/docker/distribution"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	projectapiv1 "github.com/openshift/api/project/v1"
)

type MasterContainer struct {
	ID              string
	Port            int
	NetworkSettings struct {
		IPAddress string
	}
}

func StartMasterContainer(configDir string) (*MasterContainer, error) {
	cli, err := client.NewEnvClient()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()

	progress, err := cli.ImagePull(ctx, originImageRef, types.ImagePullOptions{})
	if err != nil {
		return nil, fmt.Errorf("pull image for master container: %v", err)
	}
	_, copyErr := io.Copy(ioutil.Discard, progress)
	if err := progress.Close(); err != nil {
		return nil, fmt.Errorf("close pull progress for master container: %v", err)
	}
	if copyErr != nil {
		return nil, fmt.Errorf("read pull progress for master container: %v", copyErr)
	}

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image:      originImageRef,
		Entrypoint: []string{"/bin/sh"},
		Cmd: []string{"-ec", `
			openshift start master --write-config=/var/lib/origin/openshift.local.config/master
			sed -i'' -e '/- domainName:/d' /var/lib/origin/openshift.local.config/master/master-config.yaml
			exec openshift start master --config=/var/lib/origin/openshift.local.config/master/master-config.yaml
		`},
	}, nil, nil, "")
	if err != nil {
		return nil, fmt.Errorf("create master container: %v", err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return nil, fmt.Errorf("start master container: %v", err)
	}

	c := &MasterContainer{
		ID:   resp.ID,
		Port: 8443,
	}

	inspectResult, err := cli.ContainerInspect(ctx, resp.ID)
	if err != nil {
		// TODO(dmage): log error
		_ = c.Stop()
		return nil, fmt.Errorf("inspect master container: %v", err)
	}

	c.NetworkSettings.IPAddress = inspectResult.NetworkSettings.IPAddress

	if err := WaitTCP(c.NetworkSettings.IPAddress + ":" + strconv.Itoa(c.Port)); err != nil {
		_ = c.Stop()
		return c, err
	}

	if err := c.WriteConfigs(configDir); err != nil {
		_ = c.Stop()
		return c, err
	}

	if err := c.WaitHealthz(configDir); err != nil {
		_ = c.Stop()
		return c, err
	}

	return c, nil
}

func (c *MasterContainer) WriteConfigs(configDir string) error {
	cli, err := client.NewEnvClient()
	if err != nil {
		return err
	}

	ctx := context.Background()
	// If SRC_PATH does end with `/.`, the content of the source directory is copied.
	// https://docs.docker.com/engine/reference/commandline/cp/#extended-description
	srcPath := "/var/lib/origin/openshift.local.config/."
	dstPath := configDir

	content, stat, err := cli.CopyFromContainer(ctx, c.ID, srcPath)
	if err != nil {
		return fmt.Errorf("get configs from master container: %v", err)
	}

	srcInfo := archive.CopyInfo{
		Path:   srcPath,
		Exists: true,
		IsDir:  stat.Mode.IsDir(),
	}

	if err := archive.CopyTo(content, srcInfo, dstPath); err != nil {
		return fmt.Errorf("unpack archive with configs from master container: %v", err)
	}

	return nil
}

func (c *MasterContainer) WaitHealthz(configDir string) error {
	caBundlePath := path.Join(configDir, "master", "ca-bundle.crt")

	caBundle, err := ioutil.ReadFile(caBundlePath)
	if err != nil {
		return fmt.Errorf("unable to read CA bundle: %v", err)
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

	return WaitHTTP(rt, fmt.Sprintf("https://%s:%d/healthz", c.NetworkSettings.IPAddress, c.Port))
}

func (c *MasterContainer) Stop() error {
	cli, err := client.NewEnvClient()
	if err != nil {
		return err
	}

	ctx := context.Background()

	if err := cli.ContainerRemove(ctx, c.ID, types.ContainerRemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	}); err != nil {
		return fmt.Errorf("remove master container %s: %v", c.ID, err)
	}

	return nil
}

type User struct {
	Name       string
	Token      string
	kubeConfig *rest.Config
}

func (u *User) KubeConfig() *rest.Config {
	return u.kubeConfig
}

type Repository struct {
	distribution.Repository
	baseURL   string
	repoName  string
	transport http.RoundTripper
}

func (r *Repository) BaseURL() string {
	return r.baseURL
}

func (r *Repository) RepoName() string {
	return r.repoName
}

func (r *Repository) Transport() http.RoundTripper {
	return r.transport
}

type Master struct {
	t               *testing.T
	tmpDir          string
	container       *MasterContainer
	adminKubeConfig *rest.Config
}

func NewMaster(t *testing.T) *Master {
	tmpDir, err := ioutil.TempDir("", "image-registry-test-")
	if err != nil {
		t.Fatalf("failed to create a temporary directory for the master container: %v", err)
	}

	container, err := StartMasterContainer(tmpDir)
	if err != nil {
		if removeErr := os.RemoveAll(tmpDir); removeErr != nil {
			t.Logf("failed to remove the temporary directory: %v", removeErr)
		}
		t.Fatal(err)
	}

	m := &Master{
		t:         t,
		tmpDir:    tmpDir,
		container: container,
	}
	if err := m.WaitForRoles(); err != nil {
		if removeErr := os.RemoveAll(tmpDir); removeErr != nil {
			t.Logf("failed to remove the temporary directory: %v", removeErr)
		}
		t.Fatal(err)
	}
	return m
}

func (m *Master) WaitForRoles() error {
	// wait until the cluster roles have been aggregated
	err := wait.Poll(time.Second, time.Minute, func() (bool, error) {
		kubeClient, err := kubeclient.NewForConfig(m.AdminKubeConfig())
		if err != nil {
			return false, err
		}
		admin, err := kubeClient.RbacV1().ClusterRoles().Get("admin", metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if len(admin.Rules) == 0 {
			return false, nil
		}
		edit, err := kubeClient.RbacV1().ClusterRoles().Get("edit", metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if len(edit.Rules) == 0 {
			return false, nil
		}
		view, err := kubeClient.RbacV1().ClusterRoles().Get("view", metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if len(view.Rules) == 0 {
			return false, nil
		}

		return true, nil
	})
	if err != nil {
		m.t.Fatalf("cluster roles did not aggregate: %v", err)
	}
	return err
}

func (m *Master) Close() {
	if err := m.container.Stop(); err != nil {
		m.t.Logf("failed to stop the master container: %v", err)
	}

	if err := os.RemoveAll(m.tmpDir); err != nil {
		m.t.Logf("failed to remove the temporary directory: %v", err)
	}
}

func (m *Master) AdminKubeConfigPath() string {
	return path.Join(m.tmpDir, "master", "admin.kubeconfig")
}

func (m *Master) AdminKubeConfig() *rest.Config {
	if m.adminKubeConfig != nil {
		return m.adminKubeConfig
	}

	config, err := ConfigFromFile(m.AdminKubeConfigPath())
	if err != nil {
		m.t.Fatalf("failed to read the admin kubeconfig file: %v", err)
	}

	m.adminKubeConfig = config

	return config
}

func (m *Master) StartRegistry(t *testing.T, options ...RegistryOption) *Registry {
	ln, closeFn := StartTestRegistry(t, m.AdminKubeConfigPath(), options...)
	return &Registry{
		t:        t,
		listener: ln,
		closeFn:  closeFn,
	}
}

func (m *Master) CreateUser(username string, password string) *User {
	_, user, err := GetClientForUser(m.AdminKubeConfig(), username)
	if err != nil {
		m.t.Fatalf("failed to get a token for the user %s: %v", username, err)
	}
	return &User{
		Name:       username,
		Token:      user.BearerToken,
		kubeConfig: UserClientConfig(m.AdminKubeConfig(), user.BearerToken),
	}
}

func (m *Master) CreateProject(namespace, user string) *projectapiv1.Project {
	return CreateProject(m.t, m.AdminKubeConfig(), namespace, user)
}
