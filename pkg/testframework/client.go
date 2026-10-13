package testframework

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"

	kerrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"

	oauthapi "github.com/openshift/api/oauth/v1"
	userapi "github.com/openshift/api/user/v1"
	oauthclient "github.com/openshift/client-go/oauth/clientset/versioned"
	userclient "github.com/openshift/client-go/user/clientset/versioned"
)

func GenerateRandomBytes(n int) []byte {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	return b
}

func GenerateOAuthTokenPair() (privToken, pubToken string) {
	randomBytes := GenerateRandomBytes(8)
	randomToken := base64.URLEncoding.EncodeToString(randomBytes)
	hashed := sha256.Sum256([]byte(randomToken))
	return "sha256~" + randomToken, "sha256~" + base64.RawURLEncoding.EncodeToString(hashed[:])
}

func GetClientForUser(clusterAdminConfig *restclient.Config, username string) (kclientset.Interface, *restclient.Config, error) {
	userClient, err := userclient.NewForConfig(clusterAdminConfig)
	if err != nil {
		return nil, nil, err
	}

	user, err := userClient.UserV1().Users().Get(context.Background(), username, metav1.GetOptions{})
	if err != nil {
		user = &userapi.User{
			ObjectMeta: metav1.ObjectMeta{Name: username},
		}
		user, err = userClient.UserV1().Users().Create(context.Background(), user, metav1.CreateOptions{})
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
	if _, err := oauthClient.OauthV1().OAuthClients().Create(context.Background(), oauthClientObj, metav1.CreateOptions{}); err != nil && !kerrs.IsAlreadyExists(err) {
		return nil, nil, err
	}

	privToken, pubToken := GenerateOAuthTokenPair()
	token := &oauthapi.OAuthAccessToken{
		ObjectMeta: metav1.ObjectMeta{
			Name: pubToken,
		},
		ClientName:  oauthClientObj.Name,
		UserName:    username,
		UserUID:     string(user.UID),
		Scopes:      []string{"user:full"},
		RedirectURI: "https://localhost:8443/oauth/token/implicit",
	}
	if _, err := oauthClient.OauthV1().OAuthAccessTokens().Create(context.Background(), token, metav1.CreateOptions{}); err != nil {
		return nil, nil, err
	}

	userClientConfig := restclient.AnonymousClientConfig(clusterAdminConfig)
	userClientConfig.BearerToken = privToken

	kubeClientset, err := kclientset.NewForConfig(userClientConfig)
	if err != nil {
		return nil, nil, err
	}

	return kubeClientset, userClientConfig, nil
}
