package server

import (
	"fmt"
	"strings"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest/schema2"

	dockerapiv10 "github.com/openshift/api/image/docker10"
	imageapiv1 "github.com/openshift/api/image/v1"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/cache"
	registrymanifest "github.com/openshift/image-registry/pkg/dockerregistry/server/manifest"
	"github.com/openshift/image-registry/pkg/origin-common/image/registryclient"
)

func getNamespaceName(resourceName string) (string, string, error) {
	repoParts := strings.Split(resourceName, "/")
	if len(repoParts) != 2 {
		return "", "", distribution.ErrRepositoryNameInvalid{
			Name:   resourceName,
			Reason: fmt.Errorf("it must be of the format <project>/<name>"),
		}
	}
	ns := repoParts[0]
	if len(ns) == 0 {
		return "", "", ErrNamespaceRequired
	}
	name := repoParts[1]
	if len(name) == 0 {
		return "", "", ErrNamespaceRequired
	}
	return ns, name, nil
}

// getImportContext loads secrets and returns a context for getting
// distribution clients to remote repositories.
func getImportContext(ctx context.Context, secretsGetter secretsGetter) registryclient.RepositoryRetriever {
	secrets, err := secretsGetter()
	if err != nil {
		context.GetLogger(ctx).Errorf("error getting secrets: %v", err)
	}
	credentials := registryclient.NewCredentialsForSecrets(secrets)
	return registryclient.NewContext(secureTransport, insecureTransport).WithCredentials(credentials)
}

// RememberLayersOfImage caches the layer digests of given image.
func RememberLayersOfImage(ctx context.Context, cache cache.RepositoryDigest, image *imageapiv1.Image, cacheName string) {
	if len(image.DockerImageLayers) > 0 {
		for _, layer := range image.DockerImageLayers {
			_ = cache.AddDigest(digest.Digest(layer.Name), cacheName)
		}
		meta, ok := image.DockerImageMetadata.Object.(*dockerapiv10.DockerImage)
		if !ok {
			context.GetLogger(ctx).Errorf("image %s does not have metadata", image.Name)
			return
		}
		// remember reference to manifest config as well for schema 2
		if image.DockerImageManifestMediaType == schema2.MediaTypeManifest && len(meta.ID) > 0 {
			_ = cache.AddDigest(digest.Digest(meta.ID), cacheName)
		}
		return
	}

	manifest, err := registrymanifest.NewFromImage(image)
	if err != nil {
		context.GetLogger(ctx).Errorf("cannot remember layers of image %s: %v", image.Name, err)
		return
	}
	for _, ref := range manifest.References() {
		_ = cache.AddDigest(ref.Digest, cacheName)
	}
}
