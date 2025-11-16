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

// SetupSecurityGroupRuleWebhookWithManager registers the webhook for SecurityGroupRule in the manager.
func SetupSecurityGroupRuleWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&otcv1alpha1.SecurityGroupRule{}).
		WithValidator(&SecurityGroupRuleCustomValidator{}).
		Complete()
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:path=/validate-otc-peertech-de-v1alpha1-securitygrouprule,mutating=false,failurePolicy=fail,sideEffects=None,groups=otc.peertech.de,resources=securitygrouprules,verbs=create;update,versions=v1alpha1,name=vsecuritygrouprule-v1alpha1.kb.io,admissionReviewVersions=v1

// SecurityGroupRuleCustomValidator struct is responsible for validating the SecurityGroupRule resource
// when it is created, updated, or deleted.
type SecurityGroupRuleCustomValidator struct{}

var _ webhook.CustomValidator = &SecurityGroupRuleCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type SecurityGroupRule.
func (v *SecurityGroupRuleCustomValidator) ValidateCreate(
	_ context.Context,
	obj runtime.Object,
) (admission.Warnings, error) {
	securityGroupRule, ok := obj.(*otcv1alpha1.SecurityGroupRule)
	if !ok {
		return nil, fmt.Errorf("expected a SecurityGroupRule object but got %T", obj)
	}

	var warnings admission.Warnings
	var errors field.ErrorList

	// Validate the resource name
	if !validName.MatchString(securityGroupRule.Name) {
		errors = append(errors, field.Invalid(
			field.NewPath("metadata", "name"),
			securityGroupRule.Name,
			"name must contain only letters, digits, underscores (_), hyphens (-), and periods (.)",
		))
	}

	// Validate ProviderConfigRef
	if err := validateProviderConfigRefName(securityGroupRule.Spec.ProviderConfigRef); err != nil {
		errors = append(errors, err)
	}

	// Validate that exactly one security group dependency method is specified
	if err := validateSecurityGroupDependency(securityGroupRule.Spec.SecurityGroup); err != nil {
		errors = append(
			errors,
			field.Invalid(
				field.NewPath("spec", "securityGroup"),
				securityGroupRule.Spec.SecurityGroup,
				err.Error(),
			),
		)
	}

	// Warn about orphanOnDelete if true
	if securityGroupRule.Spec.OrphanOnDelete {
		warnings = append(
			warnings,
			"orphanOnDelete is true: external security group rule will not be deleted when this resource is deleted",
		)
	}

	if len(errors) == 0 {
		return warnings, nil
	}

	return warnings, apierrors.NewInvalid(
		securityGroupRule.GroupVersionKind().GroupKind(),
		securityGroupRule.Name,
		errors,
	)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type SecurityGroupRule.
func (v *SecurityGroupRuleCustomValidator) ValidateUpdate(
	_ context.Context,
	oldObj, newObj runtime.Object,
) (admission.Warnings, error) {
	oldSecurityGroupRule, ok := oldObj.(*otcv1alpha1.SecurityGroupRule)
	if !ok {
		return nil, fmt.Errorf(
			"expected a SecurityGroupRule object for the oldObj but got %T",
			newObj,
		)
	}
	newSecurityGroupRule, ok := newObj.(*otcv1alpha1.SecurityGroupRule)
	if !ok {
		return nil, fmt.Errorf(
			"expected a SecurityGroupRule object for the newObj but got %T",
			newObj,
		)
	}

	var warnings admission.Warnings
	var errors field.ErrorList

	// Check immutable ProviderConfigRef
	if !equalProviderConfigRef(
		oldSecurityGroupRule.Spec.ProviderConfigRef,
		newSecurityGroupRule.Spec.ProviderConfigRef,
	) {
		errors = append(
			errors,
			field.Forbidden(
				field.NewPath("spec", "providerConfigRef"),
				"is immutable and cannot be changed after creation",
			),
		)
	}

	// Check immutable Security Group dependency
	if !equalSecurityGroupDependency(
		oldSecurityGroupRule.Spec.SecurityGroup,
		newSecurityGroupRule.Spec.SecurityGroup,
	) {
		errors = append(
			errors,
			field.Forbidden(
				field.NewPath("spec", "securityGroup"),
				"is immutable and cannot be changed after creation",
			),
		)
	}

	// Warn if orphanOnDelete is being changed from false to true
	if !oldSecurityGroupRule.Spec.OrphanOnDelete && newSecurityGroupRule.Spec.OrphanOnDelete {
		warnings = append(
			warnings,
			"orphanOnDelete changed to true: external security group rule will not be deleted when this resource is deleted",
		)
	}

	// Warn if orphanOnDelete is being changed from true to false
	if oldSecurityGroupRule.Spec.OrphanOnDelete && !newSecurityGroupRule.Spec.OrphanOnDelete {
		warnings = append(
			warnings,
			"orphanOnDelete changed to false: external security group rule will be deleted when this resource is deleted",
		)
	}

	if len(errors) == 0 {
		return warnings, nil
	}

	return warnings, apierrors.NewInvalid(
		oldSecurityGroupRule.GroupVersionKind().GroupKind(),
		oldSecurityGroupRule.Name,
		errors,
	)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type SecurityGroupRule.
func (v *SecurityGroupRuleCustomValidator) ValidateDelete(
	ctx context.Context,
	obj runtime.Object,
) (admission.Warnings, error) {
	return nil, nil
}
