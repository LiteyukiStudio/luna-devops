package kubeaccess

import (
	"strings"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
)

func TestRenderKubeconfigUsesOnlyPlatformGateway(t *testing.T) {
	content, err := RenderKubeconfig("https://devops.example.com", "tok_demo", "luna_devops_kube_secret", []BindingSummary{
		{ID: "kbd_one", ProjectID: "prj_one", RuntimeClusterID: "clu_one", Namespace: "project-one"},
		{ID: "kbd_two", ProjectID: "prj_one", RuntimeClusterID: "clu_one", ApplicationID: "app_two", Namespace: "project-one"},
	})
	if err != nil {
		t.Fatal(err)
	}
	config, err := clientcmd.Load([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Clusters) != 2 || len(config.Contexts) != 2 || len(config.AuthInfos) != 1 {
		t.Fatalf("unexpected kubeconfig: %#v", config)
	}
	for _, cluster := range config.Clusters {
		if !strings.HasPrefix(cluster.Server, "https://devops.example.com/kube/v1/bindings/kbd_") || cluster.InsecureSkipTLSVerify || len(cluster.CertificateAuthorityData) != 0 {
			t.Fatalf("unsafe cluster entry: %#v", cluster)
		}
	}
	if config.AuthInfos["luna/tok_demo"].Token != "luna_devops_kube_secret" {
		t.Fatal("credential was not written to the dedicated auth info")
	}
}

func TestRenderKubeconfigRejectsRequestDerivedOrMalformedBaseURL(t *testing.T) {
	for _, baseURL := range []string{"", "/relative", "https://user:pass@example.com", "https://example.com?next=evil", "ftp://example.com"} {
		if _, err := RenderKubeconfig(baseURL, "tok_demo", "luna_devops_kube_secret", []BindingSummary{{ID: "kbd_one", ProjectID: "prj", RuntimeClusterID: "clu", Namespace: "ns"}}); err == nil {
			t.Fatalf("base URL %q was accepted", baseURL)
		}
	}
}
