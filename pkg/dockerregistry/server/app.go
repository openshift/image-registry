package server

import (
	"net/http"

	"github.com/docker/distribution"
	"github.com/docker/distribution/configuration"
	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/client"
	registryconfig "github.com/openshift/image-registry/pkg/dockerregistry/server/configuration"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/maxconnections"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/supermiddleware"
)

const (
	// Default values
	defaultDigestToRepositoryCacheSize = 2048
)

// appMiddleware should be used only in tests.
type appMiddleware interface {
	Apply(supermiddleware.App) supermiddleware.App
}

// App is a global registry application object. Shared resources can be placed
// on this object that will be accessible from all requests.
type App struct {
	// ctx is the parent context.
	ctx context.Context

	registryClient client.RegistryClient
	extraConfig    *registryconfig.Configuration
	writeLimiter   maxconnections.Limiter

	// driver gives access to the blob store.
	// This variable holds the object created by docker/distribution. We
	// import it into our namespace because there are no other ways to access
	// it. In other cases it is hidden from us.
	driver storagedriver.StorageDriver

	// registry represents a collection of repositories, addressable by name.
	// This variable holds the object created by docker/distribution. We
	// import it into our namespace because there are no other ways to access
	// it. In other cases it is hidden from us.
	registry distribution.Namespace

	// cachedLayers is a shared cache of blob digests to repositories that have previously been identified as
	// containing that blob. Thread safe and reused by all middleware layers. It contains two kinds of
	// associations:
	//  1. <blobdigest> <-> <registry>/<namespace>/<name>
	//  2. <blobdigest> <-> <namespace>/<name>
	// The first associates a blob with a remote repository. Such an entry is set and used by pullthrough
	// middleware. The second associates a blob with a local repository. Such a blob is expected to reside on
	// local storage. It's set and used by blobDescriptorService middleware.
	cachedLayers digestToRepositoryCache

	// quotaEnforcing contains shared caches of quota objects keyed by project
	// name. Will be initialized only if the quota is enforced.
	quotaEnforcing *quotaEnforcingConfig
}

func (app *App) Storage(driver storagedriver.StorageDriver, options map[string]interface{}) (storagedriver.StorageDriver, error) {
	app.driver = driver
	return driver, nil
}

func (app *App) Registry(registry distribution.Namespace, options map[string]interface{}) (distribution.Namespace, error) {
	app.registry = registry
	return registry, nil
}

// NewApp configures the registry application and returns http.Handler for it.
// The program will be terminated if an error happens.
func NewApp(ctx context.Context, registryClient client.RegistryClient, dockerConfig *configuration.Configuration, extraConfig *registryconfig.Configuration, writeLimiter maxconnections.Limiter) http.Handler {
	app := &App{
		ctx:            ctx,
		registryClient: registryClient,
		extraConfig:    extraConfig,
		writeLimiter:   writeLimiter,
		quotaEnforcing: newQuotaEnforcingConfig(ctx, extraConfig.Quota),
	}

	cache, err := newDigestToRepositoryCache(defaultDigestToRepositoryCacheSize)
	if err != nil {
		panic(err)
	}
	app.cachedLayers = cache

	superapp := supermiddleware.App(app)
	if am := appMiddlewareFrom(ctx); am != nil {
		superapp = am.Apply(superapp)
	}
	dockerApp := supermiddleware.NewApp(ctx, dockerConfig, superapp)

	if app.driver == nil {
		context.GetLogger(ctx).Fatalf("configuration error: the storage driver middleware %q is not activated", supermiddleware.Name)
	}
	if app.registry == nil {
		context.GetLogger(ctx).Fatalf("configuration error: the registry middleware %q is not activated", supermiddleware.Name)
	}

	// Add a token handling endpoint
	if dockerConfig.Auth.Type() == supermiddleware.Name {
		tokenRealm, err := registryconfig.TokenRealm(extraConfig.Auth.TokenRealm)
		if err != nil {
			context.GetLogger(dockerApp).Fatalf("error setting up token auth: %s", err)
		}
		err = dockerApp.NewRoute().Methods("GET").PathPrefix(tokenRealm.Path).Handler(NewTokenHandler(ctx, registryClient)).GetError()
		if err != nil {
			context.GetLogger(dockerApp).Fatalf("error setting up token endpoint at %q: %v", tokenRealm.Path, err)
		}
		context.GetLogger(dockerApp).Debugf("configured token endpoint at %q", tokenRealm.String())
	}

	app.registerBlobHandler(dockerApp)

	// Registry extensions endpoint provides extra functionality to handle the image
	// signatures.
	isImageClient, err := registryClient.Client()
	if err != nil {
		context.GetLogger(dockerApp).Fatalf("unable to get client for signatures: %v", err)
	}
	RegisterSignatureHandler(dockerApp, isImageClient)

	// Registry extensions endpoint provides prometheus metrics.
	if extraConfig.Metrics.Enabled {
		if len(extraConfig.Metrics.Secret) == 0 {
			context.GetLogger(dockerApp).Fatalf("openshift.metrics.secret field cannot be empty when metrics are enabled")
		}
		RegisterMetricHandler(dockerApp)
	}

	// Advertise features supported by OpenShift
	if dockerApp.Config.HTTP.Headers == nil {
		dockerApp.Config.HTTP.Headers = http.Header{}
	}
	dockerApp.Config.HTTP.Headers.Set("X-Registry-Supports-Signatures", "1")

	dockerApp.RegisterHealthChecks()

	return dockerApp
}
