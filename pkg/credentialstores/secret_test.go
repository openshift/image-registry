package credentialstores

import (
	"io/ioutil"
	"net/url"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/json"
)

func TestCredentialsForSecrets(t *testing.T) {
	data, err := ioutil.ReadFile("test/image-secrets.json")
	if err != nil {
		t.Fatal(err)
	}

	var secrets corev1.SecretList
	if err := json.Unmarshal(data, &secrets); err != nil {
		t.Fatal(err)
	}

	store := NewForSecrets(secrets.Items)
	user, pass := store.Basic(&url.URL{Scheme: "https", Host: "172.30.213.112:5000"})
	if user != "serviceaccount" || len(pass) == 0 {
		t.Errorf("unexpected username and password: %s %s", user, pass)
	}
}
