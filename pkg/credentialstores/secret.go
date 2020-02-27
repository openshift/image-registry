package credentialstores

import (
	"net/url"
	"sync"

	"github.com/openshift/image-registry/pkg/kubernetes-common/credentialprovider"

	"github.com/golang/glog"
	"github.com/openshift/library-go/pkg/image/registryclient"
	corev1 "k8s.io/api/core/v1"
)

var (
	emptyKeyring = &credentialprovider.BasicDockerKeyring{}
)

// secretsRetriever is a function that returns a list of kubernetes secrets.
type secretsRetriever func() ([]corev1.Secret, error)

// NewForSecrets returns a credential store populated with a list of kubernetes
// secrets. Secrets are filtered as SecretCredentialStore uses only the ones
// containing docker credentials.
func NewForSecrets(secrets []corev1.Secret) *SecretCredentialStore {
	return &SecretCredentialStore{
		secrets:           secrets,
		RefreshTokenStore: registryclient.NewRefreshTokenStore(),
	}
}

// NewLazyForSecrets returns a credential store populated with the return of
// fn(). The return of fn() is filtered as SecretCredentialStore uses only
// secrets that contain docker credentials.
func NewLazyForSecrets(secretsFn secretsRetriever) *SecretCredentialStore {
	return &SecretCredentialStore{
		secretsFn:         secretsFn,
		RefreshTokenStore: registryclient.NewRefreshTokenStore(),
	}
}

// SecretCredentialStore holds docker credentials. It uses a list of secrets
// from where it extracts docker credentials, allowing callers to retrieve
// BasicAuth information by URL.
type SecretCredentialStore struct {
	lock      sync.Mutex
	secrets   []corev1.Secret
	secretsFn secretsRetriever
	err       error
	keyring   credentialprovider.DockerKeyring

	registryclient.RefreshTokenStore
}

// Basic returns BasicAuth information for the given url (user and password).
// If url does not exist on SecretCredentialStore's internal keyring empty
// strings are returned.
func (s *SecretCredentialStore) Basic(url *url.URL) (string, string) {
	s.init()
	return basicCredentialsFromKeyring(s.keyring, url)
}

// Err returns SecretCredentialStore's internal error.
func (s *SecretCredentialStore) Err() error {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.err
}

// init runs only once and is reponsible for loading the internal keyring with
// Secrets data (if a secretsRetriever function was specified). This function
// initializes the internal keyring. In case of errors, internal err is set.
func (s *SecretCredentialStore) init() {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.keyring != nil {
		return
	}

	// lazily load the secrets
	if s.secrets == nil && s.secretsFn != nil {
		s.secrets, s.err = s.secretsFn()
	}

	keyring, err := credentialprovider.MakeDockerKeyring(s.secrets, emptyKeyring)
	if err != nil {
		glog.V(5).Infof("Loading keyring failed for credential store: %v", err)
		s.err = err
		keyring = emptyKeyring
	}
	s.keyring = keyring
}
