package server

import (
	"fmt"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/docker/distribution/registry/api/errcode"
	disterrors "github.com/docker/distribution/registry/api/v2"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dockerapiv10 "github.com/openshift/api/image/docker10"
	imageapiv1 "github.com/openshift/api/image/v1"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/cache"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/client"
	imageapi "github.com/openshift/image-registry/pkg/origin-common/image/apis/image"
	quotautil "github.com/openshift/image-registry/pkg/origin-common/quota/util"
	util "github.com/openshift/image-registry/pkg/origin-common/util"
)

type imageStream struct {
	namespace string
	name      string

	registryOSClient client.Interface

	// cachedImages contains images cached for the lifetime of the request being handled.
	cachedImages map[digest.Digest]*imageapiv1.Image
	// imageStreamGetter fetches and caches an image stream. The image stream stays cached for the entire time of handling single repository-scoped request.
	imageStreamGetter *cachedImageStreamGetter
	// cache is used to associate a digest with a repository name.
	cache cache.RepositoryDigest
}

func (is *imageStream) Reference() string {
	return fmt.Sprintf("%s/%s", is.namespace, is.name)
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
	if err := util.ImageWithMetadata(image); err != nil {
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

	tagEvent, err := util.ResolveImageID(stream, dgst.String())
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
			_ = is.cache.AddDigest(digest.Digest(layer.Name), cacheName)
		}
		meta, ok := image.DockerImageMetadata.Object.(*dockerapiv10.DockerImage)
		if !ok {
			context.GetLogger(ctx).Errorf("image %s does not have metadata", image.Name)
			return
		}
		// remember reference to manifest config as well for schema 2
		if image.DockerImageManifestMediaType == schema2.MediaTypeManifest && len(meta.ID) > 0 {
			_ = is.cache.AddDigest(digest.Digest(meta.ID), cacheName)
		}
		return
	}

	manifest, err := NewManifestFromImage(image)
	if err != nil {
		context.GetLogger(ctx).Errorf("cannot remember layers of image %s: %v", image.Name, err)
		return
	}
	for _, ref := range manifest.References() {
		_ = is.cache.AddDigest(ref.Digest, cacheName)
	}
}

func (is *imageStream) getSecrets() ([]corev1.Secret, error) {
	secrets, err := is.registryOSClient.ImageStreamSecrets(is.namespace).Secrets(is.name, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting secrets for repository %s: %v", is.Reference(), err)
	}
	return secrets.Items, nil
}

// tagIsInsecure returns true if the given image stream or its tag allow for
// insecure transport.
func (is *imageStream) tagIsInsecure(tag string, dgst digest.Digest) (bool, error) {
	stream, err := is.imageStreamGetter.get()
	if err != nil {
		return false, err
	}

	if insecure, _ := stream.Annotations[imageapi.InsecureRepositoryAnnotation]; insecure == "true" {
		return true, nil
	}

	if len(tag) == 0 {
		// if the client pulled by digest, find the corresponding tag in the image stream
		tag, _ = util.LatestImageTagEvent(stream, dgst.String())
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

func (is *imageStream) identifyCandidateRepositories(primary bool) ([]string, map[string]imagePullthroughSpec, error) {
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
	image, err := is.registryOSClient.Images().Get(dgst.String(), metav1.GetOptions{})
	if err != nil {
		context.GetLogger(ctx).Errorf("unable to get image: %s", dgst.String())
		return err
	}
	image.SetResourceVersion("")

	if !pullthroughEnabled && !isImageManaged(image) {
		return distribution.ErrRepositoryUnknown{Name: is.Reference()}
	}

	ism := imageapiv1.ImageStreamMapping{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: is.namespace,
			Name:      is.name,
		},
		Tag:   tag,
		Image: *image,
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

	te := util.LatestTaggedImage(stream, tag)
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

		if !isImageManaged(image) {
			return distribution.ErrTagUnknown{Tag: tag}
		}
	}

	return is.registryOSClient.ImageStreamTags(is.namespace).Delete(imageapi.JoinImageStreamTag(is.name, tag), &metav1.DeleteOptions{})
}
