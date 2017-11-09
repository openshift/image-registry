package testframework

import (
	"fmt"
	"testing"

	"github.com/docker/distribution/configuration"
	"github.com/docker/distribution/context"

	"github.com/openshift/image-registry/pkg/cmd/dockerregistry"
	registryconfig "github.com/openshift/image-registry/pkg/dockerregistry/server/configuration"
	registrytest "github.com/openshift/image-registry/pkg/dockerregistry/testutil"
)

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

	return registryAddr, WaitTCP(registryAddr)
}
