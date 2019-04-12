package configuration

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"reflect"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"

	//"github.com/docker/distribution/registry/auth"
	"github.com/docker/distribution/configuration"
)

// Environment variables.
const (
	// dockerRegistryURLEnvVar is a mandatory environment variable name specifying url of internal docker
	// registry. All references to pushed images will be prefixed with its value.
	// DEPRECATED: Use the REGISTRY_OPENSHIFT_SERVER_ADDR instead.
	dockerRegistryURLEnvVar = "DOCKER_REGISTRY_URL"

	// openShiftDockerRegistryURLEnvVar is an optional environment that overrides the
	// DOCKER_REGISTRY_URL.
	// DEPRECATED: Use the REGISTRY_OPENSHIFT_SERVER_ADDR instead.
	openShiftDockerRegistryURLEnvVar = "REGISTRY_MIDDLEWARE_REPOSITORY_OPENSHIFT_DOCKERREGISTRYURL"

	// openShiftDefaultRegistryEnvVar overrides the dockerRegistryURLEnvVar as in OpenShift the
	// default registry URL is controlled by this environment variable.
	// DEPRECATED: Use the REGISTRY_OPENSHIFT_SERVER_ADDR instead.
	openShiftDefaultRegistryEnvVar = "OPENSHIFT_DEFAULT_REGISTRY"

	// enforceQuotaEnvVar is a boolean environment variable that allows to turn quota enforcement on or off.
	// By default, quota enforcement is off. It overrides openshift middleware configuration option.
	// Recognized values are "true" and "false".
	// DEPRECATED: Use the REGISTRY_OPENSHIFT_QUOTA_ENABLED instead.
	enforceQuotaEnvVar = "REGISTRY_MIDDLEWARE_REPOSITORY_OPENSHIFT_ENFORCEQUOTA"

	// projectCacheTTLEnvVar is an environment variable specifying an eviction timeout for project quota
	// objects. It takes a valid time duration string (e.g. "2m"). If empty, you get the default timeout. If
	// zero (e.g. "0m"), caching is disabled.
	// DEPRECATED: Use the REGISTRY_OPENSHIFT_CACHE_QUOTATTL instead.
	projectCacheTTLEnvVar = "REGISTRY_MIDDLEWARE_REPOSITORY_OPENSHIFT_PROJECTCACHETTL"

	// acceptSchema2EnvVar is a boolean environment variable that allows to accept manifest schema v2
	// on manifest put requests.
	// DEPRECATED: Use the REGISTRY_OPENSHIFT_COMPATIBILITY_ACCEPTSCHEMA2 instead.
	acceptSchema2EnvVar = "REGISTRY_MIDDLEWARE_REPOSITORY_OPENSHIFT_ACCEPTSCHEMA2"

	// blobRepositoryCacheTTLEnvVar  is an environment variable specifying an eviction timeout for <blob
	// belongs to repository> entries. The higher the value, the faster queries but also a higher risk of
	// leaking a blob that is no longer tagged in given repository.
	// DEPRECATED: Use the REGISTRY_OPENSHIFT_CACHE_BLOBREPOSITORYTTL instead.
	blobRepositoryCacheTTLEnvVar = "REGISTRY_MIDDLEWARE_REPOSITORY_OPENSHIFT_BLOBREPOSITORYCACHETTL"

	// pullthroughEnvVar is a boolean environment variable that controls whether pullthrough is enabled.
	// DEPRECATED: Use the REGISTRY_OPENSHIFT_PULLTHROUGH_ENABLED instead.
	pullthroughEnvVar = "REGISTRY_MIDDLEWARE_REPOSITORY_OPENSHIFT_PULLTHROUGH"

	// mirrorPullthroughEnvVar is a boolean environment variable that controls mirroring of blobs on pullthrough.
	// DEPRECATED: Use the REGISTRY_OPENSHIFT_PULLTHROUGH_MIRROR instead.
	mirrorPullthroughEnvVar = "REGISTRY_MIDDLEWARE_REPOSITORY_OPENSHIFT_MIRRORPULLTHROUGH"

	realmKey         = "realm"
	tokenRealmKey    = "tokenrealm"
	defaultTokenPath = "/openshift/token"

	middlewareName = "openshift"

	// Default values
	defaultBlobRepositoryCacheTTL = time.Minute * 10
	defaultProjectCacheTTL        = time.Minute
)

