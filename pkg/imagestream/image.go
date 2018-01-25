package imagestream

import (
	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	imageapiv1 "github.com/openshift/api/image/v1"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/client"
	imageapi "github.com/openshift/image-registry/pkg/origin-common/image/apis/image"
	util "github.com/openshift/image-registry/pkg/origin-common/util"
)

func IsImageManaged(image *imageapiv1.Image) bool {
	managed, ok := image.ObjectMeta.Annotations[imageapi.ManagedByOpenShiftAnnotation]
	return ok && managed == "true"
}

// wrapKStatusErrorOnGetImage transforms the given kubernetes status error into a distribution one. Upstream
// handler do not allow us to propagate custom error messages except for ErrManifetUnknownRevision. All the
// other errors will result in an internal server error with details made out of returned error.
func wrapKStatusErrorOnGetImage(repoName string, dgst digest.Digest, err error) error {
	switch {
	case kerrors.IsNotFound(err):
		// This is the only error type we can propagate unchanged to the client.
		return distribution.ErrManifestUnknownRevision{
			Name:     repoName,
			Revision: dgst,
		}
	case err != nil:
		// We don't turn this error to distribution error on purpose: Upstream manifest handler wraps any
		// error but distribution.ErrManifestUnknownRevision with errcode.ErrorCodeUnknown. If we wrap the
		// original error with distribution.ErrorCodeUnknown, the "unknown error" will appear twice in the
		// resulting error message.
		return err
	}

	return nil
}

type imageGetter interface {
	Get(ctx context.Context, dgst digest.Digest) (*imageapiv1.Image, error)
}

type cachedImageGetter struct {
	client client.Interface
	cache  map[digest.Digest]*imageapiv1.Image
}

func newCachedImageGetter(client client.Interface) imageGetter {
	return &cachedImageGetter{
		client: client,
		cache:  make(map[digest.Digest]*imageapiv1.Image),
	}
}

// Get retrieves the Image resource with the digest dgst. No authorization check is made.
func (ig *cachedImageGetter) Get(ctx context.Context, dgst digest.Digest) (*imageapiv1.Image, error) {
	if image, ok := ig.cache[dgst]; ok {
		context.GetLogger(ctx).Infof("(*cachedImageGetter).Get: found image %s in cache", image.Name)
		return image, nil
	}

	image, err := ig.client.Images().Get(dgst.String(), metav1.GetOptions{})
	if err != nil {
		context.GetLogger(ctx).Errorf("failed to get image %s: %v", dgst, err)
		return nil, err
	}

	context.GetLogger(ctx).Infof("(*cachedImageGetter).Get: got image %s from server", image.Name)

	if err := util.ImageWithMetadata(image); err != nil {
		return nil, err
	}

	ig.cache[dgst] = image

	return image, nil
}
