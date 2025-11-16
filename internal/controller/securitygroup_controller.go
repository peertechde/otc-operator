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
	securityGroupFinalizerName = "securitygroup.otc.peertech.de/finalizer"
	securityGroupRequeueDelay  = 30 * time.Second
)

func NewSecurityGroupReconciler(
	c client.Client,
	scheme *runtime.Scheme,
	logger zerolog.Logger,
	providers *ProviderCache,
) *SecurityGroupReconciler {
	return &SecurityGroupReconciler{
		Client:    c,
		Scheme:    scheme,
		logger:    logger.With().Str("controller", "security-group").Logger(),
		providers: providers,
	}
}

// SecurityGroupReconciler reconciles a SecurityGroup object
type SecurityGroupReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	logger    zerolog.Logger
	providers *ProviderCache
}

// +kubebuilder:rbac:groups=otc.peertech.de,resources=securitygroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=otc.peertech.de,resources=securitygroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=otc.peertech.de,resources=securitygroups/finalizers,verbs=update
// +kubebuilder:rbac:groups=otc.peertech.de,resources=providerconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *SecurityGroupReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	scopedLogger := r.logger.With().
		Str("security-group", req.NamespacedName.Name).
		Str("namespace", req.NamespacedName.Namespace).
		Logger()

	var securityGroup otcv1alpha1.SecurityGroup
	if err := r.Get(ctx, req.NamespacedName, &securityGroup); err != nil {
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
		object:         &securityGroup,
		originalObject: securityGroup.DeepCopy(),
		conditions:     &securityGroup.Status.Conditions,
		generation:     securityGroup.Generation,
		finalizerName:  securityGroupFinalizerName,
		requeueAfter:   securityGroupRequeueDelay,
	}

	// Ensure the status is updated.
	defer rc.UpdateStatus(ctx)

	// Handle deletion.
	if !securityGroup.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, rc, &securityGroup)
	}

	// Ensure the finalizer is present.
	if added, result, err := rc.AddFinalizer(ctx); added {
		return result, err
	}

	// Check if the referenced ProviderConfig is ready.
	_, shouldReque, result, err := rc.CheckProviderConfig(
		ctx,
		securityGroup.Spec.ProviderConfigRef,
	)
	if shouldReque {
		return result, err
	}

	// Get or create cached provider client.
	p, _, err := r.providers.GetOrCreate(
		ctx,
		securityGroup.Spec.ProviderConfigRef,
		securityGroup.Namespace,
	)
	if err != nil {
		rc.SetReconciliationFailed(
			WithReason(reasonProviderConfigError),
			WithMessage(err.Error()),
		)
		scopedLogger.Error().Err(err).Msg("Failed to get or create provider client")
		return ctrl.Result{RequeueAfter: securityGroupRequeueDelay}, nil
	}

	return r.reconcile(ctx, scopedLogger, rc, &securityGroup, p)
}

func (r *SecurityGroupReconciler) reconcile(
	ctx context.Context,
	logger zerolog.Logger,
	rc *Reconciler,
	securityGroup *otcv1alpha1.SecurityGroup,
	p provider.Provider,
) (ctrl.Result, error) {
	// If the external resource has no known ID, it needs to be created.
	if securityGroup.Status.ExternalID == "" {
		return r.reconcileCreate(ctx, logger, rc, securityGroup, p)
	}

	return r.reconcileUpdate(ctx, logger, rc, securityGroup, p)
}

// reconcileCreate handles the logic for creating a new external resource.
func (r *SecurityGroupReconciler) reconcileCreate(
	ctx context.Context,
	logger zerolog.Logger,
	rc *Reconciler,
	securityGroup *otcv1alpha1.SecurityGroup,
	p provider.Provider,
) (ctrl.Result, error) {
	logger.Info().Msg("Creating security group")

	// Set creating status.
	rc.SetCreating()

	resp, err := p.CreateSecurityGroup(
		ctx,
		provider.CreateSecurityGroupRequest{
			Name:        securityGroup.GetName(),
			Description: securityGroup.Spec.Description,
		},
	)
	if err != nil {
		rc.SetReconciliationFailed(
			WithReason(reasonProvisioningFailed),
			WithMessagef("Failed to create resource: %v", err),
		)
		logger.Error().Err(err).Msg("Failed to create security group")
		return ctrl.Result{RequeueAfter: securityGroupRequeueDelay}, nil
	}

	// Update status fields.
	securityGroup.Status.ExternalID = resp.ID
	securityGroup.Status.LastAppliedSpec = securityGroup.Spec.DeepCopy()

	logger.Info().
		Str("external-id", resp.ID).
		Msg("Successfully created security group")

	return ctrl.Result{}, nil
}

