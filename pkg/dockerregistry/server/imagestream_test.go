package server

import (
	"testing"
	"time"

	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"

	imageapiv1 "github.com/openshift/api/image/v1"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/cache"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/client"
)

func newTestImageStream(ctx context.Context, t *testing.T, namespace, name string, client client.Interface) *imageStream {
	imageStreamGetter := &cachedImageStreamGetter{
		ctx:          ctx,
		namespace:    namespace,
		name:         name,
		isNamespacer: client,
	}

	digestCache, err := cache.NewBlobDigest(
		defaultDescriptorCacheSize,
		defaultDigestToRepositoryCacheSize,
		24*time.Hour, // for tests it's virtually forever
	)
	if err != nil {
		t.Fatalf("unable to create cache: %v", err)
	}

	return &imageStream{
		namespace:         namespace,
		name:              name,
		registryOSClient:  client,
		cachedImages:      make(map[digest.Digest]*imageapiv1.Image),
		imageStreamGetter: imageStreamGetter,
		cache: &cache.RepoDigest{
			Cache: digestCache,
		},
	}
}
