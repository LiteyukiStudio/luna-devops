package worker

import (
	"context"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

var (
	testCertificateGVR = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}
	testGatewayGVR     = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"}
	testHTTPRouteGVR   = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "httproutes"}
)

func TestGatewayRouteDeleteReleasesSharedCertificatesOnlyAfterLastRoute(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "gateway_certificate_cleanup_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.Project{}, &model.RuntimeCluster{}, &model.DeploymentTarget{}, &model.GatewayRoute{}, &model.AuditLog{})
		},
	})
	project := model.Project{ID: "prj_cert", Identifier: "cert", Name: "Cert", KubernetesNamespace: "luna-cert"}
	cluster := model.RuntimeCluster{
		ID: "rcl_cert", Name: "Cert", Scope: "global", IsDefault: true, DeleteStatus: "active",
		GatewayName: "luna-gateway", GatewayNamespace: "kube-system", GatewayExternalTLSMode: "gateway",
		GatewayWildcardCertEnabled: true, GatewayWildcardCertDomain: "*.example.com",
	}
	target := model.DeploymentTarget{ID: "dplt_cert", ProjectID: project.ID, ApplicationID: "app_cert", ClusterID: cluster.ID, Name: "Prod", Stage: "prod"}
	startedAt := time.Now()
	routes := []model.GatewayRoute{
		{ID: "gwr_cert_a", ProjectID: project.ID, ApplicationID: target.ApplicationID, DeploymentTargetID: target.ID, Host: "api.example.com", Path: "/a", TLSMode: "http-challenge", DeleteStatus: "deleting", DeleteStartedAt: &startedAt},
		{ID: "gwr_cert_b", ProjectID: project.ID, ApplicationID: target.ApplicationID, DeploymentTargetID: target.ID, Host: "api.example.com", Path: "/b", TLSMode: "http-challenge", DeleteStatus: "deleting", DeleteStartedAt: &startedAt},
	}
	for _, row := range []any{&project, &cluster, &target, &routes} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("create fixture %T: %v", row, err)
		}
	}

	certNamespace := gatewayCertificateNamespace(cluster, project.KubernetesNamespace)
	routeSecret := gatewayTLSSecretName(routes[0])
	wildcardSecret := gatewayWildcardCertificateSecretName(cluster)
	kube := kubefake.NewSimpleClientset(
		testIssuedSecret(certNamespace, routeSecret),
		testIssuedSecret(certNamespace, wildcardSecret),
	)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		testCertificateGVR: "CertificateList",
		testGatewayGVR:     "GatewayList",
		testHTTPRouteGVR:   "HTTPRouteList",
	},
		testManagedCertificate(certNamespace, gatewayRuntimeName(routes[0]), routeSecret, routes[0].ID),
		testManagedCertificate(certNamespace, gatewayRuntimeName(routes[1]), routeSecret, routes[1].ID),
		testManagedCertificate(certNamespace, wildcardSecret, wildcardSecret, ""),
		testManagedHTTPRoute(project.KubernetesNamespace, gatewayRuntimeName(routes[0])),
		testManagedHTTPRoute(project.KubernetesNamespace, gatewayRuntimeName(routes[1])),
	)
	gateway := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "Gateway",
		"metadata":   map[string]any{"name": "luna-gateway", "namespace": "kube-system"},
		"spec": map[string]any{"listeners": []any{map[string]any{
			"name": "websecure", "port": int64(8443), "protocol": "HTTPS",
			"tls": map[string]any{"mode": "Terminate", "certificateRefs": []any{
				map[string]any{"name": wildcardSecret},
				map[string]any{"name": routeSecret},
			}},
		}}},
	}}
	if _, err := dynamicClient.Resource(testGatewayGVR).Namespace("kube-system").Create(t.Context(), gateway, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create gateway: %v", err)
	}
	runner := newRunner(db, Options{})
	runner.kubernetesManagerFactory = func(model.DeploymentTarget) (kubeprovider.NamespaceManager, error) {
		return kubeprovider.NewClientForInterfaces(kube, dynamicClient), nil
	}
	deleteRoute := func(route model.GatewayRoute) {
		t.Helper()
		payload := tasks.ResourceCleanupPayload{ResourceType: "gateway_route", ResourceID: route.ID, ProjectID: project.ID, ActorID: "usr_owner"}
		if err := runner.handleResourceCleanupPayload(t.Context(), payload); err != nil {
			t.Fatalf("cleanup %s: %v", route.ID, err)
		}
	}

	deleteRoute(routes[0])
	assertDynamicExists(t, dynamicClient, testCertificateGVR, certNamespace, gatewayRuntimeName(routes[0]), false)
	assertDynamicExists(t, dynamicClient, testCertificateGVR, certNamespace, gatewayRuntimeName(routes[1]), true)
	assertDynamicExists(t, dynamicClient, testCertificateGVR, certNamespace, wildcardSecret, true)
	assertSecretExists(t, kube, certNamespace, routeSecret, true)
	if refs := testGatewayRefNames(t, dynamicClient); len(refs) != 2 {
		t.Fatalf("gateway refs after first delete = %#v", refs)
	}

	deleteRoute(routes[1])
	assertDynamicExists(t, dynamicClient, testCertificateGVR, certNamespace, gatewayRuntimeName(routes[1]), false)
	assertDynamicExists(t, dynamicClient, testCertificateGVR, certNamespace, wildcardSecret, false)
	assertSecretExists(t, kube, certNamespace, routeSecret, false)
	assertSecretExists(t, kube, certNamespace, wildcardSecret, false)
	if refs := testGatewayRefNames(t, dynamicClient); len(refs) != 0 {
		t.Fatalf("gateway refs after last delete = %#v", refs)
	}

	var audits []model.AuditLog
	if err := db.Where("action = ?", gatewayCertificateReleaseAuditAction).Order("created_at asc").Find(&audits).Error; err != nil {
		t.Fatalf("load audits: %v", err)
	}
	if len(audits) != 3 || audits[0].Resource != routes[0].ID || audits[1].Resource != routes[1].ID || audits[2].Resource != cluster.ID || audits[0].UserID != "usr_owner" {
		t.Fatalf("certificate release audits = %#v", audits)
	}

	// 重复执行已完成的回收（例如恢复任务重放）不再产生变更或审计。
	if err := runner.releaseGatewayCertificates(t.Context(), kubeprovider.NewClientForInterfaces(kube, dynamicClient), target, project, routes, "usr_owner"); err != nil {
		t.Fatalf("repeat release: %v", err)
	}
	var count int64
	if err := db.Model(&model.AuditLog{}).Where("action = ?", gatewayCertificateReleaseAuditAction).Count(&count).Error; err != nil || count != 3 {
		t.Fatalf("audits after repeat = %d, %v", count, err)
	}
}

