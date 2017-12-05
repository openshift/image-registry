package server

import (
	"fmt"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/reference"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/wrapped"
)

// newPendingErrorsWrapper ensures auth completed and there were no errors relevant to the repo.
func newPendingErrorsWrapper(repo *repository) wrapped.Wrapper {
	return func(ctx context.Context, funcname string, f func(ctx context.Context) error) error {
		if err := repo.checkPendingErrors(ctx); err != nil {
			return err
		}
		return f(ctx)
	}
}

func newPendingErrorsBlobStore(bs distribution.BlobStore, repo *repository) distribution.BlobStore {
	return wrapped.NewBlobStore(bs, newPendingErrorsWrapper(repo))
}

func newPendingErrorsManifestService(ms distribution.ManifestService, repo *repository) distribution.ManifestService {
	return wrapped.NewManifestService(ms, newPendingErrorsWrapper(repo))
}

func newPendingErrorsTagService(ts distribution.TagService, repo *repository) distribution.TagService {
	return wrapped.NewTagService(ts, newPendingErrorsWrapper(repo))
}

type errorBlobStore struct {
	distribution.BlobStore
	repo *repository
}

// FIXME(dmage): do we need this? Add a test case or remove this code.
func (r *errorBlobStore) Create(ctx context.Context, options ...distribution.BlobCreateOption) (distribution.BlobWriter, error) {
	opts, err := effectiveCreateOptions(options)
	if err != nil {
		return nil, err
	}
	err = checkPendingCrossMountErrors(ctx, opts)

	if err != nil {
		context.GetLogger(ctx).Infof("disabling cross-repo mount because of an error: %v", err)
		options = append(options, guardCreateOptions{DisableCrossMount: true})
	} else if !opts.Mount.ShouldMount {
		options = append(options, guardCreateOptions{DisableCrossMount: true})
	} else {
		context.GetLogger(ctx).Debugf("attempting cross-repo mount")
		options = append(options, statCrossMountCreateOptions{
			ctx:      ctx,
			destRepo: r.repo,
		})
	}

	return r.BlobStore.Create(ctx, options...)
}

// checkPendingCrossMountErrors returns true if a cross-repo mount has been requested with given create
// options. If requested and there are pending authorization errors for source repository, the error will be
// returned. Cross-repo mount must not be allowed in case of error.
func checkPendingCrossMountErrors(ctx context.Context, opts *distribution.CreateOptions) error {
	if !opts.Mount.ShouldMount {
		return nil
	}
	namespace, name, err := getNamespaceName(opts.Mount.From.Name())
	if err != nil {
		return err
	}
	return checkPendingErrors(ctx, context.GetLogger(ctx), namespace, name)
}

// guardCreateOptions ensures the expected options type is passed, and optionally disables cross mounting
type guardCreateOptions struct {
	DisableCrossMount bool
}

var _ distribution.BlobCreateOption = guardCreateOptions{}

func (f guardCreateOptions) Apply(v interface{}) error {
	opts, ok := v.(*distribution.CreateOptions)
	if !ok {
		return fmt.Errorf("Unexpected create options: %#v", v)
	}
	if f.DisableCrossMount {
		opts.Mount.ShouldMount = false
	}
	return nil
}

// statCrossMountCreateOptions ensures the expected options type is passed, and optionally pre-fills the cross-mount stat info
type statCrossMountCreateOptions struct {
	ctx      context.Context
	destRepo *repository
}

var _ distribution.BlobCreateOption = statCrossMountCreateOptions{}

func (f statCrossMountCreateOptions) Apply(v interface{}) error {
	opts, ok := v.(*distribution.CreateOptions)
	if !ok {
		return fmt.Errorf("Unexpected create options: %#v", v)
	}

	if !opts.Mount.ShouldMount {
		return nil
	}

	desc, err := statSourceRepository(f.ctx, f.destRepo, opts.Mount.From, opts.Mount.From.Digest())
	if err != nil {
		context.GetLogger(f.ctx).Infof("cannot mount blob %s from repository %s: %v - disabling cross-repo mount",
			opts.Mount.From.Digest().String(),
			opts.Mount.From.Name(),
			err)
		opts.Mount.ShouldMount = false
		return nil
	}

	opts.Mount.Stat = &desc

	return nil
}

func statSourceRepository(
	ctx context.Context,
	destRepo *repository,
	sourceRepoName reference.Named,
	dgst digest.Digest,
) (desc distribution.Descriptor, err error) {
	upstreamRepo, err := destRepo.app.registry.Repository(ctx, sourceRepoName)
	if err != nil {
		return distribution.Descriptor{}, err
	}
	namespace, name, err := getNamespaceName(sourceRepoName.Name())
	if err != nil {
		return distribution.Descriptor{}, err
	}

	repo := *destRepo
	repo.namespace = namespace
	repo.name = name
	repo.Repository = upstreamRepo

	return repo.Blobs(ctx).Stat(ctx, dgst)
}
