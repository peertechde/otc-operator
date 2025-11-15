package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:validation:Enum=BGP;Mail
type PublicIPType string

const (
	PublicIPBGP  PublicIPType = "BGP"
	PublicIPMail PublicIPType = "Mail"
)

// +kubebuilder:validation:Enum=Dedicated;Shared
type PublicIPBandwidthShareType string

const (
	PublicIPBandwidthDedicated PublicIPBandwidthShareType = "Dedicated"
	PublicIPBandwidthShared    PublicIPBandwidthShareType = "Shared"
)

// PublicIPSpec defines the desired state of PublicIP
type PublicIPSpec struct {
	// ProviderConfigRef references the ProviderConfig to use for authentication
	// +kubebuilder:validation:Required
	ProviderConfigRef ProviderConfigReference `json:"providerConfigRef"`

	// Type is the public IP type (BGP or Mail)
	// +kubebuilder:validation:Required
	Type PublicIPType `json:"type"`

	// +kubebuilder:validation:Required
	BandwidthSize int `json:"bandwidthSize"`

	// +kubebuilder:validation:Required
	BandwidthShareType PublicIPBandwidthShareType `json:"bandwidthShareType"`

	// OrphanOnDelete prevents deletion of the external resource when the CR is deleted
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	OrphanOnDelete bool `json:"orphanOnDelete,omitempty"`
}

// PublicIPStatus defines the observed state of PublicIP.
type PublicIPStatus struct {
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
	LastAppliedSpec *PublicIPSpec `json:"lastAppliedSpec,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// PublicIP is the Schema for the publicips API
type PublicIP struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	Spec   PublicIPSpec   `json:"spec"`
	Status PublicIPStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PublicIPList contains a list of PublicIP
type PublicIPList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PublicIP `json:"items"`
}

// GetItems returns the list of items as a slice of client.Object.
func (pl *PublicIPList) GetItems() []client.Object {
	items := make([]client.Object, len(pl.Items))
	for i := range pl.Items {
		items[i] = &pl.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&PublicIP{}, &PublicIPList{})
}
