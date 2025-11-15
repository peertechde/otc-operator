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
	natGatewayFinalizerName = "natgateway.otc.peertech.de/finalizer"
	natGatewayRequeueDelay  = 30 * time.Second
)

func NewNATGatewayReconciler(
	c client.Client,
	scheme *runtime.Scheme,
	logger zerolog.Logger,
	providers *ProviderCache,
) *NATGatewayReconciler {
	return &NATGatewayReconciler{
		Client:    c,
		Scheme:    scheme,
		logger:    logger.With().Str("controller", "nat-gateway").Logger(),
		providers: providers,
	}
}

// NATGatewayReconciler reconciles a NAT gateway object
type NATGatewayReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	logger    zerolog.Logger
	providers *ProviderCache
}

// +kubebuilder:rbac:groups=otc.peertech.de,resources=natgateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=otc.peertech.de,resources=natgateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=otc.peertech.de,resources=natgateways/finalizers,verbs=update
// +kubebuilder:rbac:groups=otc.peertech.de,resources=networks,verbs=get;list;watch
// +kubebuilder:rbac:groups=otc.peertech.de,resources=subnets,verbs=get;list;watch
// +kubebuilder:rbac:groups=otc.peertech.de,resources=providerconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *NATGatewayReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	scopedLogger := r.logger.With().
		Str("op", "Reconcile").
		Str("nat-gateway", req.NamespacedName.Name).
		Str("namespace", req.NamespacedName.Namespace).
		Logger()

	var natGateway otcv1alpha1.NATGateway
	if err := r.Get(ctx, req.NamespacedName, &natGateway); err != nil {
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
		object:         &natGateway,
		originalObject: natGateway.DeepCopy(),
		conditions:     &natGateway.Status.Conditions,
		generation:     natGateway.Generation,
		finalizerName:  natGatewayFinalizerName,
		requeueAfter:   natGatewayRequeueDelay,
	}

	// Ensure the status is updated.
	defer rc.UpdateStatus(ctx)

	// Handle deletion.
	if !natGateway.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, rc, &natGateway)
	}

	// Ensure the finalizer is present.
	if added, result, err := rc.AddFinalizer(ctx); added {
		return result, err
	}

	// Check if the referenced ProviderConfig is ready.
	_, shouldReque, result, err := rc.CheckProviderConfig(
		ctx,
		natGateway.Spec.ProviderConfigRef,
	)
	if shouldReque {
		return result, err
	}

	// Get or create cached provider client.
	p, _, err := r.providers.GetOrCreate(
		ctx,
		natGateway.Spec.ProviderConfigRef,
		natGateway.Namespace,
	)
	if err != nil {
		rc.SetReconciliationFailed(
			WithReason(reasonProviderConfigError),
			WithMessage(err.Error()),
		)
		scopedLogger.Error().Err(err).Msg("Failed to get or create provider client")
		return ctrl.Result{RequeueAfter: natGatewayRequeueDelay}, nil
	}

	return r.reconcile(ctx, scopedLogger, rc, &natGateway, p)
}

func (r *NATGatewayReconciler) reconcile(
	ctx context.Context,
	logger zerolog.Logger,
	rc *Reconciler,
	natGateway *otcv1alpha1.NATGateway,
	p provider.Provider,
) (ctrl.Result, error) {
	// If the external resource has no known ID, it needs to be created.
	if natGateway.Status.ExternalID == "" {
		return r.reconcileCreate(ctx, logger, rc, natGateway, p)
	}

	return r.reconcileUpdate(ctx, logger, rc, natGateway, p)
}

