package v1alpha1

import (
	"context"
	"fmt"
	"net"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	otcv1alpha1 "github.com/peertech.de/otc-operator/api/v1alpha1"
)

// SetupSubnetWebhookWithManager registers the webhook for Subnet in the manager.
func SetupSubnetWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&otcv1alpha1.Subnet{}).
		WithValidator(&SubnetCustomValidator{}).
		Complete()
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:path=/validate-otc-peertech-de-v1alpha1-subnet,mutating=false,failurePolicy=fail,sideEffects=None,groups=otc.peertech.de,resources=subnets,verbs=create;update,versions=v1alpha1,name=vsubnet-v1alpha1.kb.io,admissionReviewVersions=v1

// SubnetCustomValidator struct is responsible for validating the Subnet resource
// when it is created, updated, or deleted.
type SubnetCustomValidator struct{}

var _ webhook.CustomValidator = &SubnetCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Subnet.
func (v *SubnetCustomValidator) ValidateCreate(
	_ context.Context,
	obj runtime.Object,
) (admission.Warnings, error) {
	subnet, ok := obj.(*otcv1alpha1.Subnet)
	if !ok {
		return nil, fmt.Errorf("expected a Subnet object but got %T", obj)
	}

	var warnings admission.Warnings
	var errors field.ErrorList

	// Validate the resource name
	if !validName.MatchString(subnet.Name) {
		errors = append(errors, field.Invalid(
			field.NewPath("metadata", "name"),
			subnet.Name,
			"name must contain only letters, digits, underscores (_), hyphens (-), and periods (.)",
		))
	}

	// Validate ProviderConfigRef
	if name := subnet.Spec.ProviderConfigRef.Name; name == "" {
		errors = append(
			errors,
			field.Required(
				field.NewPath("spec", "providerConfigRef", "name"),
				"name is required",
			),
		)
	}

	// Validate that exactly one network dependency method is specified
	if err := validateNetworkDependency(subnet.Spec.Network); err != nil {
		errors = append(
			errors,
			field.Invalid(
				field.NewPath("spec", "network"),
				subnet.Spec.Network,
				err.Error(),
			),
		)
	}

	// Validate CIDR format
	if err := validateCIDR(subnet.Spec.Cidr); err != nil {
		errors = append(
			errors,
			field.Invalid(
				field.NewPath("spec", "cidr"),
				subnet.Spec.Cidr,
				err.Error(),
			),
		)
	}

	// Validate GatewayIP format and that it's within the CIDR
	if err := validateGatewayIP(subnet.Spec.GatewayIP, subnet.Spec.Cidr); err != nil {
		errors = append(
			errors,
			field.Invalid(
				field.NewPath("spec", "gatewayIP"),
				subnet.Spec.Cidr,
				err.Error(),
			),
		)
	}

	// Warn about orphanOnDelete if true
	if subnet.Spec.OrphanOnDelete {
		warnings = append(
			warnings,
			"orphanOnDelete is true: external subnet will not be deleted when this resource is deleted",
		)
	}

	if len(errors) == 0 {
		return warnings, nil
	}

	return warnings, apierrors.NewInvalid(
		subnet.GroupVersionKind().GroupKind(),
		subnet.Name,
		errors,
	)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Subnet.
func (v *SubnetCustomValidator) ValidateUpdate(
	_ context.Context,
	oldObj, newObj runtime.Object,
) (admission.Warnings, error) {
	oldSubnet, ok := oldObj.(*otcv1alpha1.Subnet)
	if !ok {
		return nil, fmt.Errorf("expected a Subnet object for the oldObj but got %T", newObj)
	}
	newSubnet, ok := newObj.(*otcv1alpha1.Subnet)
	if !ok {
		return nil, fmt.Errorf("expected a Subnet object for the newObj but got %T", newObj)
	}

	var warnings admission.Warnings
	var errors field.ErrorList

	// Check immutable ProviderConfigRef
	if !equalProviderConfigRef(
		oldSubnet.Spec.ProviderConfigRef,
		newSubnet.Spec.ProviderConfigRef,
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
	if !equalNetworkDependency(oldSubnet.Spec.Network, newSubnet.Spec.Network) {
		errors = append(
			errors,
			field.Forbidden(
				field.NewPath("spec", "network"),
				"is immutable and cannot be changed after creation",
			),
		)
	}

	// Validate unsupported operations
	if newSubnet.Spec.Cidr != oldSubnet.Spec.Cidr {
		errors = append(
			errors,
			field.Forbidden(
				field.NewPath("spec", "cidr"),
				"is immutable and cannot be changed after creation",
			),
		)
	}

	// Check immutable GatewayIP field
	if newSubnet.Spec.GatewayIP != oldSubnet.Spec.GatewayIP {
		errors = append(
			errors,
			field.Forbidden(
				field.NewPath("spec", "gatewayIP"),
				"is immutable and cannot be changed after creation",
			),
		)
	}

	// Warn if orphanOnDelete is being changed from false to true
	if !oldSubnet.Spec.OrphanOnDelete && newSubnet.Spec.OrphanOnDelete {
		warnings = append(
			warnings,
			"orphanOnDelete changed to true: external subnet will not be deleted when this resource is deleted",
		)
	}

	// Warn if orphanOnDelete is being changed from true to false
	if oldSubnet.Spec.OrphanOnDelete && !newSubnet.Spec.OrphanOnDelete {
		warnings = append(
			warnings,
			"orphanOnDelete changed to false: external subnet will be deleted when this resource is deleted",
		)
	}

	return warnings, apierrors.NewInvalid(
		oldSubnet.GroupVersionKind().GroupKind(),
		oldSubnet.Name,
		errors,
	)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Subnet.
func (v *SubnetCustomValidator) ValidateDelete(
	ctx context.Context,
	obj runtime.Object,
) (admission.Warnings, error) {
	return nil, nil
}

// validateGatewayIP validates that the gateway IP is a valid IPv4 address and
// within the provided CIDR range
func validateGatewayIP(gatewayIP, cidr string) error {
	// Parse the gateway IP
	ip := net.ParseIP(gatewayIP)
	if ip == nil {
		return fmt.Errorf("'%s' is not a valid IP address", gatewayIP)
	}

	// Ensure it's IPv4
	ip = ip.To4()
	if ip == nil {
		return fmt.Errorf("'%s' must be a valid IPv4 address", gatewayIP)
	}

	// Parse the CIDR
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("cannot validate gatewayIP against invalid CIDR '%s': %w", cidr, err)
	}

	// Check if the gateway IP is within the CIDR range
	if !ipNet.Contains(ip) {
		return fmt.Errorf("'%s' is not within the CIDR range '%s'", gatewayIP, cidr)
	}

	return nil
}
