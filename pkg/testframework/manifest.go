package testframework

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/docker/distribution"
	"github.com/opencontainers/go-digest"
	"github.com/pborman/uuid"

	"github.com/openshift/image-registry/pkg/testutil"
)

type Schema2ImageData struct {
	ConfigMediaType   string
	Config            []byte
	ConfigDigest      digest.Digest
	LayerMediaType    string
	Layer             []byte
	LayerDigest       digest.Digest
	ManifestMediaType string
	Manifest          []byte
	ManifestDigest    digest.Digest
}

func NewSchema2ImageData() (Schema2ImageData, error) {
	cfg := map[string]interface{}{
		"rootfs": map[string]interface{}{
			"diff_ids": make([]string, 1),
		},
		"history": make([]struct{}, 1),
	}

	configContent, err := json.Marshal(&cfg)
	if err != nil {
		return Schema2ImageData{}, fmt.Errorf("marshal image config: %w", err)
	}

	data := Schema2ImageData{
		ConfigMediaType:   "application/vnd.docker.container.image.v1+json",
		Config:            configContent,
		LayerMediaType:    "application/vnd.docker.image.rootfs.diff.tar.gzip",
		Layer:             []byte("image-registry-integration-test-" + uuid.NewRandom().String()),
		ManifestMediaType: "application/vnd.docker.distribution.manifest.v2+json",
	}
	data.ConfigDigest = digest.FromBytes(data.Config)
	data.LayerDigest = digest.FromBytes(data.Layer)

	manifest, err := json.Marshal(map[string]interface{}{
		"schemaVersion": 2,
		"mediaType":     data.ManifestMediaType,
		"config": map[string]interface{}{
			"mediaType": data.ConfigMediaType,
			"size":      len(data.Config),
			"digest":    data.ConfigDigest.String(),
		},
		"layers": []map[string]interface{}{
			{
				"mediaType": data.LayerMediaType,
				"size":      len(data.Layer),
				"digest":    data.LayerDigest.String(),
			},
		},
	})
	if err != nil {
		return data, fmt.Errorf("unable to create image manifest: %v", err)
	}

	data.Manifest = manifest
	data.ManifestDigest = digest.FromBytes(data.Manifest)

	return data, nil
}

func ServeV2(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == "GET" && r.URL.Path == "/v2/" {
		_, _ = w.Write([]byte(`{}`))
		return true
	}
	return false
}

func ServeImage(w http.ResponseWriter, r *http.Request, image string, data Schema2ImageData, tags []string) bool {
	prefix := "/v2/" + image

	isGetManifest := func() bool {
		if r.Method == "GET" {
			if r.URL.Path == prefix+"/manifests/"+data.ManifestDigest.String() {
				return true
			}
			for _, tag := range tags {
				if r.URL.Path == prefix+"/manifests/"+tag {
					return true
				}
			}
		}
		return false
	}

	switch {
	case isGetManifest():
		w.Header().Set("Content-Type", data.ManifestMediaType)
		_, _ = w.Write(data.Manifest)
	case r.Method == "GET" && r.URL.Path == prefix+"/blobs/"+data.ConfigDigest.String():
		_, _ = w.Write(data.Config)
	case r.Method == "GET" && r.URL.Path == prefix+"/blobs/"+data.LayerDigest.String():
		_, _ = w.Write(data.Layer)
	case r.Method == "HEAD" && r.URL.Path == prefix+"/blobs/"+data.ConfigDigest.String():
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data.Config)))
		w.WriteHeader(http.StatusOK)
	case r.Method == "HEAD" && r.URL.Path == prefix+"/blobs/"+data.LayerDigest.String():
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data.Layer)))
		w.WriteHeader(http.StatusOK)
	default:
		return false
	}
	return true
}

func PushSchema2ImageData(ctx context.Context, repo distribution.Repository, tag string, data Schema2ImageData) (distribution.Manifest, error) {
	manifest, _, err := distribution.UnmarshalManifest(data.ManifestMediaType, data.Manifest)
	if err != nil {
		return manifest, fmt.Errorf("parse manifest: %w", err)
	}

	layerDesc := distribution.Descriptor{
		Digest: data.LayerDigest,
		Size:   int64(len(data.Layer)),
	}

	if err := testutil.UploadBlob(ctx, repo, layerDesc, data.Layer); err != nil {
		return nil, fmt.Errorf("upload layer: %w", err)
	}

	configDesc := distribution.Descriptor{
		Digest: data.ConfigDigest,
		Size:   int64(len(data.Config)),
	}

	if err := testutil.UploadBlob(ctx, repo, configDesc, data.Config); err != nil {
		return nil, fmt.Errorf("upload image config: %w", err)
	}

	if err := testutil.UploadManifest(ctx, repo, tag, manifest); err != nil {
		return manifest, fmt.Errorf("upload manifest: %w", err)
	}

	return manifest, nil
}
