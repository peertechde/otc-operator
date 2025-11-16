package v1alpha1

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	otcv1alpha1 "github.com/peertech.de/otc-operator/api/v1alpha1"
)

// SetupNATGatewayWebhookWithManager registers the webhook for NATGateway in the manager.
func SetupNATGatewayWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&otcv1alpha1.NATGateway{}).
		WithValidator(&NATGatewayCustomValidator{}).
		Complete()
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:path=/validate-otc-peertech-de-v1alpha1-natgateway,mutating=false,failurePolicy=fail,sideEffects=None,groups=otc.peertech.de,resources=natgateways,verbs=create;update,versions=v1alpha1,name=vnatgateway-v1alpha1.kb.io,admissionReviewVersions=v1

// NATGatewayCustomValidator struct is responsible for validating the NATGateway resource
// when it is created, updated, or deleted.
type NATGatewayCustomValidator struct{}

var _ webhook.CustomValidator = &NATGatewayCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type NATGateway.
func (v *NATGatewayCustomValidator) ValidateCreate(
	_ context.Context,
	obj runtime.Object,
) (admission.Warnings, error) {
	natGateway, ok := obj.(*otcv1alpha1.NATGateway)
	if !ok {
		return nil, fmt.Errorf("expected a NATGateway object but got %T", obj)
	}

	var warnings admission.Warnings
	var errors field.ErrorList

	// Validate the resource name
	if !validName.MatchString(natGateway.Name) {
		errors = append(errors, field.Invalid(
			field.NewPath("metadata", "name"),
			natGateway.Name,
			"name must contain only letters, digits, underscores (_), hyphens (-), and periods (.)",
		))
	}

	// Validate ProviderConfigRef
	if name := natGateway.Spec.ProviderConfigRef.Name; name == "" {
		errors = append(
			errors,
			field.Required(
				field.NewPath("spec", "providerConfigRef", "name"),
				"name is required",
			),
		)
	}

	// Validate that exactly one network dependency method is specified
	if err := validateNetworkDependency(natGateway.Spec.Network); err != nil {
		errors = append(
			errors,
			field.Invalid(
				field.NewPath("spec", "network"),
				natGateway.Spec.Network,
				err.Error(),
			),
		)
	}

	// Validate that exactly one subnet dependency method is specified
	if err := validateSubnetDependency(natGateway.Spec.Subnet); err != nil {
		errors = append(
			errors,
			field.Invalid(
				field.NewPath("spec", "subnet"),
				natGateway.Spec.Subnet,
				err.Error(),
			),
		)
	}

	// Validate Type
	if natGateway.Spec.Type == "" {
		errors = append(
			errors,
			field.Required(
				field.NewPath("spec", "type"),
				"type is required",
			),
		)
	}

	// Warn about orphanOnDelete if true
	if natGateway.Spec.OrphanOnDelete {
		warnings = append(
			warnings,
			"orphanOnDelete is true: external NAT gateway will not be deleted when this resource is deleted",
		)
	}

	if len(errors) == 0 {
		return warnings, nil
	}

	return warnings, apierrors.NewInvalid(
		natGateway.GroupVersionKind().GroupKind(),
		natGateway.Name,
		errors,
	)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type NATGateway.
func (v *NATGatewayCustomValidator) ValidateUpdate(
	_ context.Context,
	oldObj, newObj runtime.Object,
) (admission.Warnings, error) {
	oldNATGateway, ok := oldObj.(*otcv1alpha1.NATGateway)
	if !ok {
		return nil, fmt.Errorf("expected a NATGateway object for the oldObj but got %T", newObj)
	}
	newNATGateway, ok := newObj.(*otcv1alpha1.NATGateway)
	if !ok {
		return nil, fmt.Errorf("expected a NATGateway object for the newObj but got %T", newObj)
	}

	var warnings admission.Warnings
	var errors field.ErrorList

	// Check immutable ProviderConfigRef
	if !equalProviderConfigRef(
		oldNATGateway.Spec.ProviderConfigRef,
		newNATGateway.Spec.ProviderConfigRef,
	) {
		errors = append(
			errors,
			field.Forbidden(
				field.NewPath("spec", "providerConfigRef"),
				"is immutable and cannot be changed after creation",
			),
		)
	}

	// Check immutable Network dependency
	if !equalNetworkDependency(oldNATGateway.Spec.Network, newNATGateway.Spec.Network) {
		errors = append(
			errors,
			field.Forbidden(
				field.NewPath("spec", "network"),
				"is immutable and cannot be changed after creation",
			),
		)
	}

	// Check immutable Subnet dependency
	if !equalSubnetDependency(oldNATGateway.Spec.Subnet, newNATGateway.Spec.Subnet) {
		errors = append(
			errors,
			field.Forbidden(
				field.NewPath("spec", "subnet"),
				"is immutable and cannot be changed after creation",
			),
		)
	}

	// Warn about type changes
	if oldNATGateway.Spec.Type != newNATGateway.Spec.Type {
		warnings = append(
			warnings,
			fmt.Sprintf(
				"changing type from %s to %s may cause service disruption",
				oldNATGateway.Spec.Type,
				newNATGateway.Spec.Type,
			),
		)
	}

	// Warn if orphanOnDelete is being changed from false to true
	if !oldNATGateway.Spec.OrphanOnDelete && newNATGateway.Spec.OrphanOnDelete {
		warnings = append(
			warnings,
			"orphanOnDelete changed to true: external NAT gateway will not be deleted when this resource is deleted",
		)
	}

	// Warn if orphanOnDelete is being changed from true to false
	if oldNATGateway.Spec.OrphanOnDelete && !newNATGateway.Spec.OrphanOnDelete {
		warnings = append(
			warnings,
			"orphanOnDelete changed to false: external NAT gateway will be deleted when this resource is deleted",
		)
	}

	if len(errors) == 0 {
		return warnings, nil
	}

	return warnings, apierrors.NewInvalid(
		oldNATGateway.GroupVersionKind().GroupKind(),
		oldNATGateway.Name,
		errors,
	)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type NATGateway.
func (v *NATGatewayCustomValidator) ValidateDelete(
	ctx context.Context,
	obj runtime.Object,
) (admission.Warnings, error) {
	return nil, nil
}
