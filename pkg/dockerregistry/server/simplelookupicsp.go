package server

import (
	"context"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	operatorv1alpha1 "github.com/openshift/api/operator/v1alpha1"
	operatorv1alpha1client "github.com/openshift/client-go/operator/clientset/versioned/typed/operator/v1alpha1"
	reference "github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/library-go/pkg/image/registryclient"
)

// simpleLookupICSP holds ImageContentSourcePolicy variables to look up image sources. Satisfies
// *Context AlternativeBlobSourceStrategy interface.
type simpleLookupICSP struct {
	icspClient operatorv1alpha1client.ImageContentSourcePolicyInterface
}

// NewSimpleLookupICSPStrategy returns a new entity of simpleLookupICSP using provided client
// to obtain cluster wide ICSP configuration.
func NewSimpleLookupICSPStrategy(
	cli operatorv1alpha1client.ImageContentSourcePolicyInterface,
) registryclient.AlternateBlobSourceStrategy {
	return &simpleLookupICSP{
		icspClient: cli,
	}
}

// FirstRequest returns a list of sources to use when searching for a given repository. Returns
// the whole list of mirrors followed by the original image reference.
func (s *simpleLookupICSP) FirstRequest(
	ctx context.Context, ref reference.DockerImageReference,
) ([]reference.DockerImageReference, error) {
	klog.V(5).Infof("reading ICSP from cluster")
	icspList, err := s.icspClient.List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.Errorf("unable to list ICSP config: %s", err)
		return []reference.DockerImageReference{ref.AsRepository()}, nil
	}

	imageRefList, err := s.alternativeImageSources(ref, icspList.Items)
	if err != nil {
		klog.Errorf("error looking for alternate repositories: %s", err)
		return []reference.DockerImageReference{ref.AsRepository()}, nil
	}

	imageRefList = append(imageRefList, ref.AsRepository())
	return imageRefList, nil
}

func (s *simpleLookupICSP) OnFailure(
	ctx context.Context, ref reference.DockerImageReference,
) ([]reference.DockerImageReference, error) {
	return nil, nil
}

func isSubrepo(repo, ancestor string) bool {
	if repo == ancestor {
		return true
	}
	if len(repo) > len(ancestor) {
		return strings.HasPrefix(repo, ancestor) && repo[len(ancestor)] == '/'
	}
	return false
}

// alternativeImageSources returns unique list of DockerImageReference objects from list of
// ImageContentSourcePolicy objects
func (s *simpleLookupICSP) alternativeImageSources(
	ref reference.DockerImageReference, icspList []operatorv1alpha1.ImageContentSourcePolicy,
) ([]reference.DockerImageReference, error) {
	repo := ref.AsRepository().Exact()

	imageSources := []reference.DockerImageReference{}
	uniqueMirrors := map[reference.DockerImageReference]bool{}
	for _, icsp := range icspList {
		for _, rdm := range icsp.Spec.RepositoryDigestMirrors {
			rdmSourceRef, err := reference.Parse(rdm.Source)
			if err != nil {
				return nil, err
			}

			rdmRepo := rdmSourceRef.AsRepository().Exact()
			if !isSubrepo(repo, rdmRepo) {
				continue
			}

			suffix := repo[len(rdmRepo):]

			for _, m := range rdm.Mirrors {
				mRef, err := reference.Parse(m + suffix)
				if err != nil {
					return nil, err
				}

				if _, ok := uniqueMirrors[mRef]; ok {
					continue
				}

				imageSources = append(imageSources, mRef)
				uniqueMirrors[mRef] = true
			}
		}
	}

	klog.V(2).Infof("Found sources: %v for image: %v", imageSources, ref)
	return imageSources, nil
}
