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
	securityGroupRuleFinalizerName = "securitygrouprule.otc.peertech.de/finalizer"
	securityGroupRuleRequeueDelay  = 30 * time.Second
)

func NewSecurityGroupRuleReconciler(
	c client.Client,
	scheme *runtime.Scheme,
	logger zerolog.Logger,
	providers *ProviderCache,
) *SecurityGroupRuleReconciler {
	return &SecurityGroupRuleReconciler{
		Client:    c,
		Scheme:    scheme,
		logger:    logger.With().Str("controller", "security-group-rule").Logger(),
		providers: providers,
	}
}

// SecurityGroupRuleReconciler reconciles a SecurityGroupRule object
type SecurityGroupRuleReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	logger    zerolog.Logger
	providers *ProviderCache
}

// +kubebuilder:rbac:groups=otc.peertech.de,resources=securitygrouprules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=otc.peertech.de,resources=securitygrouprules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=otc.peertech.de,resources=securitygrouprules/finalizers,verbs=update
// +kubebuilder:rbac:groups=otc.peertech.de,resources=securitygroups,verbs=get;list;watch
// +kubebuilder:rbac:groups=otc.peertech.de,resources=providerconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *SecurityGroupRuleReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	scopedLogger := r.logger.With().
		Str("op", "Reconcile").
		Str("security-group-rule", req.NamespacedName.Name).
		Str("namespace", req.NamespacedName.Namespace).
		Logger()

	var securityGroupRule otcv1alpha1.SecurityGroupRule
	if err := r.Get(ctx, req.NamespacedName, &securityGroupRule); err != nil {
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
		object:         &securityGroupRule,
		originalObject: securityGroupRule.DeepCopy(),
		conditions:     &securityGroupRule.Status.Conditions,
		generation:     securityGroupRule.Generation,
		finalizerName:  securityGroupRuleFinalizerName,
		requeueAfter:   securityGroupRuleRequeueDelay,
	}

	// Ensure the status is updated.
	defer rc.UpdateStatus(ctx)

	// Handle deletion.
	if !securityGroupRule.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, rc, &securityGroupRule)
	}

	// Ensure the finalizer is present.
	if added, result, err := rc.AddFinalizer(ctx); added {
		return result, err
	}

	// Check if the referenced ProviderConfig is ready.
	_, shouldReque, result, err := rc.CheckProviderConfig(
		ctx,
		securityGroupRule.Spec.ProviderConfigRef,
	)
	if shouldReque {
		return result, err
	}

	// Get or create cached provider client.
	p, _, err := r.providers.GetOrCreate(
		ctx,
		securityGroupRule.Spec.ProviderConfigRef,
		securityGroupRule.Namespace,
	)
	if err != nil {
		rc.SetReconciliationFailed(
			WithReason(reasonProviderConfigError),
			WithMessage(err.Error()),
		)
		scopedLogger.Error().Err(err).Msg("Failed to get or create provider client")
		return ctrl.Result{RequeueAfter: securityGroupRuleRequeueDelay}, nil
	}

	return r.reconcile(ctx, scopedLogger, rc, &securityGroupRule, p)
}

func (r *SecurityGroupRuleReconciler) reconcile(
	ctx context.Context,
	logger zerolog.Logger,
	rc *Reconciler,
	securityGroupRule *otcv1alpha1.SecurityGroupRule,
	p provider.Provider,
) (ctrl.Result, error) {
	// If the external resource has no known ID, it needs to be created.
	if securityGroupRule.Status.ExternalID == "" {
		return r.reconcileCreate(ctx, logger, rc, securityGroupRule, p)
	}

	return r.reconcileUpdate(ctx, logger, rc, securityGroupRule, p)
}

