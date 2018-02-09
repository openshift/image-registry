package server

import (
	"github.com/docker/distribution"
	"github.com/docker/distribution/context"

	"github.com/openshift/image-registry/pkg/imagestream"
)

type tagService struct {
	distribution.TagService

	imageStream        imagestream.ImageStream
	pullthroughEnabled bool
}

func (t tagService) Get(ctx context.Context, tag string) (distribution.Descriptor, error) {
	ok, err := t.imageStream.Exists()
	if err != nil {
		return distribution.Descriptor{}, err
	}
	if !ok {
		return distribution.Descriptor{}, distribution.ErrRepositoryUnknown{Name: t.imageStream.Reference()}
	}

	tags, err := t.imageStream.Tags(ctx)
	if err != nil {
		return distribution.Descriptor{}, err
	}

	dgst, ok := tags[tag]
	if !ok {
		return distribution.Descriptor{}, distribution.ErrTagUnknown{Tag: tag}
	}

	if !t.pullthroughEnabled {
		image, err := t.imageStream.GetImageOfImageStream(ctx, dgst)
		if err != nil {
			return distribution.Descriptor{}, err
		}

		if !imagestream.IsImageManaged(image) {
			return distribution.Descriptor{}, distribution.ErrTagUnknown{Tag: tag}
		}
	}

	return distribution.Descriptor{Digest: dgst}, nil
}

func (t tagService) All(ctx context.Context) ([]string, error) {
	ok, err := t.imageStream.Exists()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, distribution.ErrRepositoryUnknown{Name: t.imageStream.Reference()}
	}

	tags, err := t.imageStream.Tags(ctx)
	if err != nil {
		return nil, err
	}

	tagList := []string{}
	managedImages := make(map[string]bool)
	for tag, dgst := range tags {
		if t.pullthroughEnabled {
			tagList = append(tagList, tag)
			continue
		}

		managed, found := managedImages[dgst.String()]
		if !found {
			image, err := t.imageStream.GetImageOfImageStream(ctx, dgst)
			if err != nil {
				context.GetLogger(ctx).Errorf("unable to get image %s %s: %v", t.imageStream.Reference(), dgst.String(), err)
				continue
			}
			managed = imagestream.IsImageManaged(image)
			managedImages[dgst.String()] = managed
		}

		if !managed {
			continue
		}

		tagList = append(tagList, tag)
	}
	return tagList, nil
}

func (t tagService) Lookup(ctx context.Context, desc distribution.Descriptor) ([]string, error) {
	ok, err := t.imageStream.Exists()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, distribution.ErrRepositoryUnknown{Name: t.imageStream.Reference()}
	}

	tags, err := t.imageStream.Tags(ctx)
	if err != nil {
		return nil, err
	}

	tagList := []string{}
	managedImages := make(map[string]bool)
	for tag, dgst := range tags {
		if dgst != desc.Digest {
			continue
		}

		if t.pullthroughEnabled {
			tagList = append(tagList, tag)
			continue
		}

		managed, found := managedImages[dgst.String()]
		if !found {
			image, err := t.imageStream.GetImageOfImageStream(ctx, dgst)
			if err != nil {
				context.GetLogger(ctx).Errorf("unable to get image %s %s: %v", t.imageStream.Reference(), dgst.String(), err)
				continue
			}
			managed = imagestream.IsImageManaged(image)
			managedImages[dgst.String()] = managed
		}

		if !managed {
			continue
		}

		tagList = append(tagList, tag)
	}
	return tagList, nil
}

func (t tagService) Tag(ctx context.Context, tag string, desc distribution.Descriptor) error {
	ok, err := t.imageStream.Exists()
	if err != nil {
		return err
	}
	if !ok {
		return distribution.ErrRepositoryUnknown{Name: t.imageStream.Reference()}
	}

	return t.imageStream.Tag(ctx, tag, desc.Digest, t.pullthroughEnabled)
}

func (t tagService) Untag(ctx context.Context, tag string) error {
	ok, err := t.imageStream.Exists()
	if err != nil {
		return err
	}
	if !ok {
		return distribution.ErrRepositoryUnknown{Name: t.imageStream.Reference()}
	}

	return t.imageStream.Untag(ctx, tag, t.pullthroughEnabled)
}