func testManagedCertificate(namespace, name, secretName, routeID string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "cert-manager.io/v1",
		"kind":       "Certificate",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"labels":    map[string]any{kubeprovider.ManagedByLabel: kubeprovider.ManagedByValue, "luna.devops/route-id": routeID},
		},
		"spec": map[string]any{"secretName": secretName},
	}}
}

func testManagedHTTPRoute(namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "HTTPRoute",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"labels":    map[string]any{kubeprovider.ManagedByLabel: kubeprovider.ManagedByValue},
		},
	}}
}

func testIssuedSecret(namespace, name string) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name: name, Namespace: namespace,
		Annotations: map[string]string{"cert-manager.io/certificate-name": name},
	}}
}

func assertDynamicExists(t *testing.T, client *dynamicfake.FakeDynamicClient, gvr schema.GroupVersionResource, namespace, name string, want bool) {
	t.Helper()
	_, err := client.Resource(gvr).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if got := err == nil; got != want {
		t.Fatalf("%s %s/%s exists = %v (err %v), want %v", gvr.Resource, namespace, name, got, err, want)
	}
	if err != nil && !apierrors.IsNotFound(err) {
		t.Fatalf("get %s %s/%s: %v", gvr.Resource, namespace, name, err)
	}
}

func assertSecretExists(t *testing.T, client *kubefake.Clientset, namespace, name string, want bool) {
	t.Helper()
	_, err := client.CoreV1().Secrets(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if got := err == nil; got != want {
		t.Fatalf("secret %s/%s exists = %v (err %v), want %v", namespace, name, got, err, want)
	}
}

func testGatewayRefNames(t *testing.T, client *dynamicfake.FakeDynamicClient) []string {
	t.Helper()
	gateway, err := client.Resource(testGatewayGVR).Namespace("kube-system").Get(context.Background(), "luna-gateway", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get gateway: %v", err)
	}
	listeners, _, _ := unstructured.NestedSlice(gateway.Object, "spec", "listeners")
	names := []string{}
	for _, item := range listeners {
		refs, _, _ := unstructured.NestedSlice(item.(map[string]any), "tls", "certificateRefs")
		for _, ref := range refs {
			names = append(names, ref.(map[string]any)["name"].(string))
		}
	}
	return names
}
