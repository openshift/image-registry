package imagestream

import (
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	imageapiv1 "github.com/openshift/api/image/v1"

	imageapi "github.com/openshift/image-registry/pkg/origin-common/image/apis/image"
)

// ProjectObjectListStore represents a cache of objects indexed by a project name.
// Used to store a list of items per namespace.
type ProjectObjectListStore interface {
	Add(namespace string, obj runtime.Object) error
	Get(namespace string) (obj runtime.Object, exists bool, err error)
}

// ImagePullthroughSpec contains a reference of remote image to pull associated with an insecure flag for the
// corresponding registry.
type ImagePullthroughSpec struct {
	DockerImageReference *imageapi.DockerImageReference
	Insecure             bool
}

type ImageStream interface {
	Reference() string
	Exists() (bool, error)

	GetStoredImageOfImageStream(ctx context.Context, dgst digest.Digest) (*imageapiv1.Image, *imageapiv1.TagEvent, *imageapiv1.ImageStream, error)
	GetImageOfImageStream(ctx context.Context, dgst digest.Digest) (*imageapiv1.Image, *imageapiv1.ImageStream, error)
	CreateImageStreamMapping(ctx context.Context, tag string, image *imageapiv1.Image) error
	UpdateImage(image *imageapiv1.Image) (*imageapiv1.Image, error)
	GetImage(ctx context.Context, dgst digest.Digest) (*imageapiv1.Image, error)
	RememberLayersOfImage(ctx context.Context, image *imageapiv1.Image, cacheName string)

	HasBlob(ctx context.Context, dgst digest.Digest, requireManaged bool) bool
	IdentifyCandidateRepositories(primary bool) ([]string, map[string]ImagePullthroughSpec, error)
	GetLimitRangeList(ctx context.Context, cache ProjectObjectListStore) (*corev1.LimitRangeList, error)
	GetSecrets() ([]corev1.Secret, error)

	TagIsInsecure(tag string, dgst digest.Digest) (bool, error)
	Tags(ctx context.Context) (map[string]digest.Digest, error)
	Tag(ctx context.Context, tag string, dgst digest.Digest, pullthroughEnabled bool) error
	Untag(ctx context.Context, tag string, pullthroughEnabled bool) error
}
