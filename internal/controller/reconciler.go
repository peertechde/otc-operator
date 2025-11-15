package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	otcv1alpha1 "github.com/peertech.de/otc-operator/api/v1alpha1"
	provider "github.com/peertech.de/otc-operator/internal/provider"
)

type ReferenceCheck interface {
	// Check returns names of referencing objects (empty slice if none).
	Check(ctx context.Context, c client.Client, namespace, externalID string) ([]string, error)
	// Resource returns the human readable plural (e.g. "SecurityGroupRules").
	Resource() string
}

// Reconciler provides common reconciliation operations for resources. It
// encapsulates state management, condition handling, finalizer logic and
// provider interactions for resource lifecycle management.
type Reconciler struct {
	logger         zerolog.Logger
	client         client.Client
	providers      *ProviderCache
	object         client.Object
	originalObject client.Object
	conditions     *[]metav1.Condition
	generation     int64
	finalizerName  string
	requeueAfter   time.Duration
}

// AddFinalizer adds the finalizer if not present.
func (rc *Reconciler) AddFinalizer(ctx context.Context) (bool, ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(rc.object, rc.finalizerName) {
		return false, ctrl.Result{}, nil
	}

	rc.logger.Debug().Msg("Adding finalizer")
	controllerutil.AddFinalizer(rc.object, rc.finalizerName)
	if err := rc.client.Update(ctx, rc.object); err != nil {
		rc.logger.Debug().Msg("Failed to add finalizer")
		return true, ctrl.Result{}, err
	}
	rc.logger.Debug().Msg("Successfully added finalizer")

	return true, ctrl.Result{Requeue: true}, nil
}

// RemoveFinalizer removes the finalizer and updates the object.
func (rc *Reconciler) RemoveFinalizer(ctx context.Context) error {
	controllerutil.RemoveFinalizer(rc.object, rc.finalizerName)
	return rc.client.Update(ctx, rc.object)
}

// UpdateStatus updates the status subresource.
func (rc *Reconciler) UpdateStatus(ctx context.Context) error {
	err := rc.client.Status().Patch(
		ctx,
		rc.object,
		client.MergeFrom(rc.originalObject),
	)
	if err != nil {
		rc.logger.Error().Err(err).Msg("Failed to update status")
		return err
	}

	return nil
}

// CheckProviderConfig validates the provider config and returns the provider.
func (rc *Reconciler) CheckProviderConfig(
	ctx context.Context,
	ref otcv1alpha1.ProviderConfigReference,
) (provider otcv1alpha1.ProviderConfig, shouldRequeue bool, result ctrl.Result, err error) {
	pc, err := CheckProviderConfigReady(
		ctx,
		rc.client,
		&ref,
		rc.object,
	)
	if err != nil {
		rc.SetProviderConfigNotReady(err.Error())
		rc.logger.Error().Err(err).Msg("Dependency check failed for ProviderConfig")
		return otcv1alpha1.ProviderConfig{}, true, ctrl.Result{RequeueAfter: rc.requeueAfter}, nil
	}

	rc.SetProviderConfigReady()
	return pc, false, ctrl.Result{}, nil
}

// BlockOnAnyReference runs all provided reference checks and blocks deletion
// if any references exist.
func (rc *Reconciler) BlockOnAnyReference(
	ctx context.Context,
	namespace string,
	externalID string,
	checks ...ReferenceCheck,
) (blocked bool, result ctrl.Result, err error) {
	var allRefs []string

	for _, chk := range checks {
		names, err := chk.Check(ctx, rc.client, namespace, externalID)
		if err != nil {
			rc.SetReconciliationFailed(
				WithMessagef("Failed reference check (%s): %v", chk.Resource(), err),
			)
			return true, ctrl.Result{RequeueAfter: rc.requeueAfter}, err
		}
		if len(names) > 0 {
			allRefs = append(allRefs, names...)
			rc.logger.Debug().
				Str("external-id", externalID).
				Strs("referencers", names).
				Str("referencer-kind", chk.Resource()).
				Msg("Found deletion-blocking references")
		}
	}

	if len(allRefs) > 0 {
		rc.logger.Info().
			Str("external-id", externalID).
			Strs("referencers", allRefs).
			Msg("Deletion blocked by active references")

		rc.SetDeletionBlocked(
			WithMessagef("Still referenced by %v", allRefs),
		)
		return true, ctrl.Result{RequeueAfter: rc.requeueAfter}, nil
	}

	return false, ctrl.Result{}, nil
}

type SecurityGroupRuleReferenceCheck struct{}

func (SecurityGroupRuleReferenceCheck) Resource() string { return "SecurityGroupRules" }

