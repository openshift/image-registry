package cache

import (
	"reflect"
	"sort"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/clock"

	"github.com/docker/distribution"
	"github.com/docker/distribution/digest"
)

func TestRepoDigest(t *testing.T) {
	digests := []struct {
		Digest digest.Digest
		Repo   string
	}{
		{
			Digest: digest.Digest("sha256:4355a46b19d348dc2f57c046f8ef63d4538ebb936000f3c9ee954a27460dd865"),
			Repo:   "foo",
		},
		{
			Digest: digest.Digest("sha256:53c234e5e8472b6ac51c1ae1cab3fe06fad053beb8ebfd8977b010655bfdd3c3"),
			Repo:   "foo",
		},
		{
			Digest: digest.Digest("sha256:1121cfccd5913f0a63fec40a6ffd44ea64f9dc135c66634ba001d10bcf4302a2"),
			Repo:   "bar",
		},
		{
			Digest: digest.Digest("sha256:1121cfccd5913f0a63fec40a6ffd44ea64f9dc135c66634ba001d10bcf4302a2"),
			Repo:   "foo",
		},
	}

	now := time.Now()
	clock := clock.NewFakeClock(now)

	cache, err := NewBlobDigest(2, 3, ttl1m)
	if err != nil {
		t.Fatal(err)
	}

	cache.(*BlobDigest).clock = clock

	r := &RepoDigest{
		Cache: cache,
	}

	for _, v := range digests {
		err := r.AddDigest(v.Digest, v.Repo)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	repos, err := r.Repositories(digest.Digest("sha256:1121cfccd5913f0a63fec40a6ffd44ea64f9dc135c66634ba001d10bcf4302a2"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(repos)

	if !reflect.DeepEqual(repos, []string{"bar", "foo"}) {
		t.Fatalf("unexpected list of repositories: %#+v", repos)
	}

	clock.Step(ttl5m)

	_, err = r.Repositories(digest.Digest("sha256:1121cfccd5913f0a63fec40a6ffd44ea64f9dc135c66634ba001d10bcf4302a2"))
	if err != distribution.ErrBlobUnknown {
		t.Fatalf("item not expired")
	}
}

func TestRepoDigestRemove(t *testing.T) {
	dgst := digest.Digest("sha256:1121cfccd5913f0a63fec40a6ffd44ea64f9dc135c66634ba001d10bcf4302a2")

	digests := []struct {
		Digest digest.Digest
		Repo   string
	}{
		{
			Digest: dgst,
			Repo:   "bar",
		},
		{
			Digest: dgst,
			Repo:   "foo",
		},
	}

	now := time.Now()
	clock := clock.NewFakeClock(now)

	cache, err := NewBlobDigest(2, 3, ttl1m)
	if err != nil {
		t.Fatal(err)
	}

	cache.(*BlobDigest).clock = clock

	r := &RepoDigest{
		Cache: cache,
	}

	for _, v := range digests {
		err := r.AddDigest(v.Digest, v.Repo)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	repos, err := r.Repositories(dgst)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(repos)

	if !reflect.DeepEqual(repos, []string{"bar", "foo"}) {
		t.Fatalf("unexpected list of repositories: %#+v", repos)
	}

	for _, n := range repos {
		if !r.ContainsRepository(dgst, n) {
			t.Fatalf("%q not found", n)
		}
	}

	err = r.RemoveDigest(dgst, "bar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	repos, err = r.Repositories(dgst)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sort.Strings(repos)

	if !reflect.DeepEqual(repos, []string{"foo"}) {
		t.Fatalf("unexpected list of repositories: %#+v", repos)
	}

	if r.ContainsRepository(dgst, "bar") {
		t.Fatalf("%q was found", "bar")
	}

	if !r.ContainsRepository(dgst, "foo") {
		t.Fatalf("%q not found", "foo")
	}

}
