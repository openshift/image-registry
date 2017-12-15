package supermiddleware

import (
	"github.com/Sirupsen/logrus"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/reference"
	registrymw "github.com/docker/distribution/registry/middleware/registry"
	"github.com/docker/distribution/registry/storage"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/wrapped"
)

type blobDescriptorServiceFactoryFunc func(svc distribution.BlobDescriptorService) distribution.BlobDescriptorService

var _ distribution.BlobDescriptorServiceFactory = blobDescriptorServiceFactoryFunc(nil)

func (f blobDescriptorServiceFactoryFunc) BlobAccessController(svc distribution.BlobDescriptorService) distribution.BlobDescriptorService {
	return f(svc)
}

type blobDescriptorServiceFactoryContextKey struct{}

func withBlobDescriptorServiceFactory(ctx context.Context, f distribution.BlobDescriptorServiceFactory) context.Context {
	return context.WithValue(ctx, blobDescriptorServiceFactoryContextKey{}, f)
}

func blobDescriptorServiceFactoryFrom(ctx context.Context) distribution.BlobDescriptorServiceFactory {
	f, _ := ctx.Value(blobDescriptorServiceFactoryContextKey{}).(distribution.BlobDescriptorServiceFactory)
	return f
}

type blobDescriptorServiceFactory struct{}

var _ distribution.BlobDescriptorServiceFactory = &blobDescriptorServiceFactory{}

func (f *blobDescriptorServiceFactory) BlobAccessController(svc distribution.BlobDescriptorService) distribution.BlobDescriptorService {
	return &blobDescriptorService{upstream: svc}
}

type blobDescriptorService struct {
	upstream distribution.BlobDescriptorService
	impl     distribution.BlobDescriptorService
}

var _ distribution.BlobDescriptorService = &blobDescriptorService{}

func (bds *blobDescriptorService) getImpl(ctx context.Context) (distribution.BlobDescriptorService, error) {
	if bds.impl == nil {
		bds.impl = bds.upstream
		if factory := blobDescriptorServiceFactoryFrom(ctx); factory != nil {
			bds.impl = factory.BlobAccessController(bds.impl)
		}
	}
	return bds.impl, nil
}

func (bds *blobDescriptorService) Stat(ctx context.Context, dgst digest.Digest) (distribution.Descriptor, error) {
	impl, err := bds.getImpl(ctx)
	if err != nil {
		return distribution.Descriptor{}, err
	}
	return impl.Stat(ctx, dgst)
}

func (bds *blobDescriptorService) SetDescriptor(ctx context.Context, dgst digest.Digest, desc distribution.Descriptor) error {
	impl, err := bds.getImpl(ctx)
	if err != nil {
		return err
	}
	return impl.SetDescriptor(ctx, dgst, desc)
}

func (bds *blobDescriptorService) Clear(ctx context.Context, dgst digest.Digest) error {
	impl, err := bds.getImpl(ctx)
	if err != nil {
		return err
	}
	return impl.Clear(ctx, dgst)
}

func init() {
	err := registrymw.RegisterOptions(storage.BlobDescriptorServiceFactory(&blobDescriptorServiceFactory{}))
	if err != nil {
		logrus.Fatalf("Unable to register BlobDescriptorServiceFactory: %v", err)
	}
}

func newBlobDescriptorServiceRepository(repo distribution.Repository, factory distribution.BlobDescriptorServiceFactory) distribution.Repository {
	return wrapped.NewRepository(repo, func(ctx context.Context, funcname string, f func(ctx context.Context) error) error {
		ctx = withBlobDescriptorServiceFactory(ctx, factory)
		return f(ctx)
	})
}

// effectiveCreateOptions find out what the blob creation options are going to do by dry-running them.
func effectiveCreateOptions(options []distribution.BlobCreateOption) (*distribution.CreateOptions, error) {
	opts := &distribution.CreateOptions{}
	for _, createOptions := range options {
		err := createOptions.Apply(opts)
		if err != nil {
			return nil, err
		}
	}
	return opts, nil
}

type blobDescriptorServiceBlobStore struct {
	distribution.BlobStore
	inst *instance
}

func (bs blobDescriptorServiceBlobStore) Create(ctx context.Context, options ...distribution.BlobCreateOption) (distribution.BlobWriter, error) {
	opts, err := effectiveCreateOptions(options)
	if err != nil {
		return nil, err
	}

	if opts.Mount.ShouldMount {
		named, err := reference.WithName(opts.Mount.From.Name())
		if err != nil {
			return nil, err
		}
		sourceRepo, err := bs.inst.registry.Repository(ctx, named)
		if err != nil {
			return nil, err
		}
		_, bdsf, err := bs.inst.App.Repository(ctx, sourceRepo, true)
		if err != nil {
			return nil, err
		}
		ctx = withBlobDescriptorServiceFactory(ctx, bdsf)
	}

	return bs.BlobStore.Create(ctx, options...)
}

type blobDescriptorServiceRepository struct {
	distribution.Repository
	inst *instance
}

func (r blobDescriptorServiceRepository) Blobs(ctx context.Context) distribution.BlobStore {
	bs := r.Repository.Blobs(ctx)
	return blobDescriptorServiceBlobStore{
		BlobStore: bs,
		inst:      r.inst,
	}
}
