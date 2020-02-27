package credentialstores

import (
	"fmt"
	"net/url"
	"os"
	"testing"
)

func TestNewNodeCredentialStore(t *testing.T) {
	store := NewNodeCredentialStore()
	if store.Err() == nil {
		t.Error("able to create with invalid docker credentials path")
	}
}

func TestBasic(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	oldDir := NodeCredentialsDir
	NodeCredentialsDir = fmt.Sprintf("%s/test/", dir)
	store := NewNodeCredentialStore()
	NodeCredentialsDir = oldDir

	if store.Err() != nil {
		t.Fatalf("unexpected credentials store error: %v", err)
	}

	for _, tt := range []struct {
		name string
		url  *url.URL
		user string
		pass string
	}{
		{
			name: "valid registry",
			url: &url.URL{
				Host: "registry0.redhat.io",
			},
			user: "registry0",
			pass: "registry0",
		},
		{
			name: "invalid registry",
			url: &url.URL{
				Host: "invalidregistry.redhat.io",
			},
		},
		{
			name: "nil url",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			user, pass := store.Basic(tt.url)
			if user != tt.user || pass != tt.pass {
				t.Error("invalid user/pass pair")
			}
		})
	}
}