// reconcileUpdate handles the logic for an existing external resource. It
// checks for drift, updates the resource and reports its status.
func (r *SecurityGroupReconciler) reconcileUpdate(
	ctx context.Context,
	logger zerolog.Logger,
	rc *Reconciler,
	securityGroup *otcv1alpha1.SecurityGroup,
	p provider.Provider,
) (ctrl.Result, error) {
	lastAppliedSpec := securityGroup.Status.LastAppliedSpec
	if lastAppliedSpec == nil {
		logger.Warn().Msg("LastAppliedSpec is not set, establishing baseline from current spec.")
		securityGroup.Status.LastAppliedSpec = securityGroup.Spec.DeepCopy()
		// Requeue to ensure the status update is persisted before proceeding.
		return ctrl.Result{Requeue: true}, nil
	}

	// Fetch the external resource.
	info, err := p.GetSecurityGroup(ctx, securityGroup.Status.ExternalID)
	if err != nil {
		// TODO: this might be to harsh, as the resource could be fully
		// functional, but the server API is unreachable.
		rc.SetReconciliationFailed(
			WithReason(reasonProviderError),
			WithMessagef("Failed to check existing SecurityGroup: %v", err),
		)
		logger.Error().Err(err).Msg("Failed to check existing security group")
		return ctrl.Result{RequeueAfter: securityGroupRequeueDelay}, nil
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
				securityGroup.Status.ExternalID,
			),
		)
		rc.SetNotReady(
			WithReason(reasonNotFound),
			WithMessage("Resource needs to be recreated"),
		)

		// Reset status fields.
		securityGroup.Status.ExternalID = ""
		securityGroup.Status.LastAppliedSpec = nil
		return ctrl.Result{Requeue: true}, nil
	}

	logger.Debug().
		Str("external-id", info.ID).
		Msg("Found existing security group")

	updateReq, needsUpdate := r.detectDrift(logger, securityGroup)
	if needsUpdate {
		return r.handleDrift(ctx, logger, p, rc, securityGroup, updateReq)
	}

	// Check readiness status.
	return r.checkReadiness(rc, securityGroup, info)
}

func (r *SecurityGroupReconciler) detectDrift(
	_ zerolog.Logger,
	_ *otcv1alpha1.SecurityGroup,
) (provider.UpdateSecurityGroupRequest, bool) {
	return provider.UpdateSecurityGroupRequest{}, false
}

// handleDrift applies updates to the drifted resource.
func (r *SecurityGroupReconciler) handleDrift(
	_ context.Context,
	_ zerolog.Logger,
	_ provider.Provider,
	_ *Reconciler,
	_ *otcv1alpha1.SecurityGroup,
	_ provider.UpdateSecurityGroupRequest,
) (ctrl.Result, error) {
	// Requeue immediately to re-check the status after the update.
	return ctrl.Result{Requeue: true}, nil
}

// checkReadiness updates the status conditions based on the provider's reported status.
func (r *SecurityGroupReconciler) checkReadiness(
	rc *Reconciler,
	securityGroup *otcv1alpha1.SecurityGroup,
	info *provider.SecurityGroupInfo,
) (ctrl.Result, error) {
	switch info.State() {
	case provider.Ready:
		now := metav1.Now()

		isNewlyProvisioned := securityGroup.Status.LastSyncTime == nil
		securityGroup.Status.LastSyncTime = &now

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
		return ctrl.Result{RequeueAfter: securityGroupRequeueDelay}, nil
	case provider.Provisioning:
		rc.SetProvisioning(WithMessage(info.Message()))
		return ctrl.Result{RequeueAfter: securityGroupRequeueDelay}, nil
	default:
		rc.SetReconciliationFailed(
			WithReason(reasonUnknown),
			WithMessage(info.Message()),
		)
		return ctrl.Result{RequeueAfter: securityGroupRequeueDelay}, nil
	}
}

func (r *SecurityGroupReconciler) reconcileDelete(
	ctx context.Context,
	rc *Reconciler,
	securityGroup *otcv1alpha1.SecurityGroup,
) (ctrl.Result, error) {
	// If the security group never got an external ID, it couldn't have had any
	// rules created for it, so we can safely proceed with deletion.
	if securityGroup.Status.ExternalID == "" {
		return rc.Delete(
			ctx,
			securityGroup.Spec.ProviderConfigRef,
			securityGroup.Spec.OrphanOnDelete,
			securityGroup.Status.ExternalID,
			func(c context.Context, p provider.Provider) error {
				return nil
			},
		)
	}

	// Check if any SecurityGroupRules are still referencing this SecurityGroup.
	blocked, result, err := rc.BlockOnAnyReference(
		ctx,
		securityGroup.Namespace,
		securityGroup.Status.ExternalID,
		SecurityGroupRuleReferenceCheck{},
	)
	if blocked {
		return result, err
	}

	return rc.Delete(
		ctx,
		securityGroup.Spec.ProviderConfigRef,
		securityGroup.Spec.OrphanOnDelete,
		securityGroup.Status.ExternalID,
		func(c context.Context, p provider.Provider) error {
			return p.DeleteSecurityGroup(c, securityGroup.Status.ExternalID)
		},
	)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SecurityGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&otcv1alpha1.SecurityGroup{}).
		Named("securitygroup").
		Complete(r)
}
