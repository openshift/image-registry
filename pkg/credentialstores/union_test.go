package credentialstores

import (
	"fmt"
	"net/url"
	"testing"
)

type StoreMock struct {
	keyring map[string][]string
	err     error
}

func (s *StoreMock) Basic(target *url.URL) (string, string) {
	if s.keyring == nil {
		return "", ""
	}

	dat, ok := s.keyring[target.Host]
	if !ok {
		return "", ""
	}
	return dat[0], dat[1]
}

func (s *StoreMock) Err() error {
	return s.err
}

func TestUnionCredentialStoreBasic(t *testing.T) {
	for _, tt := range []struct {
		name        string
		stores      []CredentialStore
		url         *url.URL
		user        string
		pass        string
		errExpected bool
	}{
		{
			name:   "empty stores",
			stores: []CredentialStore{},
			url:    nil,
		},
		{
			name: "not found",
			stores: []CredentialStore{
				&StoreMock{},
				&StoreMock{},
			},
			url: nil,
		},
		{
			name: "found on first store",
			stores: []CredentialStore{
				&StoreMock{
					keyring: map[string][]string{
						"registry0": {
							"user0",
							"pass0",
						},
					},
				},
				&StoreMock{
					keyring: map[string][]string{
						"registry0": {
							"user1",
							"pass1",
						},
					},
				},
			},
			url: &url.URL{
				Host: "registry0",
			},
			user: "user0",
			pass: "pass0",
		},
		{
			name: "found on second store",
			stores: []CredentialStore{
				&StoreMock{
					keyring: map[string][]string{
						"registry0": {
							"user0",
							"pass0",
						},
					},
				},
				&StoreMock{
					keyring: map[string][]string{
						"registry1": {
							"user1",
							"pass1",
						},
					},
				},
			},
			url: &url.URL{
				Host: "registry1",
			},
			user: "user1",
			pass: "pass1",
		},
		{
			name: "error",
			stores: []CredentialStore{
				&StoreMock{
					err: fmt.Errorf("error"),
				},
			},
			url: &url.URL{
				Host: "registry1",
			},
			user:        "",
			pass:        "",
			errExpected: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := NewUnionCredentialStore(tt.stores...)
			user, pass := store.Basic(tt.url)
			if user != tt.user || pass != tt.pass {
				t.Error("invalid user/pass pair returned")
			}
			err := store.Err()
			if tt.errExpected && err == nil {
				t.Error("expecting error, nil received")
			}
			if !tt.errExpected && err != nil {
				t.Error("unexpected error returned")
			}
		})
	}
}