// TokenRealm returns the template URL to use as the token realm redirect.
// An empty scheme/host in the returned URL means to match the scheme/host on incoming requests.
func TokenRealm(tokenRealmString string) (*url.URL, error) {
	if len(tokenRealmString) == 0 {
		// If not specified, default to "/openshift/token", auto-detecting the scheme and host
		return &url.URL{Path: defaultTokenPath}, nil
	}

	tokenRealm, err := url.Parse(tokenRealmString)
	if err != nil {
		return nil, fmt.Errorf("error parsing URL in %s config option: %v", tokenRealmKey, err)
	}
	if len(tokenRealm.RawQuery) > 0 || len(tokenRealm.Fragment) > 0 {
		return nil, fmt.Errorf("%s config option may not contain query parameters or a fragment", tokenRealmKey)
	}
	if len(tokenRealm.Path) > 0 {
		return nil, fmt.Errorf("%s config option may not contain a path (%q was specified)", tokenRealmKey, tokenRealm.Path)
	}

	// pin to "/openshift/token"
	tokenRealm.Path = defaultTokenPath

	return tokenRealm, nil
}

var (
	// CurrentVersion is the most recent Version that can be parsed.
	CurrentVersion = configuration.MajorMinorVersion(1, 0)

	ErrUnsupportedVersion = errors.New("Unsupported openshift configuration version")
)

type openshiftConfig struct {
	Openshift Configuration
}

type Configuration struct {
	Version       configuration.Version `yaml:"version"`
	Metrics       Metrics               `yaml:"metrics"`
	Requests      Requests              `yaml:"requests"`
	KubeConfig    string                `yaml:"kubeconfig"`
	Server        *Server               `yaml:"server"`
	Auth          *Auth                 `yaml:"auth"`
	Audit         *Audit                `yaml:"audit"`
	Cache         *Cache                `yaml:"cache"`
	Quota         *Quota                `yaml:"quota"`
	Pullthrough   *Pullthrough          `yaml:"pullthrough"`
	Compatibility *Compatibility        `yaml:"compatibility"`
}

type Metrics struct {
	Enabled bool   `yaml:"enabled"`
	Secret  string `yaml:"secret"`
}

type Requests struct {
	Read  RequestsLimits `yaml:"read"`
	Write RequestsLimits `yaml:"write"`
}

type RequestsLimits struct {
	MaxRunning     int           `yaml:"maxrunning"`
	MaxInQueue     int           `yaml:"maxinqueue"`
	MaxWaitInQueue time.Duration `yaml:"maxwaitinqueue"`
}

type Server struct {
	Addr string `yaml:"addr"`
}

type Auth struct {
	Realm      string `yaml:"realm"`
	TokenRealm string `yaml:"tokenrealm"`
}

type Audit struct {
	Enabled bool `yaml:"enabled"`
}

type Cache struct {
	Disabled          bool          `yaml:"disabled"`
	BlobRepositoryTTL time.Duration `yaml:"blobrepositoryttl"`
}

type Quota struct {
	Enabled  bool          `yaml:"enabled"`
	CacheTTL time.Duration `yaml:"cachettl"`
}

type Pullthrough struct {
	Enabled bool `yaml:"enabled"`
	Mirror  bool `yaml:"mirror"`
}

type Compatibility struct {
	AcceptSchema2 bool `yaml:"acceptschema2"`
}

type versionInfo struct {
	Openshift struct {
		Version *configuration.Version
	}
}

