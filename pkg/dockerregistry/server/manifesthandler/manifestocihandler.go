package manifesthandler

import (
	"context"
	"time"

	"github.com/distribution/distribution/v3"
	dcontext "github.com/distribution/distribution/v3/context"
	"github.com/distribution/distribution/v3/manifest/ocischema"
	"github.com/opencontainers/go-digest"
	"k8s.io/apimachinery/pkg/util/wait"

	imageapiv1 "github.com/openshift/api/image/v1"
)

type manifestOCIHandler struct {
	blobStore    distribution.BlobStore
	manifest     *ocischema.DeserializedManifest
	cachedConfig []byte
}

var _ ManifestHandler = &manifestOCIHandler{}

func (h *manifestOCIHandler) Config(ctx context.Context) ([]byte, error) {
	if h.cachedConfig == nil {
		blob, err := h.blobStore.Get(ctx, h.manifest.Config.Digest)
		if err != nil {
			dcontext.GetLogger(ctx).Errorf("failed to get manifest config: %v", err)
			return nil, err
		}
		h.cachedConfig = blob
	}

	return h.cachedConfig, nil
}

func (h *manifestOCIHandler) Digest() (digest.Digest, error) {
	_, p, err := h.manifest.Payload()
	if err != nil {
		return "", err
	}
	return digest.FromBytes(p), nil
}

func (h *manifestOCIHandler) Manifest() distribution.Manifest {
	return h.manifest
}

func (h *manifestOCIHandler) Layers(ctx context.Context) (string, []imageapiv1.ImageLayer, error) {
	layers := make([]imageapiv1.ImageLayer, len(h.manifest.Layers))
	for i, layer := range h.manifest.Layers {
		layers[i].Name = layer.Digest.String()
		layers[i].LayerSize = layer.Size
		layers[i].MediaType = layer.MediaType
	}
	return imageapiv1.DockerImageLayersOrderAscending, layers, nil
}

func (h *manifestOCIHandler) Payload() (mediaType string, payload []byte, canonical []byte, err error) {
	mt, p, err := h.manifest.Payload()
	return mt, p, p, err
}

func (h *manifestOCIHandler) verifyLayer(ctx context.Context, fsLayer distribution.Descriptor) error {
	// https://bugzilla.redhat.com/show_bug.cgi?id=1745743
	// AWS S3 (and potentially other object stores) only have eventual
	// consistency guarantees. Stat can fail here if an image layer was
	// recently pushed. In the event Stat returns `ErrBlobUnknown`, retry
	// up to 3 seconds.
	var desc distribution.Descriptor
	if err := wait.ExponentialBackoff(
		wait.Backoff{
			Duration: 100 * time.Millisecond,
			Factor:   2,
			Steps:    6,
		},
		func() (done bool, err error) {
			desc, err = h.blobStore.Stat(ctx, fsLayer.Digest)
			switch {
			case err == nil:
				return true, nil
			case err == distribution.ErrBlobUnknown:
				return false, nil
			default:
				return true, err
			}
		},
	); err != nil {
		if err == wait.ErrWaitTimeout {
			return distribution.ErrBlobUnknown
		}
		return err
	}

	if fsLayer.Size != desc.Size {
		return ErrManifestBlobBadSize{
			Digest:         fsLayer.Digest,
			ActualSize:     desc.Size,
			SizeInManifest: fsLayer.Size,
		}
	}

	return nil
}

func (h *manifestOCIHandler) Verify(ctx context.Context, skipDependencyVerification bool) error {
	var errs distribution.ErrManifestVerification

	if skipDependencyVerification {
		return nil
	}

	// we want to verify that referenced blobs exist locally or accessible over
	// pullthroughBlobStore. The base image of this image can be remote repository
	// and since we use pullthroughBlobStore all the layer existence checks will be
	// successful. This means that the docker client will not attempt to send them
	// to us as it will assume that the registry has them.

	for _, fsLayer := range h.manifest.References() {
		if err := h.verifyLayer(ctx, fsLayer); err != nil {
			if err != distribution.ErrBlobUnknown {
				errs = append(errs, err)
				continue
			}

			// On error here, we always append unknown blob errors.
			errs = append(errs, distribution.ErrManifestBlobUnknown{Digest: fsLayer.Digest})
		}
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}
