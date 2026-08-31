package kubernetes

import (
	"context"
	"fmt"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

type GatewayTokenRequestOptions struct {
	Audiences  []string
	Expiration time.Duration
}

func (m *KubectlGatewayManager) RequestGatewayServiceAccountToken(ctx context.Context, options GatewayTokenRequestOptions) (string, time.Time, error) {
	if m == nil || m.client == nil || m.client.client == nil {
		return "", time.Time{}, fmt.Errorf("kubectl gateway client is unavailable")
	}
	expiration := options.Expiration
	if expiration <= 0 || expiration > 10*time.Minute {
		expiration = 10 * time.Minute
	}
	expirationSeconds := int64(expiration / time.Second)
	request, err := m.client.client.CoreV1().ServiceAccounts(KubectlGatewaySystemNamespaceName).CreateToken(ctx, KubectlGatewayServiceAccountName, &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			Audiences:         normalizeStringSlice(options.Audiences),
			ExpirationSeconds: &expirationSeconds,
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return "", time.Time{}, err
	}
	return request.Status.Token, request.Status.ExpirationTimestamp.Time, nil
}

func (m *KubectlGatewayManager) BuildGatewayProxyRESTConfig(ctx context.Context, options GatewayTokenRequestOptions) (*rest.Config, error) {
	if m == nil || m.client == nil || m.client.restConfig == nil {
		return nil, fmt.Errorf("kubectl gateway rest config is unavailable")
	}
	token, _, err := m.RequestGatewayServiceAccountToken(ctx, options)
	if err != nil {
		return nil, err
	}
	config := rest.AnonymousClientConfig(m.client.restConfig)
	config.BearerToken = token
	config.BearerTokenFile = ""
	config.Username = ""
	config.Password = ""
	config.CertData = nil
	config.KeyData = nil
	config.CertFile = ""
	config.KeyFile = ""
	config.Impersonate = rest.ImpersonationConfig{}
	return InstrumentRESTConfig(config), nil
}

func (m *KubectlGatewayManager) NewGatewayProxyClient(ctx context.Context, options GatewayTokenRequestOptions) (*Client, error) {
	config, err := m.BuildGatewayProxyRESTConfig(ctx, options)
	if err != nil {
		return nil, err
	}
	return NewClientForConfig(config)
}
