package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ProviderConfigReference struct {
	// Name of the ProviderConfig
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace of the ProviderConfig
	Namespace string `json:"namespace,omitempty"`
}

// ProviderConfigSpec defines the desired state of ProviderConfig
type ProviderConfigSpec struct {
	// IdentityEndpoint is the OpenStack identity/Keystone endpoint
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^https?://`
	IdentityEndpoint string `json:"identityEndpoint"`

	// Region is the OpenStack region
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Region string `json:"region"`

	// ProjectID is the OpenStack project/tenant ID
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ProjectID string `json:"projectID"`

	// DomainName is the OpenStack domain name
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	DomainName string `json:"domainName"`

	// CredentialsSecretRef references a Secret containing authentication details
	// The Secret should contain keys: username, password
	// +kubebuilder:validation:Required
	CredentialsSecretRef corev1.SecretReference `json:"credentialsSecretRef"`
}

// ProviderConfigStatus defines the observed state of ProviderConfig
type ProviderConfigStatus struct {
	// Conditions represent the latest available observations of the ProviderConfig's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the latest generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastValidationTime is when credentials were last validated
	// +optional
	LastValidationTime *metav1.Time `json:"lastValidationTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=pc;providerconfig,categories=provider
// +kubebuilder:printcolumn:name="Region",type=string,JSONPath=`.spec.region`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ProviderConfig is the Schema for the providerconfigs API
type ProviderConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	Spec   ProviderConfigSpec   `json:"spec"`
	Status ProviderConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProviderConfigList contains a list of ProviderConfig
type ProviderConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProviderConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ProviderConfig{}, &ProviderConfigList{})
}
