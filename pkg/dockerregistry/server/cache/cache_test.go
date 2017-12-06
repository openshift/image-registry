package cache

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/clock"

	"github.com/docker/distribution"
	"github.com/docker/distribution/digest"
)

const (
	ttl1m = time.Minute
	ttl5m = time.Minute * 5
)

func TestDigestCacheAddDigest(t *testing.T) {
	dgst := digest.Digest("sha256:4355a46b19d348dc2f57c046f8ef63d4538ebb936000f3c9ee954a27460dd865")
	repo := "foo"
	now := time.Now()
	clock := clock.NewFakeClock(now)

	cache, err := NewBlobDigest(5, 3, ttl1m)
	if err != nil {
		t.Fatal(err)
	}

	cache.(*BlobDigest).clock = clock

	cache.Add(dgst, &DigestValue{
		desc: &distribution.Descriptor{
			Digest: dgst,
			Size:   1234,
		},
		repo: &repo,
	})

	item, err := cache.Get(dgst)
	if err != nil {
		t.Fatal(err)
	}

	if item.repositories == nil {
		t.Fatalf("unexpected empty repositories")
	}

	if item.desc.Digest != dgst {
		t.Fatalf("unexpected item: %#+v", item)
	}

	if !item.repositories.Contains(repo) {
		t.Fatalf("%q not found in the repositories", repo)
	}

	clock.Step(ttl5m)

	item, err = cache.Get(dgst)
	if err == nil || err != distribution.ErrBlobUnknown {
		t.Fatalf("item not expired")
	}

	return
}

func TestDigestCacheRemoveDigest(t *testing.T) {
	dgst := digest.Digest("sha256:4355a46b19d348dc2f57c046f8ef63d4538ebb936000f3c9ee954a27460dd865")
	repo := "foo"

	cache, err := NewBlobDigest(5, 3, ttl1m)
	if err != nil {
		t.Fatal(err)
	}

	cache.Add(dgst, &DigestValue{
		repo: &repo,
	})

	_, err = cache.Get(dgst)
	if err != nil {
		t.Fatal(err)
	}

	err = cache.Remove(dgst)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	return
}

func TestDigestCacheAddRepository(t *testing.T) {
	dgst := digest.Digest("sha256:4355a46b19d348dc2f57c046f8ef63d4538ebb936000f3c9ee954a27460dd865")
	repos := []string{"foo", "bar", "baz"}

	cache, err := NewBlobDigest(5, 1, ttl1m)
	if err != nil {
		t.Fatal(err)
	}

	for _, v := range repos {
		cache.Add(dgst, &DigestValue{
			repo: &v,
		})
	}

	item, err := cache.Get(dgst)
	if err != nil {
		t.Fatal(err)
	}

	for _, v := range repos[0:2] {
		if item.repositories.Contains(v) {
			t.Fatalf("%q found in the repositories", v)
		}
	}

	if !item.repositories.Contains(repos[len(repos)-1]) {
		t.Fatalf("%q not found in the repositories", repos[len(repos)-1])
	}
}

func TestDigestCacheRemoveRepository(t *testing.T) {
	dgst := digest.Digest("sha256:4355a46b19d348dc2f57c046f8ef63d4538ebb936000f3c9ee954a27460dd865")
	repos := []string{"foo", "bar", "baz"}

	cache, err := NewBlobDigest(5, 3, ttl1m)
	if err != nil {
		t.Fatal(err)
	}

	for _, v := range repos {
		cache.Add(dgst, &DigestValue{
			repo: &v,
		})
	}

	item, err := cache.Get(dgst)
	if err != nil {
		t.Fatal(err)
	}

	for _, v := range repos {
		if !item.repositories.Contains(v) {
			t.Fatalf("%q found in the repositories", v)
		}
	}

	for _, v := range repos {
		err = cache.RemoveRepository(dgst, v)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		item, err := cache.Get(dgst)
		if err != nil {
			t.Fatal(err)
		}

		if item.repositories.Contains(v) {
			t.Fatalf("%q found in the repositories", v)
		}
	}
}

func TestDigestCacheInvalidDigest(t *testing.T) {
	cache, err := NewBlobDigest(5, 3, ttl1m)
	if err != nil {
		t.Fatal(err)
	}

	_, err = cache.Get(digest.Digest("XXX"))
	if err != digest.ErrDigestInvalidFormat {
		t.Fatalf("unexpected answer: %v", err)
	}

	err = cache.Add(digest.Digest("XXX"), &DigestValue{})
	if err != digest.ErrDigestInvalidFormat {
		t.Fatalf("unexpected answer: %v", err)
	}

	err = cache.Remove(digest.Digest("XXX"))
	if err != digest.ErrDigestInvalidFormat {
		t.Fatalf("unexpected answer: %v", err)
	}

	err = cache.RemoveRepository(digest.Digest("XXX"), "foo")
	if err != digest.ErrDigestInvalidFormat {
		t.Fatalf("unexpected answer: %v", err)
	}
}

func TestDigestCachePurge(t *testing.T) {
	digests := []digest.Digest{
		digest.Digest("sha256:4355a46b19d348dc2f57c046f8ef63d4538ebb936000f3c9ee954a27460dd865"),
		digest.Digest("sha256:53c234e5e8472b6ac51c1ae1cab3fe06fad053beb8ebfd8977b010655bfdd3c3"),
		digest.Digest("sha256:1121cfccd5913f0a63fec40a6ffd44ea64f9dc135c66634ba001d10bcf4302a2"),
	}

	cache, err := NewBlobDigest(5, 3, ttl1m)
	if err != nil {
		t.Fatal(err)
	}

	for _, v := range digests {
		if err := cache.Add(v, &DigestValue{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	cache.Purge()

	for _, v := range digests {
		_, err = cache.Get(v)
		if err != distribution.ErrBlobUnknown {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestDigestCacheDigestMigration(t *testing.T) {
	dgst256 := digest.Digest("sha256:4355a46b19d348dc2f57c046f8ef63d4538ebb936000f3c9ee954a27460dd865")
	dgst512 := digest.Digest("sha512:3abb6677af34ac57c0ca5828fd94f9d886c26ce59a8ce60ecf6778079423dccff1d6f19cb655805d56098e6d38a1a710dee59523eed7511e5a9e4b8ccb3a4686")

	cache, err := NewBlobDigest(5, 3, ttl1m)
	if err != nil {
		t.Fatal(err)
	}

	cache.Add(dgst256, &DigestValue{
		desc: &distribution.Descriptor{
			Digest: dgst512,
			Size:   1234,
		},
	})

	item256, err := cache.Get(dgst256)
	if err != nil {
		t.Fatal(err)
	}

	item512, err := cache.Get(dgst512)
	if err != nil {
		t.Fatal(err)
	}

	if item256.desc.Digest != item512.desc.Digest {
		t.Fatalf("unexpected digest: %#+v != %#+v", item256.desc.Digest, item512.desc.Digest)
	}
}
