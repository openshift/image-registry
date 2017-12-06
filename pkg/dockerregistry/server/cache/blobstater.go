package cache

import (
	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
)

type BlobStatter struct {
	Svc   distribution.BlobStatter
	Cache DigestCache
}

var _ distribution.BlobStatter = &BlobStatter{}

// Stat provides metadata about a blob identified by the digest.
func (bs *BlobStatter) Stat(ctx context.Context, dgst digest.Digest) (distribution.Descriptor, error) {
	item, err := bs.Cache.Get(dgst)

	if err != nil && err != distribution.ErrBlobUnknown {
		return distribution.Descriptor{}, err
	}

	if item.desc == nil {
		desc, err := bs.Svc.Stat(ctx, dgst)
		if err != nil {
			return distribution.Descriptor{}, err
		}

		_ = bs.Cache.Add(dgst, &DigestValue{
			desc: &desc,
		})

		return desc, nil
	}

	return *item.desc, nil
}
