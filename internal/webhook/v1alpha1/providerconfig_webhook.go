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

// SetupProviderConfigWebhookWithManager registers the webhook for ProviderConfig in the manager.
func SetupProviderConfigWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&otcv1alpha1.ProviderConfig{}).
		WithValidator(&ProviderConfigCustomValidator{}).
		Complete()
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:path=/validate-otc-peertech-de-v1alpha1-providerconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=otc.peertech.de,resources=providerconfigs,verbs=create;update,versions=v1alpha1,name=vproviderconfig-v1alpha1.kb.io,admissionReviewVersions=v1

// ProviderConfigCustomValidator struct is responsible for validating the ProviderConfig resource
// when it is created, updated, or deleted.
type ProviderConfigCustomValidator struct{}

var _ webhook.CustomValidator = &ProviderConfigCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ProviderConfig.
func (v *ProviderConfigCustomValidator) ValidateCreate(
	_ context.Context,
	obj runtime.Object,
) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ProviderConfig.
func (v *ProviderConfigCustomValidator) ValidateUpdate(
	_ context.Context,
	oldObj, newObj runtime.Object,
) (admission.Warnings, error) {
	oldProviderConfig, ok := oldObj.(*otcv1alpha1.ProviderConfig)
	if !ok {
		return nil, fmt.Errorf("expected a ProviderConfig object for the oldObj but got %T", newObj)
	}
	newProviderConfig, ok := newObj.(*otcv1alpha1.ProviderConfig)
	if !ok {
		return nil, fmt.Errorf("expected a ProviderConfig object for the newObj but got %T", newObj)
	}

	var warnings admission.Warnings
	var errors field.ErrorList

	// Check immutable IdentityEndpoint
	if newProviderConfig.Spec.IdentityEndpoint != oldProviderConfig.Spec.IdentityEndpoint {
		errors = append(
			errors,
			field.Forbidden(
				field.NewPath("spec", "identityEndpoint"),
				"is immutable and cannot be changed after creation",
			),
		)
	}

	// Check immutable Region
	if newProviderConfig.Spec.Region != oldProviderConfig.Spec.Region {
		errors = append(
			errors,
			field.Forbidden(
				field.NewPath("spec", "region"),
				"is immutable and cannot be changed after creation",
			),
		)
	}

	// Check immutable ProjectID
	if newProviderConfig.Spec.ProjectID != oldProviderConfig.Spec.ProjectID {
		errors = append(
			errors,
			field.Forbidden(
				field.NewPath("spec", "projectID"),
				"is immutable and cannot be changed after creation",
			),
		)
	}

	// Check immutable DomainName
	if newProviderConfig.Spec.DomainName != oldProviderConfig.Spec.DomainName {
		errors = append(
			errors,
			field.Forbidden(
				field.NewPath("spec", "domainName"),
				"is immutable and cannot be changed after creation",
			),
		)
	}

	if len(errors) == 0 {
		return warnings, nil
	}

	return warnings, apierrors.NewInvalid(
		oldProviderConfig.GroupVersionKind().GroupKind(),
		oldProviderConfig.Name,
		errors,
	)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ProviderConfig.
func (v *ProviderConfigCustomValidator) ValidateDelete(
	ctx context.Context,
	obj runtime.Object,
) (admission.Warnings, error) {
	return nil, nil
}
