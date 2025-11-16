package controller

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	otcv1alpha1 "github.com/peertech.de/otc-operator/api/v1alpha1"
	provider "github.com/peertech.de/otc-operator/internal/provider"
)

const (
	subnetFinalizerName = "subnet.otc.peertech.de/finalizer"
	subnetRequeueDelay  = 30 * time.Second
)

func NewSubnetReconciler(
	c client.Client,
	scheme *runtime.Scheme,
	logger zerolog.Logger,
	providers *ProviderCache,
) *SubnetReconciler {
	return &SubnetReconciler{
		Client:    c,
		Scheme:    scheme,
		logger:    logger.With().Str("controller", "subnet").Logger(),
		providers: providers,
	}
}

// SubnetReconciler reconciles a subnet object
type SubnetReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	logger    zerolog.Logger
	providers *ProviderCache
}

// +kubebuilder:rbac:groups=otc.peertech.de,resources=subnets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=otc.peertech.de,resources=subnets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=otc.peertech.de,resources=subnets/finalizers,verbs=update
// +kubebuilder:rbac:groups=otc.peertech.de,resources=networks,verbs=get;list;watch
// +kubebuilder:rbac:groups=otc.peertech.de,resources=providerconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *SubnetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	scopedLogger := r.logger.With().
		Str("op", "Reconcile").
		Str("subnet", req.NamespacedName.Name).
		Str("namespace", req.NamespacedName.Namespace).
		Logger()

	var subnet otcv1alpha1.Subnet
	if err := r.Get(ctx, req.NamespacedName, &subnet); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		scopedLogger.Error().Err(err).Msg("Failed to get resource")
		return ctrl.Result{}, err
	}

	rc := &Reconciler{
		logger:         scopedLogger,
		client:         r.Client,
		providers:      r.providers,
		object:         &subnet,
		originalObject: subnet.DeepCopy(),
		conditions:     &subnet.Status.Conditions,
		generation:     subnet.Generation,
		finalizerName:  subnetFinalizerName,
		requeueAfter:   subnetRequeueDelay,
	}

	// Ensure the status is updated.
	defer rc.UpdateStatus(ctx)

	// Handle deletion.
	if !subnet.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, rc, &subnet)
	}

	// Ensure the finalizer is present.
	if added, result, err := rc.AddFinalizer(ctx); added {
		return result, err
	}

	// Check if the referenced ProviderConfig is ready.
	_, shouldReque, result, err := rc.CheckProviderConfig(
		ctx,
		subnet.Spec.ProviderConfigRef,
	)
	if shouldReque {
		return result, err
	}

	// Get or create cached provider client.
	p, _, err := r.providers.GetOrCreate(ctx, subnet.Spec.ProviderConfigRef, subnet.Namespace)
	if err != nil {
		rc.SetReconciliationFailed(
			WithReason(reasonProviderConfigError),
			WithMessage(err.Error()),
		)
		scopedLogger.Error().Err(err).Msg("Failed to get or create provider client")
		return ctrl.Result{RequeueAfter: subnetRequeueDelay}, nil
	}

	return r.reconcile(ctx, scopedLogger, rc, &subnet, p)
}

func (r *SubnetReconciler) reconcile(
	ctx context.Context,
	logger zerolog.Logger,
	rc *Reconciler,
	subnet *otcv1alpha1.Subnet,
	p provider.Provider,
) (ctrl.Result, error) {
	// If the external resource has no known ID, it needs to be created.
	if subnet.Status.ExternalID == "" {
		return r.reconcileCreate(ctx, logger, rc, subnet, p)
	}

	return r.reconcileUpdate(ctx, logger, rc, subnet, p)
}

