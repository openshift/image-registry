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
	"path"
	"strconv"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
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

	const originImageRef = "docker.io/openshift/origin"

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
	srcPath := "/var/lib/origin/openshift.local.config"
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
