package kubeaccess

import (
	"errors"
	"net/url"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func ContextName(projectID, runtimeClusterID, applicationID string) string {
	tail := strings.TrimSpace(applicationID)
	if tail == "" {
		tail = "all"
	}
	return strings.Join([]string{"luna", strings.TrimSpace(projectID), strings.TrimSpace(runtimeClusterID), tail}, "/")
}

func RenderKubeconfig(publicBaseURL, credentialID, plaintext string, bindings []BindingSummary) (string, error) {
	base, err := url.Parse(strings.TrimSpace(publicBaseURL))
	if err != nil || base.Scheme == "" || base.Host == "" || (base.Scheme != "https" && base.Scheme != "http") || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return "", ErrPublicBaseURLRequired
	}
	if strings.TrimSpace(credentialID) == "" || strings.TrimSpace(plaintext) == "" || len(bindings) == 0 {
		return "", ErrInputInvalid
	}
	config := clientcmdapi.NewConfig()
	userName := "luna/" + strings.TrimSpace(credentialID)
	config.AuthInfos[userName] = &clientcmdapi.AuthInfo{Token: plaintext}
	for index, binding := range bindings {
		if strings.TrimSpace(binding.ID) == "" || strings.TrimSpace(binding.Namespace) == "" {
			return "", ErrContextInvalid
		}
		name := ContextName(binding.ProjectID, binding.RuntimeClusterID, binding.ApplicationID)
		server := strings.TrimRight(base.String(), "/") + "/kube/v1/bindings/" + url.PathEscape(binding.ID)
		config.Clusters[name] = &clientcmdapi.Cluster{Server: server}
		config.Contexts[name] = &clientcmdapi.Context{Cluster: name, AuthInfo: userName, Namespace: binding.Namespace}
		if index == 0 {
			config.CurrentContext = name
		}
	}
	content, err := clientcmd.Write(*config)
	if err != nil {
		return "", errors.Join(ErrInputInvalid, err)
	}
	return string(content), nil
}
