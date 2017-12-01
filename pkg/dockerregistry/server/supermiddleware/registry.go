package supermiddleware

import (
	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/reference"
	"github.com/docker/distribution/registry/storage"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
)

type registry struct {
	distribution.Namespace
	inst *instance
}

func (reg *registry) Repository(ctx context.Context, named reference.Named) (distribution.Repository, error) {
	repo, err := reg.Namespace.Repository(ctx, named)
	if err != nil {
		return repo, err
	}
	return reg.inst.Repository(ctx, repo, false)
}

// HackRegistry must not be used.
type HackRegistry struct {
	distribution.Namespace
}

// HackRepository must not be used.
func (reg HackRegistry) HackRepository(ctx context.Context, named reference.Named) (distribution.Repository, distribution.Repository, error) {
	repo, err := reg.Namespace.(*registry).Namespace.Repository(ctx, named)
	if err != nil {
		return repo, nil, err
	}

	repo = blobDescriptorServiceRepository{
		Repository: repo,
		inst:       reg.Namespace.(*registry).inst,
	}

	appRepo, bdsf, err := reg.Namespace.(*registry).inst.App.Repository(ctx, repo, false)
	if err != nil {
		return appRepo, nil, err
	}

	repo = newBlobDescriptorServiceRepository(appRepo, bdsf)

	return repo, appRepo, err
}

// NewRegistry constructs a registry object that uses app middlewares.
func NewRegistry(ctx context.Context, app App, driver storagedriver.StorageDriver, options ...storage.RegistryOption) (distribution.Namespace, error) {
	options = append(options, storage.BlobDescriptorServiceFactory(&blobDescriptorServiceFactory{}))

	inst := &instance{
		App: app,
	}

	driver, err := inst.Storage(driver, nil)
	if err != nil {
		return nil, err
	}

	reg, err := storage.NewRegistry(ctx, driver, options...)
	if err != nil {
		return reg, err
	}

	reg, err = inst.Registry(reg, nil)
	if err != nil {
		return reg, err
	}

	return &registry{
		Namespace: reg,
		inst:      inst,
	}, nil
}
