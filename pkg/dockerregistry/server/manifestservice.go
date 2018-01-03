package server

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/docker/distribution/registry/api/errcode"
	regapi "github.com/docker/distribution/registry/api/v2"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	imageapi "github.com/openshift/origin/pkg/image/apis/image"
	imageapiv1 "github.com/openshift/origin/pkg/image/apis/image/v1"
	quotautil "github.com/openshift/origin/pkg/quota/util"
)

// ErrManifestBlobBadSize is returned when the blob size in a manifest does
// not match the actual size. The docker/distribution does not check this and
// therefore does not provide an error for this.
type ErrManifestBlobBadSize struct {
	Digest         digest.Digest
	ActualSize     int64
	SizeInManifest int64
}

func (err ErrManifestBlobBadSize) Error() string {
	return fmt.Sprintf("the blob %s has the size (%d) different from the one specified in the manifest (%d)",
		err.Digest, err.ActualSize, err.SizeInManifest)
}

var _ distribution.ManifestService = &manifestService{}

type manifestService struct {
	manifests distribution.ManifestService
	blobStore distribution.BlobStore

	serverAddr  string
	imageStream *imageStream

	// acceptSchema2 allows to refuse the manifest schema version 2
	acceptSchema2 bool
}

// Exists returns true if the manifest specified by dgst exists.
func (m *manifestService) Exists(ctx context.Context, dgst digest.Digest) (bool, error) {
	context.GetLogger(ctx).Debugf("(*manifestService).Exists")

	image, _, err := m.imageStream.getImageOfImageStream(ctx, dgst)
	if err != nil {
		return false, err
	}
	return image != nil, nil
}

// Get retrieves the manifest with digest `dgst`.
func (m *manifestService) Get(ctx context.Context, dgst digest.Digest, options ...distribution.ManifestServiceOption) (distribution.Manifest, error) {
	context.GetLogger(ctx).Debugf("(*manifestService).Get")

	image, _, _, err := m.imageStream.getStoredImageOfImageStream(ctx, dgst)
	if err != nil {
		return nil, err
	}

	ref := imageapi.DockerImageReference{
		Registry:  m.serverAddr,
		Namespace: m.imageStream.namespace,
		Name:      m.imageStream.name,
	}
	if isImageManaged(image) {
		// Reference without a registry part refers to repository containing locally managed images.
		// Such an entry is retrieved, checked and set by blobDescriptorService operating only on local blobs.
		ref.Registry = ""
	} else {
		// Repository with a registry points to remote repository. This is used by pullthrough middleware.
		ref = ref.DockerClientDefaults().AsRepository()
	}

	manifest, err := m.manifests.Get(ctx, dgst, options...)
	switch err.(type) {
	case distribution.ErrManifestUnknownRevision:
		break
	case nil:
		m.imageStream.rememberLayersOfImage(ctx, image, ref.Exact())
		m.migrateManifest(ctx, image, dgst, manifest, true)
		return manifest, nil
	default:
		context.GetLogger(ctx).Errorf("unable to get manifest from storage: %v", err)
		return nil, err
	}

	if len(image.DockerImageManifest) == 0 {
		// We don't have the manifest in the storage and we don't have the manifest
		// inside the image so there is no point to continue.
		return nil, distribution.ErrManifestUnknownRevision{
			Name:     m.imageStream.Reference(),
			Revision: dgst,
		}
	}

	manifest, err = NewManifestFromImage(image)
	if err == nil {
		m.imageStream.rememberLayersOfImage(ctx, image, ref.Exact())
		m.migrateManifest(ctx, image, dgst, manifest, false)
	}

	return manifest, err
}

