package cache

import (
	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
)

type RepositoryScopedBlobDescriptor struct {
	Repo  string
	Cache DigestCache
	Svc   distribution.BlobDescriptorService
}

var _ distribution.BlobDescriptorService = &RepositoryScopedBlobDescriptor{}

// Stat provides metadata about a blob identified by the digest.
func (rbd *RepositoryScopedBlobDescriptor) Stat(ctx context.Context, dgst digest.Digest) (distribution.Descriptor, error) {
	item, err := rbd.Cache.Get(dgst)

	if err != nil && err != distribution.ErrBlobUnknown {
		return distribution.Descriptor{}, err
	}

	if item.desc == nil || !item.repositories.Contains(rbd.Repo) {
		if rbd.Svc == nil {
			return distribution.Descriptor{}, distribution.ErrBlobUnknown
		}

		desc, err := rbd.Svc.Stat(ctx, dgst)
		if err != nil {
			return distribution.Descriptor{}, err
		}

		_ = rbd.Cache.Add(dgst, &DigestValue{
			repo: &rbd.Repo,
			desc: &desc,
		})

		return desc, nil
	}

	return *item.desc, nil
}

// Clear removes digest from the repository cache
func (rbd *RepositoryScopedBlobDescriptor) Clear(ctx context.Context, dgst digest.Digest) error {
	err := rbd.Cache.RemoveRepository(dgst, rbd.Repo)
	if err != nil {
		return err
	}
	if rbd.Svc != nil {
		return rbd.Svc.Clear(ctx, dgst)
	}
	return nil
}

// SetDescriptor assigns the descriptor to the digest for repository
func (rbd *RepositoryScopedBlobDescriptor) SetDescriptor(ctx context.Context, dgst digest.Digest, desc distribution.Descriptor) error {
	err := rbd.Cache.Add(dgst, &DigestValue{
		desc: &desc,
		repo: &rbd.Repo,
	})
	if err != nil {
		return err
	}
	if rbd.Svc != nil {
		return rbd.Svc.SetDescriptor(ctx, dgst, desc)
	}
	return nil
}
