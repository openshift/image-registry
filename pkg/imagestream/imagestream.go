package imagestream

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/registry/api/errcode"
	disterrors "github.com/docker/distribution/registry/api/v2"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	imageapiv1 "github.com/openshift/api/image/v1"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/client"
	imageapi "github.com/openshift/image-registry/pkg/origin-common/image/apis/image"
	quotautil "github.com/openshift/image-registry/pkg/origin-common/quota/util"
	originutil "github.com/openshift/image-registry/pkg/origin-common/util"
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

	GetImageOfImageStream(ctx context.Context, dgst digest.Digest) (*imageapiv1.Image, error)
	CreateImageStreamMapping(ctx context.Context, userClient client.Interface, tag string, image *imageapiv1.Image) error
	ImageManifestBlobStored(ctx context.Context, image *imageapiv1.Image) error
	ResolveImageID(ctx context.Context, dgst digest.Digest) (*imageapiv1.TagEvent, error)

	HasBlob(ctx context.Context, dgst digest.Digest, requireManaged bool) *imageapiv1.Image
	IdentifyCandidateRepositories(primary bool) ([]string, map[string]ImagePullthroughSpec, error)
	GetLimitRangeList(ctx context.Context, cache ProjectObjectListStore) (*corev1.LimitRangeList, error)
	GetSecrets() ([]corev1.Secret, error)

	TagIsInsecure(tag string, dgst digest.Digest) (bool, error)
	Tags(ctx context.Context) (map[string]digest.Digest, error)
	Tag(ctx context.Context, tag string, dgst digest.Digest, pullthroughEnabled bool) error
	Untag(ctx context.Context, tag string, pullthroughEnabled bool) error
}

type imageStream struct {
	namespace string
	name      string

	registryOSClient client.Interface

	imageClient imageGetter

	// imageStreamGetter fetches and caches an image stream. The image stream stays cached for the entire time of handling single repository-scoped request.
	imageStreamGetter *cachedImageStreamGetter
}

var _ ImageStream = &imageStream{}

func New(ctx context.Context, namespace, name string, client client.Interface) ImageStream {
	return &imageStream{
		namespace:        namespace,
		name:             name,
		registryOSClient: client,
		imageClient:      newCachedImageGetter(client),
		imageStreamGetter: &cachedImageStreamGetter{
			ctx:          ctx,
			namespace:    namespace,
			name:         name,
			isNamespacer: client,
		},
	}
}

func (is *imageStream) Reference() string {
	return fmt.Sprintf("%s/%s", is.namespace, is.name)
}

// createImageStream creates a new image stream and caches it.
func (is *imageStream) createImageStream(ctx context.Context, userClient client.Interface) (*imageapiv1.ImageStream, error) {
	stream := &imageapiv1.ImageStream{}
	stream.Name = is.name

	stream, err := userClient.ImageStreams(is.namespace).Create(stream)
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
	image, err := is.imageClient.Get(ctx, dgst)
	if err != nil {
		return nil, wrapKStatusErrorOnGetImage(is.name, dgst, err)
	}
	return image, nil
}

// ResolveImageID returns latest TagEvent for specified imageID and an error if
// there's more than one image matching the ID or when one does not exist.
func (is *imageStream) ResolveImageID(ctx context.Context, dgst digest.Digest) (*imageapiv1.TagEvent, error) {
	stream, err := is.imageStreamGetter.get()
	if err != nil {
		return nil, err
	}

	tagEvent, err := originutil.ResolveImageID(stream, dgst.String())
	if err != nil {
		return nil, err
	}

	return tagEvent, nil
}

// GetStoredImageOfImageStream retrieves the Image with digest `dgst` and
// ensures that the image belongs to the image stream `is`. It uses two
// queries to master API:
//
//  1st to get a corresponding image stream
//  2nd to get the image
//
// This allows us to cache the image stream for later use.
//
// If you need the image object to be modified according to image stream tag,
// please use GetImageOfImageStream.
func (is *imageStream) getStoredImageOfImageStream(ctx context.Context, dgst digest.Digest) (*imageapiv1.Image, *imageapiv1.TagEvent, error) {
	tagEvent, err := is.ResolveImageID(ctx, dgst)
	if err != nil {
		context.GetLogger(ctx).Errorf("failed to resolve image %s in ImageStream %s: %v", dgst.String(), is.Reference(), err)
		return nil, nil, wrapKStatusErrorOnGetImage(is.name, dgst, err)
	}

	image, err := is.getImage(ctx, dgst)
	if err != nil {
		return nil, nil, wrapKStatusErrorOnGetImage(is.name, dgst, err)
	}

	return image, tagEvent, nil
}