// Put creates or updates the named manifest.
func (m *manifestService) Put(ctx context.Context, manifest distribution.Manifest, options ...distribution.ManifestServiceOption) (digest.Digest, error) {
	context.GetLogger(ctx).Debugf("(*manifestService).Put")

	mh, err := NewManifestHandler(m.serverAddr, m.blobStore, manifest)
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
	ism := imageapiv1.ImageStreamMapping{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: m.imageStream.namespace,
			Name:      m.imageStream.name,
		},
		Image: imageapiv1.Image{
			ObjectMeta: metav1.ObjectMeta{
				Name: dgst.String(),
				Annotations: map[string]string{
					imageapi.ManagedByOpenShiftAnnotation:      "true",
					imageapi.ImageManifestBlobStoredAnnotation: "true",
					imageapi.DockerImageLayersOrderAnnotation:  layerOrder,
				},
			},
			DockerImageReference:         fmt.Sprintf("%s/%s/%s@%s", m.serverAddr, m.imageStream.namespace, m.imageStream.name, dgst.String()),
			DockerImageManifest:          string(payload),
			DockerImageManifestMediaType: mediaType,
			DockerImageConfig:            string(config),
			DockerImageLayers:            layers,
		},
	}

	for _, option := range options {
		if opt, ok := option.(distribution.WithTagOption); ok {
			ism.Tag = opt.Tag
			break
		}
	}

	if _, err = m.imageStream.registryOSClient.ImageStreamMappings(m.imageStream.namespace).Create(&ism); err != nil {
		// if the error was that the image stream wasn't found, try to auto provision it
		statusErr, ok := err.(*kerrors.StatusError)
		if !ok {
			context.GetLogger(ctx).Errorf("error creating ImageStreamMapping: %s", err)
			return "", err
		}

		if quotautil.IsErrorQuotaExceeded(statusErr) {
			context.GetLogger(ctx).Errorf("denied creating ImageStreamMapping: %v", statusErr)
			return "", distribution.ErrAccessDenied
		}

		status := statusErr.ErrStatus
		kind := strings.ToLower(status.Details.Kind)
		isValidKind := kind == "imagestream" /*pre-1.2*/ || kind == "imagestreams" /*1.2 to 1.6*/ || kind == "imagestreammappings" /*1.7+*/
		if !isValidKind || status.Code != http.StatusNotFound || status.Details.Name != m.imageStream.name {
			context.GetLogger(ctx).Errorf("error creating ImageStreamMapping: %s", err)
			return "", err
		}

		if _, err := m.imageStream.createImageStream(ctx); err != nil {
			if e, ok := err.(errcode.Error); ok && e.ErrorCode() == errcode.ErrorCodeUnknown {
				// TODO: convert statusErr to distribution error
				return "", statusErr
			}
			return "", err
		}

		// try to create the ISM again
		if _, err := m.imageStream.registryOSClient.ImageStreamMappings(m.imageStream.namespace).Create(&ism); err != nil {
			if quotautil.IsErrorQuotaExceeded(err) {
				context.GetLogger(ctx).Errorf("denied a creation of ImageStreamMapping: %v", err)
				return "", distribution.ErrAccessDenied
			}
			context.GetLogger(ctx).Errorf("error creating ImageStreamMapping: %s", err)
			return "", err
		}
	}

	return dgst, nil
}

// Delete deletes the manifest with digest `dgst`. Note: Image resources
// in OpenShift are deleted via 'oc adm prune images'. This function deletes
// the content related to the manifest in the registry's storage (signatures).
func (m *manifestService) Delete(ctx context.Context, dgst digest.Digest) error {
	context.GetLogger(ctx).Debugf("(*manifestService).Delete")
	return m.manifests.Delete(ctx, dgst)
}

// manifestInflight tracks currently downloading manifests
var manifestInflight = make(map[digest.Digest]struct{})

// manifestInflightSync protects manifestInflight
var manifestInflightSync sync.Mutex

func (m *manifestService) migrateManifest(ctx context.Context, image *imageapiv1.Image, dgst digest.Digest, manifest distribution.Manifest, isLocalStored bool) {
	// Everything in its place and nothing to do.
	if isLocalStored && len(image.DockerImageManifest) == 0 {
		return
	}
	manifestInflightSync.Lock()
	if _, ok := manifestInflight[dgst]; ok {
		manifestInflightSync.Unlock()
		return
	}
	manifestInflight[dgst] = struct{}{}
	manifestInflightSync.Unlock()

	go m.storeManifestLocally(ctx, image, dgst, manifest, isLocalStored)
}

func (m *manifestService) storeManifestLocally(ctx context.Context, image *imageapiv1.Image, dgst digest.Digest, manifest distribution.Manifest, isLocalStored bool) {
	defer func() {
		manifestInflightSync.Lock()
		delete(manifestInflight, dgst)
		manifestInflightSync.Unlock()
	}()

	if !isLocalStored {
		if _, err := m.manifests.Put(ctx, manifest); err != nil {
			context.GetLogger(ctx).Errorf("unable to put manifest to storage: %v", err)
			return
		}
	}

	if len(image.DockerImageManifest) == 0 || image.Annotations[imageapi.ImageManifestBlobStoredAnnotation] == "true" {
		return
	}

	if image.Annotations == nil {
		image.Annotations = make(map[string]string)
	}
	image.Annotations[imageapi.ImageManifestBlobStoredAnnotation] = "true"

	if _, err := m.imageStream.updateImage(image); err != nil {
		context.GetLogger(ctx).Errorf("error updating Image: %v", err)
	}
}