// Parse parses an input configuration and returns docker configuration structure and
// openshift specific configuration.
// Environment variables may be used to override configuration parameters.
func Parse(rd io.Reader) (*configuration.Configuration, *Configuration, error) {
	in, err := ioutil.ReadAll(rd)
	if err != nil {
		return nil, nil, err
	}

	// We don't want to change the version from the environment variables.
	if err := os.Unsetenv("REGISTRY_OPENSHIFT_VERSION"); err != nil {
		return nil, nil, err
	}

	openshiftEnv, err := popEnv("REGISTRY_OPENSHIFT_")
	if err != nil {
		return nil, nil, err
	}

	dockerConfig, err := configuration.Parse(bytes.NewBuffer(in))
	if err != nil {
		return nil, nil, err
	}

	dockerEnv, err := popEnv("REGISTRY_")
	if err != nil {
		return nil, nil, err
	}
	if err := pushEnv(openshiftEnv); err != nil {
		return nil, nil, err
	}

	config := openshiftConfig{}

	vInfo := &versionInfo{}
	if err := yaml.Unmarshal(in, &vInfo); err != nil {
		return nil, nil, err
	}

	if vInfo.Openshift.Version != nil {
		if *vInfo.Openshift.Version != CurrentVersion {
			return nil, nil, ErrUnsupportedVersion
		}
	}

	p := configuration.NewParser("registry", []configuration.VersionedParseInfo{
		{
			Version: dockerConfig.Version,
			ParseAs: reflect.TypeOf(config),
			ConversionFunc: func(c interface{}) (interface{}, error) {
				return c, nil
			},
		},
	})

	if err = p.Parse(in, &config); err != nil {
		return nil, nil, err
	}
	if err := pushEnv(dockerEnv); err != nil {
		return nil, nil, err
	}

	if err := InitExtraConfig(dockerConfig, &config.Openshift); err != nil {
		return nil, nil, err
	}

	return dockerConfig, &config.Openshift, nil
}

type envVar struct {
	name  string
	value string
}

func popEnv(prefix string) ([]envVar, error) {
	var envVars []envVar

	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, prefix) {
			continue
		}
		envParts := strings.SplitN(env, "=", 2)
		err := os.Unsetenv(envParts[0])
		if err != nil {
			return nil, err
		}

		envVars = append(envVars, envVar{envParts[0], envParts[1]})
	}

	return envVars, nil
}

func pushEnv(environ []envVar) error {
	for _, env := range environ {
		if err := os.Setenv(env.name, env.value); err != nil {
			return err
		}
	}
	return nil
}

func setDefaultMiddleware(config *configuration.Configuration) {
	// Default to openshift middleware for relevant types
	// This allows custom configs based on old default configs to continue to work
	if config.Middleware == nil {
		config.Middleware = map[string][]configuration.Middleware{}
	}
	for _, middlewareType := range []string{"registry", "repository", "storage"} {
		found := false
		for _, middleware := range config.Middleware[middlewareType] {
			if middleware.Name != middlewareName {
				continue
			}
			if middleware.Disabled {
				log.Errorf("wrong configuration detected, openshift %s middleware should not be disabled in the config file", middlewareType)
				middleware.Disabled = false
			}
			found = true
			break
		}
		if found {
			continue
		}
		config.Middleware[middlewareType] = append(config.Middleware[middlewareType], configuration.Middleware{
			Name: middlewareName,
		})
	}
	// TODO(legion) This check breaks the tests. Uncomment when the tests will be able to work with auth middleware.
	/*
		authType := config.Auth.Type()
		if authType != middlewareName {
			log.Errorf("wrong configuration detected, registry should use openshift auth controller: %v", authType)
			config.Auth = make(configuration.Auth)
			config.Auth[middlewareName] = make(configuration.Parameters)
		}
	*/
}

func getServerAddr(options configuration.Parameters, cfgValue string) (registryAddr string, err error) {
	var found bool

	if len(registryAddr) == 0 {
		registryAddr, found = os.LookupEnv(openShiftDefaultRegistryEnvVar)
		if found {
			log.Infof("DEPRECATED: %q is deprecated, use the 'REGISTRY_OPENSHIFT_SERVER_ADDR' instead", openShiftDefaultRegistryEnvVar)
		}
	}

	if len(registryAddr) == 0 {
		registryAddr, found = os.LookupEnv(dockerRegistryURLEnvVar)
		if found {
			log.Infof("DEPRECATED: %q is deprecated, use the 'REGISTRY_OPENSHIFT_SERVER_ADDR' instead", dockerRegistryURLEnvVar)
		}
	}

	if len(registryAddr) == 0 {
		// Legacy configuration
		registryAddr, err = getStringOption(openShiftDockerRegistryURLEnvVar, "dockerregistryurl", registryAddr, options)
		if err != nil {
			return
		}
	}

	if len(registryAddr) == 0 && len(cfgValue) > 0 {
		registryAddr = cfgValue
	}

	// TODO: This is a fallback to assuming there is a service named 'docker-registry'. This
	// might change in the future and we should make this configurable.
	if len(registryAddr) == 0 && len(os.Getenv("DOCKER_REGISTRY_SERVICE_HOST")) > 0 && len(os.Getenv("DOCKER_REGISTRY_SERVICE_PORT")) > 0 {
		registryAddr = os.Getenv("DOCKER_REGISTRY_SERVICE_HOST") + ":" + os.Getenv("DOCKER_REGISTRY_SERVICE_PORT")
	}

	if len(registryAddr) == 0 {
		err = fmt.Errorf("REGISTRY_OPENSHIFT_SERVER_ADDR variable must be set when running outside of Kubernetes cluster")
	}

	return
}