// reconcileCreate handles dependency resolution, secret management and resource creation.
func (r *NATGatewayReconciler) reconcileCreate(
	ctx context.Context,
	logger zerolog.Logger,
	rc *Reconciler,
	natGateway *otcv1alpha1.NATGateway,
	p provider.Provider,
) (ctrl.Result, error) {
	// Resolve dependencies.
	resolver := NewDependencyResolver(r.Client, natGateway.Namespace)
	networkID, subnetID, err := resolver.ResolveNATGatewayDependencies(
		ctx,
		natGateway.Spec,
	)
	if err != nil {
		rc.SetDependenciesNotReady(err.Error())
		rc.SetNotReady(
			WithReason(reasonDependenciesNotResolved),
			WithMessagef("Waiting for dependencies: %v", err),
		)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	rc.SetDependenciesReady()
	natGateway.Status.ResolvedDependencies = otcv1alpha1.NATGatewayDependenciesResolved{
		NetworkID: networkID,
		SubnetID:  subnetID,
	}

	// Create the external resource.
	logger.Info().Msg("Creating NAT gateway")

	// Set creating status.
	rc.SetCreating()

	resp, err := p.CreateNATGateway(
		ctx,
		provider.CreateNATGatewayRequest{
			Name:        natGateway.GetName(),
			Description: natGateway.Spec.Description,
			Type:        string(natGateway.Spec.Type),
			NetworkID:   networkID,
			SubnetID:    subnetID,
		},
	)
	if err != nil {
		rc.SetReconciliationFailed(
			WithReason(reasonProvisioningFailed),
			WithMessagef("Failed to create resource: %v", err),
		)
		logger.Error().Err(err).Msg("Failed to create NAT gateway")
		return ctrl.Result{RequeueAfter: natGatewayRequeueDelay}, nil
	}

	// Update status fields.
	natGateway.Status.ExternalID = resp.ID
	natGateway.Status.LastAppliedSpec = natGateway.Spec.DeepCopy()

	logger.Info().
		Str("external-id", resp.ID).
		Msg("Successfully created NAT gateway")

	// Requeue immediately to check the status of the new resource.
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// reconcileUpdate handles the logic for an existing external resource. It
// checks for drift, updates the resource and reports its status.
func (r *NATGatewayReconciler) reconcileUpdate(
	ctx context.Context,
	logger zerolog.Logger,
	rc *Reconciler,
	natGateway *otcv1alpha1.NATGateway,
	p provider.Provider,
) (ctrl.Result, error) {
	lastAppliedSpec := natGateway.Status.LastAppliedSpec
	if lastAppliedSpec == nil {
		logger.Warn().Msg("LastAppliedSpec is not set, establishing baseline from current spec.")
		natGateway.Status.LastAppliedSpec = natGateway.Spec.DeepCopy()
		// Requeue to ensure the status update is persisted before proceeding.
		return ctrl.Result{Requeue: true}, nil
	}

	// Fetch the external resource.
	info, err := p.GetNATGateway(ctx, natGateway.Status.ExternalID)
	if err != nil {
		// TODO: this might be to harsh, as the resource could be fully
		// functional, but the server API is unreachable.
		rc.SetReconciliationFailed(
			WithReason(reasonProviderError),
			WithMessagef("Failed to check existing NAT gateway: %v", err),
		)
		logger.Error().Err(err).Msg("Failed to check existing NAT gateway")
		return ctrl.Result{RequeueAfter: natGatewayRequeueDelay}, nil
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
				natGateway.Status.ExternalID,
			),
		)
		rc.SetNotReady(
			WithReason(reasonNotFound),
			WithMessage("Resource needs to be recreated"),
		)

		// Reset status fields
		natGateway.Status.ExternalID = ""
		natGateway.Status.LastAppliedSpec = nil
		return ctrl.Result{Requeue: true}, nil
	}

	logger.Debug().
		Str("external-id", info.ID).
		Str("status", info.Status).
		Msg("Found existing NAT gateway")

	updateReq, needsUpdate := r.detectDrift(logger, natGateway)
	if needsUpdate {
		return r.handleDrift(ctx, logger, p, rc, natGateway, updateReq)
	}

	// Check readiness status.
	return r.checkReadiness(rc, natGateway, info)
}

