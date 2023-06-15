package server

import (
	"context"
	"fmt"

	"github.com/distribution/distribution/v3"
	dcontext "github.com/distribution/distribution/v3/context"
	"github.com/distribution/distribution/v3/manifest/schema2"
	"github.com/distribution/distribution/v3/registry/api/errcode"
	regapi "github.com/distribution/distribution/v3/registry/api/v2"
	"github.com/opencontainers/go-digest"
	imageapiv1 "github.com/openshift/api/image/v1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/cache"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/client"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/manifesthandler"
	"github.com/openshift/image-registry/pkg/imagestream"
)

var _ distribution.ManifestService = &manifestService{}

type manifestService struct {
	manifests distribution.ManifestService
	blobStore distribution.BlobStore

	serverAddr       string
	registryOSClient client.Interface
	imageStream      imagestream.ImageStream
	cache            cache.RepositoryDigest

	// acceptSchema2 allows to refuse the manifest schema version 2
	acceptSchema2 bool
}

// Exists returns true if the manifest specified by dgst exists.
func (m *manifestService) Exists(ctx context.Context, dgst digest.Digest) (bool, error) {
	dcontext.GetLogger(ctx).Debugf("(*manifestService).Exists")

	image, err := m.imageStream.GetImageOfImageStream(ctx, dgst)
	if err != nil {
		switch err.Code() {
		case imagestream.ErrImageStreamImageNotFoundCode:
			dcontext.GetLogger(ctx).Errorf("manifestService.Exists: image %s is not found in imagestream %s", dgst.String(), m.imageStream.Reference())
			fallthrough
		case imagestream.ErrImageStreamNotFoundCode:
			return false, distribution.ErrBlobUnknown
		}
		return false, err
	}
	return image != nil, nil
}

// Get retrieves the manifest with digest `dgst`.
func (m *manifestService) Get(ctx context.Context, dgst digest.Digest, options ...distribution.ManifestServiceOption) (distribution.Manifest, error) {
	dcontext.GetLogger(ctx).Debugf("(*manifestService).Get")

	image, rErr := m.imageStream.GetImageOfImageStream(ctx, dgst)
	if rErr != nil {
		switch rErr.Code() {
		case imagestream.ErrImageStreamNotFoundCode, imagestream.ErrImageStreamImageNotFoundCode:
			dcontext.GetLogger(ctx).Errorf(
				"manifestService.Get: unable to get image %s in imagestream %s: %v",
				dgst.String(),
				m.imageStream.Reference(),
				rErr,
			)
			return nil, distribution.ErrManifestUnknownRevision{
				Name:     m.imageStream.Reference(),
				Revision: dgst,
			}
		case imagestream.ErrImageStreamForbiddenCode:
			dcontext.GetLogger(ctx).Errorf(
				"manifestService.Get: unable to get access to imagestream %s to find image %s: %v",
				m.imageStream.Reference(),
				dgst.String(),
				rErr,
			)
			return nil, distribution.ErrAccessDenied
		}
		return nil, rErr
	}

	// Reference without a registry part refers to repository containing locally managed images.
	// Such an entry is retrieved, checked and set by blobDescriptorService operating only on local blobs.
	ref := m.imageStream.Reference()
	if !imagestream.IsImageManaged(image) {
		// Repository with a registry points to remote repository. This is used by pullthrough middleware.
		// TODO(dmage): should ref contain image.DockerImageReferece if the image is not managed?
		ref = fmt.Sprintf("%s/%s", m.serverAddr, ref)
	}

	manifest, err := m.manifests.Get(ctx, dgst, options...)
	if err != nil {
		return nil, err
	}

	RememberLayersOfImage(ctx, m.cache, image, ref)

	return manifest, nil
}

