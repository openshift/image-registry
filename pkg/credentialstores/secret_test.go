package credentialstores

import (
	"net/url"
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestCredentialsForSecrets(t *testing.T) {
	fp, err := os.Open("test/image-secrets.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer fp.Close()

	var secrets corev1.SecretList
	decoder := yaml.NewYAMLToJSONDecoder(fp)
	if err := decoder.Decode(&secrets); err != nil {
		t.Fatal(err)
	}

	store := NewForSecrets(secrets.Items)
	user, pass := store.Basic(&url.URL{Scheme: "https", Host: "172.30.213.112:5000"})
	if user != "serviceaccount" || len(pass) == 0 {
		t.Errorf("unexpected username and password: %s %s", user, pass)
	}
}
