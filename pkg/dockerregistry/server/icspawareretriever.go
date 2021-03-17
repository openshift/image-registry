package server

/*
package server

import (
	"context"
	"net/url"

	"github.com/docker/distribution"

	imageapi "github.com/openshift/image-registry/pkg/origin-common/image/apis/image"
	imagereference "github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/library-go/pkg/image/registryclient"
)

// ICSPAwareRetriever wraps a registryclient.Context. It changes the behaviour of
// Context's Repository method in order to attempt first to find the repository in
// any of the configured mirrors (by inspecting ICSP objects).
type ICSPAwareRetriever struct {
	retriever *registryclient.Context
	ref       *imageapi.DockerImageReference
}

// NewICSPAwareRetrieverForReference returns an ICSPAwareRetriever configured for
// the provided DockerImageReference.
func NewICSPAwareRetrieverForReference(
	ref *imageapi.DockerImageReference,
	retriever *registryclient.Context,
) *ICSPAwareRetriever {
	return &ICSPAwareRetriever{
		retriever: retriever,
		ref:       ref,
	}
}

// Repository attempts to return a distribution.Repository by first trying the configured
// mirrors (ICSP), if not able to do so then calls the original Repository() function on
// embed registryclient.Context.
func (i *ICSPAwareRetriever) Repository(
	ctx context.Context, registry *url.URL, repoName string, insecure bool,
) (distribution.Repository, error) {
	nref := imagereference.DockerImageReference{
		Registry:  i.ref.Registry,
		Namespace: i.ref.Namespace,
		Name:      i.ref.Name,
		Tag:       i.ref.Tag,
		ID:        i.ref.ID,
	}
	repo, _, err := i.retriever.RepositoryWithAlternateReference(ctx, nref, insecure)
	if err == nil {
		return repo, nil
	}
	return i.retriever.Repository(ctx, registry, repoName, insecure)
}
*/
