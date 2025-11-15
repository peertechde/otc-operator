package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:XValidation:rule="(has(self.networkID)?1:0)+(has(self.networkRef)?1:0)+(has(self.networkSelector)?1:0)==1",message="exactly one of networkID, networkRef or networkSelector must be set"

// NetworkDependency specifies a dependency on a Network resource. Exactly one
// of NetworkID, NetworkRef or NetworkSelector must be specified.
type NetworkDependency struct {
	// NetworkID is the external provider ID of the Network
	// +optional
	NetworkID *string `json:"networkID,omitempty"`
	// NetworkRef is a reference to a Network resource
	// +optional
	NetworkRef *corev1.LocalObjectReference `json:"networkRef,omitempty"`
	// NetworkSelector selects a Network by labels
	// +optional
	NetworkSelector *metav1.LabelSelector `json:"networkSelector,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="(has(self.subnetID)?1:0)+(has(self.subnetRef)?1:0)+(has(self.subnetSelector)?1:0)==1",message="exactly one of subnetID, subnetRef or subnetSelector must be set"

// SubnetDependency specifies a dependency on a Subnet resource. Exactly one of
// SubnetID, SubnetRef or SubnetSelector must be specified.
type SubnetDependency struct {
	// SubnetID is the external provider ID of the subnet
	// +optional
	SubnetID *string `json:"subnetID,omitempty"`
	// SubnetRef is a reference to a Subnet resource
	// +optional
	SubnetRef *corev1.LocalObjectReference `json:"subnetRef,omitempty"`
	// SubnetSelector selects a Subnet by labels
	// +optional
	SubnetSelector *metav1.LabelSelector `json:"subnetSelector,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="(has(self.securityGroupID)?1:0)+(has(self.securityGroupRef)?1:0)+(has(self.securityGroupSelector)?1:0)==1",message="exactly one of securityGroupID, securityGroupRef or securityGroupSelector must be set"

// SecurityGroupDependency specifies a dependency on a SecurityGroup resource.
// Exactly one of SecurityGroupID, SecurityGroupRef or SecurityGroupSelector
// must be specified.
type SecurityGroupDependency struct {
	// SecurityGroupID is the external provider ID of the security group
	// +optional
	SecurityGroupID *string `json:"securityGroupID,omitempty"`
	// SecurityGroupRef is a reference to a SecurityGroup custom resource
	// +optional
	SecurityGroupRef *corev1.LocalObjectReference `json:"securityGroupRef,omitempty"`
	// SecurityGroupSelector selects a SecurityGroup by labels
	// +optional
	SecurityGroupSelector *metav1.LabelSelector `json:"securityGroupSelector,omitempty"`
}

// NATGatewayDependency specifies a dependency on a NATGateway resource. Exactly one of
// NATGatewayID, NATGatewayRef or NATGatewaySelector must be specified.
type NATGatewayDependency struct {
	// NATGatewayID is the external provider ID of the NAT gateway
	// +optional
	NATGatewayID *string `json:"natGatewayID,omitempty"`
	// NATGatewayRef is a reference to a NAT gateway resource
	// +optional
	NATGatewayRef *corev1.LocalObjectReference `json:"natGatewayRef,omitempty"`
	// NATGatewaySelector selects a NAT gateway by labels
	// +optional
	NATGatewaySelector *metav1.LabelSelector `json:"natGatewaySelector,omitempty"`
}

// PublicIPDependency specifies a dependency on a Public IP resource. Exactly one of
// PublicIPID, PublicIPRef or PublicIPSelector must be specified.
type PublicIPDependency struct {
	// PublicIPID is the external provider ID of the public IP
	// +optional
	PublicIPID *string `json:"publicIPID,omitempty"`
	// PublicIPRef is a reference to a public IP resource
	// +optional
	PublicIPRef *corev1.LocalObjectReference `json:"publicIPRef,omitempty"`
	// PublicIPSelector selects a public IP by labels
	// +optional
	PublicIPSelector *metav1.LabelSelector `json:"publicIPSelector,omitempty"`
}
