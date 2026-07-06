package kubernetes

import (
	"context"
	"fmt"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/provider/networkpolicy"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type NamespaceManager interface {
	Ping(ctx context.Context) error
	EnsureNamespace(ctx context.Context, name string, labels map[string]string) error
	EnsureBuildNetworkPolicy(ctx context.Context, spec BuildNetworkPolicySpec) error
	EnsureBuildPolicy(ctx context.Context, policy networkpolicy.BuildPolicy) error
	ApplyGatewayTrafficProbe(ctx context.Context, spec GatewayTrafficProbeSpec) error
	EnsureGatewayTrafficProbeAccess(ctx context.Context, spec GatewayTrafficProbeSpec) error
	ApplyApplicationRuntimeConfig(ctx context.Context, spec ApplicationResourcesSpec) error
	ApplyApplicationResources(ctx context.Context, spec ApplicationResourcesSpec) error
	RunHookJob(ctx context.Context, spec HookJobSpec) (HookJobResult, error)
	GetDeploymentSnapshot(ctx context.Context, namespace, name string) (DeploymentSnapshot, error)
	DetectGatewayAPISupport(ctx context.Context) error
	EnsureGateway(ctx context.Context, spec GatewaySpec) error
	ApplyHTTPRoute(ctx context.Context, spec HTTPRouteSpec) error
	DeleteHTTPRoute(ctx context.Context, namespace, name string) error
	GetHTTPRouteStatus(ctx context.Context, namespace, name string) (HTTPRouteStatusSnapshot, error)
	GetServiceBackendSnapshot(ctx context.Context, namespace, name string, servicePort int32) (ServiceBackendSnapshot, error)
	ApplyCertificate(ctx context.Context, spec CertificateSpec) error
	GetCertificateSnapshot(ctx context.Context, namespace, name string) (CertificateSnapshot, error)
	ListManagedResources(ctx context.Context, options ResourceListOptions) ([]ResourceSnapshot, error)
	DeleteManagedResource(ctx context.Context, kind string, namespace string, name string) error
}

type Client struct {
	client     clientset.Interface
	dynamic    dynamic.Interface
	restConfig *rest.Config
}

func NewClientFromKubeconfig(kubeconfig string) (*Client, error) {
	config, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
	if err != nil {
		return nil, err
	}
	return NewClientForConfig(config)
}

func NewClientForConfig(config *rest.Config) (*Client, error) {
	client, err := clientset.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return &Client{client: client, dynamic: dynamicClient, restConfig: rest.CopyConfig(config)}, nil
}

func NewClientForInterface(client clientset.Interface) *Client {
	return &Client{client: client}
}

func NewClientForInterfaces(client clientset.Interface, dynamicClient dynamic.Interface) *Client {
	return &Client{client: client, dynamic: dynamicClient}
}

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.client.Discovery().ServerVersion()
	return err
}

func (c *Client) EnsureNamespace(ctx context.Context, name string, labels map[string]string) error {
	name = strings.TrimSpace(name)
	if errs := validation.IsDNS1123Label(name); len(errs) > 0 {
		return fmt.Errorf("invalid namespace %q: %s", name, strings.Join(errs, "; "))
	}

	namespaces := c.client.CoreV1().Namespaces()
	existing, err := namespaces.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = namespaces.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: labels,
			},
		}, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}

	changed := false
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	for key, value := range labels {
		if strings.TrimSpace(key) == "" || existing.Labels[key] == value {
			continue
		}
		existing.Labels[key] = value
		changed = true
	}
	if !changed {
		return nil
	}
	_, err = namespaces.Update(ctx, existing, metav1.UpdateOptions{})
	return err
}