// Put creates or updates the named manifest.
func (m *manifestService) Put(ctx context.Context, manifest distribution.Manifest, options ...distribution.ManifestServiceOption) (digest.Digest, error) {
	dcontext.GetLogger(ctx).Debugf("(*manifestService).Put")

	mh, err := manifesthandler.NewManifestHandler(m.serverAddr, m.blobStore, manifest)
	if err != nil {
		return "", regapi.ErrorCodeManifestInvalid.WithDetail(err)
	}

	mediaType, payload, _, err := mh.Payload()
	if err != nil {
		return "", regapi.ErrorCodeManifestInvalid.WithDetail(err)
	}

	// this is fast to check, let's do it before verification
	if !m.acceptSchema2 && mediaType == schema2.MediaTypeManifest {
		return "", regapi.ErrorCodeManifestInvalid.WithDetail(fmt.Errorf("manifest V2 schema 2 not allowed"))
	}

	// in order to stat the referenced blobs, repository need to be set on the context
	if err := mh.Verify(ctx, false); err != nil {
		return "", err
	}

	_, err = m.manifests.Put(ctx, manifest, options...)
	if err != nil {
		return "", err
	}

	config, err := mh.Config(ctx)
	if err != nil {
		return "", err
	}

	dgst, err := mh.Digest()
	if err != nil {
		return "", err
	}

	layerOrder, layers, err := mh.Layers(ctx)
	if err != nil {
		return "", err
	}

	// Upload to openshift
	uclient, ok := userClientFrom(ctx)
	if !ok {
		errmsg := "error creating user client to auto provision image stream: user client to master API unavailable"
		dcontext.GetLogger(ctx).Errorf(errmsg)
		return "", errcode.ErrorCodeUnknown.WithDetail(errmsg)
	}

	image := &imageapiv1.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name: dgst.String(),
			Annotations: map[string]string{
				imageapiv1.ManagedByOpenShiftAnnotation:      "true",
				imageapiv1.ImageManifestBlobStoredAnnotation: "true",
				imageapiv1.DockerImageLayersOrderAnnotation:  layerOrder,
			},
		},
		DockerImageReference:         fmt.Sprintf("%s/%s@%s", m.serverAddr, m.imageStream.Reference(), dgst.String()),
		DockerImageManifest:          string(payload),
		DockerImageManifestMediaType: mediaType,
		DockerImageConfig:            string(config),
		DockerImageLayers:            layers,
	}

	tag := ""
	for _, option := range options {
		if opt, ok := option.(distribution.WithTagOption); ok {
			tag = opt.Tag
			break
		}
	}

	pushByDigest := tag == ""
	if pushByDigest {
		image, err := m.registryOSClient.Images().Create(ctx, image, metav1.CreateOptions{})
		if kapierrors.IsAlreadyExists(err) {
			return dgst, nil
		}
		if err != nil {
			dcontext.GetLogger(ctx).Errorf(
				"manifestService.Put: image creation failed for image %s: %v",
				image.Name, err,
			)
			return "", err
		}
		return dgst, nil
	}

	rErr := m.imageStream.CreateImageStreamMapping(ctx, uclient, tag, image)
	if rErr != nil {
		switch rErr.Code() {
		case imagestream.ErrImageStreamNotFoundCode:
			dcontext.GetLogger(ctx).Errorf("manifestService.Put: imagestreammapping failed for image %s@%s: %v", m.imageStream.Reference(), image.Name, rErr)
			return "", distribution.ErrManifestUnknownRevision{
				Name:     m.imageStream.Reference(),
				Revision: dgst,
			}
		case imagestream.ErrImageStreamForbiddenCode:
			dcontext.GetLogger(ctx).Errorf("manifestService.Put: imagestreammapping got access denied for image %s@%s: %v", m.imageStream.Reference(), image.Name, rErr)
			return "", distribution.ErrAccessDenied
		}
		return "", rErr
	}

	return dgst, nil
}

// Delete deletes the manifest with digest `dgst`. Note: Image resources
// in OpenShift are deleted via 'oc adm prune images'. This function deletes
// the content related to the manifest in the registry's storage (signatures).
func (m *manifestService) Delete(ctx context.Context, dgst digest.Digest) error {
	dcontext.GetLogger(ctx).Debugf("(*manifestService).Delete")

	_, err := m.imageStream.GetImageOfImageStream(ctx, dgst)
	if err == nil {
		// The image stream has a reference to the manifest, so it will be
		// served even when the repository doesn't have the manifest link. In
		// other words, in this case deleting the manifest link will not
		// change the availability of the manifest, so we reject this request.
		return distribution.ErrUnsupported
	}

	switch err.Code() {
	case imagestream.ErrImageStreamNotFoundCode, imagestream.ErrImageStreamImageNotFoundCode:
		// There is no image/imagestream. Let's just delete the link.
	case imagestream.ErrImageStreamForbiddenCode:
		dcontext.GetLogger(ctx).Errorf("manifestService.Delete: unable to get access to imagestream %s to find image %s: %v", m.imageStream.Reference(), dgst.String(), err)
		return distribution.ErrAccessDenied
	default:
		return err
	}

	return m.manifests.Delete(ctx, dgst)
}
