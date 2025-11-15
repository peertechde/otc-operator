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
	networkFinalizerName = "network.otc.peertech.de/finalizer"
	networkRequeueDelay  = 30 * time.Second
)

func NewNetworkReconciler(
	c client.Client,
	scheme *runtime.Scheme,
	logger zerolog.Logger,
	providers *ProviderCache,
) *NetworkReconciler {
	return &NetworkReconciler{
		Client:    c,
		Scheme:    scheme,
		logger:    logger.With().Str("controller", "network").Logger(),
		providers: providers,
	}
}

// NetworkReconciler reconciles a Network object
type NetworkReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	logger    zerolog.Logger
	providers *ProviderCache
}

// +kubebuilder:rbac:groups=otc.peertech.de,resources=networks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=otc.peertech.de,resources=networks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=otc.peertech.de,resources=networks/finalizers,verbs=update
// +kubebuilder:rbac:groups=otc.peertech.de,resources=providerconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *NetworkReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	scopedLogger := r.logger.With().
		Str("network", req.NamespacedName.Name).
		Str("namespace", req.NamespacedName.Namespace).
		Logger()

	var network otcv1alpha1.Network
	if err := r.Get(ctx, req.NamespacedName, &network); err != nil {
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
		object:         &network,
		originalObject: network.DeepCopy(),
		conditions:     &network.Status.Conditions,
		generation:     network.Generation,
		finalizerName:  networkFinalizerName,
		requeueAfter:   networkRequeueDelay,
	}

	// Ensure the status is updated.
	defer rc.UpdateStatus(ctx)

	// Handle deletion.
	if !network.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, rc, &network)
	}

	// Ensure the finalizer is present.
	if added, result, err := rc.AddFinalizer(ctx); added {
		return result, err
	}

	// Check if the referenced ProviderConfig is ready.
	_, shouldReque, result, err := rc.CheckProviderConfig(
		ctx,
		network.Spec.ProviderConfigRef,
	)
	if shouldReque {
		return result, err
	}

	// Get or create cached provider client.
	p, _, err := r.providers.GetOrCreate(ctx, network.Spec.ProviderConfigRef, network.Namespace)
	if err != nil {
		rc.SetReconciliationFailed(
			WithReason(reasonProviderConfigError),
			WithMessage(err.Error()),
		)
		scopedLogger.Error().Err(err).Msg("Failed to get or create provider client")
		return ctrl.Result{RequeueAfter: networkRequeueDelay}, nil
	}

	return r.reconcile(ctx, scopedLogger, rc, &network, p)
}

func (r *NetworkReconciler) reconcile(
	ctx context.Context,
	logger zerolog.Logger,
	rc *Reconciler,
	network *otcv1alpha1.Network,
	p provider.Provider,
) (ctrl.Result, error) {
	// If the external resource has no known ID, it needs to be created.
	if network.Status.ExternalID == "" {
		return r.reconcileCreate(ctx, logger, rc, network, p)
	}

	return r.reconcileUpdate(ctx, logger, rc, network, p)
}

