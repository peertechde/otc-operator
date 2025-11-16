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
	publicIPFinalizerName = "publicIP.otc.peertech.de/finalizer"
	publicIPRequeueDelay  = 30 * time.Second

	bandwidthPrefix = "bandwidth-"
)

func NewPublicIPReconciler(
	c client.Client,
	scheme *runtime.Scheme,
	logger zerolog.Logger,
	providers *ProviderCache,
) *PublicIPReconciler {
	return &PublicIPReconciler{
		Client:    c,
		Scheme:    scheme,
		logger:    logger.With().Str("controller", "public-ip").Logger(),
		providers: providers,
	}
}

// PublicIPReconciler reconciles a PublicIP object
type PublicIPReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	logger    zerolog.Logger
	providers *ProviderCache
}

// +kubebuilder:rbac:groups=otc.peertech.de,resources=publicips,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=otc.peertech.de,resources=publicips/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=otc.peertech.de,resources=publicips/finalizers,verbs=update
// +kubebuilder:rbac:groups=otc.peertech.de,resources=providerconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *PublicIPReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	scopedLogger := r.logger.With().
		Str("public-ip", req.NamespacedName.Name).
		Str("namespace", req.NamespacedName.Namespace).
		Logger()

	var publicIP otcv1alpha1.PublicIP
	if err := r.Get(ctx, req.NamespacedName, &publicIP); err != nil {
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
		object:         &publicIP,
		originalObject: publicIP.DeepCopy(),
		conditions:     &publicIP.Status.Conditions,
		generation:     publicIP.Generation,
		finalizerName:  publicIPFinalizerName,
		requeueAfter:   publicIPRequeueDelay,
	}

	// Ensure the status is updated.
	defer rc.UpdateStatus(ctx)

	// Handle deletion.
	if !publicIP.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, rc, &publicIP)
	}

	// Ensure the finalizer is present.
	if added, result, err := rc.AddFinalizer(ctx); added {
		return result, err
	}

	// Check if the referenced ProviderConfig is ready.
	_, shouldReque, result, err := rc.CheckProviderConfig(
		ctx,
		publicIP.Spec.ProviderConfigRef,
	)
	if shouldReque {
		return result, err
	}

	// Get or create cached provider client.
	p, _, err := r.providers.GetOrCreate(ctx, publicIP.Spec.ProviderConfigRef, publicIP.Namespace)
	if err != nil {
		rc.SetReconciliationFailed(
			WithReason(reasonProviderConfigError),
			WithMessage(err.Error()),
		)
		scopedLogger.Error().Err(err).Msg("Failed to get or create provider client")
		return ctrl.Result{RequeueAfter: publicIPRequeueDelay}, nil
	}

	return r.reconcile(ctx, scopedLogger, rc, &publicIP, p)
}

func (r *PublicIPReconciler) reconcile(
	ctx context.Context,
	logger zerolog.Logger,
	rc *Reconciler,
	publicIP *otcv1alpha1.PublicIP,
	p provider.Provider,
) (ctrl.Result, error) {
	// If the external resource has no known ID, it needs to be created.
	if publicIP.Status.ExternalID == "" {
		return r.reconcileCreate(ctx, logger, rc, publicIP, p)
	}

	return r.reconcileUpdate(ctx, logger, rc, publicIP, p)
}

// reconcileCreate handles the logic for creating a new external resource.
func (r *PublicIPReconciler) reconcileCreate(
	ctx context.Context,
	logger zerolog.Logger,
	rc *Reconciler,
	publicIP *otcv1alpha1.PublicIP,
	p provider.Provider,
) (ctrl.Result, error) {
	logger.Info().Msg("Creating public IP")

	// Set creating status.
	rc.SetCreating()

	resp, err := p.CreatePublicIP(
		ctx,
		provider.CreatePublicIPRequest{
			Name:               publicIP.GetName(),
			Type:               publicIP.Spec.Type,
			BandwidthName:      bandwidthPrefix + publicIP.GetName(),
			BandwidthSize:      publicIP.Spec.BandwidthSize,
			BandwidthShareType: publicIP.Spec.BandwidthShareType,
		},
	)
	if err != nil {
		rc.SetReconciliationFailed(
			WithReason(reasonProvisioningFailed),
			WithMessagef("Failed to create resource: %v", err),
		)
		logger.Error().Err(err).Msg("Failed to create public IP")
		return ctrl.Result{RequeueAfter: publicIPRequeueDelay}, nil
	}

	// Update status fields.
	publicIP.Status.ExternalID = resp.ID
	publicIP.Status.LastAppliedSpec = publicIP.Spec.DeepCopy()

	logger.Info().
		Str("external-id", resp.ID).
		Msg("Successfully created public IP")

	return ctrl.Result{}, nil
}

