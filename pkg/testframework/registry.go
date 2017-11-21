package testframework

import (
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/docker/distribution/configuration"
	"github.com/docker/distribution/context"

	"github.com/openshift/image-registry/pkg/cmd/dockerregistry"
	registryconfig "github.com/openshift/image-registry/pkg/dockerregistry/server/configuration"
	registrytest "github.com/openshift/image-registry/pkg/dockerregistry/testutil"
)

// ErrorNoDefaultIP is returned when no suitable non-loopback address can be found.
var ErrorNoDefaultIP = errors.New("no suitable IP address")

// DefaultLocalIP4 returns an IPv4 address that this host can be reached
// on. Will return NoDefaultIP if no suitable address can be found.
func DefaultLocalIP4() (net.IP, error) {
	devices, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, dev := range devices {
		if (dev.Flags&net.FlagUp != 0) && (dev.Flags&net.FlagLoopback == 0) {
			addrs, err := dev.Addrs()
			if err != nil {
				continue
			}
			for i := range addrs {
				if ip, ok := addrs[i].(*net.IPNet); ok {
					if ip.IP.To4() != nil {
						return ip.IP, nil
					}
				}
			}
		}
	}
	return nil, ErrorNoDefaultIP
}

func StartTestRegistry(t *testing.T, kubeConfigPath string) (string, error) {
	port, err := FindFreeLocalPort()
	if err != nil {
		return "", fmt.Errorf("unable to find a free local port for the registry: %v", err)
	}

	localIPv4, err := DefaultLocalIP4()
	if err != nil {
		return "", err
	}

	// FIXME(dmage): the port can be claimed by someone else before the registry is started.
	registryAddr := fmt.Sprintf("%s:%d", localIPv4, port)

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

	return registryAddr, WaitTCP(registryAddr)
}
