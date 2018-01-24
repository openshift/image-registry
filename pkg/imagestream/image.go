package imagestream

import (
	"github.com/docker/distribution"
	"github.com/docker/distribution/digest"

	kerrors "k8s.io/apimachinery/pkg/api/errors"

	imageapiv1 "github.com/openshift/api/image/v1"

	imageapi "github.com/openshift/image-registry/pkg/origin-common/image/apis/image"
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
