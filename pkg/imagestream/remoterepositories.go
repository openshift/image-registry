package imagestream

import (
	"context"
	"sort"
	"time"

	dcontext "github.com/docker/distribution/context"

	imagev1 "github.com/openshift/api/image/v1"

	imageapi "github.com/openshift/image-registry/pkg/origin-common/image/apis/image"
)

// RemoteRepository contains information about a remote repository.
type RemoteRepository struct {
	DockerImageReference string
	Insecure             bool
}

// weight contains details about the repository from its image stream that may
// help determine repository preference.
type weight struct {
	Index   int
	Created time.Time
}

func (w weight) Less(other weight) bool {
	// Prefer latest sources
	if w.Index == 0 && other.Index != 0 {
		return true
	}
	if w.Index != 0 && other.Index == 0 {
		return false
	}

	// Prefer newer sources
	return w.Created.After(other.Created)
}

// remoteRepositoriesByPreference implements sort.Interface to sort
// repositories by preference. The first repository is the most preferred.
type remoteRepositoriesByPreference struct {
	RemoteRepositories []RemoteRepository
	Weights            []weight
}

func (s remoteRepositoriesByPreference) Len() int {
	return len(s.RemoteRepositories)
}

func (s remoteRepositoriesByPreference) Less(i, j int) bool {
	// Prefer latest sources
	if s.Weights[i].Index == 0 && s.Weights[j].Index != 0 {
		return true
	}
	if s.Weights[i].Index != 0 && s.Weights[j].Index == 0 {
		return false
	}

	// Prefer secure sources
	if !s.RemoteRepositories[i].Insecure && s.RemoteRepositories[j].Insecure {
		return true
	}
	if s.RemoteRepositories[i].Insecure && !s.RemoteRepositories[j].Insecure {
		return false
	}

	// Prefer newer sources
	return s.Weights[i].Created.After(s.Weights[j].Created)
}

func (s remoteRepositoriesByPreference) Swap(i, j int) {
	s.RemoteRepositories[i], s.RemoteRepositories[j] = s.RemoteRepositories[j], s.RemoteRepositories[i]
	s.Weights[i], s.Weights[j] = s.Weights[j], s.Weights[i]
}

// remoteRepositoriesForImages returns a list of repositories from which one of
// images was imported into imagestream. For the repositories that are hosted
// by the local registry, image stream references will be returned instead.
func remoteRepositoriesForImages(ctx context.Context, imagestream *imagev1.ImageStream, images []string) []RemoteRepository {
	if len(images) == 0 {
		return nil
	}

	imagesSet := map[string]bool{}
	for _, image := range images {
		imagesSet[image] = true
	}

	localRegistry := getLocalRegistryNames(ctx, imagestream)
	localRegistrySet := map[string]bool{}
	for _, registry := range localRegistry {
		localRegistrySet[registry] = true
	}

	insecureByDefault := false
	if insecure, ok := imagestream.Annotations[imageapi.InsecureRepositoryAnnotation]; ok {
		insecureByDefault = insecure == "true"
	}

	uniqueRepos := map[string]struct {
		weight
		registry string
	}{}
	insecureRegistries := map[string]bool{}
	for _, tag := range imagestream.Status.Tags {
		insecure := insecureByDefault
		for _, t := range imagestream.Spec.Tags {
			if t.Name == tag.Tag {
				insecure = insecureByDefault || t.ImportPolicy.Insecure
				break
			}
		}

		for idx, item := range tag.Items {
			if !imagesSet[item.Image] && !insecure {
				continue
			}

			ref, err := imageapi.ParseDockerImageReference(item.DockerImageReference)
			if err != nil {
				dcontext.GetLogger(ctx).Warnf(
					"remoteRepositoriesForImages: imagestram %s/%s: tag %s: unable to parse dockerImageReference %q: %v",
					imagestream.Namespace, imagestream.Name, tag.Tag, item.DockerImageReference, err,
				)
				continue
			}

			if imagesSet[item.Image] {
				repo := ref.AsRepository().Exact()
				w := weight{
					Index:   idx,
					Created: item.Created.Time,
				}
				if r, ok := uniqueRepos[repo]; !ok || w.Less(r.weight) {
					uniqueRepos[repo] = struct {
						weight
						registry string
					}{
						weight:   w,
						registry: ref.Registry,
					}
				}
			}

			if insecure {
				insecureRegistries[ref.Registry] = true
			}
		}
	}

	var repos []RemoteRepository
	var weights []weight
	for repo, r := range uniqueRepos {
		repos = append(repos, RemoteRepository{
			DockerImageReference: repo,
			Insecure:             insecureRegistries[r.registry],
		})
		weights = append(weights, r.weight)
	}

	sort.Sort(remoteRepositoriesByPreference{
		RemoteRepositories: repos,
		Weights:            weights,
	})

	return repos
}
