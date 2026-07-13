package kubernetes

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var ErrUnsafeKubeconfig = errors.New("unsafe kubeconfig")

// NormalizeSafeKubeconfig validates a user-supplied kubeconfig before any
// operation that could read local files or execute credential plugins.
func NormalizeSafeKubeconfig(kubeconfig string) (string, error) {
	config, err := loadSafeKubeconfig(kubeconfig)
	if err != nil {
		return "", err
	}
	if err := clientcmdapi.FlattenConfig(config); err != nil {
		return "", fmt.Errorf("normalize kubeconfig: %w", err)
	}
	output, err := clientcmd.Write(*config)
	if err != nil {
		return "", fmt.Errorf("serialize kubeconfig: %w", err)
	}
	return string(output), nil
}

// SafeRESTConfigFromKubeconfig applies the same validation at runtime so
// unsafe legacy or externally-written secrets cannot bypass API validation.
func SafeRESTConfigFromKubeconfig(kubeconfig string) (*rest.Config, error) {
	config, err := loadSafeKubeconfig(kubeconfig)
	if err != nil {
		return nil, err
	}
	return clientcmd.NewNonInteractiveClientConfig(
		*config,
		config.CurrentContext,
		&clientcmd.ConfigOverrides{},
		nil,
	).ClientConfig()
}

func loadSafeKubeconfig(kubeconfig string) (*clientcmdapi.Config, error) {
	config, err := clientcmd.Load([]byte(kubeconfig))
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}
	if err := validateSafeKubeconfig(config); err != nil {
		return nil, err
	}
	if err := clientcmd.Validate(*config); err != nil {
		return nil, fmt.Errorf("validate kubeconfig: %w", err)
	}
	return config, nil
}

func validateSafeKubeconfig(config *clientcmdapi.Config) error {
	if config == nil {
		return fmt.Errorf("%w: configuration is empty", ErrUnsafeKubeconfig)
	}
	for name, cluster := range config.Clusters {
		if cluster == nil {
			return unsafeKubeconfigField("cluster %q is empty", name)
		}
		if strings.TrimSpace(cluster.CertificateAuthority) != "" {
			return unsafeKubeconfigField("cluster %q uses certificate-authority file", name)
		}
		if strings.TrimSpace(cluster.ProxyURL) != "" {
			return unsafeKubeconfigField("cluster %q uses proxy-url", name)
		}
		if cluster.InsecureSkipTLSVerify {
			return unsafeKubeconfigField("cluster %q disables TLS verification", name)
		}
		if err := validateKubernetesServerURL(name, cluster.Server); err != nil {
			return err
		}
	}
	for name, authInfo := range config.AuthInfos {
		if authInfo == nil {
			return unsafeKubeconfigField("user %q is empty", name)
		}
		if authInfo.Exec != nil {
			return unsafeKubeconfigField("user %q uses exec credential plugin", name)
		}
		if authInfo.AuthProvider != nil {
			return unsafeKubeconfigField("user %q uses auth-provider", name)
		}
		if strings.TrimSpace(authInfo.TokenFile) != "" {
			return unsafeKubeconfigField("user %q uses tokenFile", name)
		}
		if strings.TrimSpace(authInfo.ClientCertificate) != "" {
			return unsafeKubeconfigField("user %q uses client-certificate file", name)
		}
		if strings.TrimSpace(authInfo.ClientKey) != "" {
			return unsafeKubeconfigField("user %q uses client-key file", name)
		}
	}
	return nil
}

func validateKubernetesServerURL(clusterName string, rawURL string) error {
	serverURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || serverURL.Scheme != "https" || serverURL.Host == "" {
		return unsafeKubeconfigField("cluster %q must use a valid HTTPS API server", clusterName)
	}
	if serverURL.User != nil || serverURL.RawQuery != "" || serverURL.Fragment != "" {
		return unsafeKubeconfigField("cluster %q API server URL contains unsupported components", clusterName)
	}
	return nil
}

func unsafeKubeconfigField(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrUnsafeKubeconfig, fmt.Sprintf(format, args...))
}
