package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NetworkSpec defines the desired state of Network
type NetworkSpec struct {
	// ProviderConfigRef references the ProviderConfig to use for authentication
	// +kubebuilder:validation:Required
	ProviderConfigRef ProviderConfigReference `json:"providerConfigRef"`

	// Description is an optional human-readable description of the network
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=255
	Description string `json:"description,omitempty"`

	// Cidr is the IPv4 CIDR block for the network (e.g. "192.168.0.0/24")
	// +kubebuilder:validation:Required
	Cidr string `json:"cidr"`

	// OrphanOnDelete prevents deletion of the external resource when the CR is deleted
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	OrphanOnDelete bool `json:"orphanOnDelete,omitempty"`
}

// NetworkStatus defines the observed state of Network.
type NetworkStatus struct {
	// Conditions represent the latest available observations of the Network's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ExternalID is the provider's ID for this Network
	// +optional
	ExternalID string `json:"externalID,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed Network spec
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastSyncTime is the timestamp of the last successful sync with the provider
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// LastAppliedSpec caches the spec that was successfully applied to the
	// external resource. It is used to detect changes to immutable fields.
	// +optional
	LastAppliedSpec *NetworkSpec `json:"lastAppliedSpec,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories=network
// +kubebuilder:printcolumn:name="CIDR",type=string,JSONPath=`.spec.cidr`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="ExternalID",type=string,JSONPath=`.status.externalID`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Network is the Schema for the networks API
type Network struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	Spec   NetworkSpec   `json:"spec"`
	Status NetworkStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NetworkList contains a list of Network
type NetworkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Network `json:"items"`
}

// GetItems returns the list of items as a slice of client.Object.
func (nl *NetworkList) GetItems() []client.Object {
	items := make([]client.Object, len(nl.Items))
	for i := range nl.Items {
		items[i] = &nl.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&Network{}, &NetworkList{})
}
