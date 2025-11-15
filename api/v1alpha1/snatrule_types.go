package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SNATRuleSpec defines the desired state of SNATRule
type SNATRuleSpec struct {
	// ProviderConfigRef references the ProviderConfig to use for authentication
	// +kubebuilder:validation:Required
	ProviderConfigRef ProviderConfigReference `json:"providerConfigRef"`

	// NATGateway defines the NAT gateway dependency
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="NAT gateway is immutable"
	NATGateway NATGatewayDependency `json:"natGateway"`

	// Subnet defines the subnet dependency
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="subnet is immutable"
	Subnet SubnetDependency `json:"subnet"`

	// PublicIP defines the public IP dependency
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="public IP is immutable"
	PublicIP PublicIPDependency `json:"publicIP"`

	// Description is an optional human-readable description of the subnet
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=255
	Description string `json:"description,omitempty"`

	// OrphanOnDelete prevents deletion of the external resource when the CR is deleted
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	OrphanOnDelete bool `json:"orphanOnDelete,omitempty"`
}

// NATGatewayNetworkResolved contains the resolved IDs for network dependencies
type SNATRuleDependenciesResolved struct {
	// NATGatewayID is the resolved NAT gateway ID
	NATGatewayID string `json:"natGatewayID,omitempty"`

	// SubnetID is the resolved Subnet ID
	SubnetID string `json:"subnetID,omitempty"`

	// PublicIPID is the resolved Public IP ID
	PublicIPID string `json:"publicIPID,omitempty"`
}

// SNATRuleStatus defines the observed state of SNATRule.
type SNATRuleStatus struct {
	// Conditions represent the latest available observations of the NAT Gateway's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ExternalID is the provider's ID for this NAT gateway
	// +optional
	ExternalID string `json:"externalID,omitempty"`

	// ResolvedDependencies contains the resolved IDs for network dependencies
	// +optional
	ResolvedDependencies SNATRuleDependenciesResolved `json:"resolvedDependencies"`

	// ObservedGeneration reflects the generation of the most recently observed NATGateway spec
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastSyncTime is the timestamp of the last successful sync with the provider
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// LastAppliedSpec caches the spec that was successfully applied to the
	// external resource. It is used to detect changes to immutable fields.
	// +optional
	LastAppliedSpec *SNATRuleSpec `json:"lastAppliedSpec,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// SNATRule is the Schema for the snatrules API
type SNATRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	Spec   SNATRuleSpec   `json:"spec"`
	Status SNATRuleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SNATRuleList contains a list of SNATRule
type SNATRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SNATRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SNATRule{}, &SNATRuleList{})
}