func migrateServerSection(cfg *Configuration, options configuration.Parameters) (err error) {
	cfgAddr := ""
	if cfg.Server != nil {
		cfgAddr = cfg.Server.Addr
	} else {
		cfg.Server = &Server{}
	}
	cfg.Server.Addr, err = getServerAddr(options, cfgAddr)
	if err != nil {
		err = fmt.Errorf("configuration error in openshift.server.addr: %v", err)
	}
	return
}

func migrateQuotaSection(cfg *Configuration, options configuration.Parameters) (err error) {
	defEnabled := false
	defCacheTTL := defaultProjectCacheTTL

	if cfg.Quota != nil {
		options = configuration.Parameters{}
		defEnabled = cfg.Quota.Enabled
		defCacheTTL = cfg.Quota.CacheTTL
	} else {
		cfg.Quota = &Quota{}
	}

	cfg.Quota.Enabled, err = getBoolOption(enforceQuotaEnvVar, "enforcequota", defEnabled, options)
	if err != nil {
		err = fmt.Errorf("configuration error in openshift.quota.enabled: %v", err)
		return
	}
	cfg.Quota.CacheTTL, err = getDurationOption(projectCacheTTLEnvVar, "projectcachettl", defCacheTTL, options)
	if err != nil {
		err = fmt.Errorf("configuration error in openshift.quota.cachettl: %v", err)
	}
	return
}

func migrateCacheSection(cfg *Configuration, options configuration.Parameters) (err error) {
	defBlobRepositoryTTL := defaultBlobRepositoryCacheTTL

	if cfg.Cache != nil {
		options = configuration.Parameters{}
		defBlobRepositoryTTL = cfg.Cache.BlobRepositoryTTL
	} else {
		cfg.Cache = &Cache{}
	}

	cfg.Cache.BlobRepositoryTTL, err = getDurationOption(blobRepositoryCacheTTLEnvVar, "blobrepositorycachettl", defBlobRepositoryTTL, options)
	if err != nil {
		err = fmt.Errorf("configuration error in openshift.cache.blobrepositoryttl: %v", err)
		return
	}
	return
}

func migratePullthroughSection(cfg *Configuration, options configuration.Parameters) (err error) {
	defEnabled := true
	defMirror := true

	if cfg.Pullthrough != nil {
		options = configuration.Parameters{}
		defEnabled = cfg.Pullthrough.Enabled
		defMirror = cfg.Pullthrough.Mirror
	} else {
		cfg.Pullthrough = &Pullthrough{}
	}

	cfg.Pullthrough.Enabled, err = getBoolOption(pullthroughEnvVar, "pullthrough", defEnabled, options)
	if err != nil {
		err = fmt.Errorf("configuration error in openshift.pullthrough.enabled: %v", err)
		return
	}
	cfg.Pullthrough.Mirror, err = getBoolOption(mirrorPullthroughEnvVar, "mirrorpullthrough", defMirror, options)
	if err != nil {
		err = fmt.Errorf("configuration error in openshift.pullthrough.mirror: %v", err)
	}

	if !cfg.Pullthrough.Enabled {
		log.Warnf("pullthrough can't be disabled anymore")
		cfg.Pullthrough.Enabled = true
	}

	return
}

func migrateCompatibilitySection(cfg *Configuration, options configuration.Parameters) (err error) {
	defAcceptSchema2 := true

	if cfg.Compatibility != nil {
		options = configuration.Parameters{}
		defAcceptSchema2 = cfg.Compatibility.AcceptSchema2
	} else {
		cfg.Compatibility = &Compatibility{}
	}

	cfg.Compatibility.AcceptSchema2, err = getBoolOption(acceptSchema2EnvVar, "acceptschema2", defAcceptSchema2, options)
	if err != nil {
		err = fmt.Errorf("configuration error in openshift.compatibility.acceptschema2: %v", err)
	}
	return
}

