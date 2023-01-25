package manifesthandler

import (
	"context"

	"github.com/docker/distribution"
	"github.com/docker/distribution/manifest/manifestlist"
	"github.com/opencontainers/go-digest"
	imageapiv1 "github.com/openshift/api/image/v1"
)

type manifestListHandler struct {
	manifest *manifestlist.DeserializedManifestList
}

var _ ManifestHandler = &manifestListHandler{}

func (h *manifestListHandler) Config(ctx context.Context) ([]byte, error) {
	return nil, nil
}

func (h *manifestListHandler) Manifest() distribution.Manifest {
	return h.manifest
}

func (h *manifestListHandler) Layers(ctx context.Context) (string, []imageapiv1.ImageLayer, error) {
	return "", nil, nil
}

func (h *manifestListHandler) Digest() (digest.Digest, error) {
	_, p, err := h.manifest.Payload()
	if err != nil {
		return "", err
	}
	return digest.FromBytes(p), nil
}

func (h *manifestListHandler) Payload() (mediaType string, payload []byte, canonical []byte, err error) {
	mt, p, err := h.manifest.Payload()
	return mt, p, p, err
}

func (h *manifestListHandler) Verify(ctx context.Context, skipDependencyVerification bool) error {
	// we could verify that the sub-manifests exist, but that would get on the way
	// of supporting sparse manifest lists in the future so we verify nothing.
	return nil
}
