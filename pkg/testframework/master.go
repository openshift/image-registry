package testframework

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"
)

type InspectResult struct {
	State struct {
		Status string
	}
	NetworkSettings struct {
		IPAddress string
	}
}

func Inspect(containerID string) (InspectResult, error) {
	cmd := exec.Command("docker", "inspect", containerID)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return InspectResult{}, err
	}

	if err := cmd.Start(); err != nil {
		return InspectResult{}, err
	}

	var v []InspectResult
	if err := json.NewDecoder(stdout).Decode(&v); err != nil {
		return InspectResult{}, err
	}

	return v[0], cmd.Wait()
}

type MasterContainer struct {
	ID   string
	Port int
	InspectResult
}

func StartMasterContainer(configDir string) (*MasterContainer, error) {
	masterContainerIDRaw, err := exec.Command(
		"docker", "run",
		"-d",
		"--entrypoint", "/bin/sh",
		"docker.io/openshift/origin",
		"-ec", `
			openshift start master --write-config=/var/lib/origin/openshift.local.config/master
			sed -i'' -e '/- domainName:/d' /var/lib/origin/openshift.local.config/master/master-config.yaml
			exec openshift start master --config=/var/lib/origin/openshift.local.config/master/master-config.yaml
		`,
	).Output()
	if err != nil {
		return nil, fmt.Errorf("start master container: %v", err)
	}

	c := &MasterContainer{
		ID:   strings.TrimSpace(string(masterContainerIDRaw)),
		Port: 8443,
	}

	c.InspectResult, err = Inspect(c.ID)
	if err != nil {
		// TODO(dmage): log error
		_ = c.Stop()
		return c, err
	}

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
	if err := exec.Command("docker", "cp", fmt.Sprintf("%s:%s", c.ID, "/var/lib/origin/openshift.local.config"), configDir).Run(); err != nil {
		return fmt.Errorf("get configs from the master container %s: %v %#+v", c.ID, err, err)
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
	if err := exec.Command("docker", "rm", "-f", "-v", c.ID).Run(); err != nil {
		return fmt.Errorf("remove master container %s: %v", c.ID, err)
	}
	return nil
}
