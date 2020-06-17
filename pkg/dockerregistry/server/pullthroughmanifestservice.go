package server

import (
	"context"
	"fmt"

	"github.com/docker/distribution"
	dcontext "github.com/docker/distribution/context"
	"github.com/opencontainers/go-digest"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/cache"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/metrics"
	"github.com/openshift/image-registry/pkg/errors"
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
	metrics                 metrics.Pullthrough
}

var _ distribution.ManifestService = &pullthroughManifestService{}

func (m *pullthroughManifestService) Get(ctx context.Context, dgst digest.Digest, options ...distribution.ManifestServiceOption) (distribution.Manifest, error) {
	dcontext.GetLogger(ctx).Debugf("(*pullthroughManifestService).Get: starting with dgst=%s", dgst.String())

	manifest, err := m.ManifestService.Get(ctx, dgst, options...)
	if _, ok := err.(distribution.ErrManifestUnknownRevision); ok {
		return m.remoteGet(ctx, dgst, options...)
	}

	return manifest, err
}

func (m *pullthroughManifestService) remoteGet(ctx context.Context, dgst digest.Digest, options ...distribution.ManifestServiceOption) (distribution.Manifest, error) {
	dcontext.GetLogger(ctx).Debugf("(*pullthroughManifestService).remoteGet: starting with dgst=%s", dgst.String())

	var firstPullthroughError error
	isQueue := []imagestream.ImageStream{m.imageStream}
	visited := map[string]bool{}
	for len(isQueue) > 0 {
		is := isQueue[0]
		isQueue = isQueue[1:]

		if visited[is.Reference()] {
			continue
		}
		visited[is.Reference()] = true

		repos, isrefs, err := is.RemoteRepositoriesForManifest(ctx, dgst)
		if err != nil {
			if err.Code() == imagestream.ErrImageStreamNotFoundCode {
				dcontext.GetLogger(ctx).Infof("Unable to get remote repositories for manifest %s (imagestream %s): %v", dgst.String(), m.imageStream.Reference(), err)
			} else {
				dcontext.GetLogger(ctx).Errorf("Unable to get remote repositories for manifest %s (imagestream %s): %v", dgst.String(), m.imageStream.Reference(), err)
			}
		}

		for _, repo := range repos {
			ref, err := imageapi.ParseDockerImageReference(repo.DockerImageReference)
			if err != nil {
				dcontext.GetLogger(ctx).Infof("Unable to parse image reference %q (imagestream %s): %v", repo.DockerImageReference, is.Reference(), err)
				continue
			}
			ref = ref.DockerClientDefaults()

			repo, err := m.getRemoteRepositoryClient(ctx, &ref, dgst, options...)
			if err != nil {
				if firstPullthroughError == nil {
					firstPullthroughError = errors.ErrorCodePullthroughManifest.WithArgs(ref.Exact(), err)
				}
				dcontext.GetLogger(ctx).Errorf("Error getting manifest %s from remote repository %s (imagestream %s): %v", dgst, ref.AsRepository().Exact(), is.Reference(), err)
				continue
			}

			pullthroughManifestService, err := repo.Manifests(ctx)
			if err != nil {
				return nil, err
			}

			manifest, err := pullthroughManifestService.Get(ctx, dgst)
			if err != nil {
				if firstPullthroughError == nil {
					firstPullthroughError = errors.ErrorCodePullthroughManifest.WithArgs(ref.Exact(), err)
				}
				dcontext.GetLogger(ctx).Errorf("Error getting manifest %s from remote repository %s (imagestream %s): %v", dgst, ref.AsRepository().Exact(), is.Reference(), err)
				continue
			}

			if m.mirror {
				if mirrorErr := m.mirrorManifest(ctx, manifest); mirrorErr != nil {
					errors.Handle(ctx, fmt.Sprintf("failed to mirror manifest from %s", ref.Exact()), mirrorErr)
				}
			}

			// FIXME(dmage): restore or remove?
			//RememberLayersOfImage(ctx, m.cache, image, ref.Exact())

			return manifest, err
		}

		for _, isref := range isrefs {
			isQueue = append(isQueue, is.Clone(isref.Namespace, isref.Name))
		}
	}

	if firstPullthroughError != nil {
		return nil, firstPullthroughError
	}

	return nil, distribution.ErrManifestUnknownRevision{
		Name:     m.imageStream.Reference(),
		Revision: dgst,
	}
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
	secrets, err := m.imageStream.GetSecrets()
	if err != nil {
		dcontext.GetLogger(ctx).Errorf("error getting secrets: %v", err)
	}

	retriever, impErr := getImportContext(ctx, ref, secrets, m.metrics)
	if impErr != nil {
		return nil, impErr
	}

	// determine, whether to fall-back to insecure transport based on a specification of image's tag
	// if the client pulls by tag, use that
	tag := ""
	for _, option := range options {
		if opt, ok := option.(distribution.WithTagOption); ok {
			tag = opt.Tag
			break
		}
	}

	insecure, err := m.imageStream.TagIsInsecure(ctx, tag, dgst)
	if err != nil {
		return nil, err
	}

	return retriever.Repository(ctx, ref.RegistryURL(), ref.RepositoryName(), insecure)
}
