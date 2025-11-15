package controller

import (
	"fmt"

	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Condition types
const (
	condReady             = "Ready"
	condDependenciesReady = "DependenciesReady"
	condSynced            = "Synced"
)

// Condition reasons for resource lifecycle
const (
	// Creation states
	reasonCreating     = "Creating"
	reasonProvisioning = "Provisioning"
	reasonProvisioned  = "Provisioned"
	reasonSynced       = "Synced"

	// Steady states
	reasonReady   = "Ready"
	reasonStopped = "Stopped"

	// Update states
	reasonReconciling = "Reconciling"

	// Deletion states
	reasonDeleting        = "Deleting"
	reasonDeleted         = "Deleted"
	reasonDeletionBlocked = "DeletionBlocked"
	reasonOrphaned        = "Orphaned"

	// Error/Unknown states
	reasonUnknown = "Unknown"
	reasonFailed  = "Failed"
)

// Condition reasons for validation
const (
	reasonValidationSuccessful = "ValidationSuccessful"
	reasonValidationFailed     = "ValidationFailed"
)

// Condition reasons for dependencies
const (
	reasonDependenciesResolved    = "DependenciesResolved"
	reasonDependenciesNotResolved = "DependenciesNotResolved"
	reasonProviderConfigReady     = "ProviderConfigReady"
	reasonProviderConfigNotReady  = "ProviderConfigNotReady"
)

// Condition reasons for specific error types
const (
	reasonProviderConfigError          = "ProviderConfigError"
	reasonProviderInitializationFailed = "ProviderInitializationFailed"
	reasonProviderError                = "ProviderError"
	reasonProvisioningFailed           = "ProvisioningFailed"
	reasonUpdateFailed                 = "UpdateFailed"
	reasonDeletionFailed               = "DeletionFailed"
	reasonNotFound                     = "NotFound"
)

// ConditionBuilder provides a fluent API for building status conditions
type ConditionBuilder struct {
	typ        string
	status     metav1.ConditionStatus
	reason     string
	message    string
	generation int64
}

// NewCondition creates a new ConditionBuilder
func NewCondition(typ string) *ConditionBuilder {
	return &ConditionBuilder{
		typ: typ,
	}
}

// WithStatus sets the condition status
func (b *ConditionBuilder) WithStatus(status metav1.ConditionStatus) *ConditionBuilder {
	b.status = status
	return b
}

// WithReason sets the condition reason
func (b *ConditionBuilder) WithReason(reason string) *ConditionBuilder {
	b.reason = reason
	return b
}

// WithMessage sets the condition message
func (b *ConditionBuilder) WithMessage(message string) *ConditionBuilder {
	b.message = message
	return b
}

// WithMessagef sets a formatted condition message
func (b *ConditionBuilder) WithMessagef(format string, args ...interface{}) *ConditionBuilder {
	b.message = fmt.Sprintf(format, args...)
	return b
}

// WithGeneration sets the observed generation
func (b *ConditionBuilder) WithGeneration(generation int64) *ConditionBuilder {
	b.generation = generation
	return b
}

// Apply applies the condition to the provided conditions slice
func (b *ConditionBuilder) Apply(conditions *[]metav1.Condition) {
	meta.SetStatusCondition(
		conditions,
		metav1.Condition{
			Type:               b.typ,
			Status:             b.status,
			Reason:             b.reason,
			Message:            b.message,
			ObservedGeneration: b.generation,
		})
}

// ConditionOption allows customizing condition reason and message
type ConditionOption func(*ConditionOptions)

type ConditionOptions struct {
	Reason  string
	Message string
}

// WithReason sets a custom reason for the condition
func WithReason(reason string) ConditionOption {
	return func(o *ConditionOptions) {
		o.Reason = reason
	}
}

// WithMessage sets a custom message for the condition
func WithMessage(message string) ConditionOption {
	return func(o *ConditionOptions) {
		o.Message = message
	}
}

// WithMessagef sets a formatted custom message for the condition
func WithMessagef(format string, args ...interface{}) ConditionOption {
	return func(o *ConditionOptions) {
		o.Message = fmt.Sprintf(format, args...)
	}
}

// applyOptions applies option overrides to default reason and message
func applyOptions(defaultReason, defaultMessage string, opts []ConditionOption) (string, string) {
	options := &ConditionOptions{
		Reason:  defaultReason,
		Message: defaultMessage,
	}
	for _, opt := range opts {
		opt(options)
	}
	return options.Reason, options.Message
}

// SetSynced marks the resource as synced
func SetSynced(conditions *[]metav1.Condition, generation int64, opts ...ConditionOption) {
	reason, message := applyOptions(
		reasonSynced,
		"External resource matches desired state",
		opts,
	)

	NewCondition(condSynced).
		WithStatus(metav1.ConditionTrue).
		WithReason(reason).
		WithMessage(message).
		WithGeneration(generation).
		Apply(conditions)
}

// SetNotSynced marks the resource as not synced
func SetNotSynced(conditions *[]metav1.Condition, generation int64, opts ...ConditionOption) {
	reason, message := applyOptions(
		reasonReconciling,
		"Resource is being reconciled",
		opts,
	)

	NewCondition(condSynced).
		WithStatus(metav1.ConditionFalse).
		WithReason(reason).
		WithMessage(message).
		WithGeneration(generation).
		Apply(conditions)
}

// SetReady marks the resource as ready
func SetReady(conditions *[]metav1.Condition, generation int64, opts ...ConditionOption) {
	reason, message := applyOptions(
		reasonReady,
		"Resource is ready",
		opts,
	)
	NewCondition(condReady).
		WithStatus(metav1.ConditionTrue).
		WithReason(reason).
		WithMessage(message).
		WithGeneration(generation).
		Apply(conditions)
}

// SetNotReady marks the resource as not ready
func SetNotReady(conditions *[]metav1.Condition, generation int64, opts ...ConditionOption) {
	reason, message := applyOptions(
		reasonFailed,
		"Resource is not ready",
		opts,
	)
	NewCondition(condReady).
		WithStatus(metav1.ConditionFalse).
		WithReason(reason).
		WithMessage(message).
		WithGeneration(generation).
		Apply(conditions)
}

// SetReconciliationFailed is a convenience function to set both Synced and Ready
// conditions to False to signal a reconciliation failure.
func SetReconciliationFailed(
	conditions *[]metav1.Condition,
	generation int64,
	opts ...ConditionOption,
) {
	SetNotSynced(conditions, generation, opts...)
	SetNotReady(conditions, generation, opts...)
}

// SetSyncedAndReady marks the resource as both Synced and Ready. This is the
// desired steady-state condition for a healthy resource.
func SetSyncedAndReady(conditions *[]metav1.Condition, generation int64) {
	SetSynced(conditions, generation)
	SetReady(conditions, generation)
}

// SetCreating marks the resource as being created
func SetCreating(conditions *[]metav1.Condition, generation int64) {
	SetNotSynced(
		conditions,
		generation,
		WithReason(reasonCreating),
		WithMessage("Creating external resource"),
	)
	SetNotReady(
		conditions,
		generation,
		WithReason(reasonCreating),
		WithMessage("Resource is being created and is not yet available"),
	)
}

// SetProvisioning marks the resource as provisioning
func SetProvisioning(conditions *[]metav1.Condition, generation int64, opts ...ConditionOption) {
	reason, message := applyOptions(
		reasonProvisioning,
		"Resource is provisioning",
		opts,
	)
	SetNotSynced(conditions, generation, WithReason(reason), WithMessage(message))
	SetNotReady(conditions, generation, WithReason(reason), WithMessage(message))
}

// SetProvisioned marks the resource as successfully provisioned
func SetProvisioned(conditions *[]metav1.Condition, generation int64) {
	SetSynced(
		conditions,
		generation,
		WithReason(reasonProvisioned),
		WithMessage("External resource has been successfully provisioned"),
	)
	SetReady(
		conditions,
		generation,
		WithReason(reasonProvisioned),
		WithMessage("Resource is ready for use"),
	)
}

// SetUpdating marks the resource as being updated.
func SetUpdating(conditions *[]metav1.Condition, generation int64) {
	SetNotSynced(
		conditions,
		generation,
		WithReason(reasonReconciling),
		WithMessage("Updating external resource to match desired state"),
	)
	SetNotReady(
		conditions,
		generation,
		WithReason(reasonReconciling),
		WithMessage("Resource is being updated"),
	)
}

// SetStopped marks the resource as stopped
func SetStopped(conditions *[]metav1.Condition, generation int64, opts ...ConditionOption) {
	reason, message := applyOptions(
		reasonStopped,
		"Resource is stopped",
		opts,
	)
	SetNotSynced(conditions, generation, WithReason(reason), WithMessage(message))
	SetNotReady(conditions, generation, WithReason(reason), WithMessage(message))
}

// SetDependenciesReady marks dependencies as ready
func SetDependenciesReady(conditions *[]metav1.Condition, generation int64) {
	NewCondition(condDependenciesReady).
		WithStatus(metav1.ConditionTrue).
		WithReason(reasonDependenciesResolved).
		WithMessage("All dependencies are ready").
		WithGeneration(generation).
		Apply(conditions)
}

// SetDependenciesNotReady marks dependencies as not ready
func SetDependenciesNotReady(
	conditions *[]metav1.Condition,
	message string,
	generation int64,
) {
	NewCondition(condDependenciesReady).
		WithStatus(metav1.ConditionFalse).
		WithReason(reasonDependenciesNotResolved).
		WithMessage(message).
		WithGeneration(generation).
		Apply(conditions)
}

// SetProviderConfigReady marks the provider config as ready
func SetProviderConfigReady(conditions *[]metav1.Condition, generation int64) {
	NewCondition(condDependenciesReady).
		WithStatus(metav1.ConditionTrue).
		WithReason(reasonProviderConfigReady).
		WithMessage("ProviderConfig is ready and available").
		WithGeneration(generation).
		Apply(conditions)
}

// SetProviderConfigNotReady marks the provider config as not ready
func SetProviderConfigNotReady(conditions *[]metav1.Condition, message string, generation int64) {
	NewCondition(condDependenciesReady).
		WithStatus(metav1.ConditionFalse).
		WithReason(reasonProviderConfigNotReady).
		WithMessage(message).
		WithGeneration(generation).
		Apply(conditions)
}

// SetProviderValidationSuccessful marks the provider config as ready after successful validation.
func SetProviderValidationSuccessful(conditions *[]metav1.Condition, generation int64) {
	NewCondition(condReady).
		WithStatus(metav1.ConditionTrue).
		WithReason(reasonValidationSuccessful).
		WithMessage("Provider credentials and permissions are valid").
		WithGeneration(generation).
		Apply(conditions)
}

// SetProviderValidationFailed marks the provider config as failed.
func SetProviderValidationFailed(
	conditions *[]metav1.Condition,
	generation int64,
	opts ...ConditionOption,
) {
	SetNotReady(
		conditions,
		generation,
		append([]ConditionOption{WithReason(reasonValidationFailed)}, opts...)...,
	)
}

// SetTerminating marks the resource as being terminated.
func SetTerminating(conditions *[]metav1.Condition, generation int64) {
	SetNotSynced(
		conditions,
		generation,
		WithReason(reasonDeleting),
		WithMessage("Resource deletion is in progress"),
	)
	SetNotReady(
		conditions,
		generation,
		WithReason(reasonDeleting),
		WithMessage("Resource is being deleted"),
	)
}

// SetDeletionBlocked marks the resource as not ready because its deletion is
// blocked by active dependencies.
func SetDeletionBlocked(conditions *[]metav1.Condition, generation int64, opts ...ConditionOption) {
	reason, message := applyOptions(
		reasonDeletionBlocked,
		"Resource deletion is blocked by dependencies",
		opts,
	)
	// Synced = False (desired state (deleted) has not been reached)
	SetNotSynced(conditions, generation, WithReason(reason), WithMessage(message))
	// Ready = False (resource is not in a usable or terminal state)
	SetNotReady(conditions, generation, WithReason(reason), WithMessage(message))
}

// SetDeleted marks the external resource as successfully deleted.
func SetDeleted(conditions *[]metav1.Condition, generation int64) {
	// Synced = True (reconciliation complete - external resource deleted)
	SetSynced(
		conditions,
		generation,
		WithReason(reasonDeleted),
		WithMessage("External resource has been successfully deleted"),
	)
	// Ready = False (resource no longer exists)
	SetNotReady(
		conditions,
		generation,
		WithReason(reasonDeleted),
		WithMessage("Resource is terminated"),
	)
}

// SetOrphaned marks the resource as orphaned (external resource preserved).
func SetOrphaned(conditions *[]metav1.Condition, generation int64, opts ...ConditionOption) {
	reason, message := applyOptions(
		reasonOrphaned,
		"External resource was preserved due to orphanOnDelete policy",
		opts,
	)
	// Synced = True (we're done managing this)
	SetSynced(conditions, generation, WithReason(reason), WithMessage(message))

	// Ready = False (no longer managed)
	SetNotReady(
		conditions,
		generation,
		WithReason(reason),
		WithMessage("Resource orphaned, no longer managed by operator"),
	)
}
