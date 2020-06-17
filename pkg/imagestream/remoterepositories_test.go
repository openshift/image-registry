package imagestream

import (
	"context"
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	imageapiv1 "github.com/openshift/api/image/v1"

	imageapi "github.com/openshift/image-registry/pkg/origin-common/image/apis/image"
	"github.com/openshift/image-registry/pkg/testutil"
)

func TestRemoteRepositoriesForImages(t *testing.T) {
	ctx := context.Background()
	ctx = testutil.WithTestLogger(ctx, t)

	const shaE3 = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	jun1, err := time.Parse("2006-01-02", "2020-06-01")
	if err != nil {
		t.Fatal(err)
	}

	jun2, err := time.Parse("2006-01-02", "2020-06-02")
	if err != nil {
		t.Fatal(err)
	}

	jun3, err := time.Parse("2006-01-02", "2020-06-03")
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name           string
		is             *imageapiv1.ImageStream
		images         []string
		expectedRepos  []RemoteRepository
		expectedISRefs []ImageStreamReference
	}{
		{
			name:           "empty image stream",
			is:             &imageapiv1.ImageStream{},
			expectedRepos:  nil,
			expectedISRefs: nil,
		},

		{
			name: "secure image stream with one image",
			is: &imageapiv1.ImageStream{
				Status: imageapiv1.ImageStreamStatus{
					DockerImageRepository: "localhost:5000/testns/testis",
					Tags: []imageapiv1.NamedTagEventList{
						{
							Tag: "tag",
							Items: []imageapiv1.TagEvent{
								{
									Image:                shaE3,
									DockerImageReference: "docker.io/busybox@" + shaE3,
								},
							},
						},
					},
				},
			},
			images: []string{shaE3},
			expectedRepos: []RemoteRepository{
				{DockerImageReference: "docker.io/busybox", Insecure: false},
			},
			expectedISRefs: nil,
		},

		{
			name: "insecure image stream",
			is: &imageapiv1.ImageStream{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{imageapi.InsecureRepositoryAnnotation: "true"},
				},
				Spec: imageapiv1.ImageStreamSpec{
					Tags: []imageapiv1.TagReference{
						{
							Name:         "insecure",
							ImportPolicy: imageapiv1.TagImportPolicy{Insecure: false},
						},
					},
				},
				Status: imageapiv1.ImageStreamStatus{
					DockerImageRepository: "localhost:5000/testns/testis",
					Tags: []imageapiv1.NamedTagEventList{
						{
							Tag: "secure",
							Items: []imageapiv1.TagEvent{
								{
									Created:              metav1.Time{Time: jun2},
									Image:                shaE3,
									DockerImageReference: "example.org/user/app@" + shaE3,
								},
								{
									Created:              metav1.Time{Time: jun1},
									Image:                shaE3,
									DockerImageReference: "example.org/app@" + shaE3,
								},
							},
						},
						{
							Tag: "insecure",
							Items: []imageapiv1.TagEvent{
								{
									Created:              metav1.Time{Time: jun3},
									Image:                shaE3,
									DockerImageReference: "registry.example.org/user/app@" + shaE3,
								},
							},
						},
					},
				},
			},
			images: []string{shaE3},
			expectedRepos: []RemoteRepository{
				{DockerImageReference: "registry.example.org/user/app", Insecure: true},
				{DockerImageReference: "example.org/user/app", Insecure: true},
				{DockerImageReference: "example.org/app", Insecure: true},
			},
			expectedISRefs: nil,
		},

		{
			name: "secure image stream with one insecure image",
			is: &imageapiv1.ImageStream{
				Spec: imageapiv1.ImageStreamSpec{
					Tags: []imageapiv1.TagReference{
						{
							Name:         "insecure",
							ImportPolicy: imageapiv1.TagImportPolicy{Insecure: true},
						},
					},
				},
				Status: imageapiv1.ImageStreamStatus{
					DockerImageRepository: "localhost:5000/testns/testis",
					Tags: []imageapiv1.NamedTagEventList{
						{
							Tag: "secure",
							Items: []imageapiv1.TagEvent{
								{
									Image:                shaE3,
									DockerImageReference: "example.org/user/app@" + shaE3,
								},
								{
									Image:                shaE3,
									DockerImageReference: "secure.example.org/user/app@" + shaE3,
								},
							},
						},
						{
							Tag: "insecure",
							Items: []imageapiv1.TagEvent{
								{
									Image:                shaE3,
									DockerImageReference: "registry.example.org/user/app@" + shaE3,
								},
								{
									Image:                shaE3,
									DockerImageReference: "other.example.org/user/app@" + shaE3,
								},
							},
						},
					},
				},
			},
			images: []string{shaE3},
			expectedRepos: []RemoteRepository{
				{DockerImageReference: "example.org/user/app", Insecure: false},
				{DockerImageReference: "registry.example.org/user/app", Insecure: true},
				{DockerImageReference: "secure.example.org/user/app", Insecure: false},
				{DockerImageReference: "other.example.org/user/app", Insecure: true},
			},
			expectedISRefs: nil,
		},

		{
			name: "insecure flag propagates to the whole registry",
			is: &imageapiv1.ImageStream{
				Spec: imageapiv1.ImageStreamSpec{
					Tags: []imageapiv1.TagReference{
						{
							Name:         "insecure",
							ImportPolicy: imageapiv1.TagImportPolicy{Insecure: true},
						},
					},
				},
				Status: imageapiv1.ImageStreamStatus{
					DockerImageRepository: "localhost:5000/testns/testis",
					Tags: []imageapiv1.NamedTagEventList{
						{
							Tag: "secure",
							Items: []imageapiv1.TagEvent{
								{
									Created:              metav1.Time{Time: jun1},
									Image:                shaE3,
									DockerImageReference: "a.b/c",
								},
							},
						},
						{
							Tag: "insecure",
							Items: []imageapiv1.TagEvent{
								{
									Created:              metav1.Time{Time: jun2},
									Image:                shaE3,
									DockerImageReference: "a.b/app@" + shaE3,
								},
							},
						},
						{
							Tag: "foo",
							Items: []imageapiv1.TagEvent{
								{
									Created:              metav1.Time{Time: jun3},
									Image:                shaE3,
									DockerImageReference: "a.b/c/foo@" + shaE3,
								},
							},
						},
						{
							Tag: "bar",
							Items: []imageapiv1.TagEvent{
								{
									Created:              metav1.Time{Time: jun1},
									Image:                shaE3,
									DockerImageReference: "other.b/bar@" + shaE3,
								},
							},
						},
						{
							Tag: "gas",
							Items: []imageapiv1.TagEvent{
								{
									Created:              metav1.Time{Time: jun2},
									Image:                shaE3,
									DockerImageReference: "a.a/app@" + shaE3,
								},
							},
						},
					},
				},
			},
			images: []string{shaE3},
			expectedRepos: []RemoteRepository{
				{DockerImageReference: "a.a/app", Insecure: false},
				{DockerImageReference: "other.b/bar", Insecure: false},
				{DockerImageReference: "a.b/c/foo", Insecure: true},
				{DockerImageReference: "a.b/app", Insecure: true},
				{DockerImageReference: "a.b/c", Insecure: true},
			},
			expectedISRefs: nil,
		},

		{
			name: "duplicate entries",
			is: &imageapiv1.ImageStream{
				Spec: imageapiv1.ImageStreamSpec{
					Tags: []imageapiv1.TagReference{
						{
							Name: "insecure", ImportPolicy: imageapiv1.TagImportPolicy{Insecure: true},
						},
					},
				},
				Status: imageapiv1.ImageStreamStatus{
					DockerImageRepository: "localhost:5000/testns/testis",
					Tags: []imageapiv1.NamedTagEventList{
						{
							Tag: "secure",
							Items: []imageapiv1.TagEvent{
								{
									Created:              metav1.Time{Time: jun1},
									Image:                shaE3,
									DockerImageReference: "a.b/foo@" + shaE3,
								},
							},
						},
						{
							Tag: "insecure",
							Items: []imageapiv1.TagEvent{
								{
									Created:              metav1.Time{Time: jun1},
									Image:                shaE3,
									DockerImageReference: "a.b/app@" + shaE3,
								},
							},
						},
						{
							Tag: "foo",
							Items: []imageapiv1.TagEvent{
								{
									Created:              metav1.Time{Time: jun2},
									Image:                shaE3,
									DockerImageReference: "a.b/app@" + shaE3,
								},
							},
						},
						{
							Tag: "bar",
							Items: []imageapiv1.TagEvent{
								{
									Created:              metav1.Time{Time: jun1},
									Image:                shaE3,
									DockerImageReference: "a.b.c/app@" + shaE3,
								},
							},
						},
						{
							Tag: "gas",
							Items: []imageapiv1.TagEvent{
								{
									Created:              metav1.Time{Time: jun3},
									Image:                shaE3,
									DockerImageReference: "a.b.c/app@" + shaE3,
								},
							},
						},
					},
				},
			},
			images: []string{shaE3},
			expectedRepos: []RemoteRepository{
				{DockerImageReference: "a.b.c/app", Insecure: false},
				{DockerImageReference: "a.b/app", Insecure: true},
				{DockerImageReference: "a.b/foo", Insecure: true},
			},
			expectedISRefs: nil,
		},

		{
			name: "imported from other image streams",
			is: &imageapiv1.ImageStream{
				Status: imageapiv1.ImageStreamStatus{
					DockerImageRepository: "localhost:5000/testns/testis",
					Tags: []imageapiv1.NamedTagEventList{
						{
							Tag: "tag",
							Items: []imageapiv1.TagEvent{
								{
									Created:              metav1.Time{Time: jun2},
									Image:                shaE3,
									DockerImageReference: "docker.io/busybox@" + shaE3,
								},
								{
									Created:              metav1.Time{Time: jun1},
									Image:                shaE3,
									DockerImageReference: "localhost:5000/testns/anotheris@" + shaE3,
								},
							},
						},
						{
							Tag: "foo",
							Items: []imageapiv1.TagEvent{
								{
									Created:              metav1.Time{Time: jun2},
									Image:                shaE3,
									DockerImageReference: "localhost:5000/anotherns/foo@" + shaE3,
								},
							},
						},
						{
							Tag: "bar",
							Items: []imageapiv1.TagEvent{
								{
									Created:              metav1.Time{Time: jun3},
									Image:                shaE3,
									DockerImageReference: "localhost:5000/anotherns/bar1@" + shaE3,
								},
								{
									Created:              metav1.Time{Time: jun2},
									Image:                shaE3,
									DockerImageReference: "localhost:5000/anotherns/bar2@" + shaE3,
								},
							},
						},
					},
				},
			},
			images: []string{shaE3},
			expectedRepos: []RemoteRepository{
				{DockerImageReference: "docker.io/busybox", Insecure: false},
			},
			expectedISRefs: []ImageStreamReference{
				{Namespace: "anotherns", Name: "bar1"},
				{Namespace: "anotherns", Name: "foo"},
				{Namespace: "anotherns", Name: "bar2"},
				{Namespace: "testns", Name: "anotheris"},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			repos, isrefs := remoteRepositoriesForImages(ctx, tc.is, tc.images)
			if !reflect.DeepEqual(repos, tc.expectedRepos) {
				t.Errorf("repos: got %v, want %v", repos, tc.expectedRepos)
			}
			if !reflect.DeepEqual(isrefs, tc.expectedISRefs) {
				t.Errorf("isrefs: got %v, want %v", isrefs, tc.expectedISRefs)
			}
		})
	}
}
