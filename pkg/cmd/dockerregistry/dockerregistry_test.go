package dockerregistry

import (
	"crypto/tls"
	"os"
	"reflect"
	"strings"
	"testing"

	registryconfig "github.com/openshift/image-registry/pkg/dockerregistry/server/configuration"
)

var minimalConfigYAML = `
version: 0.1
http:
  addr: ":5000"
  tls:
    certificate: cert.pem
storage:
  inmemory: {}
`

func TestNewServerTLSCurvesAndGroups(t *testing.T) {
	for _, tt := range []struct {
		name     string
		env      map[string]string
		err      string
		expected []tls.CurveID
	}{
		{
			name: "no curve preferences nor groups",
			env: map[string]string{
				"REGISTRY_OPENSHIFT_SERVER_ADDR": "0.0.0.0:5000",
			},
		},
		{
			name: "single curve preference",
			env: map[string]string{
				"REGISTRY_OPENSHIFT_SERVER_ADDR":                "0.0.0.0:5000",
				"OPENSHIFT_REGISTRY_HTTP_TLS_CURVE_PREFERENCES": "23",
			},
			expected: []tls.CurveID{
				tls.CurveP256,
			},
		},
		{
			name: "multiple and duplicated curve preferences",
			env: map[string]string{
				"REGISTRY_OPENSHIFT_SERVER_ADDR":                "0.0.0.0:5000",
				"OPENSHIFT_REGISTRY_HTTP_TLS_CURVE_PREFERENCES": "23,24,25,23,24,25",
			},
			expected: []tls.CurveID{
				tls.CurveP256,
				tls.CurveP384,
				tls.CurveP521,
			},
		},
		{
			name: "curve preferences and tls groups set at the same time",
			env: map[string]string{
				"REGISTRY_OPENSHIFT_SERVER_ADDR":                "0.0.0.0:5000",
				"OPENSHIFT_REGISTRY_HTTP_TLS_CURVE_PREFERENCES": "23",
				"OPENSHIFT_REGISTRY_HTTP_TLS_GROUPS":            "X25519",
			},
			expected: []tls.CurveID{
				tls.CurveP256,
				tls.X25519,
			},
		},
		{
			name: "curve preferences and tls groups overwriting each other",
			env: map[string]string{
				"REGISTRY_OPENSHIFT_SERVER_ADDR":                "0.0.0.0:5000",
				"OPENSHIFT_REGISTRY_HTTP_TLS_CURVE_PREFERENCES": "23",
				"OPENSHIFT_REGISTRY_HTTP_TLS_GROUPS":            "secp256r1",
			},
			expected: []tls.CurveID{
				tls.CurveP256,
			},
		},
		{
			name: "invalid curve preference type",
			env: map[string]string{
				"REGISTRY_OPENSHIFT_SERVER_ADDR":                "0.0.0.0:5000",
				"OPENSHIFT_REGISTRY_HTTP_TLS_CURVE_PREFERENCES": "invalid",
			},
			err: "invalid syntax",
		},
		{
			name: "curve preference out of range",
			env: map[string]string{
				"REGISTRY_OPENSHIFT_SERVER_ADDR":                "0.0.0.0:5000",
				"OPENSHIFT_REGISTRY_HTTP_TLS_CURVE_PREFERENCES": "65536",
			},
			err: "value out of range",
		},
		{
			name: "invalid tls group",
			env: map[string]string{
				"REGISTRY_OPENSHIFT_SERVER_ADDR":     "0.0.0.0:5000",
				"OPENSHIFT_REGISTRY_HTTP_TLS_GROUPS": "bogus",
			},
			err: "invalid group",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			originalEnv := map[string]string{}
			for key, value := range tt.env {
				if original, set := os.LookupEnv(key); set {
					originalEnv[key] = original
				}
				if err := os.Setenv(key, value); err != nil {
					t.Fatalf("fail to set env variable: %v", err)
				}
			}
			defer func() {
				for key := range tt.env {
					if err := os.Unsetenv(key); err != nil {
						t.Fatalf("fail to unset env variable: %v", err)
					}
				}
				for key, value := range originalEnv {
					if err := os.Setenv(key, value); err != nil {
						t.Fatalf("fail to set env variable: %v", err)
					}
				}
			}()

			config, extra, err := registryconfig.Parse(strings.NewReader(minimalConfigYAML))
			if err != nil {
				t.Errorf("failed to decode hardcoded yaml: %v", err)
				return
			}

			server, err := NewServer(t.Context(), config, extra)
			if len(tt.err) > 0 {
				if err == nil {
					t.Errorf("expected error containing %q, nil received", tt.err)
					return
				}
				if !strings.Contains(err.Error(), tt.err) {
					t.Errorf("expected error to contain %q, error returned was: %v", tt.err, err)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error %v", err)
				return
			}

			curves := server.TLSConfig.CurvePreferences
			if !reflect.DeepEqual(curves, tt.expected) {
				t.Errorf("expected %v curves, found %v", tt.expected, curves)
			}
		})
	}
}