// reconcileUpdate handles the logic for an existing external resource. It
// checks for drift, updates the resource and reports its status.
func (r *PublicIPReconciler) reconcileUpdate(
	ctx context.Context,
	logger zerolog.Logger,
	rc *Reconciler,
	publicIP *otcv1alpha1.PublicIP,
	p provider.Provider,
) (ctrl.Result, error) {
	lastAppliedSpec := publicIP.Status.LastAppliedSpec
	if lastAppliedSpec == nil {
		logger.Warn().Msg("LastAppliedSpec is not set, establishing baseline from current spec.")
		publicIP.Status.LastAppliedSpec = publicIP.Spec.DeepCopy()
		// Requeue to ensure the status update is persisted before proceeding.
		return ctrl.Result{Requeue: true}, nil
	}

	// Fetch the external resource.
	info, err := p.GetPublicIP(ctx, publicIP.Status.ExternalID)
	if err != nil {
		// TODO: this might be to harsh, as the resource could be fully
		// functional, but the server API is unreachable.
		rc.SetReconciliationFailed(
			WithReason(reasonProviderError),
			WithMessagef("Failed to check existing Public IP: %v", err),
		)
		logger.Error().Err(err).Msg("Failed to check existing public IP")
		return ctrl.Result{RequeueAfter: publicIPRequeueDelay}, nil
	}

	// Handle resource being deleted out-of-band. This can happen if the
	// resource was deleted manually from the provider. We will trigger the
	// creation logic in the next reconciliation.
	if info == nil {
		logger.Warn().
			Msg("External public IP not found by ID, resetting externalID to trigger creation")

		rc.SetNotSynced(
			WithReason(reasonNotFound),
			WithMessagef(
				"External resource with ID %s was not found and will be recreated",
				publicIP.Status.ExternalID,
			),
		)
		rc.SetNotReady(
			WithReason(reasonNotFound),
			WithMessage("Resource needs to be recreated"),
		)

		// Reset status fields
		publicIP.Status.ExternalID = ""
		publicIP.Status.LastAppliedSpec = nil
		return ctrl.Result{Requeue: true}, nil
	}

	logger.Debug().
		Str("external-id", info.ID).
		Str("status", info.Status).
		Msg("Found existing public IP")

	updateReq, needsUpdate := r.detectDrift(logger, publicIP)
	if needsUpdate {
		return r.handleDrift(ctx, logger, p, rc, publicIP, updateReq)
	}

	// Check readiness status.
	return r.checkReadiness(rc, publicIP, info)
}

func (r *PublicIPReconciler) detectDrift(
	_ zerolog.Logger,
	_ *otcv1alpha1.PublicIP,
) (provider.UpdatePublicIPRequest, bool) {
	return provider.UpdatePublicIPRequest{}, false
}

// handleDrift applies updates to the drifted resource.
func (r *PublicIPReconciler) handleDrift(
	_ context.Context,
	_ zerolog.Logger,
	_ provider.Provider,
	_ *Reconciler,
	_ *otcv1alpha1.PublicIP,
	_ provider.UpdatePublicIPRequest,
) (ctrl.Result, error) {
	// Requeue immediately to re-check the status after the update.
	return ctrl.Result{Requeue: true}, nil
}

// checkReadiness updates the status conditions based on the provider's reported status.
func (r *PublicIPReconciler) checkReadiness(
	rc *Reconciler,
	publicIP *otcv1alpha1.PublicIP,
	info *provider.PublicIPInfo,
) (ctrl.Result, error) {
	switch info.State() {
	case provider.Ready:
		now := metav1.Now()

		isNewlyProvisioned := publicIP.Status.LastSyncTime == nil
		publicIP.Status.LastSyncTime = &now

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
		return ctrl.Result{RequeueAfter: publicIPRequeueDelay}, nil
	case provider.Provisioning:
		rc.SetProvisioning(WithMessage(info.Message()))
		return ctrl.Result{RequeueAfter: publicIPRequeueDelay}, nil
	default:
		rc.SetReconciliationFailed(
			WithReason(reasonUnknown),
			WithMessage(info.Message()),
		)
		return ctrl.Result{RequeueAfter: publicIPRequeueDelay}, nil
	}
}

func (r *PublicIPReconciler) reconcileDelete(
	ctx context.Context,
	rc *Reconciler,
	publicIP *otcv1alpha1.PublicIP,
) (ctrl.Result, error) {
	// If the Public IP never got an external ID, it couldn't have had any rules
	// created for it, so we can safely proceed with deletion.
	if publicIP.Status.ExternalID == "" {
		return rc.Delete(
			ctx,
			publicIP.Spec.ProviderConfigRef,
			publicIP.Spec.OrphanOnDelete,
			publicIP.Status.ExternalID,
			func(c context.Context, p provider.Provider) error {
				return nil
			},
		)
	}

	// Check if any SNAT rules are still referencing this Subnet.
	blocked, result, err := rc.BlockOnAnyReference(
		ctx,
		publicIP.Namespace,
		publicIP.Status.ExternalID,
		SNATRuleNetworkReferenceCheck{},
	)
	if blocked {
		return result, err
	}

	return rc.Delete(
		ctx,
		publicIP.Spec.ProviderConfigRef,
		publicIP.Spec.OrphanOnDelete,
		publicIP.Status.ExternalID,
		func(c context.Context, p provider.Provider) error {
			return p.DeletePublicIP(c, publicIP.Status.ExternalID)
		},
	)
}

// SetupWithManager sets up the controller with the Manager.
func (r *PublicIPReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&otcv1alpha1.PublicIP{}).
		Named("publicip").
		Complete(r)
}