// reconcileCreate handles the logic for creating a new external resource.
func (r *NetworkReconciler) reconcileCreate(
	ctx context.Context,
	logger zerolog.Logger,
	rc *Reconciler,
	network *otcv1alpha1.Network,
	p provider.Provider,
) (ctrl.Result, error) {
	logger.Info().Msg("Creating network")

	// Set creating status.
	rc.SetCreating()

	resp, err := p.CreateNetwork(
		ctx,
		provider.CreateNetworkRequest{
			Name:        network.GetName(),
			Description: network.Spec.Description,
			Cidr:        network.Spec.Cidr,
		},
	)
	if err != nil {
		rc.SetReconciliationFailed(
			WithReason(reasonProvisioningFailed),
			WithMessagef("Failed to create resource: %v", err),
		)
		logger.Error().Err(err).Msg("Failed to create network")
		return ctrl.Result{RequeueAfter: networkRequeueDelay}, nil
	}

	// Update status fields.
	network.Status.ExternalID = resp.ID
	network.Status.LastAppliedSpec = network.Spec.DeepCopy()

	logger.Info().
		Str("external-id", resp.ID).
		Msg("Successfully created network")

	// Requeue immediately to check the status of the new resource.
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// reconcileUpdate handles the logic for an existing external resource. It
// checks for drift, updates the resource and reports its status.
func (r *NetworkReconciler) reconcileUpdate(
	ctx context.Context,
	logger zerolog.Logger,
	rc *Reconciler,
	network *otcv1alpha1.Network,
	p provider.Provider,
) (ctrl.Result, error) {
	lastAppliedSpec := network.Status.LastAppliedSpec
	if lastAppliedSpec == nil {
		logger.Warn().Msg("LastAppliedSpec is not set, establishing baseline from current spec.")
		network.Status.LastAppliedSpec = network.Spec.DeepCopy()
		// Requeue to ensure the status update is persisted before proceeding.
		return ctrl.Result{Requeue: true}, nil
	}

	// Fetch the external resource.
	info, err := p.GetNetwork(ctx, network.Status.ExternalID)
	if err != nil {
		// TODO: this might be to harsh, as the resource could be fully
		// functional, but the server API is unreachable.
		rc.SetReconciliationFailed(
			WithReason(reasonProviderError),
			WithMessagef("Failed to check existing Network: %v", err),
		)
		logger.Error().Err(err).Msg("Failed to check existing network")
		return ctrl.Result{RequeueAfter: networkRequeueDelay}, nil
	}

	// Handle resource being deleted out-of-band. This can happen if the
	// resource was deleted manually from the provider. We will trigger the
	// creation logic in the next reconciliation.
	if info == nil {
		logger.Warn().
			Msg("External network not found by ID, resetting externalID to trigger creation")

		rc.SetNotSynced(
			WithReason(reasonNotFound),
			WithMessagef(
				"External resource with ID %s was not found and will be recreated",
				network.Status.ExternalID,
			),
		)
		rc.SetNotReady(
			WithReason(reasonNotFound),
			WithMessage("Resource needs to be recreated"),
		)

		// Reset status fields.
		network.Status.ExternalID = ""
		network.Status.LastAppliedSpec = nil
		return ctrl.Result{Requeue: true}, nil
	}

	logger.Debug().
		Str("external-id", info.ID).
		Str("status", info.Status).
		Msg("Found existing network")

	updateReq, needsUpdate := r.detectDrift(logger, network)
	if needsUpdate {
		return r.handleDrift(ctx, logger, p, rc, network, updateReq)
	}

	// Check readiness status.
	return r.checkReadiness(rc, network, info)
}

func (r *NetworkReconciler) detectDrift(
	logger zerolog.Logger,
	network *otcv1alpha1.Network,
) (provider.UpdateNetworkRequest, bool) {
	var updateReq provider.UpdateNetworkRequest
	needsUpdate := false

	lastAppliedSpec := network.Status.LastAppliedSpec
	if network.Spec.Description != lastAppliedSpec.Description {
		logger.Info().
			Str("current", lastAppliedSpec.Description).
			Str("desired", network.Spec.Description).
			Msg("Drift detected in description")

		updateReq.Description = network.Spec.Description
		needsUpdate = true
	}

	return updateReq, needsUpdate
}

// handleDrift applies updates to the drifted resource.
func (r *NetworkReconciler) handleDrift(
	ctx context.Context,
	logger zerolog.Logger,
	p provider.Provider,
	rc *Reconciler,
	network *otcv1alpha1.Network,
	req provider.UpdateNetworkRequest,
) (ctrl.Result, error) {
	logger.Info().Msg("Applying updates to external resource")

	// Set updating status.
	rc.SetUpdating()

	err := p.UpdateNetwork(ctx, network.Status.ExternalID, req)
	if err != nil {
		rc.SetReconciliationFailed(
			WithReason(reasonUpdateFailed),
			WithMessagef("Failed to update resource: %v", err),
		)
		logger.Error().Err(err).Msg("Failed to update resource")
		return ctrl.Result{RequeueAfter: networkRequeueDelay}, nil
	}

	// Update LastAppliedSpec.
	network.Status.LastAppliedSpec = network.Spec.DeepCopy()

	logger.Info().Msg("Successfully updated")

	// Requeue immediately to re-check the status after the update.
	return ctrl.Result{Requeue: true}, nil
}

// checkReadiness updates the status conditions based on the provider's reported status.
func (r *NetworkReconciler) checkReadiness(
	rc *Reconciler,
	network *otcv1alpha1.Network,
	info *provider.NetworkInfo,
) (ctrl.Result, error) {
	switch info.State() {
	case provider.Ready:
		now := metav1.Now()

		isNewlyProvisioned := network.Status.LastSyncTime == nil
		network.Status.LastSyncTime = &now

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
		return ctrl.Result{RequeueAfter: networkRequeueDelay}, nil
	case provider.Provisioning:
		rc.SetProvisioning(WithMessage(info.Message()))
		return ctrl.Result{RequeueAfter: networkRequeueDelay}, nil
	default:
		rc.SetReconciliationFailed(
			WithReason(reasonUnknown),
			WithMessage(info.Message()),
		)
		return ctrl.Result{RequeueAfter: networkRequeueDelay}, nil
	}
}

func (r *NetworkReconciler) reconcileDelete(
	ctx context.Context,
	rc *Reconciler,
	network *otcv1alpha1.Network,
) (ctrl.Result, error) {
	// If the network never got an external ID, it couldn't have had any rules
	// created for it, so we can safely proceed with deletion.
	if network.Status.ExternalID == "" {
		return rc.Delete(
			ctx,
			network.Spec.ProviderConfigRef,
			network.Spec.OrphanOnDelete,
			network.Status.ExternalID,
			func(c context.Context, p provider.Provider) error {
				return nil
			},
		)
	}

	// Check if any Subnets, NATGateways are still referencing this Network.
	blocked, result, err := rc.BlockOnAnyReference(
		ctx,
		network.Namespace,
		network.Status.ExternalID,
		SubnetNetworkReferenceCheck{},
		NATGatewayNetworkReferenceCheck{},
	)
	if blocked {
		return result, err
	}

	return rc.Delete(
		ctx,
		network.Spec.ProviderConfigRef,
		network.Spec.OrphanOnDelete,
		network.Status.ExternalID,
		func(c context.Context, p provider.Provider) error {
			return p.DeleteNetwork(c, network.Status.ExternalID)
		},
	)
}

// SetupWithManager sets up the controller with the Manager.
func (r *NetworkReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&otcv1alpha1.Network{}).
		Named("network").
		Complete(r)
}
