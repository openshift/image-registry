package server

import (
	"sort"
	"time"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest/schema2"

	kerrors "k8s.io/apimachinery/pkg/api/errors"

	dockerapiv10 "github.com/openshift/api/image/docker10"
	imageapiv1 "github.com/openshift/api/image/v1"
)

const (
	// DigestSha256EmptyTar is the canonical sha256 digest of empty data
	digestSha256EmptyTar = digest.Digest("sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")

	// digestSHA256GzippedEmptyTar is the canonical sha256 digest of gzippedEmptyTar
	digestSHA256GzippedEmptyTar = digest.Digest("sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4")
)

// ByGeneration allows for sorting tag events from latest to oldest.
type ByGeneration []*imageapiv1.TagEvent

func (b ByGeneration) Less(i, j int) bool { return b[i].Generation > b[j].Generation }
func (b ByGeneration) Len() int           { return len(b) }
func (b ByGeneration) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

type blobDescriptorServiceFactoryFunc func(svc distribution.BlobDescriptorService) distribution.BlobDescriptorService

func (f blobDescriptorServiceFactoryFunc) BlobAccessController(svc distribution.BlobDescriptorService) distribution.BlobDescriptorService {
	return f(svc)
}

type blobDescriptorService struct {
	distribution.BlobDescriptorService
	repo *repository
}

// Stat returns a a blob descriptor if the given blob is either linked in repository or is referenced in
// corresponding image stream. This method is invoked from inside of upstream's linkedBlobStore. It expects
// a proper repository object to be set on given context by upper openshift middleware wrappers.
func (bs *blobDescriptorService) Stat(ctx context.Context, dgst digest.Digest) (distribution.Descriptor, error) {
	context.GetLogger(ctx).Debugf("(*blobDescriptorService).Stat: starting with digest=%s", dgst.String())

	// if there is a repo layer link, return its descriptor
	desc, err := bs.BlobDescriptorService.Stat(ctx, dgst)
	if err == nil {
		return desc, nil
	}

	context.GetLogger(ctx).Debugf("(*blobDescriptorService).Stat: could not stat layer link %s in repository %s: %v", dgst.String(), bs.repo.Named().Name(), err)

	// First attempt: looking for the blob locally
	desc, err = bs.repo.app.BlobStatter().Stat(ctx, dgst)
	if err == nil {
		context.GetLogger(ctx).Debugf("(*blobDescriptorService).Stat: blob %s exists in the global blob store", dgst.String())
		// only non-empty layers is wise to check for existence in the image stream.
		// schema v2 has no empty layers.
		if !isEmptyDigest(dgst) {
			// ensure it's referenced inside of corresponding image stream
			if !imageStreamHasBlob(ctx, bs.repo.imageStream, dgst, !bs.repo.app.config.Pullthrough.Enabled) {
				context.GetLogger(ctx).Debugf("(*blobDescriptorService).Stat: blob %s is neither empty nor referenced in image stream %s", dgst.String(), bs.repo.Named().Name())
				return distribution.Descriptor{}, distribution.ErrBlobUnknown
			}
		}
		return desc, nil
	}

	if err == distribution.ErrBlobUnknown && remoteBlobAccessCheckEnabledFrom(ctx) {
		// Second attempt: looking for the blob on a remote server
		desc, err = bs.repo.remoteBlobGetter.Stat(ctx, dgst)
	}

	return desc, err
}

