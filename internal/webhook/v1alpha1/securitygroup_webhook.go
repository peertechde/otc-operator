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

// SetupSecurityGroupWebhookWithManager registers the webhook for SecurityGroup in the manager.
func SetupSecurityGroupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&otcv1alpha1.SecurityGroup{}).
		WithValidator(&SecurityGroupCustomValidator{}).
		Complete()
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:path=/validate-otc-peertech-de-v1alpha1-securitygroup,mutating=false,failurePolicy=fail,sideEffects=None,groups=otc.peertech.de,resources=securitygroups,verbs=create;update,versions=v1alpha1,name=vsecuritygroup-v1alpha1.kb.io,admissionReviewVersions=v1

// SecurityGroupCustomValidator struct is responsible for validating the SecurityGroup resource
// when it is created, updated, or deleted.
type SecurityGroupCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &SecurityGroupCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type SecurityGroup.
func (v *SecurityGroupCustomValidator) ValidateCreate(
	_ context.Context,
	obj runtime.Object,
) (admission.Warnings, error) {
	securityGroup, ok := obj.(*otcv1alpha1.SecurityGroup)
	if !ok {
		return nil, fmt.Errorf("expected a SecurityGroup object but got %T", obj)
	}

	var warnings admission.Warnings
	var errors field.ErrorList

	// Validate the resource name
	if !validName.MatchString(securityGroup.Name) {
		errors = append(errors, field.Invalid(
			field.NewPath("metadata", "name"),
			securityGroup.Name,
			"name must contain only letters, digits, underscores (_), hyphens (-), and periods (.)",
		))
	}

	// Validate ProviderConfigRef
	if err := validateProviderConfigRefName(securityGroup.Spec.ProviderConfigRef); err != nil {
		errors = append(errors, err)
	}

	// Warn about orphanOnDelete if true
	if securityGroup.Spec.OrphanOnDelete {
		warnings = append(
			warnings,
			"orphanOnDelete is true: external security group will not be deleted when this resource is deleted",
		)
	}

	if len(errors) == 0 {
		return warnings, nil
	}

	return warnings, apierrors.NewInvalid(
		securityGroup.GroupVersionKind().GroupKind(),
		securityGroup.Name,
		errors,
	)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type SecurityGroup.
func (v *SecurityGroupCustomValidator) ValidateUpdate(
	_ context.Context,
	oldObj, newObj runtime.Object,
) (admission.Warnings, error) {
	oldSecurityGroup, ok := oldObj.(*otcv1alpha1.SecurityGroup)
	if !ok {
		return nil, fmt.Errorf("expected a SecurityGroup object for the oldObj but got %T", oldObj)
	}
	newSecurityGroup, ok := newObj.(*otcv1alpha1.SecurityGroup)
	if !ok {
		return nil, fmt.Errorf("expected a SecurityGroup object for the newObj but got %T", newObj)
	}

	var warnings admission.Warnings
	var errors field.ErrorList

	// Check immutable ProviderConfigRef
	if !equalProviderConfigRef(
		oldSecurityGroup.Spec.ProviderConfigRef,
		newSecurityGroup.Spec.ProviderConfigRef,
	) {
		errors = append(
			errors,
			field.Forbidden(
				field.NewPath("spec", "providerConfigRef"),
				"is immutable and cannot be changed after creation",
			),
		)
	}

	// Warn if orphanOnDelete is being changed from false to true
	if !oldSecurityGroup.Spec.OrphanOnDelete && newSecurityGroup.Spec.OrphanOnDelete {
		warnings = append(
			warnings,
			"orphanOnDelete changed to true: external security group will not be deleted when this resource is deleted",
		)
	}

	// Warn if orphanOnDelete is being changed from true to false
	if oldSecurityGroup.Spec.OrphanOnDelete && !newSecurityGroup.Spec.OrphanOnDelete {
		warnings = append(
			warnings,
			"orphanOnDelete changed to false: external security group will be deleted when this resource is deleted",
		)
	}

	if len(errors) == 0 {
		return warnings, nil
	}

	return warnings, apierrors.NewInvalid(
		oldSecurityGroup.GroupVersionKind().GroupKind(),
		oldSecurityGroup.Name,
		errors,
	)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type SecurityGroup.
func (v *SecurityGroupCustomValidator) ValidateDelete(
	ctx context.Context,
	obj runtime.Object,
) (admission.Warnings, error) {
	return nil, nil
}
