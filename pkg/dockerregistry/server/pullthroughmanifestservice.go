package server

import (
	"fmt"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/cache"
	"github.com/openshift/image-registry/pkg/imagestream"
	imageapi "github.com/openshift/image-registry/pkg/origin-common/image/apis/image"
)

// pullthroughManifestService wraps a distribution.ManifestService
// repositories. Since the manifest is no longer stored in the Image
// the docker-registry must pull through requests to manifests as well
// as to blobs.
type pullthroughManifestService struct {
	distribution.ManifestService
	newLocalManifestService func(ctx context.Context) (distribution.ManifestService, error)
	imageStream             imagestream.ImageStream
	cache                   cache.RepositoryDigest
	mirror                  bool
	registryAddr            string
}

var _ distribution.ManifestService = &pullthroughManifestService{}

func (m *pullthroughManifestService) Get(ctx context.Context, dgst digest.Digest, options ...distribution.ManifestServiceOption) (distribution.Manifest, error) {
	context.GetLogger(ctx).Debugf("(*pullthroughManifestService).Get: starting with dgst=%s", dgst.String())
	manifest, err := m.ManifestService.Get(ctx, dgst, options...)
	switch err.(type) {
	case distribution.ErrManifestUnknownRevision:
		break
	case nil:
		return manifest, nil
	default:
		return nil, err
	}

	return m.remoteGet(ctx, dgst, options...)
}

func (m *pullthroughManifestService) remoteGet(ctx context.Context, dgst digest.Digest, options ...distribution.ManifestServiceOption) (distribution.Manifest, error) {
	context.GetLogger(ctx).Debugf("(*pullthroughManifestService).remoteGet: starting with dgst=%s", dgst.String())
	image, err := m.imageStream.GetImageOfImageStream(ctx, dgst)
	if err != nil {
		return nil, err
	}

	ref, err := imageapi.ParseDockerImageReference(image.DockerImageReference)
	if err != nil {
		context.GetLogger(ctx).Errorf("bad DockerImageReference (%q) in Image %s@%s: %v", image.DockerImageReference, m.imageStream.Reference(), dgst.String(), err)
		return nil, err
	}
	ref = ref.DockerClientDefaults()

	// don't attempt to pullthrough from ourself
	if ref.Registry == m.registryAddr {
		return nil, distribution.ErrManifestUnknownRevision{
			Name:     m.imageStream.Reference(),
			Revision: dgst,
		}
	}
	repo, err := m.getRemoteRepositoryClient(ctx, &ref, dgst, options...)
	if err != nil {
		context.GetLogger(ctx).Errorf("error getting remote repository for image %q: %v", ref.Exact(), err)
		return nil, err
	}

	pullthroughManifestService, err := repo.Manifests(ctx)
	if err != nil {
		context.GetLogger(ctx).Errorf("error getting remote manifests for image %q: %v", ref.Exact(), err)
		return nil, err
	}

	manifest, err := pullthroughManifestService.Get(ctx, dgst)
	switch err.(type) {
	case nil:
		if m.mirror {
			if mirrorErr := m.mirrorManifest(ctx, manifest); mirrorErr != nil {
				context.GetLogger(ctx).Errorf("failed to mirror manifest %s: %v", ref.Exact(), mirrorErr)
			}
		}
		RememberLayersOfImage(ctx, m.cache, image, ref.Exact())
	case distribution.ErrManifestUnknownRevision:
		break
	default:
		context.GetLogger(ctx).Errorf("error getting manifest from remote location %q: %v", ref.Exact(), err)
	}

	return manifest, err
}

func (m *pullthroughManifestService) mirrorManifest(ctx context.Context, manifest distribution.Manifest) error {
	localManifestService, err := m.newLocalManifestService(ctx)
	if err != nil {
		return fmt.Errorf("failed to create local manifest service: %v", err)
	}

	_, err = localManifestService.Put(ctx, manifest)
	return err
}

func (m *pullthroughManifestService) getRemoteRepositoryClient(ctx context.Context, ref *imageapi.DockerImageReference, dgst digest.Digest, options ...distribution.ManifestServiceOption) (distribution.Repository, error) {
	retriever := getImportContext(ctx, m.imageStream.GetSecrets)

	// determine, whether to fall-back to insecure transport based on a specification of image's tag
	// if the client pulls by tag, use that
	tag := ""
	for _, option := range options {
		if opt, ok := option.(distribution.WithTagOption); ok {
			tag = opt.Tag
			break
		}
	}

	insecure, err := m.imageStream.TagIsInsecure(tag, dgst)
	if err != nil {
		return nil, err
	}

	return retriever.Repository(ctx, ref.RegistryURL(), ref.RepositoryName(), insecure)
}

func (m *pullthroughManifestService) Put(ctx context.Context, manifest distribution.Manifest, options ...distribution.ManifestServiceOption) (digest.Digest, error) {
	context.GetLogger(ctx).Debugf("(*pullthroughManifestService).Put: enabling remote blob access check")
	// manifest dependencies (layers and config) may not be stored locally, we need to be able to stat them in remote repositories
	ctx = withRemoteBlobAccessCheckEnabled(ctx, true)
	return m.ManifestService.Put(ctx, manifest, options...)
}