func (SecurityGroupRuleReferenceCheck) Check(
	ctx context.Context,
	c client.Client,
	namespace string,
	externalID string,
) ([]string, error) {
	var list otcv1alpha1.SecurityGroupRuleList
	err := c.List(ctx, &list, client.InNamespace(namespace))
	if err != nil {
		return nil, fmt.Errorf("list SecurityGroupRules: %w", err)
	}

	var refs []string
	for _, item := range list.Items {
		if item.Status.ResolvedDependencies.SecurityGroupID == externalID {
			refs = append(refs, item.Name)
		}
	}

	return refs, nil
}

type NATGatewayNetworkReferenceCheck struct{}

func (NATGatewayNetworkReferenceCheck) Resource() string { return "NATGateways" }

func (NATGatewayNetworkReferenceCheck) Check(
	ctx context.Context,
	c client.Client,
	namespace, externalID string,
) ([]string, error) {
	var list otcv1alpha1.NATGatewayList
	err := c.List(ctx, &list, client.InNamespace(namespace))
	if err != nil {
		return nil, fmt.Errorf("list NATGateways: %w", err)
	}

	var refs []string
	for _, item := range list.Items {
		if item.Status.ResolvedDependencies.NetworkID == externalID {
			refs = append(refs, item.Name)
		}
	}

	return refs, nil
}

type SNATRuleNetworkReferenceCheck struct{}

func (SNATRuleNetworkReferenceCheck) Resource() string { return "SNATRules" }

func (SNATRuleNetworkReferenceCheck) Check(
	ctx context.Context,
	c client.Client,
	namespace, externalID string,
) ([]string, error) {
	var list otcv1alpha1.SNATRuleList
	err := c.List(ctx, &list, client.InNamespace(namespace))
	if err != nil {
		return nil, fmt.Errorf("list SNATRules: %w", err)
	}

	var refs []string
	for _, item := range list.Items {
		if item.Status.ResolvedDependencies.NATGatewayID == externalID {
			refs = append(refs, item.Name)
		}
	}

	return refs, nil
}

type SubnetNetworkReferenceCheck struct{}

func (SubnetNetworkReferenceCheck) Resource() string { return "Subnets" }

func (SubnetNetworkReferenceCheck) Check(
	ctx context.Context,
	c client.Client,
	namespace, externalID string,
) ([]string, error) {
	var list otcv1alpha1.SubnetList
	err := c.List(ctx, &list, client.InNamespace(namespace))
	if err != nil {
		return nil, fmt.Errorf("list Subnets: %w", err)
	}

	var refs []string
	for _, item := range list.Items {
		if item.Status.ResolvedDependencies.NetworkID == externalID {
			refs = append(refs, item.Name)
		}
	}

	return refs, nil
}

// Delete performs standardized finalizer-based deletion.
func (rc *Reconciler) Delete(
	ctx context.Context,
	providerRef otcv1alpha1.ProviderConfigReference,
	orphanOnDelete bool,
	externalID string,
	fn func(context.Context, provider.Provider) error,
) (ctrl.Result, error) {
	// If the finalizer is not present, it means our cleanup logic has already
	// run and the object is just waiting for Kubernetes to garbage collect it.
	if !controllerutil.ContainsFinalizer(rc.object, rc.finalizerName) {
		return ctrl.Result{}, nil
	}

	scopedLogger := rc.logger.With().
		Str("op", "Delete").
		Str("external-id", externalID).
		Bool("orphan-on-delete", orphanOnDelete).
		Logger()

	scopedLogger.Info().Msg("Deleting resource...")

	// Set status to indicate the deletion process has started.
	rc.SetTerminating()
	rc.UpdateStatus(ctx) // Best effort status update

	// Acquire provider client
	p, _, err := rc.providers.GetOrCreate(ctx, providerRef, rc.object.GetNamespace())
	if err != nil {
		// If provider config is gone, we can remove the finalizer.
		if apierrors.IsNotFound(err) {
			scopedLogger.Warn().
				Msg("ProviderConfig not found during deletion, removing finalizer to orphan resource")

			rc.SetOrphaned(
				WithMessage("Resource was orphaned because ProviderConfig was not found"),
			)
			return ctrl.Result{}, rc.RemoveFinalizer(ctx)
		}

		rc.SetNotSynced(
			WithReason(reasonDeletionFailed),
			WithMessagef("Cannot access provider: %v", err),
		)
		rc.SetNotReady(
			WithReason(reasonDeletionFailed),
			WithMessage("Deletion blocked by provider error"),
		)

		scopedLogger.Error().Err(err).Msg("Provider access failed during deletion")
		return ctrl.Result{RequeueAfter: rc.requeueAfter}, nil
	}

	// Perform external deletion unless orphaning is requested.
	if !orphanOnDelete && externalID != "" {
		scopedLogger.Info().Msg("Deleting external resource")

		if err := fn(ctx, p); err != nil {
			rc.SetNotSynced(
				WithReason(reasonDeletionFailed),
				WithMessage(err.Error()),
			)
			rc.SetNotReady(
				WithReason(reasonDeletionFailed),
				WithMessage("External resource deletion failed"),
			)

			scopedLogger.Error().Err(err).Msg("External deletion failed")
			return ctrl.Result{RequeueAfter: rc.requeueAfter}, err
		}

		rc.SetDeleted()
		scopedLogger.Info().Msg("External resource deleted")
	} else if orphanOnDelete {
		rc.SetOrphaned()
		scopedLogger.Info().Msg("Skipping external deletion (orphanOnDelete=true)")
	}

	return ctrl.Result{}, rc.RemoveFinalizer(ctx)
}