// GetImageOfImageStream retrieves the Image with digest `dgst` for the image
// stream. The image's field DockerImageReference is modified on the fly to
// pretend that we've got the image from the source from which the image was
// tagged to match tag's DockerImageReference.
//
// NOTE: due to on the fly modification, the returned image object should
// not be sent to the master API. If you need unmodified version of the
// image object, please use getStoredImageOfImageStream.
func (is *imageStream) GetImageOfImageStream(ctx context.Context, dgst digest.Digest) (*imageapiv1.Image, error) {
	image, tagEvent, err := is.getStoredImageOfImageStream(ctx, dgst)
	if err != nil {
		return nil, err
	}

	// We don't want to mutate the origial image object, which we've got by reference.
	img := *image
	img.DockerImageReference = tagEvent.DockerImageReference

	return &img, nil
}

// ImageManifestBlobStored adds the imageapi.ImageManifestBlobStoredAnnotation annotation to image.
func (is *imageStream) ImageManifestBlobStored(ctx context.Context, image *imageapiv1.Image) error {
	image, err := is.getImage(ctx, digest.Digest(image.Name)) // ensure that we have the image object from master API
	if err != nil {
		return err
	}

	if len(image.DockerImageManifest) == 0 || image.Annotations[imageapi.ImageManifestBlobStoredAnnotation] == "true" {
		return nil
	}

	if image.Annotations == nil {
		image.Annotations = make(map[string]string)
	}
	image.Annotations[imageapi.ImageManifestBlobStoredAnnotation] = "true"

	if _, err := is.registryOSClient.Images().Update(image); err != nil {
		context.GetLogger(ctx).Errorf("error updating Image: %v", err)
		return err
	}
	return nil
}

func (is *imageStream) GetSecrets() ([]corev1.Secret, error) {
	secrets, err := is.registryOSClient.ImageStreamSecrets(is.namespace).Secrets(is.name, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting secrets for repository %s: %v", is.Reference(), err)
	}
	return secrets.Items, nil
}

// TagIsInsecure returns true if the given image stream or its tag allow for
// insecure transport.
func (is *imageStream) TagIsInsecure(tag string, dgst digest.Digest) (bool, error) {
	stream, err := is.imageStreamGetter.get()
	if err != nil {
		return false, err
	}

	if insecure, _ := stream.Annotations[imageapi.InsecureRepositoryAnnotation]; insecure == "true" {
		return true, nil
	}

	if len(tag) == 0 {
		// if the client pulled by digest, find the corresponding tag in the image stream
		tag, _ = originutil.LatestImageTagEvent(stream, dgst.String())
	}

	if len(tag) != 0 {
		for _, t := range stream.Spec.Tags {
			if t.Name == tag {
				return t.ImportPolicy.Insecure, nil
			}
		}
	}

	return false, nil
}

