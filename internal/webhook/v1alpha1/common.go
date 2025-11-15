package v1alpha1

import (
	"fmt"
	"net"
	"regexp"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	otcv1alpha1 "github.com/peertech.de/otc-operator/api/v1alpha1"
)

var validName = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

func validateProviderConfigRefName(ref otcv1alpha1.ProviderConfigReference) *field.Error {
	if ref.Name == "" {
		return field.Required(
			field.NewPath("spec", "providerConfigRef", "name"),
			"name is required",
		)
	}
	return nil
}

func validateNetworkDependency(dep otcv1alpha1.NetworkDependency) error {
	count := 0
	if dep.NetworkID != nil {
		count++
		if *dep.NetworkID == "" {
			return fmt.Errorf("networkID cannot be empty")
		}
	}
	if dep.NetworkRef != nil {
		count++
		if err := validateObjectRef(*dep.NetworkRef); err != nil {
			return fmt.Errorf("networkRef: %w", err)
		}
	}
	if dep.NetworkSelector != nil {
		count++
		if err := validateLabelSelector(*dep.NetworkSelector); err != nil {
			return fmt.Errorf("networkSelector: %w", err)
		}
	}

	if count == 0 {
		return fmt.Errorf(
			"exactly one of networkID, networkRef or networkSelector must be specified",
		)
	}
	if count > 1 {
		return fmt.Errorf("only one of networkID, networkRef or networkSelector can be specified")
	}

	return nil
}

func validateSubnetDependency(dep otcv1alpha1.SubnetDependency) error {
	count := 0
	if dep.SubnetID != nil {
		count++
		if *dep.SubnetID == "" {
			return fmt.Errorf("subnetID cannot be empty")
		}
	}
	if dep.SubnetRef != nil {
		count++
		if err := validateObjectRef(*dep.SubnetRef); err != nil {
			return fmt.Errorf("subnetRef: %w", err)
		}
	}
	if dep.SubnetSelector != nil {
		count++
		if err := validateLabelSelector(*dep.SubnetSelector); err != nil {
			return fmt.Errorf("subnetSelector: %w", err)
		}
	}

	if count == 0 {
		return fmt.Errorf("exactly one of subnetID, subnetRef or subnetSelector must be specified")
	}
	if count > 1 {
		return fmt.Errorf("only one of subnetID, subnetRef or subnetSelector can be specified")
	}

	return nil
}

func validateSecurityGroupDependency(dep otcv1alpha1.SecurityGroupDependency) error {
	count := 0
	if dep.SecurityGroupID != nil {
		count++
		if *dep.SecurityGroupID == "" {
			return fmt.Errorf("securityGroupID cannot be empty")
		}
	}
	if dep.SecurityGroupRef != nil {
		count++
		if err := validateObjectRef(*dep.SecurityGroupRef); err != nil {
			return fmt.Errorf("securityGroupRef: %w", err)
		}
	}
	if dep.SecurityGroupSelector != nil {
		count++
		if err := validateLabelSelector(*dep.SecurityGroupSelector); err != nil {
			return fmt.Errorf("securityGroupSelector: %w", err)
		}
	}

	if count == 0 {
		return fmt.Errorf(
			"exactly one of securityGroupID, securityGroupRef or securityGroupSelector must be specified",
		)
	}
	if count > 1 {
		return fmt.Errorf(
			"only one of securityGroupID, securityGroupRef or securityGroupSelector can be specified",
		)
	}

	return nil
}

func validateNATGatewayDependency(dep otcv1alpha1.NATGatewayDependency) error {
	count := 0
	if dep.NATGatewayID != nil {
		count++
		if *dep.NATGatewayID == "" {
			return fmt.Errorf("natGatewayID cannot be empty")
		}
	}
	if dep.NATGatewayRef != nil {
		count++
		if err := validateObjectRef(*dep.NATGatewayRef); err != nil {
			return fmt.Errorf("natGatewayRef: %w", err)
		}
	}
	if dep.NATGatewaySelector != nil {
		count++
		if err := validateLabelSelector(*dep.NATGatewaySelector); err != nil {
			return fmt.Errorf("natGatewaySelector: %w", err)
		}
	}

	if count == 0 {
		return fmt.Errorf(
			"exactly one of natGatewayID, natGatewayRef or natGatewaySelector must be specified",
		)
	}
	if count > 1 {
		return fmt.Errorf(
			"only one of natGatewayID, natGatewayRef or natGatewaySelector can be specified",
		)
	}

	return nil
}

