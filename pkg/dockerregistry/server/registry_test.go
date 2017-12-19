package server

import (
	"fmt"
	"testing"
	"time"

	"github.com/docker/distribution"
	dockercfg "github.com/docker/distribution/configuration"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/reference"
	"github.com/docker/distribution/registry/storage"
	"github.com/docker/distribution/registry/storage/cache"
	"github.com/docker/distribution/registry/storage/cache/memory"
	"github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/inmemory"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/client"
	registryclient "github.com/openshift/image-registry/pkg/dockerregistry/server/client"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/configuration"
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
	pullthrough bool,
	useBlobDescriptorCacheProvider bool,
) (distribution.Namespace, error) {
	cachedLayers, err := newDigestToRepositoryCache(defaultDigestToRepositoryCacheSize)
	if err != nil {
		return nil, err
	}

	cfg := &configuration.Configuration{
		Server: &configuration.Server{
			Addr: "localhost:5000",
		},
		Pullthrough: &configuration.Pullthrough{
			Enabled: pullthrough,
		},
		Cache: &configuration.Cache{
			BlobRepositoryTTL: blobrepositorycachettl,
		},
	}
	if err := configuration.InitExtraConfig(&dockercfg.Configuration{}, cfg); err != nil {
		return nil, err
	}

	app := &App{
		registryClient: &testRegistryClient{
			client: osClient,
		},
		config:       cfg,
		cachedLayers: cachedLayers,
		quotaEnforcing: &quotaEnforcingConfig{
			enforcementEnabled: false,
		},
	}

	if storageDriver == nil {
		storageDriver = inmemory.New()
	}

	opts := []storage.RegistryOption{
		storage.EnableDelete,
		storage.EnableRedirect,
	}
	if useBlobDescriptorCacheProvider {
		cacheProvider := cache.BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider())
		opts = append(opts, storage.BlobDescriptorCacheProvider(cacheProvider))
	}

	return supermiddleware.NewRegistry(ctx, app, storageDriver, opts...)
}

type testRepository struct {
	distribution.Repository

	name  reference.Named
	blobs distribution.BlobStore
}

func (r *testRepository) Named() reference.Named {
	return r.name
}

func (r *testRepository) Blobs(ctx context.Context) distribution.BlobStore {
	return r.blobs
}

type testRepositoryOptions struct {
	client            client.Interface
	enablePullThrough bool
	blobs             distribution.BlobStore
}

func newTestRepository(
	ctx context.Context,
	t *testing.T,
	namespace, repoName string,
	opts testRepositoryOptions,
) *repository {
	reg, err := newTestRegistry(ctx, opts.client, nil, 0, opts.enablePullThrough, false)
	if err != nil {
		t.Fatal(err)
	}

	named, err := reference.ParseNamed(fmt.Sprintf("%s/%s", namespace, repoName))
	if err != nil {
		t.Fatal(err)
	}

	_, appRepo, err := supermiddleware.HackRegistry{Namespace: reg}.HackRepository(ctx, named)
	if err != nil {
		t.Fatal(err)
	}

	r := appRepo.(*repository)
	// TODO(dmage): can we avoid this replacement?
	r.Repository = &testRepository{
		name:  named,
		blobs: opts.blobs,
	}
	return r
}
