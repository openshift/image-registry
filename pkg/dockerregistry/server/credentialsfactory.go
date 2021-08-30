package server

import (
	"github.com/docker/distribution/registry/client/auth"
	dockerregistry "github.com/docker/docker/registry"
	"github.com/openshift/library-go/pkg/image/registryclient"

	"github.com/openshift/image-registry/pkg/kubernetes-common/credentialprovider"
)

// credentialStoreFactory is an entity capable of providing docker registry authentication based
// in an image path (such as quay.io/fedora/fedora).
type credentialStoreFactory struct {
	keyring credentialprovider.DockerKeyring
}

// CredentialStoreFor returns authentication info for accessing "image". Returns only one
// authentication.
func (c *credentialStoreFactory) CredentialStoreFor(image string) auth.CredentialStore {
	var nocreds auth.CredentialStore = registryclient.NoCredentials
	if c.keyring == nil {
		return nocreds
	}

	auths, _ := c.keyring.Lookup(image)
	if len(auths) == 0 {
		return nocreds
	}

	return dockerregistry.NewStaticCredentialStore(&auths[0].AuthConfig)
}
