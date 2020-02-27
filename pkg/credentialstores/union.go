package credentialstores

import (
	"net/url"

	"k8s.io/apimachinery/pkg/util/errors"

	"github.com/openshift/library-go/pkg/image/registryclient"
)

// CredentialStore is an interface implemented by all credential stores.
type CredentialStore interface {
	Basic(*url.URL) (string, string)
	Err() error
}

// NewUnionCredentialStore returns an UnionCredentialStore populated with
// provided credential stores.
func NewUnionCredentialStore(stores ...CredentialStore) *UnionCredentialStore {
	return &UnionCredentialStore{
		stores:            stores,
		RefreshTokenStore: registryclient.NewRefreshTokenStore(),
	}
}

// UnionCredentialStore is a handler that holds multiple internal credential
// stores. It allows callers to aggregate multiple Docker credentials keyrings.
type UnionCredentialStore struct {
	stores []CredentialStore
	registryclient.RefreshTokenStore
}

// Basic looks for BasicAuth information for a given URL using the internal
// group of credential stores. Internal stores are sequentially queried and
// this function returns as soon as the first match happens.
func (u *UnionCredentialStore) Basic(target *url.URL) (string, string) {
	var user string
	var pass string

	for _, s := range u.stores {
		user, pass = s.Basic(target)
		if len(user) > 0 || len(pass) > 0 {
			return user, pass
		}
	}

	return user, pass
}

// Err returns all errors reported by internal credential stores.
func (u *UnionCredentialStore) Err() error {
	var errs []error
	for _, s := range u.stores {
		if err := s.Err(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.NewAggregate(errs)
}