func migrateMiddleware(dockercfg *configuration.Configuration, cfg *Configuration) (err error) {
	var repoMiddleware *configuration.Middleware
	for _, middleware := range dockercfg.Middleware["repository"] {
		if middleware.Name == middlewareName {
			repoMiddleware = &middleware
			break
		}
	}
	if repoMiddleware == nil {
		repoMiddleware = &configuration.Middleware{
			Name:    middlewareName,
			Options: make(configuration.Parameters),
		}
	}

	if cc, ok := dockercfg.Storage["cache"]; ok {
		v, ok := cc["blobdescriptor"]
		if !ok {
			// Backwards compatible: "layerinfo" == "blobdescriptor"
			v = cc["layerinfo"]
		}
		if v == "inmemory" {
			dockercfg.Storage["cache"]["blobdescriptor"] = middlewareName
		}
	}

	if cfg.Auth == nil {
		cfg.Auth = &Auth{}
		cfg.Auth.Realm, err = getStringOption("", realmKey, "origin", dockercfg.Auth.Parameters())
		if err != nil {
			err = fmt.Errorf("configuration error in openshift.auth.realm: %v", err)
			return
		}
		cfg.Auth.TokenRealm, err = getStringOption("", tokenRealmKey, "", dockercfg.Auth.Parameters())
		if err != nil {
			err = fmt.Errorf("configuration error in openshift.auth.tokenrealm: %v", err)
			return
		}
	}
	if cfg.Audit == nil {
		cfg.Audit = &Audit{}
		authParameters := dockercfg.Auth.Parameters()
		if audit, ok := authParameters["audit"]; ok {
			auditOptions := make(map[string]interface{})

			for k, v := range audit.(map[interface{}]interface{}) {
				if s, ok := k.(string); ok {
					auditOptions[s] = v
				}
			}

			cfg.Audit.Enabled, err = getBoolOption("", "enabled", false, auditOptions)
			if err != nil {
				err = fmt.Errorf("configuration error in openshift.audit.enabled: %v", err)
				return
			}
		}
	}
	for _, migrator := range []func(*Configuration, configuration.Parameters) error{
		migrateServerSection,
		migrateCacheSection,
		migrateQuotaSection,
		migratePullthroughSection,
		migrateCompatibilitySection,
	} {
		err = migrator(cfg, repoMiddleware.Options)
		if err != nil {
			return
		}
	}
	return nil
}

// staticTrustKey is provided to ensure all registries serve stable schema1
// manifests when importing / proxying from schema1 registries. Since signing
// keys in schema1 are unused and unverified, the static key here provides only
// a guarantee that the manifests pulled from this registry have a consistent
// digest when pulled sequentially.
const staticTrustKey = `
-----BEGIN EC PRIVATE KEY-----
keyID: OINE:XXVG:BLJ2:JKW4:ALVV:KV6F:IJ57:TY52:EB6U:LIKI:OVKO:YNJV

MHcCAQEEIGJWv3rS/x/w4cyuD6AfKJieuxASO/QGrZ4RqjjABkbXoAoGCCqGSM49
AwEHoUQDQgAEgwrze6IvnYZRIoVmw7Q9M/AdVZLHsL/YhmyAtEqnzJukzUDEBI50
HXY6ZXIX48v7SztCB37hHET/Vwfewi5xhA==
-----END EC PRIVATE KEY-----
`

func InitExtraConfig(dockercfg *configuration.Configuration, cfg *Configuration) error {
	setDefaultMiddleware(dockercfg)
	dockercfg.Compatibility.Schema1.Enabled = true

	// provide a stable private key to the registry to ensure content verification
	// is consistent
	f, err := ioutil.TempFile("", "static-schema1-key")
	if err != nil {
		return fmt.Errorf("unable to write static schema1 key to disk: %v", err)
	}
	if _, err := io.Copy(f, bytes.NewBufferString(staticTrustKey)); err != nil {
		return fmt.Errorf("unable to write static schema1 key to disk: %v", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("unable to write static schema1 key to disk: %v", err)
	}
	dockercfg.Compatibility.Schema1.TrustKey = f.Name()

	return migrateMiddleware(dockercfg, cfg)
}
