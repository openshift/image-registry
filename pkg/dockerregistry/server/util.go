package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/distribution"
	dcontext "github.com/docker/distribution/context"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/docker/distribution/registry/client/auth"
	dockerregistry "github.com/docker/docker/registry"
	"github.com/opencontainers/go-digest"

	corev1 "k8s.io/api/core/v1"

	dockerapiv10 "github.com/openshift/api/image/docker10"
	imageapiv1 "github.com/openshift/api/image/v1"
	operatorv1alpha1 "github.com/openshift/client-go/operator/clientset/versioned/typed/operator/v1alpha1"
	"github.com/openshift/library-go/pkg/image/registryclient"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/cache"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/metrics"
	"github.com/openshift/image-registry/pkg/kubernetes-common/credentialprovider"
	imageapi "github.com/openshift/image-registry/pkg/origin-common/image/apis/image"
	"github.com/openshift/image-registry/pkg/requesttrace"
)

var (
	installCredentialsDir = "/var/lib/kubelet/"
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
func getImportContext(ctx context.Context, ref *imageapi.DockerImageReference, secrets []corev1.Secret, m metrics.Pullthrough, icsp operatorv1alpha1.ImageContentSourcePolicyInterface) (registryclient.RepositoryRetriever, error) {
	req, err := dcontext.GetRequest(ctx)
	if err != nil {
		dcontext.GetLogger(ctx).Errorf("unable to get request from context: %v", err)
		return nil, err
	}

	installKeyring := &credentialprovider.BasicDockerKeyring{}
	if config, err := credentialprovider.ReadDockerConfigJSONFile(
		[]string{installCredentialsDir},
	); err != nil {
		dcontext.GetLogger(ctx).Infof("proceeding without installation credentials: %v", err)
	} else {
		installKeyring.Add(config)
	}

	keyring, err := credentialprovider.MakeDockerKeyring(secrets, installKeyring)
	if err != nil {
		dcontext.GetLogger(ctx).Errorf("error creating keyring: %v", err)
		return nil, err
	}

	var cred auth.CredentialStore = registryclient.NoCredentials
	if auths, _ := keyring.Lookup(ref.String()); len(auths) > 0 {
		cred = dockerregistry.NewStaticCredentialStore(&auths[0].AuthConfig)
	}

	var retriever registryclient.RepositoryRetriever
	retriever = registryclient.NewContext(
		secureTransport, insecureTransport,
	).WithRequestModifiers(
		requesttrace.New(ctx, req),
	).WithAlternateBlobSourceStrategy(
		NewSimpleLookupICSPStrategy(icsp),
	).WithCredentials(cred)

	retriever = m.RepositoryRetriever(retriever)
	return retriever, nil
}

// RememberLayersOfImage caches the layer digests of given image.
func RememberLayersOfImage(ctx context.Context, cache cache.RepositoryDigest, image *imageapiv1.Image, cacheName string) {
	for _, layer := range image.DockerImageLayers {
		_ = cache.AddDigest(digest.Digest(layer.Name), cacheName)
	}
	meta, ok := image.DockerImageMetadata.Object.(*dockerapiv10.DockerImage)
	if !ok {
		dcontext.GetLogger(ctx).Errorf("image %s does not have metadata", image.Name)
		return
	}
	// remember reference to manifest config as well for schema 2
	if image.DockerImageManifestMediaType == schema2.MediaTypeManifest && len(meta.ID) > 0 {
		_ = cache.AddDigest(digest.Digest(meta.ID), cacheName)
	}
}

// RememberLayersOfImageStream caches the layer digests of given image stream.
func RememberLayersOfImageStream(ctx context.Context, cache cache.RepositoryDigest, layers *imageapiv1.ImageStreamLayers, cacheName string) {
	for dgst := range layers.Blobs {
		_ = cache.AddDigest(digest.Digest(dgst), cacheName)
	}
	for dgst := range layers.Images {
		_ = cache.AddDigest(digest.Digest(dgst), cacheName)
	}
}
