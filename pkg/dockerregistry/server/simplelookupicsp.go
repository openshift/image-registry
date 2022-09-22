package server

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	cfgv1 "github.com/openshift/api/config/v1"
	operatorv1alpha1 "github.com/openshift/api/operator/v1alpha1"

	cfgv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	operatorv1alpha1client "github.com/openshift/client-go/operator/clientset/versioned/typed/operator/v1alpha1"
	reference "github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/library-go/pkg/image/registryclient"
)

// simpleLookupImageMirrorSets holds ImageContentSourcePolicy, ImageDigestMirrorSet, and ImageTagMirrorSet variables to look up image sources. Satisfies
// *Context AlternativeBlobSourceStrategy interface.
type simpleLookupImageMirrorSets struct {
	icspClient operatorv1alpha1client.ImageContentSourcePolicyInterface
	idmsClient cfgv1client.ImageDigestMirrorSetInterface
	itmsClient cfgv1client.ImageTagMirrorSetInterface
}

// NewSimpleLookupImageMirrorSetsStrategy returns a new entity of simpleLookupImageMirrorSets using provided client
// to obtain cluster wide ICSP or IDMS and ITMS configuration.
func NewSimpleLookupImageMirrorSetsStrategy(
	icspcli operatorv1alpha1client.ImageContentSourcePolicyInterface,
	idmscli cfgv1client.ImageDigestMirrorSetInterface,
	itmscli cfgv1client.ImageTagMirrorSetInterface,
) registryclient.AlternateBlobSourceStrategy {
	return &simpleLookupImageMirrorSets{
		icspClient: icspcli,
		idmsClient: idmscli,
		itmsClient: itmscli,
	}
}

// FirstRequest returns a list of sources to use when searching for a given repository. Returns
// the whole list of mirrors followed by the original image reference.
func (s *simpleLookupImageMirrorSets) FirstRequest(
	ctx context.Context, ref reference.DockerImageReference,
) ([]reference.DockerImageReference, error) {
	klog.V(5).Infof("reading ICSP from cluster")
	icspList, err := s.icspClient.List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.Errorf("unable to list ICSP config: %s", err)
		return []reference.DockerImageReference{ref.AsRepository()}, nil
	}

	idmsList, err := s.idmsClient.List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.Errorf("unable to list IDMS config: %s", err)
		return []reference.DockerImageReference{ref.AsRepository()}, nil
	}

	itmsList, err := s.itmsClient.List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.Errorf("unable to list ITMS config: %s", err)
		return []reference.DockerImageReference{ref.AsRepository()}, nil
	}

	if len(icspList.Items) > 0 && len(idmsList.Items) > 0 {
		err := fmt.Errorf("found both ICSP and IDMS resources, but only one or the other is supported")
		return []reference.DockerImageReference{ref.AsRepository()}, err
	}

	if len(icspList.Items) > 0 && len(itmsList.Items) > 0 {
		err := fmt.Errorf("found both ICSP and ITMS resources, but only one or the other is supported")
		return []reference.DockerImageReference{ref.AsRepository()}, err
	}

	imageRefList, err := s.alternativeImageSources(ref, icspList.Items, idmsList.Items, itmsList.Items)
	if err != nil {
		klog.Errorf("error looking for alternate repositories: %s", err)
		return []reference.DockerImageReference{ref.AsRepository()}, nil
	}

	imageRefList = append(imageRefList, ref.AsRepository())
	return imageRefList, nil
}

func (s *simpleLookupImageMirrorSets) OnFailure(
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

type mirrorSource struct {
	source  string
	mirrors []string
}

// alternativeImageSources returns unique list of DockerImageReference objects from list of
// ImageContentSourcePolicy or ImageDigestMirrorSet, ImageTagMirrorSet objects
func (s *simpleLookupImageMirrorSets) alternativeImageSources(
	ref reference.DockerImageReference, icspList []operatorv1alpha1.ImageContentSourcePolicy,
	idmsList []cfgv1.ImageDigestMirrorSet, itmsList []cfgv1.ImageTagMirrorSet,
) ([]reference.DockerImageReference, error) {
	repo := ref.AsRepository().Exact()

	mirrorSources := []mirrorSource{}
	for _, icsp := range icspList {
		s := mirrorSource{}
		for _, rdm := range icsp.Spec.RepositoryDigestMirrors {
			s.source = rdm.Source
			s.mirrors = rdm.Mirrors
			mirrorSources = append(mirrorSources, s)
		}
	}
	for _, idms := range idmsList {
		s := mirrorSource{}
		for _, idm := range idms.Spec.ImageDigestMirrors {
			s.source = idm.Source
			for _, m := range idm.Mirrors {
				s.mirrors = append(s.mirrors, string(m))
			}
			mirrorSources = append(mirrorSources, s)
		}
	}

	for _, itms := range itmsList {
		s := mirrorSource{}
		for _, itm := range itms.Spec.ImageTagMirrors {
			s.source = itm.Source
			for _, m := range itm.Mirrors {
				s.mirrors = append(s.mirrors, string(m))
			}
			mirrorSources = append(mirrorSources, s)
		}
	}

	imageSources := []reference.DockerImageReference{}
	uniqueMirrors := map[reference.DockerImageReference]bool{}

	for _, ms := range mirrorSources {
		rdmRepo := ms.source

		if !isSubrepo(repo, rdmRepo) {
			continue
		}

		suffix := repo[len(rdmRepo):]

		for _, m := range ms.mirrors {
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

	klog.V(2).Infof("Found sources: %v for image: %v", imageSources, ref)
	return imageSources, nil
}
