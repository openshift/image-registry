package cache

import (
	"github.com/docker/distribution"
	"github.com/docker/distribution/digest"
)

type RepositoryDigest interface {
	AddDigest(dgst digest.Digest, repository string) error
	RemoveDigest(dgst digest.Digest, repository string) error
	AddManifest(manifest distribution.Manifest, repository string) error
	ContainsRepository(dgst digest.Digest, repository string) bool
	Repositories(dgst digest.Digest) ([]string, error)
}

type RepoDigest struct {
	Cache DigestCache
}

var _ RepositoryDigest = &RepoDigest{}

func (rd *RepoDigest) AddDigest(dgst digest.Digest, repository string) error {
	return rd.Cache.Add(dgst, &DigestValue{
		repo: &repository,
	})
}

func (rd *RepoDigest) RemoveDigest(dgst digest.Digest, repository string) error {
	return rd.Cache.RemoveRepository(dgst, repository)
}

func (rd *RepoDigest) AddManifest(manifest distribution.Manifest, repository string) error {
	refs := manifest.References()
	for i := range refs {
		err := rd.Cache.Add(refs[i].Digest, &DigestValue{
			desc: &refs[i],
			repo: &repository,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (rd *RepoDigest) ContainsRepository(dgst digest.Digest, repository string) bool {
	item, err := rd.Cache.Get(dgst)
	if err != nil {
		return false
	}

	return item.repositories.Contains(repository)
}

func (rd *RepoDigest) Repositories(dgst digest.Digest) (repos []string, err error) {
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