// reconcileCreate handles the logic for creating a new external resource.
func (r *SubnetReconciler) reconcileCreate(
	ctx context.Context,
	logger zerolog.Logger,
	rc *Reconciler,
	subnet *otcv1alpha1.Subnet,
	p provider.Provider,
) (ctrl.Result, error) {
	// Resolve dependencies.
	resolver := NewDependencyResolver(r.Client, subnet.Namespace)
	networkID, err := resolver.ResolveNetwork(ctx, subnet.Spec.Network)
	if err != nil {
		rc.SetDependenciesNotReady(err.Error())
		rc.SetNotReady(
			WithReason(reasonDependenciesNotResolved),
			WithMessagef("Waiting for dependencies: %v", err),
		)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	rc.SetDependenciesReady()
	subnet.Status.ResolvedDependencies.NetworkID = networkID

	// Create the external resource.
	logger.Info().Msg("Creating subnet")

	// Set creating status.
	rc.SetCreating()

	resp, err := p.CreateSubnet(
		ctx,
		provider.CreateSubnetRequest{
			Name:        subnet.GetName(),
			Description: subnet.Spec.Description,
			Cidr:        subnet.Spec.Cidr,
			GatewayIP:   subnet.Spec.GatewayIP,
			NetworkID:   networkID,
		},
	)
	if err != nil {
		rc.SetReconciliationFailed(
			WithReason(reasonProvisioningFailed),
			WithMessagef("Failed to create resource: %v", err),
		)
		logger.Error().Err(err).Msg("Failed to create subnet")
		return ctrl.Result{RequeueAfter: subnetRequeueDelay}, nil
	}

	// Update status fields.
	subnet.Status.ExternalID = resp.ID
	subnet.Status.LastAppliedSpec = subnet.Spec.DeepCopy()

	logger.Info().
		Str("external-id", resp.ID).
		Msg("Successfully created subnet")

	return ctrl.Result{}, nil
}

// reconcileUpdate handles the logic for an existing external resource. It
// checks for drift, updates the resource and reports its status.
func (r *SubnetReconciler) reconcileUpdate(
	ctx context.Context,
	logger zerolog.Logger,
	rc *Reconciler,
	subnet *otcv1alpha1.Subnet,
	p provider.Provider,
) (ctrl.Result, error) {
	lastAppliedSpec := subnet.Status.LastAppliedSpec
	if lastAppliedSpec == nil {
		logger.Warn().Msg("LastAppliedSpec is not set, establishing baseline from current spec.")
		subnet.Status.LastAppliedSpec = subnet.Spec.DeepCopy()
		// Requeue to ensure the status update is persisted before proceeding.
		return ctrl.Result{Requeue: true}, nil
	}

	// Fetch the external resource.
	info, err := p.GetSubnet(ctx, subnet.Status.ExternalID)
	if err != nil {
		// TODO: this might be to harsh, as the resource could be fully
		// functional, but the server API is unreachable.
		rc.SetReconciliationFailed(
			WithReason(reasonProviderError),
			WithMessagef("Failed to check existing Subnet: %v", err),
		)
		logger.Error().Err(err).Msg("Failed to check existing subnet")
		return ctrl.Result{RequeueAfter: subnetRequeueDelay}, nil
	}

	// Handle resource being deleted out-of-band. This can happen if the
	// resource was deleted manually from the provider. We will trigger the
	// creation logic in the next reconciliation.
	if info == nil {
		logger.Warn().
			Msg("External subnet not found by ID, resetting externalID to trigger creation")

		rc.SetNotSynced(
			WithReason(reasonNotFound),
			WithMessagef(
				"External resource with ID %s was not found and will be recreated",
				subnet.Status.ExternalID,
			),
		)
		rc.SetNotReady(
			WithReason(reasonNotFound),
			WithMessage("Resource needs to be recreated"),
		)

		// Reset status fields.
		subnet.Status.ExternalID = ""
		subnet.Status.LastAppliedSpec = nil
		return ctrl.Result{Requeue: true}, nil
	}

	logger.Debug().
		Str("external-id", info.ID).
		Str("status", info.Status).
		Msg("Found existing subnet")

	updateReq, needsUpdate := r.detectDrift(logger, subnet)
	if needsUpdate {
		return r.handleDrift(ctx, logger, p, rc, subnet, updateReq)
	}

	// Check readiness status.
	return r.checkReadiness(rc, subnet, info)
}

func (r *SubnetReconciler) detectDrift(
	logger zerolog.Logger,
	subnet *otcv1alpha1.Subnet,
) (provider.UpdateSubnetRequest, bool) {
	var updateReq provider.UpdateSubnetRequest
	needsUpdate := false

	lastAppliedSpec := subnet.Status.LastAppliedSpec
	if subnet.Spec.Description != lastAppliedSpec.Description {
		logger.Info().
			Str("current", lastAppliedSpec.Description).
			Str("desired", subnet.Spec.Description).
			Msg("Drift detected in description")

		updateReq.Description = subnet.Spec.Description
		needsUpdate = true
	}

	return updateReq, needsUpdate
}

// handleDrift applies updates to the drifted resource.
func (r *SubnetReconciler) handleDrift(
	ctx context.Context,
	logger zerolog.Logger,
	p provider.Provider,
	rc *Reconciler,
	subnet *otcv1alpha1.Subnet,
	req provider.UpdateSubnetRequest,
) (ctrl.Result, error) {
	logger.Info().Msg("Applying updates to external resource")

	// Set updating status.
	rc.SetUpdating()

	err := p.UpdateSubnet(
		ctx,
		subnet.Status.ResolvedDependencies.NetworkID,
		subnet.Status.ExternalID,
		req,
	)
	if err != nil {
		rc.SetReconciliationFailed(
			WithReason(reasonUpdateFailed),
			WithMessagef("Failed to update resource: %v", err),
		)
		logger.Error().Err(err).Msg("Failed to update resource")
		return ctrl.Result{RequeueAfter: subnetRequeueDelay}, nil
	}

	// Update LastAppliedSpec.
	subnet.Status.LastAppliedSpec = subnet.Spec.DeepCopy()

	logger.Info().Msg("Successfully updated")

	// Requeue immediately to re-check the status after the update.
	return ctrl.Result{Requeue: true}, nil
}

// checkReadiness updates the status conditions based on the provider's reported status.
func (r *SubnetReconciler) checkReadiness(
	rc *Reconciler,
	subnet *otcv1alpha1.Subnet,
	info *provider.SubnetInfo,
) (ctrl.Result, error) {
	switch info.State() {
	case provider.Ready:
		now := metav1.Now()

		isNewlyProvisioned := subnet.Status.LastSyncTime == nil
		subnet.Status.LastSyncTime = &now

		if isNewlyProvisioned {
			rc.SetProvisioned()
		} else {
			rc.SetSyncedAndReady()
		}
		return ctrl.Result{}, nil
	case provider.Failed:
		rc.SetReconciliationFailed(
			WithReason(reasonFailed),
			WithMessage(info.Message()),
		)
		return ctrl.Result{RequeueAfter: subnetRequeueDelay}, nil
	case provider.Provisioning:
		rc.SetProvisioning(WithMessage(info.Message()))
		return ctrl.Result{RequeueAfter: subnetRequeueDelay}, nil
	default:
		rc.SetReconciliationFailed(
			WithReason(reasonUnknown),
			WithMessage(info.Message()),
		)
		return ctrl.Result{RequeueAfter: subnetRequeueDelay}, nil
	}
}

func (r *SubnetReconciler) reconcileDelete(
	ctx context.Context,
	rc *Reconciler,
	subnet *otcv1alpha1.Subnet,
) (ctrl.Result, error) {
	// If the Subnet never got an external ID, it couldn't have had any rules
	// created for it, so we can safely proceed with deletion.
	if subnet.Status.ExternalID == "" {
		return rc.Delete(
			ctx,
			subnet.Spec.ProviderConfigRef,
			subnet.Spec.OrphanOnDelete,
			subnet.Status.ExternalID,
			func(c context.Context, p provider.Provider) error {
				return nil
			},
		)
	}

	// Check if any NAT gateways, SNAT rules are still referencing this Subnet.
	blocked, result, err := rc.BlockOnAnyReference(
		ctx,
		subnet.Namespace,
		subnet.Status.ExternalID,
		NATGatewayNetworkReferenceCheck{},
		SNATRuleNetworkReferenceCheck{},
	)
	if blocked {
		return result, err
	}

	return rc.Delete(
		ctx,
		subnet.Spec.ProviderConfigRef,
		subnet.Spec.OrphanOnDelete,
		subnet.Status.ExternalID,
		func(c context.Context, p provider.Provider) error {
			// If we lack the resolved NetworkID we cannot call provider delete.
			if subnet.Status.ResolvedDependencies.NetworkID == "" {
				return nil // TODO: return error
			}

			return p.DeleteSubnet(
				c,
				subnet.Status.ResolvedDependencies.NetworkID,
				subnet.Status.ExternalID,
			)
		},
	)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SubnetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&otcv1alpha1.Subnet{}).
		Named("subnet").
		Complete(r)
}
