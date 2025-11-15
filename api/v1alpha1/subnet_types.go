package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SubnetSpec defines the desired state of Subnet
type SubnetSpec struct {
	// ProviderConfigRef references the ProviderConfig to use for authentication
	// +kubebuilder:validation:Required
	ProviderConfigRef ProviderConfigReference `json:"providerConfigRef"`

	// Network defines the network dependency
	// +kubebuilder:validation:Required
	Network NetworkDependency `json:"network"`

	// Description is an optional human-readable description of the subnet
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=255
	Description string `json:"description,omitempty"`

	// Cidr is the IPv4 CIDR block for the subnet (e.g. "192.168.0.0/24")
	// +kubebuilder:validation:Required
	Cidr string `json:"cidr"`

	// GatewayIP is the IPv4 gateway IP for the subnet (e.g. "192.168.0.1")
	// +kubebuilder:validation:Required
	GatewayIP string `json:"gatewayIP"`

	// OrphanOnDelete prevents deletion of the external resource when the CR is deleted
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	OrphanOnDelete bool `json:"orphanOnDelete,omitempty"`
}

// SubnetNetworkResolved contains the resolved ID for network dependency
type SubnetDependencieskResolved struct {
	// NetworkID is the resolved Network ID
	NetworkID string `json:"networkID,omitempty"`
}

// SubnetStatus defines the observed state of Subnet.
type SubnetStatus struct {
	// Conditions represent the latest available observations of the Subnet's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ExternalID is the provider's ID for this Subnet
	// +optional
	ExternalID string `json:"externalID,omitempty"`

	// ResolvedDependencies contains the resolved ID for network dependency
	// +optional
	ResolvedDependencies SubnetDependencieskResolved `json:"resolvedDependencies"`

	// ObservedGeneration reflects the generation of the most recently observed Subnet spec
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastSyncTime is the timestamp of the last successful sync with the provider
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// LastAppliedSpec caches the spec that was successfully applied to the
	// external resource. It is used to detect changes to immutable fields.
	// +optional
	LastAppliedSpec *SubnetSpec `json:"lastAppliedSpec,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories=networking
// +kubebuilder:printcolumn:name="CIDR",type=string,JSONPath=`.spec.cidr`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="ExternalID",type=string,JSONPath=`.status.externalID`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Subnet is the Schema for the subnets API
type Subnet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	Spec   SubnetSpec   `json:"spec"`
	Status SubnetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SubnetList contains a list of Subnet
type SubnetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Subnet `json:"items"`
}

// GetItems returns the list of items as a slice of client.Object.
func (sl *SubnetList) GetItems() []client.Object {
	items := make([]client.Object, len(sl.Items))
	for i := range sl.Items {
		items[i] = &sl.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&Subnet{}, &SubnetList{})
}
