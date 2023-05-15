package cache

import (
	"context"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/reference"
	"github.com/distribution/distribution/v3/registry/storage/cache"
	"github.com/opencontainers/go-digest"
)

type Provider struct {
	Cache DigestCache
}

var _ cache.BlobDescriptorCacheProvider = &Provider{}

func (c *Provider) RepositoryScoped(repo string) (distribution.BlobDescriptorService, error) {
	if _, err := reference.WithName(repo); err != nil {
		return nil, err
	}
	return &RepositoryScopedBlobDescriptor{
		Repo:  repo,
		Cache: c.Cache,
	}, nil
}

func (c *Provider) Stat(ctx context.Context, dgst digest.Digest) (distribution.Descriptor, error) {
	return c.Cache.Get(dgst)
}

func (c *Provider) SetDescriptor(ctx context.Context, dgst digest.Digest, desc distribution.Descriptor) error {
	return c.Cache.Add(dgst, &DigestValue{
		desc: &desc,
	})
}

func (c *Provider) Clear(ctx context.Context, dgst digest.Digest) error {
	return c.Cache.Remove(dgst)
}
