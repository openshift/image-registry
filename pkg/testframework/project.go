package testframework

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	kapi "k8s.io/kubernetes/pkg/api/v1"

	authorizationapiv1 "github.com/openshift/origin/pkg/authorization/apis/authorization/v1"
	authorizationv1 "github.com/openshift/origin/pkg/authorization/generated/clientset/typed/authorization/v1"
	projectapiv1 "github.com/openshift/origin/pkg/project/apis/project/v1"
	projectv1 "github.com/openshift/origin/pkg/project/generated/clientset/typed/project/v1"
)

func CreateProject(t *testing.T, clientConfig *rest.Config, namespace string, adminUser string) *projectapiv1.Project {
	projectClient := projectv1.NewForConfigOrDie(clientConfig)
	project, err := projectClient.ProjectRequests().Create(&projectapiv1.ProjectRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	authorizationClient := authorizationv1.NewForConfigOrDie(clientConfig)
	_, err = authorizationClient.RoleBindings(namespace).Update(&authorizationapiv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "admin",
		},
		UserNames: []string{adminUser},
		RoleRef: kapi.ObjectReference{
			Name: "admin",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("project %s is created", project.Name)

	return project
}

func DeleteProject(t *testing.T, clientConfig *rest.Config, name string) {
	projectClient := projectv1.NewForConfigOrDie(clientConfig)
	if err := projectClient.Projects().Delete(name, &metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Logf("project %s is deleted", name)
}
