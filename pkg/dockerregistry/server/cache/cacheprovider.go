package cache

import (
	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/reference"
	"github.com/docker/distribution/registry/storage/cache"
)

type Provider struct {
	Cache DigestCache
}

var _ cache.BlobDescriptorCacheProvider = &Provider{}

func (c *Provider) RepositoryScoped(repo string) (distribution.BlobDescriptorService, error) {
	if _, err := reference.ParseNamed(repo); err != nil {
		return nil, err
	}
	return &RepositoryScopedProvider{
		Repo:  repo,
		Cache: c.Cache,
	}, nil
}

func (c *Provider) Stat(ctx context.Context, dgst digest.Digest) (distribution.Descriptor, error) {
	item, err := c.Cache.Get(dgst)

	if err != nil && err != distribution.ErrBlobUnknown {
		return distribution.Descriptor{}, err
	}

	if item.desc == nil {
		return distribution.Descriptor{}, distribution.ErrBlobUnknown
	}

	return *item.desc, nil
}

func (c *Provider) SetDescriptor(ctx context.Context, dgst digest.Digest, desc distribution.Descriptor) error {
	return c.Cache.Add(dgst, &DigestValue{
		desc: &desc,
	})
}

func (c *Provider) Clear(ctx context.Context, dgst digest.Digest) error {
	return c.Cache.Remove(dgst)
}

type RepositoryScopedProvider struct {
	Repo  string
	Cache DigestCache
}

var _ distribution.BlobDescriptorService = &RepositoryScopedProvider{}

func (c *RepositoryScopedProvider) Stat(ctx context.Context, dgst digest.Digest) (distribution.Descriptor, error) {
	item, err := c.Cache.Get(dgst)

	if err != nil && err != distribution.ErrBlobUnknown {
		return distribution.Descriptor{}, err
	}

	if item.desc == nil || !item.repositories.Contains(c.Repo) {
		return distribution.Descriptor{}, distribution.ErrBlobUnknown
	}

	return *item.desc, nil
}

func (c *RepositoryScopedProvider) SetDescriptor(ctx context.Context, dgst digest.Digest, desc distribution.Descriptor) error {
	return c.Cache.Add(dgst, &DigestValue{
		desc: &desc,
		repo: &c.Repo,
	})
}

func (c *RepositoryScopedProvider) Clear(ctx context.Context, dgst digest.Digest) error {
	return c.Cache.RemoveRepository(dgst, c.Repo)
}
