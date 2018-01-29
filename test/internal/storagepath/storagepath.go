package storagepath

import (
	"path/filepath"
	"strings"

	"github.com/docker/distribution/digest"
)

func repopath(repo string) string {
	return filepath.Join(strings.Split(repo, "/")...)
}

// prefix returns the common prefix for all paths.
func prefix() string {
	return filepath.Join(string(filepath.Separator), "docker", "registry", "v2")
}

// Layer returns the absolute path in repo for the blob with the digest dgst.
func Layer(repo string, dgst digest.Digest) string {
	repo = repopath(repo)
	return filepath.Join(prefix(), "repositories", repo, "_layers", dgst.Algorithm().String(), dgst.Hex(), "link")
}

// Manifest returns the absolute path in repo for the manifest link.
func Manifest(repo string, dgst digest.Digest) string {
	repo = repopath(repo)
	return filepath.Join(prefix(), "repositories", repo, "_manifests", "revisions", dgst.Algorithm().String(), dgst.Hex(), "link")
}
