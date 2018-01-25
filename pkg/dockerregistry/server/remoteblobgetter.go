package server

import (
	"net/http"
	"sync"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"

	corev1 "k8s.io/api/core/v1"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/cache"
	"github.com/openshift/image-registry/pkg/imagestream"
	"github.com/openshift/image-registry/pkg/origin-common/image/registryclient"
)

// BlobGetterService combines the operations to access and read blobs.
type BlobGetterService interface {
	distribution.BlobStatter
	distribution.BlobProvider
	distribution.BlobServer
}

type secretsGetter func() ([]corev1.Secret, error)

// digestBlobStoreCache caches BlobStores by digests. It is safe to use it
// concurrently from different goroutines (from an HTTP handler and background
// mirroring, for example).
type digestBlobStoreCache struct {
	mu   sync.RWMutex
	data map[string]distribution.BlobStore
}

func newDigestBlobStoreCache() *digestBlobStoreCache {
	return &digestBlobStoreCache{
		data: make(map[string]distribution.BlobStore),
	}
}

func (c *digestBlobStoreCache) Get(dgst digest.Digest) (distribution.BlobStore, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	bs, ok := c.data[dgst.String()]
	return bs, ok
}

func (c *digestBlobStoreCache) Put(dgst digest.Digest, bs distribution.BlobStore) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[dgst.String()] = bs
}

// remoteBlobGetterService implements BlobGetterService and allows to serve blobs from remote
// repositories.
type remoteBlobGetterService struct {
	imageStream   imagestream.ImageStream
	getSecrets    secretsGetter
	cache         cache.RepositoryDigest
	digestToStore *digestBlobStoreCache
}

var _ BlobGetterService = &remoteBlobGetterService{}

// NewBlobGetterService returns a getter for remote blobs. Its cache will be shared among different middleware
// wrappers, which is a must at least for stat calls made on manifest's dependencies during its verification.
func NewBlobGetterService(
	imageStream imagestream.ImageStream,
	secretsGetter secretsGetter,
	cache cache.RepositoryDigest,
) BlobGetterService {
	return &remoteBlobGetterService{
		imageStream:   imageStream,
		getSecrets:    secretsGetter,
		cache:         cache,
		digestToStore: newDigestBlobStoreCache(),
	}
}

// Stat provides metadata about a blob identified by the digest. If the
// blob is unknown to the describer, ErrBlobUnknown will be returned.
func (rbgs *remoteBlobGetterService) Stat(ctx context.Context, dgst digest.Digest) (distribution.Descriptor, error) {
	context.GetLogger(ctx).Debugf("(*remoteBlobGetterService).Stat: starting with dgst=%s", dgst.String())
	// look up the potential remote repositories that this blob could be part of (at this time,
	// we don't know which image in the image stream surfaced the content).
	ok, err := rbgs.imageStream.Exists()
	if err != nil {
		return distribution.Descriptor{}, err
	}
	if !ok {
		return distribution.Descriptor{}, distribution.ErrBlobUnknown
	}

	cached, _ := rbgs.cache.Repositories(dgst)

	retriever := getImportContext(ctx, rbgs.getSecrets)

	// look at the first level of tagged repositories first
	repositoryCandidates, search, err := rbgs.imageStream.IdentifyCandidateRepositories(true)
	if err != nil {
		return distribution.Descriptor{}, err
	}
	if desc, err := rbgs.findCandidateRepository(ctx, repositoryCandidates, search, cached, dgst, retriever); err == nil {
		return desc, nil
	}

	// look at all other repositories tagged by the server
	repositoryCandidates, secondary, err := rbgs.imageStream.IdentifyCandidateRepositories(false)
	if err != nil {
		return distribution.Descriptor{}, err
	}
	for k := range search {
		delete(secondary, k)
	}
	if desc, err := rbgs.findCandidateRepository(ctx, repositoryCandidates, secondary, cached, dgst, retriever); err == nil {
		return desc, nil
	}

	return distribution.Descriptor{}, distribution.ErrBlobUnknown
}

func (rbgs *remoteBlobGetterService) Open(ctx context.Context, dgst digest.Digest) (distribution.ReadSeekCloser, error) {
	context.GetLogger(ctx).Debugf("(*remoteBlobGetterService).Open: starting with dgst=%s", dgst.String())
	store, ok := rbgs.digestToStore.Get(dgst)
	if ok {
		return store.Open(ctx, dgst)
	}

	desc, err := rbgs.Stat(ctx, dgst)
	if err != nil {
		context.GetLogger(ctx).Errorf("Open: failed to stat blob %q in remote repositories: %v", dgst.String(), err)
		return nil, err
	}

	store, ok = rbgs.digestToStore.Get(desc.Digest)
	if !ok {
		return nil, distribution.ErrBlobUnknown
	}

	return store.Open(ctx, desc.Digest)
}

