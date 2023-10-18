package server

import (
	"context"
	"reflect"
	"testing"

	v1 "github.com/openshift/api/config/v1"
	"github.com/openshift/api/operator/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	cfgfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	"github.com/openshift/client-go/operator/clientset/versioned/fake"
	reference "github.com/openshift/library-go/pkg/image/reference"
)

type rule struct {
	name        string
	ruleElement []element
}

type element struct {
	source  string
	mirrors []string
}

func TestFirstRequest(t *testing.T) {
	for _, tt := range []struct {
		name  string
		rules []rule
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
			rules: []rule{
				{
					name: "rule",
					ruleElement: []element{
						{source: "i.do.not.exist/repo/image",
							mirrors: []string{
								"i.exist/ns0/img0",
								"i.also.exist/ns1/img1",
								"me.too/ns2/img2",
							}},
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
			rules: []rule{
				{
					name: "rule",
					ruleElement: []element{
						{source: "i.do.not.exist/repo/image",
							mirrors: []string{
								"i.exist/namespace/img",
							}},
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
			rules: []rule{
				{
					name: "rule",
					ruleElement: []element{
						{source: "docker.io/library",
							mirrors: []string{
								"mirror.example.com/dockerio-library",
							}},
					},
				},
			},
		},
		{
			name: "mirror registry",
			ref:  "docker.io/library/busybox:latest",
			res: []reference.DockerImageReference{
				{
					Registry:  "mirror.example.com",
					Namespace: "library",
					Name:      "busybox",
				},
				{
					Registry:  "docker.io",
					Namespace: "library",
					Name:      "busybox",
				},
			},
			rules: []rule{
				{
					name: "rule",
					ruleElement: []element{
						{source: "docker.io",
							mirrors: []string{
								"mirror.example.com",
							}},
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
			rules: []rule{
				{
					name: "rule",
					ruleElement: []element{
						{source: "unrelated.io/repo/image",
							mirrors: []string{
								"i.exist/namespace/img",
							}},
						{source: "also.unrelated.io/repo/image", mirrors: []string{
							"i.exist/namespace/img",
						}},
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
			rules: []rule{
				{
					name: "rule-0",
					ruleElement: []element{
						{source: "i.do.not.exist/repo/image",
							mirrors: []string{
								"i.exist/namespace/image",
							}},
					},
				},
				{
					name: "rule-1",
					ruleElement: []element{
						{source: "i.do.not.exist/repo/image", mirrors: []string{
							"i.also.exist/ns/img",
						}},
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
			rules: []rule{
				{
					name: "rule-0",
					ruleElement: []element{
						{source: "i.do.not.exist/repo/image",
							mirrors: []string{
								"i.exist/namespace/image",
							}},
					},
				},
				{
					name: "rule-1",
					ruleElement: []element{
						{source: "i.do.not.exist/repo/image", mirrors: []string{
							"i.exist/namespace/image",
						}},
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
			rules: []rule{
				{
					name: "rule",
					ruleElement: []element{
						{source: "-92</asdf",
							mirrors: []string{
								"i.exist/namespace/img",
							}},
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
			rules: []rule{
				{
					name: "rule",
					ruleElement: []element{
						{source: "i.do.not.exist/repo/image",
							mirrors: []string{
								"-92</asfg",
							}},
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			icspRules := []runtime.Object{}
			for _, rule := range tt.rules {
				icspRules = append(icspRules, newICSPRule(rule))
			}
			cli := fake.NewSimpleClientset(icspRules...)
			cfgcli := cfgfake.NewSimpleClientset()
			lookup := NewSimpleLookupImageMirrorSetsStrategy(
				cli.OperatorV1alpha1().ImageContentSourcePolicies(),
				cfgcli.ConfigV1().ImageDigestMirrorSets(),
				cfgcli.ConfigV1().ImageTagMirrorSets(),
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

		t.Run(tt.name, func(t *testing.T) {
			tt.name = "idms-test-" + tt.name
			idmsRules := []runtime.Object{}
			for _, rule := range tt.rules {
				idmsRules = append(idmsRules, newIDMSRule(rule))
			}
			cli := fake.NewSimpleClientset()
			cfgcli := cfgfake.NewSimpleClientset(idmsRules...)
			lookup := NewSimpleLookupImageMirrorSetsStrategy(
				cli.OperatorV1alpha1().ImageContentSourcePolicies(),
				cfgcli.ConfigV1().ImageDigestMirrorSets(),
				cfgcli.ConfigV1().ImageTagMirrorSets(),
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

		t.Run(tt.name, func(t *testing.T) {
			tt.name = "itms-test-" + tt.name
			itmsRules := []runtime.Object{}
			for _, rule := range tt.rules {
				itmsRules = append(itmsRules, newITMSRule(rule))
			}
			cli := fake.NewSimpleClientset()
			cfgcli := cfgfake.NewSimpleClientset(itmsRules...)
			lookup := NewSimpleLookupImageMirrorSetsStrategy(
				cli.OperatorV1alpha1().ImageContentSourcePolicies(),
				cfgcli.ConfigV1().ImageDigestMirrorSets(),
				cfgcli.ConfigV1().ImageTagMirrorSets(),
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

func newICSPRule(r rule) runtime.Object {
	rdms := []v1alpha1.RepositoryDigestMirrors{}
	for _, e := range r.ruleElement {
		rdm := v1alpha1.RepositoryDigestMirrors{}
		rdm.Source = e.source
		rdm.Mirrors = e.mirrors
		rdms = append(rdms, rdm)
	}
	return &v1alpha1.ImageContentSourcePolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: r.name,
		},
		Spec: v1alpha1.ImageContentSourcePolicySpec{
			RepositoryDigestMirrors: rdms,
		},
	}
}

func newIDMSRule(r rule) runtime.Object {
	idms := []v1.ImageDigestMirrors{}
	for _, e := range r.ruleElement {
		idm := v1.ImageDigestMirrors{}
		idm.Source = e.source
		for _, m := range e.mirrors {
			idm.Mirrors = append(idm.Mirrors, v1.ImageMirror(m))
		}
		idms = append(idms, idm)
	}
	return &v1.ImageDigestMirrorSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: r.name,
		},
		Spec: v1.ImageDigestMirrorSetSpec{
			ImageDigestMirrors: idms,
		},
	}
}

func newITMSRule(r rule) runtime.Object {
	itms := []v1.ImageTagMirrors{}
	for _, e := range r.ruleElement {
		itm := v1.ImageTagMirrors{}
		itm.Source = e.source
		for _, m := range e.mirrors {
			itm.Mirrors = append(itm.Mirrors, v1.ImageMirror(m))
		}
		itms = append(itms, itm)
	}
	return &v1.ImageTagMirrorSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: r.name,
		},
		Spec: v1.ImageTagMirrorSetSpec{
			ImageTagMirrors: itms,
		},
	}
}

func TestFirstRequestICSPandIDMS(t *testing.T) {
	for _, tt := range []struct {
		name      string
		icsprules []rule
		idmsrules []rule
		itmsrules []rule
		ref       string
		res       []reference.DockerImageReference
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
					Registry:  "i.also.exist",
					Namespace: "ns2",
					Name:      "img2",
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
			idmsrules: []rule{
				{
					name: "rule",
					ruleElement: []element{
						{source: "i.do.not.exist/repo/image",
							mirrors: []string{
								"i.also.exist/ns1/img1",
							}},
					},
				},
			},
			icsprules: []rule{
				{
					name: "rule",
					ruleElement: []element{
						{source: "i.do.not.exist/repo/image",
							mirrors: []string{
								"i.exist/ns0/img0",
							}},
					},
				},
			},
			itmsrules: []rule{
				{
					name: "rule",
					ruleElement: []element{
						{source: "i.do.not.exist/repo/image",
							mirrors: []string{
								"i.also.exist/ns2/img2",
								"me.too/ns2/img2",
							}},
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			icspRules := []runtime.Object{}
			for _, rule := range tt.icsprules {
				icspRules = append(icspRules, newICSPRule(rule))
			}

			cli := fake.NewSimpleClientset(icspRules...)

			idmsRules := []runtime.Object{}
			for _, rule := range tt.idmsrules {
				idmsRules = append(idmsRules, newIDMSRule(rule))
			}

			itmsRules := []runtime.Object{}
			for _, rule := range tt.itmsrules {
				itmsRules = append(itmsRules, newITMSRule(rule))
			}

			cfgcli := cfgfake.NewSimpleClientset(append(idmsRules, itmsRules...)...)

			lookup := NewSimpleLookupImageMirrorSetsStrategy(
				cli.OperatorV1alpha1().ImageContentSourcePolicies(),
				cfgcli.ConfigV1().ImageDigestMirrorSets(),
				cfgcli.ConfigV1().ImageTagMirrorSets(),
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
