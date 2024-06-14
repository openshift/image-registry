package client

import (
	"context"

	authnv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreclientv1 "k8s.io/client-go/kubernetes/typed/core/v1"

	imageapiv1 "github.com/openshift/api/image/v1"
	userapiv1 "github.com/openshift/api/user/v1"
	authapiv1 "k8s.io/api/authorization/v1"

	imageclientv1 "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"
	operatorclientv1alpha1 "github.com/openshift/client-go/operator/clientset/versioned/typed/operator/v1alpha1"
	userclientv1 "github.com/openshift/client-go/user/clientset/versioned/typed/user/v1"

	cfgv1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	authnclientv1 "k8s.io/client-go/kubernetes/typed/authentication/v1"
	authclientv1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
)

type UsersInterfacer interface {
	Users() UserInterface
}

type ImageContentSourcePolicyInterfacer interface {
	ImageContentSourcePolicy() operatorclientv1alpha1.ImageContentSourcePolicyInterface
	ImageDigestMirrorSet() cfgv1.ImageDigestMirrorSetInterface
	ImageTagMirrorSet() cfgv1.ImageTagMirrorSetInterface
}

type ImagesInterfacer interface {
	Images() ImageInterface
}

type ImageSignaturesInterfacer interface {
	ImageSignatures() ImageSignatureInterface
}

type ImageStreamImagesNamespacer interface {
	ImageStreamImages(namespace string) ImageStreamImageInterface
}

type ImageStreamsNamespacer interface {
	ImageStreams(namespace string) ImageStreamInterface
}

type ImageStreamMappingsNamespacer interface {
	ImageStreamMappings(namespace string) ImageStreamMappingInterface
}

type ImageStreamSecretsNamespacer interface {
	ImageStreamSecrets(namespace string) ImageStreamSecretInterface
}

type ImageStreamTagsNamespacer interface {
	ImageStreamTags(namespace string) ImageStreamTagInterface
}

type LimitRangesGetter interface {
	LimitRanges(namespace string) LimitRangeInterface
}

type SelfSubjectReviews interface {
	SelfSubjectReviews() SelfSubjectReviewInterface
}

type LocalSubjectAccessReviewsNamespacer interface {
	LocalSubjectAccessReviews(namespace string) LocalSubjectAccessReviewInterface
}

type SelfSubjectAccessReviewsNamespacer interface {
	SelfSubjectAccessReviews() SelfSubjectAccessReviewInterface
}

type SubjectAccessReviewsNamespacer interface {
	SubjectAccessReviews() SubjectAccessReviewInterface
}

var _ ImageSignatureInterface = imageclientv1.ImageSignatureInterface(nil)

type ImageSignatureInterface interface {
	Create(ctx context.Context, imageSignature *imageapiv1.ImageSignature, opts metav1.CreateOptions) (*imageapiv1.ImageSignature, error)
}

var _ ImageStreamImageInterface = imageclientv1.ImageStreamImageInterface(nil)

type ImageStreamImageInterface interface {
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*imageapiv1.ImageStreamImage, error)
}

var _ UserInterface = userclientv1.UserInterface(nil)

type UserInterface interface {
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*userapiv1.User, error)
}

var _ ImageInterface = imageclientv1.ImageInterface(nil)

type ImageInterface interface {
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*imageapiv1.Image, error)
	Create(ctx context.Context, image *imageapiv1.Image, opts metav1.CreateOptions) (*imageapiv1.Image, error)
	Update(ctx context.Context, image *imageapiv1.Image, opts metav1.UpdateOptions) (*imageapiv1.Image, error)
	List(ctx context.Context, opts metav1.ListOptions) (*imageapiv1.ImageList, error)
}

var _ ImageStreamInterface = imageclientv1.ImageStreamInterface(nil)

type ImageStreamInterface interface {
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*imageapiv1.ImageStream, error)
	Create(ctx context.Context, imageStream *imageapiv1.ImageStream, opts metav1.CreateOptions) (*imageapiv1.ImageStream, error)
	List(ctx context.Context, opts metav1.ListOptions) (*imageapiv1.ImageStreamList, error)
	Layers(ctx context.Context, imageStreamName string, options metav1.GetOptions) (*imageapiv1.ImageStreamLayers, error)
}

var _ ImageStreamMappingInterface = imageclientv1.ImageStreamMappingInterface(nil)

type ImageStreamMappingInterface interface {
	Create(ctx context.Context, imageStreamMapping *imageapiv1.ImageStreamMapping, opts metav1.CreateOptions) (*metav1.Status, error)
}

var _ ImageStreamTagInterface = imageclientv1.ImageStreamTagInterface(nil)

type ImageStreamTagInterface interface {
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
}

var _ ImageStreamSecretInterface = imageclientv1.ImageStreamInterface(nil)

type ImageStreamSecretInterface interface {
	Secrets(ctx context.Context, imageStreamName string, options metav1.GetOptions) (*imageapiv1.SecretList, error)
}

var _ LimitRangeInterface = coreclientv1.LimitRangeInterface(nil)

type LimitRangeInterface interface {
	List(ctx context.Context, opts metav1.ListOptions) (*corev1.LimitRangeList, error)
}

var _ SelfSubjectReviewInterface = authnclientv1.SelfSubjectReviewInterface(nil)

type SelfSubjectReviewInterface interface {
	Create(ctx context.Context, selfSubjectReview *authnv1.SelfSubjectReview, opts metav1.CreateOptions) (*authnv1.SelfSubjectReview, error)
}

var _ LocalSubjectAccessReviewInterface = authclientv1.LocalSubjectAccessReviewInterface(nil)

type LocalSubjectAccessReviewInterface interface {
	Create(ctx context.Context, localSubjectAccessReview *authapiv1.LocalSubjectAccessReview, opts metav1.CreateOptions) (*authapiv1.LocalSubjectAccessReview, error)
}

var _ SelfSubjectAccessReviewInterface = authclientv1.SelfSubjectAccessReviewInterface(nil)

type SelfSubjectAccessReviewInterface interface {
	Create(ctx context.Context, selfSubjectAccessReview *authapiv1.SelfSubjectAccessReview, opts metav1.CreateOptions) (*authapiv1.SelfSubjectAccessReview, error)
}

var _ SubjectAccessReviewInterface = authclientv1.SubjectAccessReviewInterface(nil)

type SubjectAccessReviewInterface interface {
	Create(ctx context.Context, subjectAccessReview *authapiv1.SubjectAccessReview, opts metav1.CreateOptions) (*authapiv1.SubjectAccessReview, error)
}
