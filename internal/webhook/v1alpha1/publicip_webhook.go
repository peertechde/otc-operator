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

// SetupPublicIPWebhookWithManager registers the webhook for PublicIP in the manager.
func SetupPublicIPWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&otcv1alpha1.PublicIP{}).
		WithValidator(&PublicIPCustomValidator{}).
		Complete()
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:path=/validate-otc-peertech-de-v1alpha1-publicip,mutating=false,failurePolicy=fail,sideEffects=None,groups=otc.peertech.de,resources=publicips,verbs=create;update,versions=v1alpha1,name=vpublicip-v1alpha1.kb.io,admissionReviewVersions=v1

// PublicIPCustomValidator struct is responsible for validating the PublicIP resource
// when it is created, updated, or deleted.
type PublicIPCustomValidator struct{}

var _ webhook.CustomValidator = &PublicIPCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type PublicIP.
func (v *PublicIPCustomValidator) ValidateCreate(
	_ context.Context,
	obj runtime.Object,
) (admission.Warnings, error) {
	publicIP, ok := obj.(*otcv1alpha1.PublicIP)
	if !ok {
		return nil, fmt.Errorf("expected a PublicIP object but got %T", obj)
	}

	var warnings admission.Warnings
	var errors field.ErrorList

	// Validate the resource name
	if !validName.MatchString(publicIP.Name) {
		errors = append(errors, field.Invalid(
			field.NewPath("metadata", "name"),
			publicIP.Name,
			"name must contain only letters, digits, underscores (_), hyphens (-), and periods (.)",
		))
	}

	// Validate ProviderConfigRef
	if err := validateProviderConfigRefName(publicIP.Spec.ProviderConfigRef); err != nil {
		errors = append(errors, err)
	}

	// Warn about orphanOnDelete if true
	if publicIP.Spec.OrphanOnDelete {
		warnings = append(
			warnings,
			"orphanOnDelete is true: external public IP will not be deleted when this resource is deleted",
		)
	}

	if len(errors) == 0 {
		return warnings, nil
	}

	return warnings, apierrors.NewInvalid(
		publicIP.GroupVersionKind().GroupKind(),
		publicIP.Name,
		errors,
	)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type PublicIP.
func (v *PublicIPCustomValidator) ValidateUpdate(
	_ context.Context,
	oldObj, newObj runtime.Object,
) (admission.Warnings, error) {
	oldPublicIP, ok := oldObj.(*otcv1alpha1.PublicIP)
	if !ok {
		return nil, fmt.Errorf("expected a PublicIP object for the oldObj but got %T", oldObj)
	}
	newPublicIP, ok := newObj.(*otcv1alpha1.PublicIP)
	if !ok {
		return nil, fmt.Errorf("expected a PublicIP object for the newObj but got %T", newObj)
	}

	var warnings admission.Warnings
	var errors field.ErrorList

	// Check immutable ProviderConfigRef
	if !equalProviderConfigRef(
		oldPublicIP.Spec.ProviderConfigRef,
		newPublicIP.Spec.ProviderConfigRef,
	) {
		errors = append(
			errors,
			field.Forbidden(
				field.NewPath("spec", "providerConfigRef"),
				"is immutable and cannot be changed after creation",
			),
		)
	}

	// Check immutable type field
	if newPublicIP.Spec.Type != oldPublicIP.Spec.Type {
		errors = append(
			errors,
			field.Forbidden(
				field.NewPath("spec", "type"),
				"is immutable and cannot be changed after creation",
			),
		)
	}

	// TODO: make mutable
	// Check immutable bandwidth size field
	if newPublicIP.Spec.BandwidthSize != oldPublicIP.Spec.BandwidthSize {
		errors = append(
			errors,
			field.Forbidden(
				field.NewPath("spec", "bandwidthSize"),
				"is immutable and cannot be changed after creation",
			),
		)
	}

	// Check immutable bandwidth share type field
	if newPublicIP.Spec.BandwidthShareType != oldPublicIP.Spec.BandwidthShareType {
		errors = append(
			errors,
			field.Forbidden(
				field.NewPath("spec", "bandwidthShareType"),
				"is immutable and cannot be changed after creation",
			),
		)
	}

	// Warn if orphanOnDelete is being changed from false to true
	if !oldPublicIP.Spec.OrphanOnDelete && newPublicIP.Spec.OrphanOnDelete {
		warnings = append(
			warnings,
			"orphanOnDelete changed to true: external public IP will not be deleted when this resource is deleted",
		)
	}

	// Warn if orphanOnDelete is being changed from true to false
	if oldPublicIP.Spec.OrphanOnDelete && !newPublicIP.Spec.OrphanOnDelete {
		warnings = append(
			warnings,
			"orphanOnDelete changed to false: external public IP will be deleted when this resource is deleted",
		)
	}

	if len(errors) == 0 {
		return warnings, nil
	}

	return warnings, apierrors.NewInvalid(
		oldPublicIP.GroupVersionKind().GroupKind(),
		oldPublicIP.Name,
		errors,
	)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type PublicIP.
func (v *PublicIPCustomValidator) ValidateDelete(
	ctx context.Context,
	obj runtime.Object,
) (admission.Warnings, error) {
	return nil, nil
}
