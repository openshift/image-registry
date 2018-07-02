package cache

import "github.com/docker/distribution/digest"

type RepositoryDigest interface {
	AddDigest(dgst digest.Digest, repository string) error
	ContainsRepository(dgst digest.Digest, repository string) bool
	Repositories(dgst digest.Digest) ([]string, error)
}

type repositoryDigest struct {
	Cache DigestCache
}

var _ RepositoryDigest = &repositoryDigest{}

func NewRepositoryDigest(cache DigestCache) RepositoryDigest {
	return &repositoryDigest{
		Cache: cache,
	}
}

func (rd *repositoryDigest) AddDigest(dgst digest.Digest, repository string) error {
	return rd.Cache.Add(dgst, &DigestValue{
		repo: &repository,
	})
}

func (rd *repositoryDigest) ContainsRepository(dgst digest.Digest, repository string) bool {
	item, err := rd.Cache.Get(dgst)
	if err != nil {
		return false
	}

	return item.repositories.Contains(repository)
}

func (rd *repositoryDigest) Repositories(dgst digest.Digest) (repos []string, err error) {
	var item DigestItem

	item, err = rd.Cache.Get(dgst)
	if err != nil {
		return
	}

	for _, v := range item.repositories.Keys() {
		s := v.(string)
		repos = append(repos, s)
	}

	return
}
