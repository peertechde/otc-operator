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
	snatRuleFinalizerName = "snatRule.otc.peertech.de/finalizer"
	snatRuleRequeueDelay  = 30 * time.Second
)

func NewSNATRuleReconciler(
	c client.Client,
	scheme *runtime.Scheme,
	logger zerolog.Logger,
	providers *ProviderCache,
) *SNATRuleReconciler {
	return &SNATRuleReconciler{
		Client:    c,
		Scheme:    scheme,
		logger:    logger.With().Str("controller", "snat-rule").Logger(),
		providers: providers,
	}
}

// SNATRuleReconciler reconciles a SNATRule object
type SNATRuleReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	logger    zerolog.Logger
	providers *ProviderCache
}

// +kubebuilder:rbac:groups=otc.peertech.de,resources=snatrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=otc.peertech.de,resources=snatrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=otc.peertech.de,resources=snatrules/finalizers,verbs=update
// +kubebuilder:rbac:groups=otc.peertech.de,resources=natgateways,verbs=get;list;watch
// +kubebuilder:rbac:groups=otc.peertech.de,resources=subnets,verbs=get;list;watch
// +kubebuilder:rbac:groups=otc.peertech.de,resources=publicips,verbs=get;list;watch
// +kubebuilder:rbac:groups=otc.peertech.de,resources=providerconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *SNATRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	scopedLogger := r.logger.With().
		Str("snat-rule", req.NamespacedName.Name).
		Str("namespace", req.NamespacedName.Namespace).
		Logger()

	var snatRule otcv1alpha1.SNATRule
	if err := r.Get(ctx, req.NamespacedName, &snatRule); err != nil {
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
		object:         &snatRule,
		originalObject: snatRule.DeepCopy(),
		conditions:     &snatRule.Status.Conditions,
		generation:     snatRule.Generation,
		finalizerName:  snatRuleFinalizerName,
		requeueAfter:   snatRuleRequeueDelay,
	}

	// Ensure the status is updated.
	defer rc.UpdateStatus(ctx)

	// Handle deletion.
	if !snatRule.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, rc, &snatRule)
	}

	// Ensure the finalizer is present.
	if added, result, err := rc.AddFinalizer(ctx); added {
		return result, err
	}

	// Check if the referenced ProviderConfig is ready.
	_, shouldReque, result, err := rc.CheckProviderConfig(
		ctx,
		snatRule.Spec.ProviderConfigRef,
	)
	if shouldReque {
		return result, err
	}

	// Get or create cached provider client.
	p, _, err := r.providers.GetOrCreate(ctx, snatRule.Spec.ProviderConfigRef, snatRule.Namespace)
	if err != nil {
		rc.SetReconciliationFailed(
			WithReason(reasonProviderConfigError),
			WithMessage(err.Error()),
		)
		scopedLogger.Error().Err(err).Msg("Failed to get or create provider client")
		return ctrl.Result{RequeueAfter: snatRuleRequeueDelay}, nil
	}

	return r.reconcile(ctx, scopedLogger, rc, &snatRule, p)
}

func (r *SNATRuleReconciler) reconcile(
	ctx context.Context,
	logger zerolog.Logger,
	rc *Reconciler,
	snatRule *otcv1alpha1.SNATRule,
	p provider.Provider,
) (ctrl.Result, error) {
	// If the external resource has no known ID, it needs to be created.
	if snatRule.Status.ExternalID == "" {
		return r.reconcileCreate(ctx, logger, rc, snatRule, p)
	}

	return r.reconcileUpdate(ctx, logger, rc, snatRule, p)
}

