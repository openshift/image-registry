package server

import (
	"fmt"

	"github.com/docker/distribution/digest"

	imageapiv1 "github.com/openshift/origin/pkg/image/apis/image/v1"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/cache"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/client"
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
