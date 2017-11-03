package clientcmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/glog"

	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	kclientcmd "k8s.io/client-go/tools/clientcmd"
	kclientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/openshift/origin/pkg/cmd/flagtypes"
	"github.com/openshift/origin/pkg/cmd/util"
	"github.com/openshift/origin/pkg/oc/cli/config"
)

// Config contains all the necessary bits for client configuration
type Config struct {
	// MasterAddr is the address the master can be reached on (host, host:port, or URL).
	MasterAddr flagtypes.Addr
	// KubernetesAddr is the address of the Kubernetes server (host, host:port, or URL).
	// If omitted defaults to the master.
	KubernetesAddr flagtypes.Addr
	// CommonConfig is the shared base config for both the OpenShift config and Kubernetes config
	CommonConfig restclient.Config
	// Namespace is the namespace to act in
	Namespace string

	// If true, no environment is loaded (for testing, primarily)
	SkipEnv bool

	clientConfig clientcmd.ClientConfig
}

// NewConfig returns a new configuration
func NewConfig() *Config {
	return &Config{
		MasterAddr:     flagtypes.Addr{Value: "localhost:8080", DefaultScheme: "http", DefaultPort: 8080, AllowPrefix: true}.Default(),
		KubernetesAddr: flagtypes.Addr{Value: "localhost:8080", DefaultScheme: "http", DefaultPort: 8080}.Default(),
		CommonConfig:   restclient.Config{},
	}
}

func (cfg *Config) BindToFile(configPath string) *Config {
	defaultOverrides := &kclientcmd.ConfigOverrides{
		ClusterDefaults: kclientcmdapi.Cluster{
			Server: os.Getenv("KUBERNETES_MASTER"),
		},
	}

	chain := []string{}
	if envVarFile := os.Getenv(config.OpenShiftConfigPathEnvVar); len(envVarFile) != 0 {
		chain = append(chain, filepath.SplitList(envVarFile)...)
	} else if len(configPath) != 0 {
		chain = append(chain, configPath)
	} else {
		chain = append(chain, config.RecommendedHomeFile)
	}

	defaultClientConfig := kclientcmd.NewDefaultClientConfig(kclientcmdapi.Config{}, defaultOverrides)

	loadingRules := &kclientcmd.ClientConfigLoadingRules{
		Precedence:          chain,
		DefaultClientConfig: defaultClientConfig,
	}

	overrides := &kclientcmd.ConfigOverrides{
		ClusterDefaults: defaultOverrides.ClusterDefaults,
	}

	cfg.clientConfig = kclientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
	return cfg
}

func (cfg *Config) bindEnv() error {
	// bypass loading from env
	if cfg.SkipEnv {
		return nil
	}
	var err error

	// callers may not use the config file if they have specified a master directly, for backwards
	// compatibility with components that used to use env, switch to service account token, and have
	// config defined in env.
	_, masterSet := util.GetEnv("OPENSHIFT_MASTER")
	specifiedMaster := masterSet || cfg.MasterAddr.Provided

	if cfg.clientConfig != nil && !specifiedMaster {
		clientConfig, err := cfg.clientConfig.ClientConfig()
		if err != nil {
			return err
		}
		cfg.CommonConfig = *clientConfig
		cfg.Namespace, _, err = cfg.clientConfig.Namespace()
		if err != nil {
			return err
		}

		if !cfg.MasterAddr.Provided {
			cfg.MasterAddr.Set(cfg.CommonConfig.Host)
		}
		if !cfg.KubernetesAddr.Provided {
			cfg.KubernetesAddr.Set(cfg.CommonConfig.Host)
		}
		return nil
	}

	// Legacy path - preserve env vars set on pods that previously were honored.
	if value, ok := util.GetEnv("KUBERNETES_MASTER"); ok && !cfg.KubernetesAddr.Provided {
		cfg.KubernetesAddr.Set(value)
	}
	if value, ok := util.GetEnv("OPENSHIFT_MASTER"); ok && !cfg.MasterAddr.Provided {
		cfg.MasterAddr.Set(value)
	}
	if value, ok := util.GetEnv("BEARER_TOKEN"); ok && len(cfg.CommonConfig.BearerToken) == 0 {
		cfg.CommonConfig.BearerToken = value
	}
	if value, ok := util.GetEnv("BEARER_TOKEN_FILE"); ok && len(cfg.CommonConfig.BearerToken) == 0 {
		if tokenData, tokenErr := ioutil.ReadFile(value); tokenErr == nil {
			cfg.CommonConfig.BearerToken = strings.TrimSpace(string(tokenData))
			if len(cfg.CommonConfig.BearerToken) == 0 {
				err = fmt.Errorf("BEARER_TOKEN_FILE %q was empty", value)
			}
		} else {
			err = fmt.Errorf("Error reading BEARER_TOKEN_FILE %q: %v", value, tokenErr)
		}
	}

	if value, ok := util.GetEnv("OPENSHIFT_CA_FILE"); ok && len(cfg.CommonConfig.CAFile) == 0 {
		cfg.CommonConfig.CAFile = value
	} else if value, ok := util.GetEnv("OPENSHIFT_CA_DATA"); ok && len(cfg.CommonConfig.CAData) == 0 {
		cfg.CommonConfig.CAData = []byte(value)
	}

	if value, ok := util.GetEnv("OPENSHIFT_CERT_FILE"); ok && len(cfg.CommonConfig.CertFile) == 0 {
		cfg.CommonConfig.CertFile = value
	} else if value, ok := util.GetEnv("OPENSHIFT_CERT_DATA"); ok && len(cfg.CommonConfig.CertData) == 0 {
		cfg.CommonConfig.CertData = []byte(value)
	}

	if value, ok := util.GetEnv("OPENSHIFT_KEY_FILE"); ok && len(cfg.CommonConfig.KeyFile) == 0 {
		cfg.CommonConfig.KeyFile = value
	} else if value, ok := util.GetEnv("OPENSHIFT_KEY_DATA"); ok && len(cfg.CommonConfig.KeyData) == 0 {
		cfg.CommonConfig.KeyData = []byte(value)
	}

	if value, ok := util.GetEnv("OPENSHIFT_INSECURE"); ok && len(value) != 0 {
		cfg.CommonConfig.Insecure = value == "true"
	}

	return err
}

// KubeConfig returns the Kubernetes configuration
func (cfg *Config) KubeConfig() *restclient.Config {
	err := cfg.bindEnv()
	if err != nil {
		glog.Error(err)
	}

	kaddr := cfg.KubernetesAddr
	if !kaddr.Provided {
		kaddr = cfg.MasterAddr
	}

	kConfig := cfg.CommonConfig
	kConfig.Host = kaddr.URL.String()

	return &kConfig
}