func (is *imageStream) Exists() (bool, error) {
	_, err := is.imageStreamGetter.get()
	if err != nil {
		if t, ok := err.(errcode.Error); ok && t.ErrorCode() == disterrors.ErrorCodeNameUnknown {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (is *imageStream) localRegistry() (string, error) {
	stream, err := is.imageStreamGetter.get()
	if err != nil {
		return "", err
	}

	local, err := imageapi.ParseDockerImageReference(stream.Status.DockerImageRepository)
	if err != nil {
		return "", err
	}
	return local.Registry, nil
}

func (is *imageStream) IdentifyCandidateRepositories(primary bool) ([]string, map[string]ImagePullthroughSpec, error) {
	stream, err := is.imageStreamGetter.get()
	if err != nil {
		return nil, nil, err
	}

	localRegistry, _ := is.localRegistry()

	repositoryCandidates, search := identifyCandidateRepositories(stream, localRegistry, primary)
	return repositoryCandidates, search, nil
}

func (is *imageStream) Tags(ctx context.Context) (map[string]digest.Digest, error) {
	stream, err := is.imageStreamGetter.get()
	if err != nil {
		return nil, err
	}

	m := make(map[string]digest.Digest)

	for _, history := range stream.Status.Tags {
		if len(history.Items) == 0 {
			continue
		}

		tag := history.Tag

		dgst, err := digest.ParseDigest(history.Items[0].Image)
		if err != nil {
			context.GetLogger(ctx).Errorf("bad digest %s: %v", history.Items[0].Image, err)
			continue
		}

		m[tag] = dgst
	}

	return m, nil
}

func (is *imageStream) Tag(ctx context.Context, tag string, dgst digest.Digest, pullthroughEnabled bool) error {
	image, err := is.getImage(ctx, dgst)
	if err != nil {
		return err
	}

	if !pullthroughEnabled && !IsImageManaged(image) {
		return distribution.ErrRepositoryUnknown{Name: is.Reference()}
	}

	// We don't want to mutate the origial image object, which we've got by reference.
	img := *image
	img.ResourceVersion = ""

	ism := imageapiv1.ImageStreamMapping{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: is.namespace,
			Name:      is.name,
		},
		Tag:   tag,
		Image: img,
	}

	_, err = is.registryOSClient.ImageStreamMappings(is.namespace).Create(&ism)
	if quotautil.IsErrorQuotaExceeded(err) {
		context.GetLogger(ctx).Errorf("denied creating ImageStreamMapping: %v", err)
		return distribution.ErrAccessDenied
	}
	return err
}

func (is *imageStream) Untag(ctx context.Context, tag string, pullthroughEnabled bool) error {
	stream, err := is.imageStreamGetter.get()
	if err != nil {
		return err
	}

	te := originutil.LatestTaggedImage(stream, tag)
	if te == nil {
		return distribution.ErrTagUnknown{Tag: tag}
	}

	if !pullthroughEnabled {
		dgst, err := digest.ParseDigest(te.Image)
		if err != nil {
			return err
		}

		image, err := is.getImage(ctx, dgst)
		if err != nil {
			return err
		}

		if !IsImageManaged(image) {
			return distribution.ErrTagUnknown{Tag: tag}
		}
	}

	return is.registryOSClient.ImageStreamTags(is.namespace).Delete(imageapi.JoinImageStreamTag(is.name, tag), &metav1.DeleteOptions{})
}

func (is *imageStream) CreateImageStreamMapping(ctx context.Context, userClient client.Interface, tag string, image *imageapiv1.Image) error {
	ism := imageapiv1.ImageStreamMapping{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: is.namespace,
			Name:      is.name,
		},
		Image: *image,
		Tag:   tag,
	}

	if _, err := is.registryOSClient.ImageStreamMappings(is.namespace).Create(&ism); err != nil {
		// if the error was that the image stream wasn't found, try to auto provision it
		statusErr, ok := err.(*kerrors.StatusError)
		if !ok {
			context.GetLogger(ctx).Errorf("error creating ImageStreamMapping: %s", err)
			return err
		}

		if quotautil.IsErrorQuotaExceeded(statusErr) {
			context.GetLogger(ctx).Errorf("denied creating ImageStreamMapping: %v", statusErr)
			return distribution.ErrAccessDenied
		}

		status := statusErr.ErrStatus
		kind := strings.ToLower(status.Details.Kind)
		isValidKind := kind == "imagestream" /*pre-1.2*/ || kind == "imagestreams" /*1.2 to 1.6*/ || kind == "imagestreammappings" /*1.7+*/
		if !isValidKind || status.Code != http.StatusNotFound || status.Details.Name != is.name {
			context.GetLogger(ctx).Errorf("error creating ImageStreamMapping: %s", err)
			return err
		}

		if _, err := is.createImageStream(ctx, userClient); err != nil {
			if e, ok := err.(errcode.Error); ok && e.ErrorCode() == errcode.ErrorCodeUnknown {
				// TODO: convert statusErr to distribution error
				return statusErr
			}
			return err
		}

		// try to create the ISM again
		if _, err := is.registryOSClient.ImageStreamMappings(is.namespace).Create(&ism); err != nil {
			if quotautil.IsErrorQuotaExceeded(err) {
				context.GetLogger(ctx).Errorf("denied a creation of ImageStreamMapping: %v", err)
				return distribution.ErrAccessDenied
			}
			context.GetLogger(ctx).Errorf("error creating ImageStreamMapping: %s", err)
			return err
		}
	}
	return nil
}

// GetLimitRangeList returns list of limit ranges for repo.
func (is *imageStream) GetLimitRangeList(ctx context.Context, cache ProjectObjectListStore) (*corev1.LimitRangeList, error) {
	if cache != nil {
		obj, exists, _ := cache.Get(is.namespace)
		if exists {
			return obj.(*corev1.LimitRangeList), nil
		}
	}

	context.GetLogger(ctx).Debugf("listing limit ranges in namespace %s", is.namespace)

	lrs, err := is.registryOSClient.LimitRanges(is.namespace).List(metav1.ListOptions{})
	if err != nil {
		context.GetLogger(ctx).Errorf("failed to list limitranges: %v", err)
		return nil, err
	}

	if cache != nil {
		err = cache.Add(is.namespace, lrs)
		if err != nil {
			context.GetLogger(ctx).Errorf("failed to cache limit range list: %v", err)
		}
	}

	return lrs, nil
}