func (r *NATGatewayReconciler) detectDrift(
	logger zerolog.Logger,
	natGateway *otcv1alpha1.NATGateway,
) (provider.UpdateNATGatewayRequest, bool) {
	var updateReq provider.UpdateNATGatewayRequest
	needsUpdate := false

	lastAppliedSpec := natGateway.Status.LastAppliedSpec
	if natGateway.Spec.Description != lastAppliedSpec.Description {
		logger.Info().
			Str("current", lastAppliedSpec.Description).
			Str("desired", natGateway.Spec.Description).
			Msg("Drift detected in description")

		updateReq.Description = natGateway.Spec.Description
		needsUpdate = true
	}
	if natGateway.Spec.Type != lastAppliedSpec.Type {
		logger.Info().
			Str("current", string(lastAppliedSpec.Type)).
			Str("desired", string(natGateway.Spec.Type)).
			Msg("Drift detected in type")

		updateReq.Type = string(natGateway.Spec.Type)
		needsUpdate = true
	}

	return updateReq, needsUpdate
}

// handleDrift applies updates to the drifted resource.
func (r *NATGatewayReconciler) handleDrift(
	ctx context.Context,
	logger zerolog.Logger,
	p provider.Provider,
	rc *Reconciler,
	natGateway *otcv1alpha1.NATGateway,
	req provider.UpdateNATGatewayRequest,
) (ctrl.Result, error) {
	logger.Info().Msg("Applying updates to external resource")

	// Set updating status.
	rc.SetUpdating()

	err := p.UpdateNATGateway(ctx, natGateway.Status.ExternalID, req)
	if err != nil {
		rc.SetReconciliationFailed(
			WithReason(reasonUpdateFailed),
			WithMessagef("Failed to update resource: %v", err),
		)
		logger.Error().Err(err).Msg("Failed to update resource")
		return ctrl.Result{RequeueAfter: natGatewayRequeueDelay}, nil
	}

	// Update LastAppliedSpec.
	natGateway.Status.LastAppliedSpec = natGateway.Spec.DeepCopy()

	logger.Info().Msg("Successfully updated")

	// Requeue immediately to re-check the status after the update.
	return ctrl.Result{Requeue: true}, nil
}

// checkReadiness updates the status conditions based on the provider's reported status.
func (r *NATGatewayReconciler) checkReadiness(
	rc *Reconciler,
	natGateway *otcv1alpha1.NATGateway,
	info *provider.NATGatewayInfo,
) (ctrl.Result, error) {
	switch info.State() {
	case provider.Ready:
		now := metav1.Now()

		isNewlyProvisioned := natGateway.Status.LastSyncTime == nil
		natGateway.Status.LastSyncTime = &now

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
		return ctrl.Result{RequeueAfter: natGatewayRequeueDelay}, nil
	case provider.Provisioning:
		rc.SetProvisioning(WithMessage(info.Message()))
		return ctrl.Result{RequeueAfter: natGatewayRequeueDelay}, nil
	default:
		rc.SetReconciliationFailed(
			WithReason(reasonUnknown),
			WithMessage(info.Message()),
		)
		return ctrl.Result{RequeueAfter: natGatewayRequeueDelay}, nil
	}
}

func (r *NATGatewayReconciler) reconcileDelete(
	ctx context.Context,
	rc *Reconciler,
	natGateway *otcv1alpha1.NATGateway,
) (ctrl.Result, error) {
	// If the NAT gateway never got an external ID, it couldn't have had any
	// rules created for it, so we can safely proceed with deletion.
	if natGateway.Status.ExternalID == "" {
		return rc.Delete(
			ctx,
			natGateway.Spec.ProviderConfigRef,
			natGateway.Spec.OrphanOnDelete,
			natGateway.Status.ExternalID,
			func(c context.Context, p provider.Provider) error {
				return nil
			},
		)
	}

	// Check if any SNAT rules are still referencing this NAT gateway.
	blocked, result, err := rc.BlockOnAnyReference(
		ctx,
		natGateway.Namespace,
		natGateway.Status.ExternalID,
		SNATRuleNetworkReferenceCheck{},
	)
	if blocked {
		return result, err
	}

	return rc.Delete(
		ctx,
		natGateway.Spec.ProviderConfigRef,
		natGateway.Spec.OrphanOnDelete,
		natGateway.Status.ExternalID,
		func(c context.Context, p provider.Provider) error {
			return p.DeleteNATGateway(c, natGateway.Status.ExternalID)
		},
	)
}

// SetupWithManager sets up the controller with the Manager.
func (r *NATGatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&otcv1alpha1.NATGateway{}).
		Named("natgateway").
		Complete(r)
}
