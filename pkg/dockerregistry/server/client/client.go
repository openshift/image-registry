package client

import (
	authnv1 "k8s.io/client-go/kubernetes/typed/authentication/v1"
	authclientv1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
	coreclientv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"

	cfgv1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	imageclientv1 "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"
	operatorclientv1alpha1 "github.com/openshift/client-go/operator/clientset/versioned/typed/operator/v1alpha1"
	userclientv1 "github.com/openshift/client-go/user/clientset/versioned/typed/user/v1"
	"github.com/openshift/image-registry/pkg/origin-common/clientcmd"
)

// RegistryClient provides Origin and Kubernetes clients to Docker Registry.
type RegistryClient interface {
	// Client returns the authenticated client to use with the server.
	Client() (Interface, error)
	// ClientFromToken returns a client based on the user bearer token.
	ClientFromToken(token string) (Interface, error)
}

// Interface contains client methods that registry use to communicate with
// Origin or Kubernetes API.
type Interface interface {
	ImageSignaturesInterfacer
	ImagesInterfacer
	ImageStreamImagesNamespacer
	ImageStreamMappingsNamespacer
	ImageStreamSecretsNamespacer
	ImageStreamsNamespacer
	ImageStreamTagsNamespacer
	LimitRangesGetter
	SelfSubjectReviews
	LocalSubjectAccessReviewsNamespacer
	SelfSubjectAccessReviewsNamespacer
	SubjectAccessReviewsNamespacer
	ImageContentSourcePolicyInterfacer
}

type apiClient struct {
	kube     coreclientv1.CoreV1Interface
	auth     authclientv1.AuthorizationV1Interface
	authn    authnv1.AuthenticationV1Interface
	image    imageclientv1.ImageV1Interface
	user     userclientv1.UserV1Interface
	operator operatorclientv1alpha1.OperatorV1alpha1Interface
	config   cfgv1.ConfigV1Interface
}

func newAPIClient(
	kc coreclientv1.CoreV1Interface,
	authClient authclientv1.AuthorizationV1Interface,
	authnClient authnv1.AuthenticationV1Interface,
	imageClient imageclientv1.ImageV1Interface,
	userClient userclientv1.UserV1Interface,
	operatorClient operatorclientv1alpha1.OperatorV1alpha1Interface,
	configClient cfgv1.ConfigV1Interface,
) Interface {
	return &apiClient{
		kube:     kc,
		auth:     authClient,
		authn:    authnClient,
		image:    imageClient,
		user:     userClient,
		operator: operatorClient,
		config:   configClient,
	}
}

func (c *apiClient) ImageContentSourcePolicy() operatorclientv1alpha1.ImageContentSourcePolicyInterface {
	return c.operator.ImageContentSourcePolicies()
}

func (c *apiClient) ImageDigestMirrorSet() cfgv1.ImageDigestMirrorSetInterface {
	return c.config.ImageDigestMirrorSets()
}

func (c *apiClient) ImageTagMirrorSet() cfgv1.ImageTagMirrorSetInterface {
	return c.config.ImageTagMirrorSets()
}

func (c *apiClient) Images() ImageInterface {
	return c.image.Images()
}

func (c *apiClient) ImageSignatures() ImageSignatureInterface {
	return c.image.ImageSignatures()
}

func (c *apiClient) ImageStreams(namespace string) ImageStreamInterface {
	return c.image.ImageStreams(namespace)
}

func (c *apiClient) ImageStreamImages(namespace string) ImageStreamImageInterface {
	return c.image.ImageStreamImages(namespace)
}

func (c *apiClient) ImageStreamMappings(namespace string) ImageStreamMappingInterface {
	return c.image.ImageStreamMappings(namespace)
}

func (c *apiClient) ImageStreamTags(namespace string) ImageStreamTagInterface {
	return c.image.ImageStreamTags(namespace)
}

func (c *apiClient) ImageStreamSecrets(namespace string) ImageStreamSecretInterface {
	return c.image.ImageStreams(namespace)
}

func (c *apiClient) LimitRanges(namespace string) LimitRangeInterface {
	return c.kube.LimitRanges(namespace)
}

func (c *apiClient) SelfSubjectReviews() SelfSubjectReviewInterface {
	return c.authn.SelfSubjectReviews()
}

func (c *apiClient) LocalSubjectAccessReviews(namespace string) LocalSubjectAccessReviewInterface {
	return c.auth.LocalSubjectAccessReviews(namespace)
}

func (c *apiClient) SelfSubjectAccessReviews() SelfSubjectAccessReviewInterface {
	return c.auth.SelfSubjectAccessReviews()
}

func (c *apiClient) SubjectAccessReviews() SubjectAccessReviewInterface {
	return c.auth.SubjectAccessReviews()
}

type registryClient struct {
	kubeConfig *restclient.Config
}

// NewRegistryClient provides a new registry client.
func NewRegistryClient(config *clientcmd.Config) RegistryClient {
	cfg := config.KubeConfig()
	cfg.QPS = 100
	cfg.Burst = 200
	return &registryClient{
		kubeConfig: cfg,
	}
}

// Client returns the authenticated client to use with the server.
func (c *registryClient) Client() (Interface, error) {
	return newAPIClient(
		coreclientv1.NewForConfigOrDie(c.kubeConfig),
		authclientv1.NewForConfigOrDie(c.kubeConfig),
		authnv1.NewForConfigOrDie(c.kubeConfig),
		imageclientv1.NewForConfigOrDie(c.kubeConfig),
		userclientv1.NewForConfigOrDie(c.kubeConfig),
		operatorclientv1alpha1.NewForConfigOrDie(c.kubeConfig),
		cfgv1.NewForConfigOrDie(c.kubeConfig),
	), nil
}

// ClientFromToken returns the client based on the bearer token.
func (c *registryClient) ClientFromToken(token string) (Interface, error) {
	newClient := *c
	newKubeconfig := restclient.AnonymousClientConfig(newClient.kubeConfig)
	newKubeconfig.BearerToken = token
	newClient.kubeConfig = newKubeconfig

	return newClient.Client()
}