// SetSynced marks the resource as synced
func (rc *Reconciler) SetSynced(opts ...ConditionOption) {
	SetSynced(rc.conditions, rc.generation, opts...)
}

// SetNotSynced marks the resource as not synced
func (rc *Reconciler) SetNotSynced(opts ...ConditionOption) {
	SetNotSynced(rc.conditions, rc.generation, opts...)
}

// SetReady marks the resource as ready
func (rc *Reconciler) SetReady(opts ...ConditionOption) {
	SetReady(rc.conditions, rc.generation, opts...)
}

// SetNotReady marks the resource as not ready
func (rc *Reconciler) SetNotReady(opts ...ConditionOption) {
	SetNotReady(rc.conditions, rc.generation, opts...)
}

// SetReconciliationFailed sets both Synced and Ready to False
func (rc *Reconciler) SetReconciliationFailed(opts ...ConditionOption) {
	SetReconciliationFailed(rc.conditions, rc.generation, opts...)
}

// SetSyncedAndReady marks the resource as both Synced and Ready
func (rc *Reconciler) SetSyncedAndReady() {
	SetSyncedAndReady(rc.conditions, rc.generation)
}

// SetCreating marks the resource as being created
func (rc *Reconciler) SetCreating() {
	SetCreating(rc.conditions, rc.generation)
}

// SetProvisioning marks the resource as provisioning
func (rc *Reconciler) SetProvisioning(opts ...ConditionOption) {
	SetProvisioning(rc.conditions, rc.generation, opts...)
}

// SetProvisioned marks the resource as successfully provisioned
func (rc *Reconciler) SetProvisioned() {
	SetProvisioned(rc.conditions, rc.generation)
}

// SetUpdating marks the resource as being updated
func (rc *Reconciler) SetUpdating() {
	SetUpdating(rc.conditions, rc.generation)
}

// SetStopped marks the resource as stopped
func (rc *Reconciler) SetStopped(opts ...ConditionOption) {
	SetStopped(rc.conditions, rc.generation, opts...)
}

// SetDependenciesReady marks dependencies as ready
func (rc *Reconciler) SetDependenciesReady() {
	SetDependenciesReady(rc.conditions, rc.generation)
}

// SetDependenciesNotReady marks dependencies as not ready
func (rc *Reconciler) SetDependenciesNotReady(message string) {
	SetDependenciesNotReady(rc.conditions, message, rc.generation)
}

// SetProviderConfigReady marks the provider config as ready
func (rc *Reconciler) SetProviderConfigReady() {
	SetProviderConfigReady(rc.conditions, rc.generation)
}

// SetProviderConfigNotReady marks the provider config as not ready
func (rc *Reconciler) SetProviderConfigNotReady(message string) {
	SetProviderConfigNotReady(rc.conditions, message, rc.generation)
}

// SetProviderValidationSuccessful marks the provider config as ready after validation
func (rc *Reconciler) SetProviderValidationSuccessful() {
	SetProviderValidationSuccessful(rc.conditions, rc.generation)
}

// SetProviderValidationFailed marks the provider config as failed
func (rc *Reconciler) SetProviderValidationFailed(opts ...ConditionOption) {
	SetProviderValidationFailed(rc.conditions, rc.generation, opts...)
}

// SetTerminating marks the resource as being terminated
func (rc *Reconciler) SetTerminating() {
	SetTerminating(rc.conditions, rc.generation)
}

// SetDeletionBlocked marks the resource as not ready because its deletion is
// blocked by active dependencies.
func (rc *Reconciler) SetDeletionBlocked(opts ...ConditionOption) {
	SetDeletionBlocked(rc.conditions, rc.generation, opts...)
}

// SetDeleted marks the external resource as successfully deleted
func (rc *Reconciler) SetDeleted() {
	SetDeleted(rc.conditions, rc.generation)
}

// SetOrphaned marks the resource as orphaned
func (rc *Reconciler) SetOrphaned(opts ...ConditionOption) {
	SetOrphaned(rc.conditions, rc.generation, opts...)
}