// imageStreamHasBlob returns true if the given blob digest is referenced in image stream corresponding to
// given repository. If not found locally, image stream's images will be iterated and fetched from newest to
// oldest until found. Each processed image will update local cache of blobs.
func imageStreamHasBlob(ctx context.Context, imageStream *imageStream, dgst digest.Digest, requireManaged bool) bool {
	repoCacheName := imageStream.Reference()
	if imageStream.cache.ContainsRepository(dgst, repoCacheName) {
		context.GetLogger(ctx).Debugf("found cached blob %q in repository %s", dgst.String(), imageStream.Reference())
		return true
	}

	context.GetLogger(ctx).Debugf("verifying presence of blob %q in image stream %s", dgst.String(), imageStream.Reference())
	started := time.Now()
	logFound := func(found bool) bool {
		elapsed := time.Since(started)
		if found {
			context.GetLogger(ctx).Debugf("verified presence of blob %q in image stream %s after %s", dgst.String(), imageStream.Reference(), elapsed.String())
		} else {
			context.GetLogger(ctx).Debugf("detected absence of blob %q in image stream %s after %s", dgst.String(), imageStream.Reference(), elapsed.String())
		}
		return found
	}

	// verify directly with etcd
	is, err := imageStream.imageStreamGetter.get()
	if err != nil {
		context.GetLogger(ctx).Errorf("failed to get image stream: %v", err)
		return logFound(false)
	}

	tagEvents := []*imageapiv1.TagEvent{}
	event2Name := make(map[*imageapiv1.TagEvent]string)
	for _, eventList := range is.Status.Tags {
		name := eventList.Tag
		for i := range eventList.Items {
			event := &eventList.Items[i]
			tagEvents = append(tagEvents, event)
			event2Name[event] = name
		}
	}
	// search from youngest to oldest
	sort.Sort(ByGeneration(tagEvents))

	processedImages := map[string]struct{}{}

	for _, tagEvent := range tagEvents {
		if _, processed := processedImages[tagEvent.Image]; processed {
			continue
		}

		processedImages[tagEvent.Image] = struct{}{}

		context.GetLogger(ctx).Debugf("getting image %s", tagEvent.Image)
		image, err := imageStream.getImage(ctx, digest.Digest(tagEvent.Image))
		if err != nil {
			if kerrors.IsNotFound(err) {
				context.GetLogger(ctx).Debugf("image %q not found", tagEvent.Image)
			} else {
				context.GetLogger(ctx).Errorf("failed to get image: %v", err)
			}
			continue
		}

		// in case of pullthrough disabled, client won't be able to download a blob belonging to not managed image
		// (image stored in external registry), thus don't consider them as candidates
		if requireManaged && !isImageManaged(image) {
			context.GetLogger(ctx).Debugf("skipping not managed image")
			continue
		}

		if imageHasBlob(ctx, image, dgst) {
			tagName := event2Name[tagEvent]
			context.GetLogger(ctx).Debugf("blob found under istag %s:%s in image %s", imageStream.Reference(), tagName, tagEvent.Image)
			// remember all the layers of matching image
			imageStream.rememberLayersOfImage(ctx, image, repoCacheName)
			return logFound(true)
		}
	}

	context.GetLogger(ctx).Warnf("blob %q exists locally but is not referenced in repository %s", dgst.String(), imageStream.Reference())

	return logFound(false)
}

// imageHasBlob returns true if the image identified by imageName refers to the given blob.
func imageHasBlob(ctx context.Context, image *imageapiv1.Image, blobDigest digest.Digest) bool {
	// someone asks for manifest
	if image.Name == blobDigest.String() {
		return true
	}

	if len(image.DockerImageLayers) == 0 && len(image.DockerImageManifestMediaType) > 0 {
		// If the media type is set, we can safely assume that the best effort to
		// fill the image layers has already been done. There are none.
		return false
	}

	for _, layer := range image.DockerImageLayers {
		if layer.Name == blobDigest.String() {
			return true
		}
	}

	meta, ok := image.DockerImageMetadata.Object.(*dockerapiv10.DockerImage)
	if !ok {
		context.GetLogger(ctx).Errorf("image does not have metadata %s", image.Name)
		return false
	}

	// only manifest V2 schema2 has docker image config filled where dockerImage.Metadata.id is its digest
	if image.DockerImageManifestMediaType == schema2.MediaTypeManifest && meta.ID == blobDigest.String() {
		return true
	}

	return false
}

func isEmptyDigest(dgst digest.Digest) bool {
	return dgst == digestSha256EmptyTar || dgst == digestSHA256GzippedEmptyTar
}
