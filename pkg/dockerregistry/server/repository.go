package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/docker/distribution/registry/api/errcode"
	registrystorage "github.com/docker/distribution/registry/storage"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	restclient "k8s.io/client-go/rest"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/audit"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/cache"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/metrics"
	imageapi "github.com/openshift/origin/pkg/image/apis/image"
	imageapiv1 "github.com/openshift/origin/pkg/image/apis/image/v1"
	quotautil "github.com/openshift/origin/pkg/quota/util"
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

	imageStream *imageStream

	// remoteBlobGetter is used to fetch blobs from remote registries if pullthrough is enabled.
	remoteBlobGetter BlobGetterService
}

// Repository returns a new repository middleware.
func (app *App) Repository(ctx context.Context, repo distribution.Repository, crossmount bool) (distribution.Repository, distribution.BlobDescriptorServiceFactory, error) {
	registryOSClient, err := app.registryClient.Client()
	if err != nil {
		return nil, nil, err
	}

	context.GetLogger(ctx).Infof("Using %q as Docker Registry URL", app.config.Server.Addr)

	nameParts := strings.SplitN(repo.Named().Name(), "/", 2)
	if len(nameParts) != 2 {
		return nil, nil, fmt.Errorf("invalid repository name %q: it must be of the format <project>/<name>", repo.Named().Name())
	}
	namespace, name := nameParts[0], nameParts[1]

	imageStreamGetter := &cachedImageStreamGetter{
		ctx:          ctx,
		namespace:    namespace,
		name:         name,
		isNamespacer: registryOSClient,
	}

	r := &repository{
		Repository: repo,

		ctx:        ctx,
		app:        app,
		crossmount: crossmount,

		imageStream: &imageStream{
			namespace:         nameParts[0],
			name:              nameParts[1],
			registryOSClient:  registryOSClient,
			cachedImages:      make(map[digest.Digest]*imageapiv1.Image),
			imageStreamGetter: imageStreamGetter,
			cache: &cache.RepoDigest{
				Cache: app.cache,
			},
		},
	}

	if app.config.Pullthrough.Enabled {
		r.remoteBlobGetter = NewBlobGetterService(
			r.imageStream.namespace,
			r.imageStream.name,
			imageStreamGetter.get,
			registryOSClient,
			r.imageStream.cache)
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
		acceptSchema2: r.app.config.Compatibility.AcceptSchema2,
	}

	if r.app.config.Pullthrough.Enabled {
		ms = &pullthroughManifestService{
			ManifestService: ms,
			repo:            r,
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

			imageStream:      r.imageStream,
			remoteBlobGetter: r.remoteBlobGetter,
			writeLimiter:     r.app.writeLimiter,
			mirror:           r.app.config.Pullthrough.Mirror,
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
		TagService: ts,
		repo:       r,
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

// createImageStream creates a new image stream and caches it.
func (is *imageStream) createImageStream(ctx context.Context) (*imageapiv1.ImageStream, error) {
	stream := &imageapiv1.ImageStream{}
	stream.Name = is.name

	uclient, ok := userClientFrom(ctx)
	if !ok {
		errmsg := "error creating user client to auto provision image stream: user client to master API unavailable"
		context.GetLogger(ctx).Errorf(errmsg)
		return nil, errcode.ErrorCodeUnknown.WithDetail(errmsg)
	}

	stream, err := uclient.ImageStreams(is.namespace).Create(stream)
	switch {
	case kerrors.IsAlreadyExists(err), kerrors.IsConflict(err):
		context.GetLogger(ctx).Infof("conflict while creating ImageStream: %v", err)
		return is.imageStreamGetter.get()
	case kerrors.IsForbidden(err), kerrors.IsUnauthorized(err), quotautil.IsErrorQuotaExceeded(err):
		context.GetLogger(ctx).Errorf("denied creating ImageStream: %v", err)
		return nil, errcode.ErrorCodeDenied.WithDetail(err)
	case err != nil:
		context.GetLogger(ctx).Errorf("error auto provisioning ImageStream: %s", err)
		return nil, errcode.ErrorCodeUnknown.WithDetail(err)
	}

	is.imageStreamGetter.cacheImageStream(stream)
	return stream, nil
}

// getImage retrieves the Image with digest `dgst`. No authorization check is done.
func (is *imageStream) getImage(ctx context.Context, dgst digest.Digest) (*imageapiv1.Image, error) {
	if image, exists := is.cachedImages[dgst]; exists {
		context.GetLogger(ctx).Infof("(*imageStream).getImage: returning cached copy of %s", image.Name)
		return image, nil
	}

	image, err := is.registryOSClient.Images().Get(dgst.String(), metav1.GetOptions{})
	if err != nil {
		context.GetLogger(ctx).Errorf("failed to get image: %v", err)
		return nil, wrapKStatusErrorOnGetImage(is.name, dgst, err)
	}

	context.GetLogger(ctx).Infof("(*imageStream).getImage: got image %s", image.Name)
	if err := imageapiv1.ImageWithMetadata(image); err != nil {
		return nil, err
	}
	is.cachedImages[dgst] = image
	return image, nil
}

// getStoredImageOfImageStream retrieves the Image with digest `dgst` and
// ensures that the image belongs to the image stream `is`. It uses two
// queries to master API:
//
//  1st to get a corresponding image stream
//  2nd to get the image
//
// This allows us to cache the image stream for later use.
//
// If you need the image object to be modified according to image stream tag,
// please use getImageOfImageStream.
func (is *imageStream) getStoredImageOfImageStream(ctx context.Context, dgst digest.Digest) (*imageapiv1.Image, *imageapiv1.TagEvent, *imageapiv1.ImageStream, error) {
	stream, err := is.imageStreamGetter.get()
	if err != nil {
		context.GetLogger(ctx).Errorf("failed to get ImageStream: %v", err)
		return nil, nil, nil, wrapKStatusErrorOnGetImage(is.name, dgst, err)
	}

	tagEvent, err := imageapiv1.ResolveImageID(stream, dgst.String())
	if err != nil {
		context.GetLogger(ctx).Errorf("failed to resolve image %s in ImageStream %s: %v", dgst.String(), is.Reference(), err)
		return nil, nil, nil, wrapKStatusErrorOnGetImage(is.name, dgst, err)
	}

	image, err := is.getImage(ctx, dgst)
	if err != nil {
		return nil, nil, nil, wrapKStatusErrorOnGetImage(is.name, dgst, err)
	}

	return image, tagEvent, stream, nil
}

// getImageOfImageStream retrieves the Image with digest `dgst` for the image
// stream. The image's field DockerImageReference is modified on the fly to
// pretend that we've got the image from the source from which the image was
// tagged to match tag's DockerImageReference.
//
// NOTE: due to on the fly modification, the returned image object should
// not be sent to the master API. If you need unmodified version of the
// image object, please use getStoredImageOfImageStream.
func (is *imageStream) getImageOfImageStream(ctx context.Context, dgst digest.Digest) (*imageapiv1.Image, *imageapiv1.ImageStream, error) {
	image, tagEvent, stream, err := is.getStoredImageOfImageStream(ctx, dgst)
	if err != nil {
		return nil, nil, err
	}

	image.DockerImageReference = tagEvent.DockerImageReference

	return image, stream, nil
}

// updateImage modifies the Image.
func (is *imageStream) updateImage(image *imageapiv1.Image) (*imageapiv1.Image, error) {
	return is.registryOSClient.Images().Update(image)
}

// rememberLayersOfImage caches the layer digests of given image
func (is *imageStream) rememberLayersOfImage(ctx context.Context, image *imageapiv1.Image, cacheName string) {
	if len(image.DockerImageLayers) > 0 {
		for _, layer := range image.DockerImageLayers {
			var desc *distribution.Descriptor
			if isImageManaged(image) {
				desc = &distribution.Descriptor{
					Digest:    digest.Digest(layer.Name),
					Size:      layer.LayerSize,
					MediaType: layer.MediaType,
				}
			}
			_ = is.cache.AddDigest(digest.Digest(layer.Name), cacheName, desc)
		}
		meta, ok := image.DockerImageMetadata.Object.(*imageapi.DockerImage)
		if !ok {
			context.GetLogger(ctx).Errorf("image %s does not have metadata", image.Name)
			return
		}
		// remember reference to manifest config as well for schema 2
		if image.DockerImageManifestMediaType == schema2.MediaTypeManifest && len(meta.ID) > 0 {
			_ = is.cache.AddDigest(digest.Digest(meta.ID), cacheName, nil)
		}
		return
	}

	manifest, err := NewManifestFromImage(image)
	if err != nil {
		context.GetLogger(ctx).Errorf("cannot remember layers of image %s: %v", image.Name, err)
		return
	}
	refs := manifest.References()
	for i := range refs {
		var desc *distribution.Descriptor
		if isImageManaged(image) {
			desc = &refs[i]
		}
		_ = is.cache.AddDigest(refs[i].Digest, cacheName, desc)
	}
}

func (r *repository) checkPendingErrors(ctx context.Context) error {
	return checkPendingErrors(ctx, context.GetLogger(r.ctx), r.imageStream.namespace, r.imageStream.name)
}

func checkPendingErrors(ctx context.Context, logger context.Logger, namespace, name string) error {
	if !authPerformed(ctx) {
		return fmt.Errorf("openshift.auth.completed missing from context")
	}

	deferredErrors, haveDeferredErrors := deferredErrorsFrom(ctx)
	if !haveDeferredErrors {
		return nil
	}

	repoErr, haveRepoErr := deferredErrors.Get(namespace, name)
	if !haveRepoErr {
		return nil
	}

	logger.Debugf("Origin auth: found deferred error for %s/%s: %v", namespace, name, repoErr)
	return repoErr
}