// reconcileCreate handles the logic for creating a new external resource.
func (r *SNATRuleReconciler) reconcileCreate(
	ctx context.Context,
	logger zerolog.Logger,
	rc *Reconciler,
	snatRule *otcv1alpha1.SNATRule,
	p provider.Provider,
) (ctrl.Result, error) {
	// Resolve dependencies.
	resolver := NewDependencyResolver(r.Client, snatRule.Namespace)
	natGatewayID, subnetID, publicIPID, err := resolver.ResolveSNATRuleDependencies(
		ctx,
		snatRule.Spec,
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
	snatRule.Status.ResolvedDependencies = otcv1alpha1.SNATRuleDependenciesResolved{
		NATGatewayID: natGatewayID,
		SubnetID:     subnetID,
		PublicIPID:   publicIPID,
	}

	// Create the external resource.
	logger.Info().Msg("Creating SNAT rule")

	// Set creating status.
	rc.SetCreating()

	resp, err := p.CreateSNATRule(
		ctx,
		provider.CreateSNATRuleRequest{
			Description:  snatRule.Spec.Description,
			NATGatewayID: natGatewayID,
			SubnetID:     subnetID,
			PublicIPID:   publicIPID,
		},
	)
	if err != nil {
		rc.SetReconciliationFailed(
			WithReason(reasonProvisioningFailed),
			WithMessagef("Failed to create resource: %v", err),
		)
		logger.Error().Err(err).Msg("Failed to create SNAT rule")
		return ctrl.Result{RequeueAfter: snatRuleRequeueDelay}, nil
	}

	// Update status fields.
	snatRule.Status.ExternalID = resp.ID
	snatRule.Status.LastAppliedSpec = snatRule.Spec.DeepCopy()

	logger.Info().
		Str("external-id", resp.ID).
		Msg("Successfully created SNAT rule")

	// Requeue immediately to check the status of the new resource.
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// reconcileUpdate handles the logic for an existing external resource. It
// checks for drift, updates the resource and reports its status.
func (r *SNATRuleReconciler) reconcileUpdate(
	ctx context.Context,
	logger zerolog.Logger,
	rc *Reconciler,
	snatRule *otcv1alpha1.SNATRule,
	p provider.Provider,
) (ctrl.Result, error) {
	lastAppliedSpec := snatRule.Status.LastAppliedSpec
	if lastAppliedSpec == nil {
		logger.Warn().Msg("LastAppliedSpec is not set, establishing baseline from current spec.")
		snatRule.Status.LastAppliedSpec = snatRule.Spec.DeepCopy()
		// Requeue to ensure the status update is persisted before proceeding.
		return ctrl.Result{Requeue: true}, nil
	}

	// Fetch the external resource.
	info, err := p.GetSNATRule(ctx, snatRule.Status.ExternalID)
	if err != nil {
		// TODO: this might be to harsh, as the resource could be fully
		// functional, but the server API is unreachable.
		rc.SetReconciliationFailed(
			WithReason(reasonProviderError),
			WithMessagef("Failed to check existing Public IP: %v", err),
		)
		logger.Error().Err(err).Msg("Failed to check existing public IP")
		return ctrl.Result{RequeueAfter: snatRuleRequeueDelay}, nil
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
				snatRule.Status.ExternalID,
			),
		)
		rc.SetNotReady(
			WithReason(reasonNotFound),
			WithMessage("Resource needs to be recreated"),
		)

		// Reset status fields.
		snatRule.Status.ExternalID = ""
		snatRule.Status.LastAppliedSpec = nil
		return ctrl.Result{Requeue: true}, nil
	}

	logger.Debug().
		Str("external-id", info.ID).
		Str("status", info.Status).
		Msg("Found existing public IP")

	updateReq, needsUpdate := r.detectDrift(logger, snatRule)
	if needsUpdate {
		return r.handleDrift(ctx, logger, p, rc, snatRule, updateReq)
	}

	// Check readiness status.
	return r.checkReadiness(rc, snatRule, info)
}

func (r *SNATRuleReconciler) detectDrift(
	_ zerolog.Logger,
	_ *otcv1alpha1.SNATRule,
) (provider.UpdateSNATRuleRequest, bool) {
	return provider.UpdateSNATRuleRequest{}, false
}

// handleDrift applies updates to the drifted resource.
func (r *SNATRuleReconciler) handleDrift(
	_ context.Context,
	_ zerolog.Logger,
	_ provider.Provider,
	_ *Reconciler,
	_ *otcv1alpha1.SNATRule,
	_ provider.UpdateSNATRuleRequest,
) (ctrl.Result, error) {
	// Requeue immediately to re-check the status after the update.
	return ctrl.Result{Requeue: true}, nil
}

// checkReadiness updates the status conditions based on the provider's reported status.
func (r *SNATRuleReconciler) checkReadiness(
	rc *Reconciler,
	snatRule *otcv1alpha1.SNATRule,
	info *provider.SNATRuleInfo,
) (ctrl.Result, error) {
	switch info.State() {
	case provider.Ready:
		now := metav1.Now()

		isNewlyProvisioned := snatRule.Status.LastSyncTime == nil
		snatRule.Status.LastSyncTime = &now

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
		return ctrl.Result{RequeueAfter: snatRuleRequeueDelay}, nil
	case provider.Provisioning:
		rc.SetProvisioning(WithMessage(info.Message()))
		return ctrl.Result{RequeueAfter: snatRuleRequeueDelay}, nil
	default:
		rc.SetReconciliationFailed(
			WithReason(reasonUnknown),
			WithMessage(info.Message()),
		)
		return ctrl.Result{RequeueAfter: snatRuleRequeueDelay}, nil
	}
}

func (r *SNATRuleReconciler) reconcileDelete(
	ctx context.Context,
	rc *Reconciler,
	snatRule *otcv1alpha1.SNATRule,
) (ctrl.Result, error) {
	return rc.Delete(
		ctx,
		snatRule.Spec.ProviderConfigRef,
		snatRule.Spec.OrphanOnDelete,
		snatRule.Status.ExternalID,
		func(c context.Context, p provider.Provider) error {
			return p.DeleteSNATRule(c, snatRule.Status.ExternalID)
		},
	)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SNATRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&otcv1alpha1.SNATRule{}).
		Named("snatrule").
		Complete(r)
}