// reconcileCreate handles the logic for creating a new external resource.
func (r *SecurityGroupRuleReconciler) reconcileCreate(
	ctx context.Context,
	logger zerolog.Logger,
	rc *Reconciler,
	securityGroupRule *otcv1alpha1.SecurityGroupRule,
	p provider.Provider,
) (ctrl.Result, error) {
	// Resolve dependencies.
	resolver := NewDependencyResolver(r.Client, securityGroupRule.Namespace)
	securityGroupID, err := resolver.ResolveSecurityGroup(ctx, securityGroupRule.Spec.SecurityGroup)
	if err != nil {
		rc.SetDependenciesNotReady(err.Error())
		rc.SetNotReady(
			WithReason(reasonDependenciesNotResolved),
			WithMessagef("Waiting for dependencies: %v", err),
		)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	rc.SetDependenciesReady()
	securityGroupRule.Status.ResolvedDependencies.SecurityGroupID = securityGroupID

	// Create the external resource.
	logger.Info().Msg("Creating security group rule")

	// Set creating status.
	rc.SetCreating()

	createReq := provider.CreateSecurityGroupRuleRequest{
		Name:        securityGroupRule.GetName(),
		Description: securityGroupRule.Spec.Description,
		Direction:   string(securityGroupRule.Spec.Direction),
		Protocol:    string(securityGroupRule.Spec.Protocol),
		EtherType:   string(securityGroupRule.Spec.Ethertype),
		Multiport:   securityGroupRule.Spec.Multiport,
		Action:      string(securityGroupRule.Spec.Action),
		Priority:    securityGroupRule.Spec.Priority,
	}
	if securityGroupRule.Spec.Priority != nil {
		createReq.Priority = securityGroupRule.Spec.Priority
	}

	resp, err := p.CreateSecurityGroupRule(
		ctx,
		createReq,
	)
	if err != nil {
		rc.SetReconciliationFailed(
			WithReason(reasonProvisioningFailed),
			WithMessagef("Failed to create resource: %v", err),
		)
		logger.Error().Err(err).Msg("Failed to create security group rule")
		return ctrl.Result{RequeueAfter: securityGroupRuleRequeueDelay}, nil
	}

	// Update status fields.
	securityGroupRule.Status.ExternalID = resp.ID
	securityGroupRule.Status.LastAppliedSpec = securityGroupRule.Spec.DeepCopy()

	logger.Info().
		Str("external-id", resp.ID).
		Msg("Successfully created security group rule")

	return ctrl.Result{}, nil
}

// reconcileUpdate handles the logic for an existing external resource. It
// checks for drift, updates the resource and reports its status.
func (r *SecurityGroupRuleReconciler) reconcileUpdate(
	ctx context.Context,
	logger zerolog.Logger,
	rc *Reconciler,
	securityGroupRule *otcv1alpha1.SecurityGroupRule,
	p provider.Provider,
) (ctrl.Result, error) {
	lastAppliedSpec := securityGroupRule.Status.LastAppliedSpec
	if lastAppliedSpec == nil {
		logger.Warn().Msg("LastAppliedSpec is not set, establishing baseline from current spec.")
		securityGroupRule.Status.LastAppliedSpec = securityGroupRule.Spec.DeepCopy()
		// Requeue to ensure the status update is persisted before proceeding.
		return ctrl.Result{Requeue: true}, nil
	}

	// Fetch the external resource.
	info, err := p.GetSecurityGroupRule(ctx, securityGroupRule.Status.ExternalID)
	if err != nil {
		// TODO: this might be to harsh, as the resource could be fully
		// functional, but the server API is unreachable.
		rc.SetReconciliationFailed(
			WithReason(reasonProviderError),
			WithMessagef("Failed to check existing SecurityGroupRule: %v", err),
		)
		logger.Error().Err(err).Msg("Failed to check existing security group rule")
		return ctrl.Result{RequeueAfter: securityGroupRuleRequeueDelay}, nil
	}

	// Handle resource being deleted out-of-band. This can happen if the
	// resource was deleted manually from the provider. We will trigger the
	// creation logic in the next reconciliation.
	if info == nil {
		logger.Warn().
			Msg("External security group not found by ID, resetting externalID to trigger creation")

		rc.SetNotSynced(
			WithReason(reasonNotFound),
			WithMessagef(
				"External resource with ID %s was not found and will be recreated",
				securityGroupRule.Status.ExternalID,
			),
		)
		rc.SetNotReady(
			WithReason(reasonNotFound),
			WithMessage("Resource needs to be recreated"),
		)

		// Reset status fields.
		securityGroupRule.Status.ExternalID = ""
		securityGroupRule.Status.LastAppliedSpec = nil
		return ctrl.Result{Requeue: true}, nil
	}

	logger.Debug().
		Str("external-id", info.ID).
		Msg("Found existing security group")

	updateReq, needsUpdate := r.detectDrift(logger, securityGroupRule)
	if needsUpdate {
		return r.handleDrift(ctx, logger, p, rc, securityGroupRule, updateReq)
	}

	// Check readiness status.
	return r.checkReadiness(rc, securityGroupRule, info)
}

func (r *SecurityGroupRuleReconciler) detectDrift(
	_ zerolog.Logger,
	_ *otcv1alpha1.SecurityGroupRule,
) (provider.UpdateSecurityGroupRuleRequest, bool) {
	return provider.UpdateSecurityGroupRuleRequest{}, false
}

// handleDrift applies updates to the drifted resource.
func (r *SecurityGroupRuleReconciler) handleDrift(
	_ context.Context,
	_ zerolog.Logger,
	_ provider.Provider,
	_ *Reconciler,
	_ *otcv1alpha1.SecurityGroupRule,
	_ provider.UpdateSecurityGroupRuleRequest,
) (ctrl.Result, error) {
	// Requeue immediately to re-check the status after the update.
	return ctrl.Result{Requeue: true}, nil
}

// checkReadiness updates the status conditions based on the provider's reported status.
func (r *SecurityGroupRuleReconciler) checkReadiness(
	rc *Reconciler,
	securityGroupRule *otcv1alpha1.SecurityGroupRule,
	info *provider.SecurityGroupRuleInfo,
) (ctrl.Result, error) {
	switch info.State() {
	case provider.Ready:
		now := metav1.Now()

		isNewlyProvisioned := securityGroupRule.Status.LastSyncTime == nil
		securityGroupRule.Status.LastSyncTime = &now

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
		return ctrl.Result{RequeueAfter: securityGroupRuleRequeueDelay}, nil
	case provider.Provisioning:
		rc.SetProvisioning(WithMessage(info.Message()))
		return ctrl.Result{RequeueAfter: securityGroupRuleRequeueDelay}, nil
	default:
		rc.SetReconciliationFailed(
			WithReason(reasonUnknown),
			WithMessage(info.Message()),
		)
		return ctrl.Result{RequeueAfter: securityGroupRuleRequeueDelay}, nil
	}
}

func (r *SecurityGroupRuleReconciler) reconcileDelete(
	ctx context.Context,
	rc *Reconciler,
	securityGroupRule *otcv1alpha1.SecurityGroupRule,
) (ctrl.Result, error) {
	return rc.Delete(
		ctx,
		securityGroupRule.Spec.ProviderConfigRef,
		securityGroupRule.Spec.OrphanOnDelete,
		securityGroupRule.Status.ExternalID,
		func(c context.Context, p provider.Provider) error {
			return p.DeleteSecurityGroupRule(c, securityGroupRule.Status.ExternalID)
		},
	)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SecurityGroupRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&otcv1alpha1.SecurityGroupRule{}).
		Named("securitygrouprule").
		Complete(r)
}
