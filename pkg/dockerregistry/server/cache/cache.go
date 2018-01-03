package cache

import (
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/simplelru"
	"k8s.io/apimachinery/pkg/util/clock"

	"github.com/docker/distribution"
	"github.com/docker/distribution/digest"
)

type DigestCache interface {
	Get(dgst digest.Digest) (DigestItem, error)
	Remove(dgst digest.Digest) error
	RemoveRepository(dgst digest.Digest, repository string) error
	Add(dgst digest.Digest, value *DigestValue) error
	Purge()
}

type DigestValue struct {
	desc *distribution.Descriptor
	repo *string
}

type DigestItem struct {
	expireTime   time.Time
	desc         *distribution.Descriptor
	repositories *simplelru.LRU
}

type BlobDigest struct {
	ttl      time.Duration
	repoSize int

	mu    sync.Mutex
	clock clock.Clock
	lru   *simplelru.LRU
}

func NewBlobDigest(digestSize, repoSize int, itemTTL time.Duration) (DigestCache, error) {
	digestCache, err := simplelru.NewLRU(digestSize, nil)
	if err != nil {
		return nil, err
	}

	return &BlobDigest{
		ttl:      itemTTL,
		repoSize: repoSize,
		clock:    clock.RealClock{},
		lru:      digestCache,
	}, nil
}

func (gbd *BlobDigest) get(dgst digest.Digest) *DigestItem {
	if value, ok := gbd.lru.Get(dgst); ok {
		d, _ := value.(*DigestItem)
		return d
	}
	return nil
}

func (gbd *BlobDigest) peek(dgst digest.Digest) *DigestItem {
	if value, ok := gbd.lru.Peek(dgst); ok {
		d, _ := value.(*DigestItem)
		return d
	}
	return nil
}

func (gbd *BlobDigest) Purge() {
	gbd.mu.Lock()
	defer gbd.mu.Unlock()

	gbd.lru.Purge()
}

func (gbd *BlobDigest) Get(dgst digest.Digest) (DigestItem, error) {
	if err := dgst.Validate(); err != nil {
		return DigestItem{}, err
	}

	if gbd.ttl == 0 {
		return DigestItem{}, distribution.ErrBlobUnknown
	}

	gbd.mu.Lock()
	defer gbd.mu.Unlock()

	value := gbd.get(dgst)

	if value == nil {
		return DigestItem{}, distribution.ErrBlobUnknown
	}

	if value.expireTime.Before(gbd.clock.Now()) {
		gbd.lru.Remove(dgst)
		return DigestItem{}, distribution.ErrBlobUnknown
	}

	return *value, nil
}

func (gbd *BlobDigest) Remove(dgst digest.Digest) error {
	if err := dgst.Validate(); err != nil {
		return err
	}

	if gbd.ttl == 0 {
		return nil
	}

	gbd.mu.Lock()
	defer gbd.mu.Unlock()

	gbd.lru.Remove(dgst)
	return nil
}

func (gbd *BlobDigest) RemoveRepository(dgst digest.Digest, repository string) error {
	if err := dgst.Validate(); err != nil {
		return err
	}

	if gbd.ttl == 0 {
		return nil
	}

	gbd.mu.Lock()
	defer gbd.mu.Unlock()

	value := gbd.peek(dgst)

	if value == nil {
		return nil
	}

	value.repositories.Remove(repository)
	return nil
}

func (gbd *BlobDigest) Add(dgst digest.Digest, item *DigestValue) error {
	if err := dgst.Validate(); err != nil {
		return err
	}

	if item == nil || (item.desc == nil && item.repo == nil) {
		return nil
	}

	if gbd.ttl == 0 {
		return nil
	}

	gbd.mu.Lock()
	defer gbd.mu.Unlock()

	value := gbd.get(dgst)

	if value == nil {
		lru, err := simplelru.NewLRU(gbd.repoSize, nil)
		if err != nil {
			return err
		}

		value = &DigestItem{
			expireTime:   gbd.clock.Now().Add(gbd.ttl),
			repositories: lru,
		}
	}

	if item.repo != nil {
		value.repositories.Add(*item.repo, struct{}{})
	}

	if item.desc != nil {
		value.desc = item.desc

		if dgst.Algorithm() != item.desc.Digest.Algorithm() && dgst != item.desc.Digest {
			// if the digests differ, set the other canonical mapping
			gbd.lru.Add(item.desc.Digest, value)
		}
	}

	gbd.lru.Add(dgst, value)

	return nil
}
