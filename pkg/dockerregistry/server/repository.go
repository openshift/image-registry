package server

import (
	"fmt"
	"net/http"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	registrystorage "github.com/docker/distribution/registry/storage"

	restclient "k8s.io/client-go/rest"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/audit"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/cache"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/metrics"
	"github.com/openshift/image-registry/pkg/imagestream"
)

var (
	// secureTransport is the transport pool used for pullthrough to remote registries marked as
	// secure.
	secureTransport http.RoundTripper
	// insecureTransport is the transport pool that does not verify remote TLS certificates for use
	// during pullthrough against registries marked as insecure.
	insecureTransport http.RoundTripper
)

func init() {
	secureTransport = http.DefaultTransport
	var err error
	insecureTransport, err = restclient.TransportFor(&restclient.Config{TLSClientConfig: restclient.TLSClientConfig{Insecure: true}})
	if err != nil {
		panic(fmt.Sprintf("Unable to configure a default transport for importing insecure images: %v", err))
	}
}

// repository wraps a distribution.Repository and allows manifests to be served from the OpenShift image
// API.
type repository struct {
	distribution.Repository

	ctx        context.Context
	app        *App
	crossmount bool

	imageStream imagestream.ImageStream

	// remoteBlobGetter is used to fetch blobs from remote registries if pullthrough is enabled.
	remoteBlobGetter BlobGetterService
	cache            cache.RepositoryDigest
}

// Repository returns a new repository middleware.
func (app *App) Repository(ctx context.Context, repo distribution.Repository, crossmount bool) (distribution.Repository, distribution.BlobDescriptorServiceFactory, error) {
	registryOSClient, err := app.registryClient.Client()
	if err != nil {
		return nil, nil, err
	}

	context.GetLogger(ctx).Infof("Using %q as Docker Registry URL", app.config.Server.Addr)

	namespace, name, err := getNamespaceName(repo.Named().Name())
	if err != nil {
		return nil, nil, err
	}

	r := &repository{
		Repository: repo,

		ctx:        ctx,
		app:        app,
		crossmount: crossmount,

		imageStream: imagestream.New(ctx, namespace, name, registryOSClient),
		cache: &cache.RepoDigest{
			Cache: app.cache,
		},
	}

	if app.config.Pullthrough.Enabled {
		r.remoteBlobGetter = NewBlobGetterService(
			r.imageStream,
			r.imageStream.GetSecrets,
			r.cache,
		)
	}

	bdsf := blobDescriptorServiceFactoryFunc(r.BlobDescriptorService)

	return r, bdsf, nil
}

// Manifests returns r, which implements distribution.ManifestService.
func (r *repository) Manifests(ctx context.Context, options ...distribution.ManifestServiceOption) (distribution.ManifestService, error) {
	// we do a verification of our own
	// TODO: let upstream do the verification once they pass correct context object to their manifest handler
	opts := append(options, registrystorage.SkipLayerVerification())
	ms, err := r.Repository.Manifests(ctx, opts...)
	if err != nil {
		return nil, err
	}

	ms = &manifestService{
		manifests:     ms,
		blobStore:     r.Blobs(ctx),
		serverAddr:    r.app.config.Server.Addr,
		imageStream:   r.imageStream,
		cache:         r.cache,
		acceptSchema2: r.app.config.Compatibility.AcceptSchema2,
	}

	if r.app.config.Pullthrough.Enabled {
		ms = &pullthroughManifestService{
			ManifestService: ms,
			newLocalManifestService: func(ctx context.Context) (distribution.ManifestService, error) {
				return r.Repository.Manifests(ctx, opts...)
			},
			imageStream:  r.imageStream,
			cache:        r.cache,
			mirror:       r.app.config.Pullthrough.Mirror,
			registryAddr: r.app.config.Server.Addr,
		}
	}

	ms = newPendingErrorsManifestService(ms, r)

	if audit.LoggerExists(ctx) {
		ms = audit.NewManifestService(ctx, ms)
	}

	if r.app.config.Metrics.Enabled {
		ms = metrics.NewManifestService(ms, r.Named().Name())
	}

	return ms, nil
}

// Blobs returns a blob store which can delegate to remote repositories.
func (r *repository) Blobs(ctx context.Context) distribution.BlobStore {
	bs := r.Repository.Blobs(ctx)

	if r.app.quotaEnforcing.enforcementEnabled {
		bs = &quotaRestrictedBlobStore{
			BlobStore: bs,

			repo: r,
		}
	}

	if r.app.config.Pullthrough.Enabled {
		bs = &pullthroughBlobStore{
			BlobStore: bs,

			remoteBlobGetter:  r.remoteBlobGetter,
			writeLimiter:      r.app.writeLimiter,
			mirror:            r.app.config.Pullthrough.Mirror,
			newLocalBlobStore: r.Repository.Blobs,
		}
	}

	bs = newPendingErrorsBlobStore(bs, r)

	if audit.LoggerExists(ctx) {
		bs = audit.NewBlobStore(ctx, bs)
	}

	if r.app.config.Metrics.Enabled {
		bs = metrics.NewBlobStore(bs, r.Named().Name())
	}

	return bs
}

// Tags returns a reference to this repository tag service.
func (r *repository) Tags(ctx context.Context) distribution.TagService {
	ts := r.Repository.Tags(ctx)

	ts = &tagService{
		TagService:         ts,
		imageStream:        r.imageStream,
		pullthroughEnabled: r.app.config.Pullthrough.Enabled,
	}

	ts = newPendingErrorsTagService(ts, r)

	if audit.LoggerExists(ctx) {
		ts = audit.NewTagService(ctx, ts)
	}

	if r.app.config.Metrics.Enabled {
		ts = metrics.NewTagService(ts, r.Named().Name())
	}

	return ts
}

func (r *repository) BlobDescriptorService(svc distribution.BlobDescriptorService) distribution.BlobDescriptorService {
	svc = &cache.RepositoryScopedBlobDescriptor{
		Repo:  r.Named().String(),
		Cache: r.app.cache,
		Svc:   svc,
	}
	svc = &blobDescriptorService{svc, r}
	svc = newPendingErrorsBlobDescriptorService(svc, r)
	return svc
}

func (r *repository) checkPendingErrors(ctx context.Context) error {
	return checkPendingErrors(ctx, context.GetLogger(r.ctx), r.imageStream.Reference())
}

func checkPendingErrors(ctx context.Context, logger context.Logger, ref string) error {
	if !authPerformed(ctx) {
		return fmt.Errorf("openshift.auth.completed missing from context")
	}

	deferredErrors, haveDeferredErrors := deferredErrorsFrom(ctx)
	if !haveDeferredErrors {
		return nil
	}

	repoErr, haveRepoErr := deferredErrors.Get(ref)
	if !haveRepoErr {
		return nil
	}

	logger.Debugf("Origin auth: found deferred error for %s: %v", ref, repoErr)
	return repoErr
}
