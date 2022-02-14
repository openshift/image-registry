package server

import (
	"context"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openshift/api/operator/v1alpha1"
	"github.com/openshift/client-go/operator/clientset/versioned/fake"
	reference "github.com/openshift/library-go/pkg/image/reference"
)

func TestFirstRequest(t *testing.T) {
	for _, tt := range []struct {
		name  string
		rules []runtime.Object
		ref   string
		res   []reference.DockerImageReference
	}{
		{
			name: "multiple mirrors",
			ref:  "i.do.not.exist/repo/image:latest",
			res: []reference.DockerImageReference{
				{
					Registry:  "i.exist",
					Namespace: "ns0",
					Name:      "img0",
				},
				{
					Registry:  "i.also.exist",
					Namespace: "ns1",
					Name:      "img1",
				},
				{
					Registry:  "me.too",
					Namespace: "ns2",
					Name:      "img2",
				},
				{
					Registry:  "i.do.not.exist",
					Namespace: "repo",
					Name:      "image",
				},
			},
			rules: []runtime.Object{
				&v1alpha1.ImageContentSourcePolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: "icsp-rule",
					},
					Spec: v1alpha1.ImageContentSourcePolicySpec{
						RepositoryDigestMirrors: []v1alpha1.RepositoryDigestMirrors{
							{
								Source: "i.do.not.exist/repo/image",
								Mirrors: []string{
									"i.exist/ns0/img0",
									"i.also.exist/ns1/img1",
									"me.too/ns2/img2",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "happy path",
			ref:  "i.do.not.exist/repo/image:latest",
			res: []reference.DockerImageReference{
				{
					Registry:  "i.exist",
					Namespace: "namespace",
					Name:      "img",
				},
				{
					Registry:  "i.do.not.exist",
					Namespace: "repo",
					Name:      "image",
				},
			},
			rules: []runtime.Object{
				&v1alpha1.ImageContentSourcePolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: "icsp-rule",
					},
					Spec: v1alpha1.ImageContentSourcePolicySpec{
						RepositoryDigestMirrors: []v1alpha1.RepositoryDigestMirrors{
							{
								Source: "i.do.not.exist/repo/image",
								Mirrors: []string{
									"i.exist/namespace/img",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "nested repository",
			ref:  "docker.io/library/busybox:latest",
			res: []reference.DockerImageReference{
				{
					Registry:  "mirror.example.com",
					Namespace: "dockerio-library",
					Name:      "busybox",
				},
				{
					Registry:  "docker.io",
					Namespace: "library",
					Name:      "busybox",
				},
			},
			rules: []runtime.Object{
				&v1alpha1.ImageContentSourcePolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: "icsp-rule",
					},
					Spec: v1alpha1.ImageContentSourcePolicySpec{
						RepositoryDigestMirrors: []v1alpha1.RepositoryDigestMirrors{
							{
								Source: "docker.io/library",
								Mirrors: []string{
									"mirror.example.com/dockerio-library",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "multiple unrelated rules",
			ref:  "i.do.not.exist/repo/image:latest",
			res: []reference.DockerImageReference{
				{
					Registry:  "i.do.not.exist",
					Namespace: "repo",
					Name:      "image",
				},
			},
			rules: []runtime.Object{
				&v1alpha1.ImageContentSourcePolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: "icsp-rule",
					},
					Spec: v1alpha1.ImageContentSourcePolicySpec{
						RepositoryDigestMirrors: []v1alpha1.RepositoryDigestMirrors{
							{
								Source: "unrelated.io/repo/image",
								Mirrors: []string{
									"i.exist/namespace/img",
								},
							},
							{
								Source: "also.unrelated.io/repo/image",
								Mirrors: []string{
									"i.exist/namespace/img",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "multiple related rules",
			ref:  "i.do.not.exist/repo/image:latest",
			res: []reference.DockerImageReference{
				{
					Registry:  "i.exist",
					Namespace: "namespace",
					Name:      "image",
				},
				{
					Registry:  "i.also.exist",
					Namespace: "ns",
					Name:      "img",
				},
				{
					Registry:  "i.do.not.exist",
					Namespace: "repo",
					Name:      "image",
				},
			},
			rules: []runtime.Object{
				&v1alpha1.ImageContentSourcePolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: "icsp-rule-0",
					},
					Spec: v1alpha1.ImageContentSourcePolicySpec{
						RepositoryDigestMirrors: []v1alpha1.RepositoryDigestMirrors{
							{
								Source: "i.do.not.exist/repo/image",
								Mirrors: []string{
									"i.exist/namespace/image",
								},
							},
						},
					},
				},
				&v1alpha1.ImageContentSourcePolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: "icsp-rule-1",
					},
					Spec: v1alpha1.ImageContentSourcePolicySpec{
						RepositoryDigestMirrors: []v1alpha1.RepositoryDigestMirrors{
							{
								Source: "i.do.not.exist/repo/image",
								Mirrors: []string{
									"i.also.exist/ns/img",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "dedup rules",
			ref:  "i.do.not.exist/repo/image:latest",
			res: []reference.DockerImageReference{
				{
					Registry:  "i.exist",
					Namespace: "namespace",
					Name:      "image",
				},
				{
					Registry:  "i.do.not.exist",
					Namespace: "repo",
					Name:      "image",
				},
			},
			rules: []runtime.Object{
				&v1alpha1.ImageContentSourcePolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: "icsp-rule-0",
					},
					Spec: v1alpha1.ImageContentSourcePolicySpec{
						RepositoryDigestMirrors: []v1alpha1.RepositoryDigestMirrors{
							{
								Source: "i.do.not.exist/repo/image",
								Mirrors: []string{
									"i.exist/namespace/image",
								},
							},
						},
					},
				},
				&v1alpha1.ImageContentSourcePolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: "icsp-rule-1",
					},
					Spec: v1alpha1.ImageContentSourcePolicySpec{
						RepositoryDigestMirrors: []v1alpha1.RepositoryDigestMirrors{
							{
								Source: "i.do.not.exist/repo/image",
								Mirrors: []string{
									"i.exist/namespace/image",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "invalid mirror source reference",
			ref:  "i.do.not.exist/repo/image:latest",
			res: []reference.DockerImageReference{
				{
					Registry:  "i.do.not.exist",
					Namespace: "repo",
					Name:      "image",
				},
			},
			rules: []runtime.Object{
				&v1alpha1.ImageContentSourcePolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: "icsp-rule",
					},
					Spec: v1alpha1.ImageContentSourcePolicySpec{
						RepositoryDigestMirrors: []v1alpha1.RepositoryDigestMirrors{
							{
								Source: "-92</asdf",
								Mirrors: []string{
									"i.exist/namespace/img",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "invalid mirror source reference",
			ref:  "i.do.not.exist/repo/image:latest",
			res: []reference.DockerImageReference{
				{
					Registry:  "i.do.not.exist",
					Namespace: "repo",
					Name:      "image",
				},
			},
			rules: []runtime.Object{
				&v1alpha1.ImageContentSourcePolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: "icsp-rule",
					},
					Spec: v1alpha1.ImageContentSourcePolicySpec{
						RepositoryDigestMirrors: []v1alpha1.RepositoryDigestMirrors{
							{
								Source: "i.do.not.exist/repo/image",
								Mirrors: []string{
									"-92</asfg",
								},
							},
						},
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cli := fake.NewSimpleClientset(tt.rules...)
			lookup := NewSimpleLookupICSPStrategy(
				cli.OperatorV1alpha1().ImageContentSourcePolicies(),
			)

			ref, err := reference.Parse(tt.ref)
			if err != nil {
				t.Fatalf("unexpected error parsing reference: %s", err)
			}

			alternates, err := lookup.FirstRequest(context.Background(), ref)
			if err != nil {
				t.Fatalf("FirstRequest does not return error, received: %s", err)
			}

			if !reflect.DeepEqual(alternates, tt.res) {
				t.Errorf("expected %+v, received %+v", tt.res, alternates)
			}
		})
	}
}
