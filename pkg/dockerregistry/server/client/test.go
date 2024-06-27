package client

import (
	coreclientv1 "k8s.io/client-go/kubernetes/typed/core/v1"

	cfgfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	imageclientv1 "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"
	operatorfake "github.com/openshift/client-go/operator/clientset/versioned/fake"
)

type fakeRegistryClient struct {
	RegistryClient

	images imageclientv1.ImageV1Interface
}

func NewFakeRegistryClient(imageclient imageclientv1.ImageV1Interface) RegistryClient {
	return &fakeRegistryClient{
		RegistryClient: &registryClient{},
		images:         imageclient,
	}
}

func (c *fakeRegistryClient) Client() (Interface, error) {
	icsp := operatorfake.NewSimpleClientset().OperatorV1alpha1()
	cfgclient := cfgfake.NewSimpleClientset().ConfigV1()
	return newAPIClient(nil, nil, nil, c.images, nil, icsp, cfgclient), nil
}

func NewFakeRegistryAPIClient(kc coreclientv1.CoreV1Interface, imageclient imageclientv1.ImageV1Interface) Interface {
	icsp := operatorfake.NewSimpleClientset().OperatorV1alpha1()
	idms := cfgfake.NewSimpleClientset().ConfigV1()
	return newAPIClient(nil, nil, nil, imageclient, nil, icsp, idms)
}
