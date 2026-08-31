package kubernetes

import (
	"testing"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
)

func TestBuildGatewayProxyRESTConfigUsesShortLivedTokenAndStripsManagementCredentials(t *testing.T) {
	clientset := fake.NewSimpleClientset(&corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: KubectlGatewayServiceAccountName, Namespace: KubectlGatewaySystemNamespaceName},
	})
	clientset.PrependReactor("create", "serviceaccounts", func(action k8stesting.Action) (bool, runtime.Object, error) {
		create := action.(k8stesting.CreateAction)
		if create.GetSubresource() != "token" {
			return false, nil, nil
		}
		return true, &authenticationv1.TokenRequest{
			Status: authenticationv1.TokenRequestStatus{
				Token:               "short-lived-token",
				ExpirationTimestamp: metav1.NewTime(time.Now().Add(10 * time.Minute)),
			},
		}, nil
	})
	client := NewClientForInterface(clientset)
	client.restConfig = &rest.Config{
		Host:        "https://cluster.example.test",
		BearerToken: "management-token",
		Username:    "admin",
		Password:    "secret",
	}
	manager := NewKubectlGatewayManager(client)
	config, err := manager.BuildGatewayProxyRESTConfig(t.Context(), GatewayTokenRequestOptions{})
	if err != nil {
		t.Fatalf("BuildGatewayProxyRESTConfig() error = %v", err)
	}
	if config.BearerToken != "short-lived-token" || config.Username != "" || config.Password != "" {
		t.Fatalf("proxy config = %#v", config)
	}
}
