package kubernetes

import (
	"errors"
	"strings"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestSafeKubeconfigAcceptsInlineCredentialsAndHTTPS(t *testing.T) {
	config := validSafeKubeconfig()
	config.Clusters["cluster"].CertificateAuthorityData = []byte("inline-ca")
	config.AuthInfos["user"].ClientCertificateData = []byte("inline-cert")
	config.AuthInfos["user"].ClientKeyData = []byte("inline-key")
	kubeconfig := writeKubeconfigForTest(t, config)

	normalized, err := NormalizeSafeKubeconfig(kubeconfig)
	if err != nil {
		t.Fatalf("NormalizeSafeKubeconfig returned error: %v", err)
	}
	if !strings.Contains(normalized, "certificate-authority-data") || !strings.Contains(normalized, "client-certificate-data") {
		t.Fatalf("normalized kubeconfig did not preserve inline credentials: %s", normalized)
	}

	restConfig, err := SafeRESTConfigFromKubeconfig(kubeconfig)
	if err != nil {
		t.Fatalf("SafeRESTConfigFromKubeconfig returned error: %v", err)
	}
	if restConfig.Host != "https://kubernetes.example.com:6443" {
		t.Fatalf("rest config host = %q", restConfig.Host)
	}
}

func TestSafeKubeconfigRejectsUnsafeFeatures(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*clientcmdapi.Config)
	}{
		{
			name: "exec credential plugin",
			mutate: func(config *clientcmdapi.Config) {
				config.AuthInfos["user"].Exec = &clientcmdapi.ExecConfig{Command: "sh", APIVersion: "client.authentication.k8s.io/v1"}
			},
		},
		{
			name: "auth provider",
			mutate: func(config *clientcmdapi.Config) {
				config.AuthInfos["user"].AuthProvider = &clientcmdapi.AuthProviderConfig{Name: "oidc"}
			},
		},
		{
			name: "token file",
			mutate: func(config *clientcmdapi.Config) {
				config.AuthInfos["user"].TokenFile = "/var/run/secrets/token"
			},
		},
		{
			name: "proxy URL",
			mutate: func(config *clientcmdapi.Config) {
				config.Clusters["cluster"].ProxyURL = "http://127.0.0.1:8080"
			},
		},
		{
			name: "certificate authority file",
			mutate: func(config *clientcmdapi.Config) {
				config.Clusters["cluster"].CertificateAuthority = "/etc/kubernetes/ca.crt"
			},
		},
		{
			name: "client certificate file",
			mutate: func(config *clientcmdapi.Config) {
				config.AuthInfos["user"].ClientCertificate = "/etc/kubernetes/client.crt"
			},
		},
		{
			name: "client key file",
			mutate: func(config *clientcmdapi.Config) {
				config.AuthInfos["user"].ClientKey = "/etc/kubernetes/client.key"
			},
		},
		{
			name: "HTTP API server",
			mutate: func(config *clientcmdapi.Config) {
				config.Clusters["cluster"].Server = "http://kubernetes.example.com:8080"
			},
		},
		{
			name: "disabled TLS verification",
			mutate: func(config *clientcmdapi.Config) {
				config.Clusters["cluster"].InsecureSkipTLSVerify = true
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := validSafeKubeconfig()
			test.mutate(config)
			kubeconfig := writeKubeconfigForTest(t, config)

			if _, err := NormalizeSafeKubeconfig(kubeconfig); !errors.Is(err, ErrUnsafeKubeconfig) {
				t.Fatalf("NormalizeSafeKubeconfig error = %v, want ErrUnsafeKubeconfig", err)
			}
			if _, err := SafeRESTConfigFromKubeconfig(kubeconfig); !errors.Is(err, ErrUnsafeKubeconfig) {
				t.Fatalf("SafeRESTConfigFromKubeconfig error = %v, want ErrUnsafeKubeconfig", err)
			}
		})
	}
}

func TestSafeKubeconfigRejectsMalformedContext(t *testing.T) {
	config := validSafeKubeconfig()
	config.CurrentContext = "missing"

	_, err := SafeRESTConfigFromKubeconfig(writeKubeconfigForTest(t, config))
	if err == nil {
		t.Fatal("expected malformed current context to be rejected")
	}
}

func validSafeKubeconfig() *clientcmdapi.Config {
	return &clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"cluster": {Server: "https://kubernetes.example.com:6443"},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"user": {Token: "inline-token"},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"context": {Cluster: "cluster", AuthInfo: "user"},
		},
		CurrentContext: "context",
	}
}

func writeKubeconfigForTest(t *testing.T, config *clientcmdapi.Config) string {
	t.Helper()
	output, err := clientcmd.Write(*config)
	if err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	return string(output)
}
