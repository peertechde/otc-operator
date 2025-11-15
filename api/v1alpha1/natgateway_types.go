package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:validation:Enum=micro;small;medium;large;extra-large
type NATGatewayType string

const (
	TypeMicro      NATGatewayType = "micro"
	TypeSmall      NATGatewayType = "small"
	TypeMedium     NATGatewayType = "medium"
	TypeLarge      NATGatewayType = "large"
	TypeExtraLarge NATGatewayType = "extra-large"
)

// NATGatewaySpec defines the desired state of NATGateway
type NATGatewaySpec struct {
	// ProviderConfigRef references the ProviderConfig to use for authentication
	// +kubebuilder:validation:Required
	ProviderConfigRef ProviderConfigReference `json:"providerConfigRef"`

	// Network defines the network dependency
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="network is immutable"
	Network NetworkDependency `json:"network"`

	// Subnet defines the subnet dependency
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="subnet is immutable"
	Subnet SubnetDependency `json:"subnet"`

	// Description is an optional human-readable description of the subnet
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=255
	Description string `json:"description,omitempty"`

	// Type is the NAT gateway type (micro, small, medium, large, extra-large)
	// +kubebuilder:validation:Required
	Type NATGatewayType `json:"type"`

	// OrphanOnDelete prevents deletion of the external resource when the CR is deleted
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	OrphanOnDelete bool `json:"orphanOnDelete,omitempty"`
}

// NATGatewayNetworkResolved contains the resolved IDs for network dependencies
type NATGatewayDependenciesResolved struct {
	// NetworkID is the resolved Network ID
	NetworkID string `json:"networkID,omitempty"`
	// SubnetID is the resolved Subnet ID
	SubnetID string `json:"subnetID,omitempty"`
}

// NATGatewayStatus defines the observed state of NATGateway.
type NATGatewayStatus struct {
	// Conditions represent the latest available observations of the NAT Gateway's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ExternalID is the provider's ID for this NAT gateway
	// +optional
	ExternalID string `json:"externalID,omitempty"`

	// ResolvedDependencies contains the resolved IDs for network dependencies
	// +optional
	ResolvedDependencies NATGatewayDependenciesResolved `json:"resolvedDependencies"`

	// ObservedGeneration reflects the generation of the most recently observed NATGateway spec
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastSyncTime is the timestamp of the last successful sync with the provider
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// LastAppliedSpec caches the spec that was successfully applied to the
	// external resource. It is used to detect changes to immutable fields.
	// +optional
	LastAppliedSpec *NATGatewaySpec `json:"lastAppliedSpec,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories=networking
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="ExternalID",type=string,JSONPath=`.status.externalID`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// NATGateway is the Schema for the natgateways API
type NATGateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	Spec   NATGatewaySpec   `json:"spec"`
	Status NATGatewayStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NATGatewayList contains a list of NATGateway
type NATGatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NATGateway `json:"items"`
}

// GetItems returns the list of items as a slice of client.Object.
func (ngl *NATGatewayList) GetItems() []client.Object {
	items := make([]client.Object, len(ngl.Items))
	for i := range ngl.Items {
		items[i] = &ngl.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&NATGateway{}, &NATGatewayList{})
}
