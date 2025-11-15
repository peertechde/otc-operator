package controller

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	otcv1alpha1 "github.com/peertech.de/otc-operator/api/v1alpha1"
)

const (
	providerConfigFinalizerName = "providerconfig.otc.peertech.de/finalizer"
	validationRequeueDelay      = 5 * time.Minute
	providerConfigRequeueDelay  = 30 * time.Second
)

func NewProviderConfigReconciler(
	c client.Client,
	scheme *runtime.Scheme,
	logger zerolog.Logger,
	providers *ProviderCache,
) *ProviderConfigReconciler {
	return &ProviderConfigReconciler{
		Client:    c,
		Scheme:    scheme,
		logger:    logger.With().Str("controller", "providerconfig").Logger(),
		providers: providers,
	}
}

type ProviderConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	logger    zerolog.Logger
	providers *ProviderCache
}

// +kubebuilder:rbac:groups=otc.peertech.de,resources=providerconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=otc.peertech.de,resources=providerconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=otc.peertech.de,resources=providerconfigs/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *ProviderConfigReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	scopedLogger := r.logger.With().
		Str("providerconfig", req.NamespacedName.String()).
		Logger()

	var pc otcv1alpha1.ProviderConfig
	if err := r.Get(ctx, req.NamespacedName, &pc); err != nil {
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
		object:         &pc,
		originalObject: pc.DeepCopy(),
		conditions:     &pc.Status.Conditions,
		generation:     pc.Generation,
		finalizerName:  providerConfigFinalizerName,
		requeueAfter:   providerConfigRequeueDelay,
	}

	// Handle deletion.
	if !pc.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, scopedLogger, rc, &pc)
	}

	// Ensure the status is updated.
	defer rc.UpdateStatus(ctx)

	// Ensure the finalizer is present.
	if added, result, err := rc.AddFinalizer(ctx); added {
		return result, err
	}

	// Validate credentials and establish a Ready condition.
	ref := otcv1alpha1.ProviderConfigReference{Name: pc.Name, Namespace: pc.Namespace}
	prov, _, err := r.providers.GetOrCreate(ctx, ref, pc.Namespace)
	if err != nil {
		SetNotReady(
			&pc.Status.Conditions,
			pc.Generation,
			WithReason(reasonProviderInitializationFailed),
			WithMessagef("Failed to initialize provider client: %v", err),
		)

		// NOTE: We will be requeued either based on the watch for the secret or
		// by our providerConfigRequeueDelay.
		return ctrl.Result{RequeueAfter: providerConfigRequeueDelay}, nil
	}

	// Validate the provider client connection.
	if err := prov.Validate(ctx); err != nil {
		SetProviderValidationFailed(
			&pc.Status.Conditions,
			pc.Generation,
			WithMessagef("Provider validation failed: %v", err),
		)

		// The cached provider client is no longer valid. Invalidate it to force
		// a rebuild on the next reconciliation attempt.
		scopedLogger.Info().Msg("Provider validation failed, invalidating client cache.")
		r.providers.Invalidate(ref, pc.Namespace)

		return ctrl.Result{RequeueAfter: validationRequeueDelay}, nil
	}

	// Update status fields.
	SetProviderValidationSuccessful(&pc.Status.Conditions, pc.Generation)
	pc.Status.LastValidationTime = &metav1.Time{Time: time.Now()}

	return ctrl.Result{RequeueAfter: validationRequeueDelay}, nil
}

func (r *ProviderConfigReconciler) reconcileDelete(
	ctx context.Context,
	logger zerolog.Logger,
	rc *Reconciler,
	pc *otcv1alpha1.ProviderConfig,
) (ctrl.Result, error) {
	// If the finalizer is not present, it means our cleanup logic has already
	// run and the object is just waiting for Kubernetes to garbage collect it.
	if !controllerutil.ContainsFinalizer(pc, providerConfigFinalizerName) {
		return ctrl.Result{}, nil
	}

	logger.Info().Msg("Deleting provider config...")

	// Invalidate the provider from the cache.
	ref := otcv1alpha1.ProviderConfigReference{Name: pc.Name, Namespace: pc.Namespace}
	r.providers.Invalidate(ref, pc.Namespace)
	logger.Info().Msg("Successfully invalidated provider from cache during deletion")

	return ctrl.Result{}, rc.RemoveFinalizer(ctx)
}

func (r *ProviderConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&otcv1alpha1.ProviderConfig{}).
		// Watch for changes to Secrets that are referenced by ProviderConfigs.
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findProviderConfigsForSecret),
		).
		Named("providerconfig").
		Complete(r)
}

func (r *ProviderConfigReconciler) findProviderConfigsForSecret(
	ctx context.Context,
	secret client.Object,
) []reconcile.Request {
	var providerConfigs otcv1alpha1.ProviderConfigList
	err := r.List(
		ctx,
		&providerConfigs,
		client.InNamespace(secret.GetNamespace()),
	)
	if err != nil {
		r.logger.Error().Err(err).Msg("failed to list ProviderConfigs for secret watch")
		return nil
	}

	requests := make([]reconcile.Request, 0)
	for _, pc := range providerConfigs.Items {
		if pc.Spec.CredentialsSecretRef.Name == secret.GetName() {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      pc.Name,
					Namespace: pc.Namespace,
				},
			})
		}
	}
	return requests
}
