package imagestream

import (
	"context"
	"time"

	dcontext "github.com/distribution/distribution/v3/context"
	"github.com/opencontainers/go-digest"

	imageapiv1 "github.com/openshift/api/image/v1"
)

// HasBlob returns true if the given blob digest is referenced in image stream corresponding to
// given repository. If not found locally, image stream's images will be iterated and fetched from newest to
// oldest until found. Each processed image will update local cache of blobs.
func (is *imageStream) HasBlob(ctx context.Context, dgst digest.Digest) (bool, *imageapiv1.ImageStreamLayers, *imageapiv1.Image) {
	dcontext.GetLogger(ctx).Debugf("verifying presence of blob %q in image stream %s", dgst.String(), is.Reference())
	started := time.Now()
	logFound := func(found bool, layers *imageapiv1.ImageStreamLayers, image *imageapiv1.Image) (bool, *imageapiv1.ImageStreamLayers, *imageapiv1.Image) {
		elapsed := time.Since(started)
		if found {
			dcontext.GetLogger(ctx).Debugf("verified presence of blob %q in image stream %s after %s", dgst.String(), is.Reference(), elapsed.String())
		} else {
			dcontext.GetLogger(ctx).Debugf("detected absence of blob %q in image stream %s after %s", dgst.String(), is.Reference(), elapsed.String())
		}
		return found, layers, image
	}

	// perform the more efficient check for a layer in the image stream
	layers, err := is.imageStreamGetter.layers()
	if err != nil {
		dcontext.GetLogger(ctx).Errorf("imageStream.HasBlob: failed to get image stream layers: %v", err)
		return logFound(false, nil, nil)
	}

	// check for the blob in the layers
	if _, ok := layers.Blobs[dgst.String()]; ok {
		return logFound(true, layers, nil)
	}

	// check for the manifest as a blob
	if _, ok := layers.Images[dgst.String()]; ok {
		return logFound(true, layers, nil)
	}

	return logFound(false, layers, nil)
}
