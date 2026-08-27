package kubernetes

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/LiteyukiStudio/devops/internal/provider/networkpolicy"
)

func (c *Client) EnsureBuildPolicy(ctx context.Context, buildPolicy networkpolicy.BuildPolicy) error {
	if err := validateBuildPolicy(buildPolicy); err != nil {
		return err
	}

	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildPolicy.Name,
			Namespace: buildPolicy.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "luna-devops",
				"luna.devops/scope":            "build",
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: buildPolicy.PodLabels,
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
			Egress: kubernetesEgressRules(buildPolicy.Egress),
		},
	}

	policies := c.client.NetworkingV1().NetworkPolicies(buildPolicy.Namespace)
	existing, err := policies.Get(ctx, buildPolicy.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = policies.Create(ctx, policy, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}

	existing.Labels = policy.Labels
	existing.Spec = policy.Spec
	_, err = policies.Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func validateBuildPolicy(policy networkpolicy.BuildPolicy) error {
	if errs := validation.IsDNS1123Label(strings.TrimSpace(policy.Name)); len(errs) > 0 {
		return fmt.Errorf("invalid network policy name %q: %s", policy.Name, strings.Join(errs, "; "))
	}
	if errs := validation.IsDNS1123Label(strings.TrimSpace(policy.Namespace)); len(errs) > 0 {
		return fmt.Errorf("invalid build namespace %q: %s", policy.Namespace, strings.Join(errs, "; "))
	}
	if len(policy.PodLabels) == 0 {
		return fmt.Errorf("network policy pod selector is required")
	}
	return nil
}

func tcpPort(port int) networkingv1.NetworkPolicyPort {
	return networkingv1.NetworkPolicyPort{
		Protocol: ptrProtocol("TCP"),
		Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: int32(port)},
	}
}

func udpPort(port int) networkingv1.NetworkPolicyPort {
	return networkingv1.NetworkPolicyPort{
		Protocol: ptrProtocol("UDP"),
		Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: int32(port)},
	}
}

func kubernetesEgressRules(rules []networkpolicy.EgressRule) []networkingv1.NetworkPolicyEgressRule {
	result := make([]networkingv1.NetworkPolicyEgressRule, 0, len(rules))
	for _, rule := range rules {
		ports := make([]networkingv1.NetworkPolicyPort, 0, len(rule.Ports))
		for _, port := range rule.Ports {
			switch strings.ToUpper(strings.TrimSpace(port.Protocol)) {
			case "UDP":
				ports = append(ports, udpPort(port.Number))
			default:
				ports = append(ports, tcpPort(port.Number))
			}
		}
		peers := make([]networkingv1.NetworkPolicyPeer, 0, len(rule.To))
		for _, peer := range rule.To {
			kubePeer := networkingv1.NetworkPolicyPeer{}
			if strings.TrimSpace(peer.CIDR) != "" {
				kubePeer.IPBlock = &networkingv1.IPBlock{
					CIDR:   strings.TrimSpace(peer.CIDR),
					Except: peer.Except,
				}
			}
			if len(peer.NamespaceLabels) > 0 {
				kubePeer.NamespaceSelector = &metav1.LabelSelector{MatchLabels: peer.NamespaceLabels}
			}
			if len(peer.PodLabels) > 0 {
				kubePeer.PodSelector = &metav1.LabelSelector{MatchLabels: peer.PodLabels}
			}
			if kubePeer.IPBlock == nil && kubePeer.NamespaceSelector == nil && kubePeer.PodSelector == nil {
				continue
			}
			peers = append(peers, kubePeer)
		}
		result = append(result, networkingv1.NetworkPolicyEgressRule{To: peers, Ports: ports})
	}
	return result
}

func ptrProtocol(value string) *v1Protocol {
	protocol := v1Protocol(value)
	return &protocol
}

type v1Protocol = corev1.Protocol
