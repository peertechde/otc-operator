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

// SetupSNATRuleWebhookWithManager registers the webhook for SNATRule in the manager.
func SetupSNATRuleWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&otcv1alpha1.SNATRule{}).
		WithValidator(&SNATRuleCustomValidator{}).
		Complete()
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:path=/validate-otc-peertech-de-v1alpha1-snatrule,mutating=false,failurePolicy=fail,sideEffects=None,groups=otc.peertech.de,resources=snatrules,verbs=create;update,versions=v1alpha1,name=vsnatrule-v1alpha1.kb.io,admissionReviewVersions=v1

// SNATRuleCustomValidator struct is responsible for validating the SNATRule resource
// when it is created, updated, or deleted.
type SNATRuleCustomValidator struct{}

var _ webhook.CustomValidator = &SNATRuleCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type SNATRule.
func (v *SNATRuleCustomValidator) ValidateCreate(
	_ context.Context,
	obj runtime.Object,
) (admission.Warnings, error) {
	snatRule, ok := obj.(*otcv1alpha1.SNATRule)
	if !ok {
		return nil, fmt.Errorf("expected a SNATRule object but got %T", obj)
	}

	var warnings admission.Warnings
	var errors field.ErrorList

	// Validate the resource name
	if !validName.MatchString(snatRule.Name) {
		errors = append(errors, field.Invalid(
			field.NewPath("metadata", "name"),
			snatRule.Name,
			"name must contain only letters, digits, underscores (_), hyphens (-), and periods (.)",
		))
	}

	// Validate ProviderConfigRef
	if err := validateProviderConfigRefName(snatRule.Spec.ProviderConfigRef); err != nil {
		errors = append(errors, err)
	}

	// Validate that exactly one NAT gateway dependency method is specified
	if err := validateNATGatewayDependency(snatRule.Spec.NATGateway); err != nil {
		errors = append(
			errors,
			field.Invalid(
				field.NewPath("spec", "natGatetway"),
				snatRule.Spec.NATGateway,
				err.Error(),
			),
		)
	}

	// Validate that exactly one subnet dependency method is specified
	if err := validateSubnetDependency(snatRule.Spec.Subnet); err != nil {
		errors = append(
			errors,
			field.Invalid(
				field.NewPath("spec", "subnet"),
				snatRule.Spec.Subnet,
				err.Error(),
			),
		)
	}

	// Validate that exactly one public IP dependency method is specified
	if err := validatePublicIPDependency(snatRule.Spec.PublicIP); err != nil {
		errors = append(
			errors,
			field.Invalid(
				field.NewPath("spec", "publicIP"),
				snatRule.Spec.PublicIP,
				err.Error(),
			),
		)
	}

	// Warn about orphanOnDelete if true
	if snatRule.Spec.OrphanOnDelete {
		warnings = append(
			warnings,
			"orphanOnDelete is true: external SNAT rule will not be deleted when this resource is deleted",
		)
	}

	if len(errors) == 0 {
		return warnings, nil
	}

	return warnings, apierrors.NewInvalid(
		snatRule.GroupVersionKind().GroupKind(),
		snatRule.Name,
		errors,
	)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type SNATRule.
func (v *SNATRuleCustomValidator) ValidateUpdate(
	_ context.Context,
	oldObj, newObj runtime.Object,
) (admission.Warnings, error) {
	oldSNATRule, ok := oldObj.(*otcv1alpha1.SNATRule)
	if !ok {
		return nil, fmt.Errorf("expected a SNATRule object for the oldObj but got %T", oldObj)
	}
	newSNATRule, ok := newObj.(*otcv1alpha1.SNATRule)
	if !ok {
		return nil, fmt.Errorf("expected a SNATRule object for the newObj but got %T", newObj)
	}

	var warnings admission.Warnings
	var errors field.ErrorList

	// Check immutable ProviderConfigRef
	if !equalProviderConfigRef(
		oldSNATRule.Spec.ProviderConfigRef,
		newSNATRule.Spec.ProviderConfigRef,
	) {
		errors = append(
			errors,
			field.Forbidden(
				field.NewPath("spec", "providerConfigRef"),
				"is immutable and cannot be changed after creation",
			),
		)
	}

	// Check immutable NAT gateway dependency
	if !equalNATGatewayDependency(oldSNATRule.Spec.NATGateway, newSNATRule.Spec.NATGateway) {
		errors = append(
			errors,
			field.Forbidden(
				field.NewPath("spec", "natGatetway"),
				"is immutable and cannot be changed after creation",
			),
		)
	}

	// Check immutable Subnet dependency
	if !equalSubnetDependency(oldSNATRule.Spec.Subnet, newSNATRule.Spec.Subnet) {
		errors = append(
			errors,
			field.Forbidden(
				field.NewPath("spec", "subnet"),
				"is immutable and cannot be changed after creation",
			),
		)
	}

	// Check immutable Public IP dependency
	if !equalPublicIPDependency(oldSNATRule.Spec.PublicIP, newSNATRule.Spec.PublicIP) {
		errors = append(
			errors,
			field.Forbidden(
				field.NewPath("spec", "publicIP"),
				"is immutable and cannot be changed after creation",
			),
		)
	}

	// Warn if orphanOnDelete is being changed from false to true
	if !oldSNATRule.Spec.OrphanOnDelete && newSNATRule.Spec.OrphanOnDelete {
		warnings = append(
			warnings,
			"orphanOnDelete changed to true: external SNAT rule will not be deleted when this resource is deleted",
		)
	}

	// Warn if orphanOnDelete is being changed from true to false
	if oldSNATRule.Spec.OrphanOnDelete && !newSNATRule.Spec.OrphanOnDelete {
		warnings = append(
			warnings,
			"orphanOnDelete changed to false: external SNAT rule will be deleted when this resource is deleted",
		)
	}

	if len(errors) == 0 {
		return warnings, nil
	}

	return warnings, apierrors.NewInvalid(
		oldSNATRule.GroupVersionKind().GroupKind(),
		oldSNATRule.Name,
		errors,
	)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type SNATRule.
func (v *SNATRuleCustomValidator) ValidateDelete(
	ctx context.Context,
	obj runtime.Object,
) (admission.Warnings, error) {
	return nil, nil
}
