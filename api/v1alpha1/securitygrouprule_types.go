package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Enum=ingress;egress
type SecurityGroupRuleDirection string

const (
	DirectionIngress SecurityGroupRuleDirection = "ingress"
	DirectionEgress  SecurityGroupRuleDirection = "egress"
)

// +kubebuilder:validation:Enum=all;icmp;tcp;udp
type SecurityGroupRuleProtocol string

const (
	ProtocolAll  SecurityGroupRuleProtocol = "all"
	ProtocolICMP SecurityGroupRuleProtocol = "icmp"
	ProtocolTCP  SecurityGroupRuleProtocol = "tcp"
	ProtocolUDP  SecurityGroupRuleProtocol = "udp"
)

// +kubebuilder:validation:Enum=IPv4;IPv6
type SecurityGroupRuleEthertype string

const (
	EthertypeIPv4 SecurityGroupRuleEthertype = "IPv4"
	EthertypeIPv6 SecurityGroupRuleEthertype = "IPv6"
)

// +kubebuilder:validation:Enum=allow;deny
type SecurityGroupRuleAction string

const (
	ActionAllow SecurityGroupRuleAction = "allow"
	ActionDeny  SecurityGroupRuleAction = "deny"
)

// SecurityGroupRuleSpec defines the desired state of SecurityGroupRule
type SecurityGroupRuleSpec struct {
	// ProviderConfigRef references the ProviderConfig to use for authentication
	// +kubebuilder:validation:Required
	ProviderConfigRef ProviderConfigReference `json:"providerConfigRef"`

	// SecurityGroup defines the security group dependency
	// +kubebuilder:validation:Required
	SecurityGroup SecurityGroupDependency `json:"securityGroup"`

	// Description is an optional human-readable description of the security group rule
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=255
	Description string `json:"description,omitempty"`

	// Direction specifies whether the rule applies to ingress or egress traffic
	// +kubebuilder:validation:Required
	Direction SecurityGroupRuleDirection `json:"direction"`

	// Protocol specifies the network protocol
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=all
	Protocol SecurityGroupRuleProtocol `json:"protocol,omitempty"`

	// Ethertype specifies the IP version
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=IPv4
	Ethertype SecurityGroupRuleEthertype `json:"ethertype,omitempty"`

	// Multiport specifies port ranges (e.g. "80,443" or "8000-9000")
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern=`^[0-9,-]+$`
	// +kubebuilder:validation:MaxLength=255
	Multiport string `json:"multiport,omitempty"`

	// Action specifies whether to allow or deny traffic
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=allow
	Action SecurityGroupRuleAction `json:"action,omitempty"`

	// Priority defines the rule priority
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	Priority *int `json:"priority,omitempty"`

	// OrphanOnDelete prevents deletion of the external resource when the CR is deleted
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	OrphanOnDelete bool `json:"orphanOnDelete,omitempty"`
}

// SecurityGroupResolved contains the resolved ID for security group dependency
type SecurityGroupResolved struct {
	// SecurityGroupID is the resolved Security Group ID
	// +optional
	SecurityGroupID string `json:"securityGroupID,omitempty"`
}

// SecurityGroupRuleStatus defines the observed state of SecurityGroupRule.
type SecurityGroupRuleStatus struct {
	// Conditions represent the latest available observations of the Security Group Rule's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ExternalID is the provider's ID for this Subnet
	// +optional
	ExternalID string `json:"externalID,omitempty"`

	// ResolvedDependencies contains the resolved ID for security group dependency
	// +optional
	ResolvedDependencies SecurityGroupResolved `json:"resolvedDependencies"`

	// ObservedGeneration reflects the generation of the most recently observed Subnet spec
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastSyncTime is the timestamp of the last successful sync with the provider
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// LastAppliedSpec caches the spec that was successfully applied to the
	// external resource. It is used to detect changes to immutable fields.
	// +optional
	LastAppliedSpec *SecurityGroupRuleSpec `json:"lastAppliedSpec,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories=networking
// +kubebuilder:printcolumn:name="Direction",type=string,JSONPath=`.spec.direction`
// +kubebuilder:printcolumn:name="Protocol",type=string,JSONPath=`.spec.protocol`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="ExternalID",type=string,JSONPath=`.status.externalID`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SecurityGroupRule is the Schema for the securitygrouprules API
type SecurityGroupRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	Spec   SecurityGroupRuleSpec   `json:"spec"`
	Status SecurityGroupRuleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SecurityGroupRuleList contains a list of SecurityGroupRule
type SecurityGroupRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SecurityGroupRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SecurityGroupRule{}, &SecurityGroupRuleList{})
}
