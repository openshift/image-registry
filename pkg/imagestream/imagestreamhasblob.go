package imagestream

import (
	"sort"
	"time"

	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest/schema2"

	kerrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/openshift/api/image/docker10"
	imageapiv1 "github.com/openshift/api/image/v1"
)

// ByGeneration allows for sorting tag events from latest to oldest.
type ByGeneration []*imageapiv1.TagEvent

func (b ByGeneration) Less(i, j int) bool { return b[i].Generation > b[j].Generation }
func (b ByGeneration) Len() int           { return len(b) }
func (b ByGeneration) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

// HasBlob returns true if the given blob digest is referenced in image stream corresponding to
// given repository. If not found locally, image stream's images will be iterated and fetched from newest to
// oldest until found. Each processed image will update local cache of blobs.
func (is *imageStream) HasBlob(ctx context.Context, dgst digest.Digest, requireManaged bool) *imageapiv1.Image {
	context.GetLogger(ctx).Debugf("verifying presence of blob %q in image stream %s", dgst.String(), is.Reference())
	started := time.Now()
	logFound := func(found *imageapiv1.Image) *imageapiv1.Image {
		elapsed := time.Since(started)
		if found != nil {
			context.GetLogger(ctx).Debugf("verified presence of blob %q in image stream %s after %s", dgst.String(), is.Reference(), elapsed.String())
		} else {
			context.GetLogger(ctx).Debugf("detected absence of blob %q in image stream %s after %s", dgst.String(), is.Reference(), elapsed.String())
		}
		return found
	}

	// verify directly with etcd
	stream, err := is.imageStreamGetter.get()
	if err != nil {
		context.GetLogger(ctx).Errorf("failed to get image stream: %v", err)
		return logFound(nil)
	}

	tagEvents := []*imageapiv1.TagEvent{}
	event2Name := make(map[*imageapiv1.TagEvent]string)
	for _, eventList := range stream.Status.Tags {
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
		image, err := is.getImage(ctx, digest.Digest(tagEvent.Image))
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
		if requireManaged && !IsImageManaged(image) {
			context.GetLogger(ctx).Debugf("skipping not managed image")
			continue
		}

		if imageHasBlob(ctx, image, dgst) {
			tagName := event2Name[tagEvent]
			context.GetLogger(ctx).Debugf("blob found under istag %s:%s in image %s", is.Reference(), tagName, tagEvent.Image)
			return logFound(image)
		}
	}

	context.GetLogger(ctx).Warnf("blob %q exists locally but is not referenced in repository %s", dgst.String(), is.Reference())

	return logFound(nil)
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

	meta, ok := image.DockerImageMetadata.Object.(*docker10.DockerImage)
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
