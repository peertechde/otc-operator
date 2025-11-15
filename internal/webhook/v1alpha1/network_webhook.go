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

// SetupNetworkWebhookWithManager registers the webhook for Network in the manager.
func SetupNetworkWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&otcv1alpha1.Network{}).
		WithValidator(&NetworkCustomValidator{}).
		Complete()
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:path=/validate-otc-peertech-de-v1alpha1-network,mutating=false,failurePolicy=fail,sideEffects=None,groups=otc.peertech.de,resources=networks,verbs=create;update,versions=v1alpha1,name=vnetwork-v1alpha1.kb.io,admissionReviewVersions=v1

// NetworkCustomValidator struct is responsible for validating the Network resource
// when it is created, updated, or deleted.
type NetworkCustomValidator struct{}

var _ webhook.CustomValidator = &NetworkCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Network.
func (v *NetworkCustomValidator) ValidateCreate(
	_ context.Context,
	obj runtime.Object,
) (admission.Warnings, error) {
	network, ok := obj.(*otcv1alpha1.Network)
	if !ok {
		return nil, fmt.Errorf("expected a Network object but got %T", obj)
	}

	var warnings admission.Warnings
	var errors field.ErrorList

	// Validate the resource name
	if !validName.MatchString(network.Name) {
		errors = append(errors, field.Invalid(
			field.NewPath("metadata", "name"),
			network.Name,
			"name must contain only letters, digits, underscores (_), hyphens (-), and periods (.)",
		))
	}

	// Validate ProviderConfigRef
	if name := network.Spec.ProviderConfigRef.Name; name == "" {
		errors = append(
			errors,
			field.Required(
				field.NewPath("spec", "providerConfigRef", "name"),
				"name is required",
			),
		)
	}

	// Validate CIDR format
	if err := validateCIDR(network.Spec.Cidr); err != nil {
		errors = append(
			errors,
			field.Invalid(
				field.NewPath("spec", "cidr"),
				network.Spec.Cidr,
				err.Error(),
			),
		)
	}

	// Warn about orphanOnDelete if true
	if network.Spec.OrphanOnDelete {
		warnings = append(
			warnings,
			"orphanOnDelete is true: external network will not be deleted when this resource is deleted",
		)
	}

	if len(errors) == 0 {
		return warnings, nil
	}

	return warnings, apierrors.NewInvalid(
		network.GroupVersionKind().GroupKind(),
		network.Name,
		errors,
	)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Network.
func (v *NetworkCustomValidator) ValidateUpdate(
	_ context.Context,
	oldObj, newObj runtime.Object,
) (admission.Warnings, error) {
	oldNetwork, ok := oldObj.(*otcv1alpha1.Network)
	if !ok {
		return nil, fmt.Errorf("expected a Network object for the oldObj but got %T", newObj)
	}
	newNetwork, ok := newObj.(*otcv1alpha1.Network)
	if !ok {
		return nil, fmt.Errorf("expected a Network object for the newObj but got %T", newObj)
	}

	var warnings admission.Warnings
	var errors field.ErrorList

	// Check immutable ProviderConfigRef
	if !equalProviderConfigRef(
		oldNetwork.Spec.ProviderConfigRef,
		newNetwork.Spec.ProviderConfigRef,
	) {
		errors = append(
			errors,
			field.Forbidden(
				field.NewPath("spec", "providerConfigRef"),
				"is immutable and cannot be changed after creation",
			),
		)
	}

	// Check immutable CIDR field
	if newNetwork.Spec.Cidr != oldNetwork.Spec.Cidr {
		errors = append(
			errors,
			field.Forbidden(
				field.NewPath("spec", "cidr"),
				"is immutable and cannot be changed after creation",
			),
		)
	}

	// Warn if orphanOnDelete is being changed from false to true
	if !oldNetwork.Spec.OrphanOnDelete && newNetwork.Spec.OrphanOnDelete {
		warnings = append(
			warnings,
			"orphanOnDelete changed to true: external network will not be deleted when this resource is deleted",
		)
	}

	// Warn if orphanOnDelete is being changed from true to false
	if oldNetwork.Spec.OrphanOnDelete && !newNetwork.Spec.OrphanOnDelete {
		warnings = append(
			warnings,
			"orphanOnDelete changed to false: external network will be deleted when this resource is deleted",
		)
	}

	if len(errors) == 0 {
		return warnings, nil
	}

	return warnings, apierrors.NewInvalid(
		oldNetwork.GroupVersionKind().GroupKind(),
		oldNetwork.Name,
		errors,
	)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Network.
func (v *NetworkCustomValidator) ValidateDelete(
	ctx context.Context,
	obj runtime.Object,
) (admission.Warnings, error) {
	return nil, nil
}
