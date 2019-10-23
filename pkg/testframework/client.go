package testframework

import (
	"encoding/base64"

	"github.com/pborman/uuid"

	kerrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"

	oauthapi "github.com/openshift/api/oauth/v1"
	userapi "github.com/openshift/api/user/v1"
	oauthclient "github.com/openshift/client-go/oauth/clientset/versioned"
	userclient "github.com/openshift/client-go/user/clientset/versioned"
)

func GetClientForUser(clusterAdminConfig *restclient.Config, username string) (kclientset.Interface, *restclient.Config, error) {
	userClient, err := userclient.NewForConfig(clusterAdminConfig)
	if err != nil {
		return nil, nil, err
	}

	user, err := userClient.UserV1().Users().Get(username, metav1.GetOptions{})
	if err != nil {
		user = &userapi.User{
			ObjectMeta: metav1.ObjectMeta{Name: username},
		}
		user, err = userClient.UserV1().Users().Create(user)
		if err != nil {
			return nil, nil, err
		}
	}

	oauthClient, err := oauthclient.NewForConfig(clusterAdminConfig)
	if err != nil {
		return nil, nil, err
	}

	oauthClientObj := &oauthapi.OAuthClient{
		ObjectMeta:  metav1.ObjectMeta{Name: "test-integration-client"},
		GrantMethod: oauthapi.GrantHandlerAuto,
	}
	if _, err := oauthClient.OauthV1().OAuthClients().Create(oauthClientObj); err != nil && !kerrs.IsAlreadyExists(err) {
		return nil, nil, err
	}

	randomToken := uuid.NewRandom()
	accesstoken := base64.RawURLEncoding.EncodeToString([]byte(randomToken))
	// make sure the token is long enough to pass validation
	for i := len(accesstoken); i < 32; i++ {
		accesstoken += "A"
	}
	token := &oauthapi.OAuthAccessToken{
		ObjectMeta:  metav1.ObjectMeta{Name: accesstoken},
		ClientName:  oauthClientObj.Name,
		UserName:    username,
		UserUID:     string(user.UID),
		Scopes:      []string{"user:full"},
		RedirectURI: "https://localhost:8443/oauth/token/implicit",
	}
	if _, err := oauthClient.OauthV1().OAuthAccessTokens().Create(token); err != nil {
		return nil, nil, err
	}

	userClientConfig := restclient.AnonymousClientConfig(clusterAdminConfig)
	userClientConfig.BearerToken = token.Name

	kubeClientset, err := kclientset.NewForConfig(userClientConfig)
	if err != nil {
		return nil, nil, err
	}

	return kubeClientset, userClientConfig, nil
}
