package server

import (
	"github.com/docker/distribution"
	"github.com/docker/distribution/context"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/wrapped"
)

func newWithRepositoryWrapper(repo *repository) wrapped.Wrapper {
	return func(ctx context.Context, funcname string, f func(ctx context.Context) error) error {
		return f(withRepository(ctx, repo))
	}
}

func newWithRepositoryBlobStore(bs distribution.BlobStore, repo *repository) distribution.BlobStore {
	return wrapped.NewBlobStore(bs, newWithRepositoryWrapper(repo))
}

func newWithRepositoryManifestService(ms distribution.ManifestService, repo *repository) distribution.ManifestService {
	return wrapped.NewManifestService(ms, newWithRepositoryWrapper(repo))
}

func newWithRepositoryTagService(ts distribution.TagService, repo *repository) distribution.TagService {
	return wrapped.NewTagService(ts, newWithRepositoryWrapper(repo))
}
