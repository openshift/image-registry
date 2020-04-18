package testframework

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	authorizationapiv1 "github.com/openshift/api/authorization/v1"
	projectapiv1 "github.com/openshift/api/project/v1"
	authorizationv1 "github.com/openshift/client-go/authorization/clientset/versioned/typed/authorization/v1"
	projectv1 "github.com/openshift/client-go/project/clientset/versioned/typed/project/v1"
)

func CreateProject(t *testing.T, clientConfig *rest.Config, namespace string, adminUser string) *projectapiv1.Project {
	projectClient := projectv1.NewForConfigOrDie(clientConfig)
	project, err := projectClient.ProjectRequests().Create(context.Background(), &projectapiv1.ProjectRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	authorizationClient := authorizationv1.NewForConfigOrDie(clientConfig)
	_, err = authorizationClient.RoleBindings(namespace).Update(context.Background(), &authorizationapiv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "admin",
		},
		UserNames: []string{adminUser},
		RoleRef: corev1.ObjectReference{
			Name: "admin",
		},
	}, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("project %s is created", project.Name)

	return project
}

func DeleteProject(t *testing.T, clientConfig *rest.Config, name string) {
	projectClient := projectv1.NewForConfigOrDie(clientConfig)
	if err := projectClient.Projects().Delete(context.Background(), name, metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Logf("project %s is deleted", name)
}