func (rbgs *remoteBlobGetterService) ServeBlob(ctx context.Context, w http.ResponseWriter, req *http.Request, dgst digest.Digest) error {
	context.GetLogger(ctx).Debugf("(*remoteBlobGetterService).ServeBlob: starting with dgst=%s", dgst.String())
	store, ok := rbgs.digestToStore.Get(dgst)
	if ok {
		return store.ServeBlob(ctx, w, req, dgst)
	}

	desc, err := rbgs.Stat(ctx, dgst)
	if err != nil {
		context.GetLogger(ctx).Errorf("ServeBlob: failed to stat blob %q in remote repositories: %v", dgst.String(), err)
		return err
	}

	store, ok = rbgs.digestToStore.Get(desc.Digest)
	if !ok {
		return distribution.ErrBlobUnknown
	}

	return store.ServeBlob(ctx, w, req, desc.Digest)
}

// proxyStat attempts to locate the digest in the provided remote repository or returns an error. If the digest is found,
// rbgs.digestToStore saves the store.
func (rbgs *remoteBlobGetterService) proxyStat(
	ctx context.Context,
	retriever registryclient.RepositoryRetriever,
	spec *imagestream.ImagePullthroughSpec,
	dgst digest.Digest,
) (distribution.Descriptor, error) {
	ref := spec.DockerImageReference
	insecureNote := ""
	if spec.Insecure {
		insecureNote = " with a fall-back to insecure transport"
	}
	context.GetLogger(ctx).Infof("Trying to stat %q from %q%s", dgst, ref.AsRepository().Exact(), insecureNote)
	repo, err := retriever.Repository(ctx, ref.RegistryURL(), ref.RepositoryName(), spec.Insecure)
	if err != nil {
		context.GetLogger(ctx).Errorf("Error getting remote repository for image %q: %v", ref.AsRepository().Exact(), err)
		return distribution.Descriptor{}, err
	}

	pullthroughBlobStore := repo.Blobs(ctx)
	desc, err := pullthroughBlobStore.Stat(ctx, dgst)
	if err != nil {
		if err != distribution.ErrBlobUnknown {
			context.GetLogger(ctx).Errorf("Error statting blob %s in remote repository %q: %v", dgst, ref.AsRepository().Exact(), err)
		}
		return distribution.Descriptor{}, err
	}

	rbgs.digestToStore.Put(dgst, pullthroughBlobStore)
	return desc, nil
}

// Get attempts to fetch the requested blob by digest using a remote proxy store if necessary.
func (rbgs *remoteBlobGetterService) Get(ctx context.Context, dgst digest.Digest) ([]byte, error) {
	context.GetLogger(ctx).Debugf("(*remoteBlobGetterService).Get: starting with dgst=%s", dgst.String())
	store, ok := rbgs.digestToStore.Get(dgst)
	if ok {
		return store.Get(ctx, dgst)
	}

	desc, err := rbgs.Stat(ctx, dgst)
	if err != nil {
		context.GetLogger(ctx).Errorf("Get: failed to stat blob %q in remote repositories: %v", dgst.String(), err)
		return nil, err
	}

	store, ok = rbgs.digestToStore.Get(desc.Digest)
	if !ok {
		return nil, distribution.ErrBlobUnknown
	}

	return store.Get(ctx, desc.Digest)
}

// findCandidateRepository looks in search for a particular blob, referring to previously cached items
func (rbgs *remoteBlobGetterService) findCandidateRepository(
	ctx context.Context,
	repositoryCandidates []string,
	search map[string]imagestream.ImagePullthroughSpec,
	cachedRepos []string,
	dgst digest.Digest,
	retriever registryclient.RepositoryRetriever,
) (distribution.Descriptor, error) {
	// no possible remote locations to search, exit early
	if len(search) == 0 {
		return distribution.Descriptor{}, distribution.ErrBlobUnknown
	}

	// see if any of the previously located repositories containing this digest are in this
	// image stream
	for _, repo := range cachedRepos {
		spec, ok := search[repo]
		if !ok {
			continue
		}
		desc, err := rbgs.proxyStat(ctx, retriever, &spec, dgst)
		if err != nil {
			delete(search, repo)
			continue
		}
		context.GetLogger(ctx).Infof("Found digest location from cache %q in %q", dgst, repo)
		return desc, nil
	}

	// search the remaining registries for this digest
	for _, repo := range repositoryCandidates {
		spec, ok := search[repo]
		if !ok {
			continue
		}
		desc, err := rbgs.proxyStat(ctx, retriever, &spec, dgst)
		if err != nil {
			continue
		}
		_ = rbgs.cache.AddDigest(dgst, repo)
		context.GetLogger(ctx).Infof("Found digest location by search %q in %q", dgst, repo)
		return desc, nil
	}

	return distribution.Descriptor{}, distribution.ErrBlobUnknown
}
