package worker

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
)

func TestServiceBindingValuesUsesStableServiceDNS(t *testing.T) {
	values, err := serviceBindingValues(
		model.Project{ID: "prj_c119e462fb7c5eed20ec4ca4", KubernetesNamespace: "ns-c119e462fb"},
		model.ServiceBinding{ID: "sbind_api", Protocol: "http", Path: "/v1", InjectionMode: "url", URLEnvVar: "API_URL"},
		model.DeploymentTarget{ID: "dplt_b530527f18113463aa3bf8a7", KubernetesName: "dplt-b530527f18"},
		model.DeploymentServicePort{Name: "http", Port: 8080},
	)
	if err != nil {
		t.Fatalf("serviceBindingValues returned error: %v", err)
	}
	want := "http://dplt-b530527f18.ns-c119e462fb.svc.cluster.local:8080/v1"
	if values["API_URL"] != want {
		t.Fatalf("API_URL = %q, want %q", values["API_URL"], want)
	}
}

func TestServiceBindingValuesSupportsHostAndPort(t *testing.T) {
	values, err := serviceBindingValues(
		model.Project{ID: "prj_demo", KubernetesNamespace: "ns-demo"},
		model.ServiceBinding{ID: "sbind_db", Protocol: "tcp", InjectionMode: "host_port", HostEnvVar: "DB_HOST", PortEnvVar: "DB_PORT"},
		model.DeploymentTarget{ID: "dplt_database", KubernetesName: "dplt-database"},
		model.DeploymentServicePort{Name: "postgres", Port: 5432},
	)
	if err != nil {
		t.Fatalf("serviceBindingValues returned error: %v", err)
	}
	if values["DB_HOST"] != "dplt-database.ns-demo.svc.cluster.local" || values["DB_PORT"] != "5432" {
		t.Fatalf("values = %#v", values)
	}
}

func TestServiceBindingValuesUsesPersistedReadableServiceDNS(t *testing.T) {
	values, err := serviceBindingValues(
		model.Project{
			ID:                  "prj_payments",
			KubernetesNamespace: "luna-payments",
		},
		model.ServiceBinding{
			ID:            "sbind_db",
			Protocol:      "tcp",
			InjectionMode: "host_port",
			HostEnvVar:    "DB_HOST",
			PortEnvVar:    "DB_PORT",
		},
		model.DeploymentTarget{
			ID:             "dplt_payments_database_prod",
			KubernetesName: "luna-database-prod",
		},
		model.DeploymentServicePort{Name: "postgres", Port: 5432},
	)
	if err != nil {
		t.Fatalf("serviceBindingValues returned error: %v", err)
	}
	if values["DB_HOST"] != "luna-database-prod.luna-payments.svc.cluster.local" || values["DB_PORT"] != "5432" {
		t.Fatalf("values = %#v", values)
	}
}

func TestApplyServiceBindingConfigRejectsRuntimeAndSecretCollisions(t *testing.T) {
	for _, spec := range []kubeprovider.ApplicationResourcesSpec{
		{ConfigData: map[string]string{"API_URL": "manual"}, SecretData: map[string]string{}},
		{ConfigData: map[string]string{}, SecretData: map[string]string{"API_URL": "secret"}},
	} {
		err := applyServiceBindingConfig(&spec, resolvedServiceBindingConfig{Values: map[string]string{"API_URL": "generated"}})
		if err == nil {
			t.Fatal("expected environment variable collision")
		}
	}
}

func TestServiceBindingConfigDigestIsDeterministic(t *testing.T) {
	left, err := serviceBindingConfigDigest([]serviceBindingDigestEntry{
		{ID: "sbind_b", Values: map[string]string{"B_PORT": "5432", "B_HOST": "database"}},
		{ID: "sbind_a", Values: map[string]string{"API_URL": "http://api:8080"}},
	})
	if err != nil {
		t.Fatalf("serviceBindingConfigDigest returned error: %v", err)
	}
	right, err := serviceBindingConfigDigest([]serviceBindingDigestEntry{
		{ID: "sbind_a", Values: map[string]string{"API_URL": "http://api:8080"}},
		{ID: "sbind_b", Values: map[string]string{"B_HOST": "database", "B_PORT": "5432"}},
	})
	if err != nil {
		t.Fatalf("serviceBindingConfigDigest returned error: %v", err)
	}
	if left == "" || left != right {
		t.Fatalf("digests = %q and %q", left, right)
	}
}
