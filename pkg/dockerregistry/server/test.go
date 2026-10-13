package server

import (
	"context"
	"time"

	kubecache "k8s.io/apimachinery/pkg/util/cache"

	"github.com/distribution/distribution/v3"
	dockercfg "github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/registry/storage"
	dockercache "github.com/distribution/distribution/v3/registry/storage/cache"
	"github.com/distribution/distribution/v3/registry/storage/cache/memory"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/cache"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/client"
	registryclient "github.com/openshift/image-registry/pkg/dockerregistry/server/client"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/configuration"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/metrics"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/supermiddleware"
)

type testRegistryClient struct {
	client client.Interface
}

func (rc *testRegistryClient) Client() (client.Interface, error) {
	return rc.client, nil
}

func (rc *testRegistryClient) ClientFromToken(token string) (client.Interface, error) {
	return rc.client, nil
}

func newTestRegistry(
	ctx context.Context,
	osClient registryclient.Interface,
	storageDriver driver.StorageDriver,
	blobrepositorycachettl time.Duration,
	useBlobDescriptorCacheProvider bool,
) (distribution.Namespace, error) {
	cfg := &configuration.Configuration{
		Server: &configuration.Server{
			Addr: "localhost:5000",
		},
		Pullthrough: &configuration.Pullthrough{
			Enabled: true,
		},
		Cache: &configuration.Cache{
			BlobRepositoryTTL: blobrepositorycachettl,
		},
	}
	if err := configuration.InitExtraConfig(&dockercfg.Configuration{}, cfg); err != nil {
		return nil, err
	}

	digestCache, err := cache.NewBlobDigest(
		defaultDescriptorCacheSize,
		defaultDigestToRepositoryCacheSize,
		cfg.Cache.BlobRepositoryTTL,
		metrics.NewNoopMetrics(),
	)
	if err != nil {
		return nil, err
	}

	app := &App{
		registryClient: &testRegistryClient{
			client: osClient,
		},
		config: cfg,
		cache:  digestCache,
		quotaEnforcing: &quotaEnforcingConfig{
			enforcementEnabled: false,
		},
		metrics:         metrics.NewNoopMetrics(),
		paginationCache: kubecache.NewLRUExpireCache(128),
	}

	if storageDriver == nil {
		storageDriver = inmemory.New()
	}

	opts := []storage.RegistryOption{
		storage.EnableDelete,
		storage.EnableRedirect,
	}
	if useBlobDescriptorCacheProvider {
		cacheProvider := dockercache.BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider(-1))
		opts = append(opts, storage.BlobDescriptorCacheProvider(cacheProvider))
	}

	return supermiddleware.NewRegistry(ctx, app, storageDriver, opts...)
}