func validatePublicIPDependency(dep otcv1alpha1.PublicIPDependency) error {
	count := 0
	if dep.PublicIPID != nil {
		count++
		if *dep.PublicIPID == "" {
			return fmt.Errorf("publicIPID cannot be empty")
		}
	}
	if dep.PublicIPRef != nil {
		count++
		if err := validateObjectRef(*dep.PublicIPRef); err != nil {
			return fmt.Errorf("publicIPRef: %w", err)
		}
	}
	if dep.PublicIPSelector != nil {
		count++
		if err := validateLabelSelector(*dep.PublicIPSelector); err != nil {
			return fmt.Errorf("publicIPSelector: %w", err)
		}
	}

	if count == 0 {
		return fmt.Errorf(
			"exactly one of publicIPID, publicIPRef or publicIPSelector must be specified",
		)
	}
	if count > 1 {
		return fmt.Errorf(
			"only one of publicIPID, publicIPRef or publicIPSelector can be specified",
		)
	}

	return nil
}

func validateObjectRef(ref corev1.LocalObjectReference) error {
	if ref.Name == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}

func validateLabelSelector(selector metav1.LabelSelector) error {
	if len(selector.MatchLabels) == 0 {
		return fmt.Errorf("matchLabels cannot be empty")
	}
	for key, value := range selector.MatchLabels {
		if key == "" {
			return fmt.Errorf("label key cannot be empty")
		}
		if value == "" {
			return fmt.Errorf("label value for key %s cannot be empty", key)
		}
	}
	return nil
}

// validateCIDR validates that the CIDR is a valid IPv4 CIDR notation
func validateCIDR(cidr string) error {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("must be a valid IPv4 CIDR notation: %w", err)
	}

	// Ensure it's IPv4
	if ipNet.IP.To4() == nil {
		return fmt.Errorf("must be a valid IPv4 CIDR notation")
	}

	return nil
}

func validateSecretKeySelector(selector corev1.SecretKeySelector) error {
	if selector.Name == "" {
		return fmt.Errorf("secret name is required")
	}
	if selector.Key == "" {
		return fmt.Errorf("secret key is required")
	}
	return nil
}

func equalProviderConfigRef(a, b otcv1alpha1.ProviderConfigReference) bool {
	return a.Name == b.Name
}

func equalNetworkDependency(a, b otcv1alpha1.NetworkDependency) bool {
	return equalStringPtr(a.NetworkID, b.NetworkID) &&
		equalObjectRef(a.NetworkRef, b.NetworkRef) &&
		equalLabelSelector(a.NetworkSelector, b.NetworkSelector)
}

func equalSubnetDependency(a, b otcv1alpha1.SubnetDependency) bool {
	return equalStringPtr(a.SubnetID, b.SubnetID) &&
		equalObjectRef(a.SubnetRef, b.SubnetRef) &&
		equalLabelSelector(a.SubnetSelector, b.SubnetSelector)
}

func equalNATGatewayDependency(a, b otcv1alpha1.NATGatewayDependency) bool {
	return equalStringPtr(a.NATGatewayID, b.NATGatewayID) &&
		equalObjectRef(a.NATGatewayRef, b.NATGatewayRef) &&
		equalLabelSelector(a.NATGatewaySelector, b.NATGatewaySelector)
}

func equalPublicIPDependency(a, b otcv1alpha1.PublicIPDependency) bool {
	return equalStringPtr(a.PublicIPID, b.PublicIPID) &&
		equalObjectRef(a.PublicIPRef, b.PublicIPRef) &&
		equalLabelSelector(a.PublicIPSelector, b.PublicIPSelector)
}

func equalSecurityGroupDependency(a, b otcv1alpha1.SecurityGroupDependency) bool {
	return equalStringPtr(a.SecurityGroupID, b.SecurityGroupID) &&
		equalObjectRef(a.SecurityGroupRef, b.SecurityGroupRef) &&
		equalLabelSelector(a.SecurityGroupSelector, b.SecurityGroupSelector)
}

func equalStringPtr(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func equalObjectRef(a, b *corev1.LocalObjectReference) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Name == b.Name
}

func equalLabelSelector(a, b *metav1.LabelSelector) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if len(a.MatchLabels) != len(b.MatchLabels) {
		return false
	}
	for k, v := range a.MatchLabels {
		if b.MatchLabels[k] != v {
			return false
		}
	}
	return true
}

func equalPort(a, b *int32) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	if a != nil && b != nil {
		return *a == *b
	}
	return true
}
