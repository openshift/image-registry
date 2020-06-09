package server

import (
	"context"
	"io/ioutil"
	"net/http"
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/openshift/library-go/pkg/image/registryclient"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/metrics"
	imageapi "github.com/openshift/image-registry/pkg/origin-common/image/apis/image"
	"github.com/openshift/image-registry/pkg/testutil"
)

type mockMetricsPullThrough struct{}

func (m *mockMetricsPullThrough) RepositoryRetriever(r registryclient.RepositoryRetriever) registryclient.RepositoryRetriever {
	return r
}

func (m *mockMetricsPullThrough) DigestBlobStoreCache() metrics.Cache {
	return nil
}

func Test_getImportContext(t *testing.T) {
	tmpCredDir, err := ioutil.TempDir("", "credentials")
	if err != nil {
		t.Fatalf("error creating temp dir: %v", err)
	}

	originalCredDir := installCredentialsDir
	installCredentialsDir = tmpCredDir

	defer func() {
		installCredentialsDir = originalCredDir
		if err := os.RemoveAll(tmpCredDir); err != nil {
			t.Logf("error removing temp dir: %v", err)
		}
	}()

	for _, tt := range []struct {
		creds   []byte
		err     string
		name    string
		pass    string
		ref     *imageapi.DockerImageReference
		req     bool
		secrets []corev1.Secret
		user    string
	}{
		{
			name: "context without http request",
			err:  "no http request in context",
			ref:  &imageapi.DockerImageReference{},
		},
		{
			name:  "invalid json",
			creds: []byte(`<{Dd`),
			ref:   &imageapi.DockerImageReference{},
			req:   true,
		},
		{
			name:  "credential present on node",
			creds: []byte(`{ "auths": { "192.168.122.19:8000": { "auth": "dXNlcjpwYXNz" } } }`),
			pass:  "pass",
			ref: &imageapi.DockerImageReference{
				Name:     "192.168.122.19:8000/test",
				Registry: "192.168.122.19:8000/test",
			},
			req:  true,
			user: "user",
		},
		{
			name: "broken secret",
			err:  "invalid character '<' looking for beginning of value",
			ref:  &imageapi.DockerImageReference{},
			req:  true,
			secrets: []corev1.Secret{
				{
					Type: corev1.SecretTypeDockerConfigJson,
					Data: map[string][]byte{
						".dockerconfigjson": []byte(`<$"`),
					},
				},
			},
		},
		{
			name:  "secrets over node credentials priority",
			creds: []byte(`{"auths":{"192.168.122.19:8000":{"auth":"dXNlcjpwYXNz"}}}`),
			ref: &imageapi.DockerImageReference{
				Name:     "192.168.122.19:8000/test",
				Registry: "192.168.122.19:8000",
			},
			req: true,
			secrets: []corev1.Secret{
				{
					Type: corev1.SecretTypeDockerConfigJson,
					Data: map[string][]byte{
						".dockerconfigjson": []byte(`{"auths":{"192.168.122.19:8000":{"auth":"dXNlcm9uc2VjcmV0OnBhc3NvbnNlY3JldA=="}}}`),
					},
				},
			},
			user: "useronsecret",
			pass: "passonsecret",
		},
		{
			name: "no credentials",
			ref: &imageapi.DockerImageReference{
				Name:     "192.168.122.19:8000/test",
				Registry: "192.168.122.19:8000",
			},
			req:  true,
			user: "",
			pass: "",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			ctx = testutil.WithTestLogger(ctx, t)
			if tt.req {
				ctx = context.WithValue(ctx, "http.request", &http.Request{})
			}

			if len(tt.creds) > 0 {
				if err := ioutil.WriteFile(
					tmpCredDir+"/config.json", tt.creds, 0644,
				); err != nil {
					t.Errorf("error writing config.json: %v", err)
					return
				}

				defer func() {
					if err = os.Remove(tmpCredDir + "/config.json"); err != nil {
						t.Errorf("unable to temp credentials: %v", err)
					}
				}()
			}

			retriever, err := getImportContext(
				ctx, tt.ref, tt.secrets, &mockMetricsPullThrough{},
			)
			if err != nil {
				if len(tt.err) == 0 {
					t.Errorf("unexpected error: %v", err)
				} else if err.Error() != tt.err {
					t.Errorf("error mismatch, expecting %q, received %q", tt.err, err.Error())
				}
				return
			}

			if len(tt.err) > 0 {
				t.Error("expected error, nil received instead")
				return
			}

			regctx, ok := retriever.(*registryclient.Context)
			if !ok {
				t.Errorf("unable to cast %T", retriever)
				return
			}

			user, pass := regctx.Credentials.Basic(nil)
			if user != tt.user || pass != tt.pass {
				t.Errorf("expected %q/%q, received %q,%q", tt.user, tt.pass, user, pass)
			}
		})
	}
}
