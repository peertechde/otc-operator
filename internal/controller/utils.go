package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	otcv1alpha1 "github.com/peertech.de/otc-operator/api/v1alpha1"
)

// ObjectListWithItems is an interface that combines client.ObjectList with a
// method to retrieve its items in a generic way.
type ObjectListWithItems interface {
	client.ObjectList
	GetItems() []client.Object
}

// CheckProviderConfigReady validates that the referenced ProviderConfig exists
// and is ready. It returns the fetched ProviderConfig on success or an error if
// it's not found or not ready.
func CheckProviderConfigReady(
	ctx context.Context,
	c client.Client,
	ref *otcv1alpha1.ProviderConfigReference,
	owner client.Object,
) (otcv1alpha1.ProviderConfig, error) {
	pcKey := client.ObjectKey{
		Name:      ref.Name,
		Namespace: ref.Namespace,
	}
	// Default to the owner's namespace if the reference's namespace is not set.
	if pcKey.Namespace == "" {
		pcKey.Namespace = owner.GetNamespace()
	}

	var pc otcv1alpha1.ProviderConfig
	if err := c.Get(ctx, pcKey, &pc); err != nil {
		if apierrors.IsNotFound(err) {
			return otcv1alpha1.ProviderConfig{}, fmt.Errorf(
				"ProviderConfig '%s' not found in namespace '%s'",
				pcKey.Name,
				pcKey.Namespace,
			)
		}
		return otcv1alpha1.ProviderConfig{}, fmt.Errorf(
			"failed to get ProviderConfig '%s': %w",
			pcKey.Name,
			err,
		)
	}

	if !meta.IsStatusConditionTrue(pc.Status.Conditions, condReady) {
		// Find the Ready condition to provide a more detailed message.
		cond := meta.FindStatusCondition(pc.Status.Conditions, condReady)
		if cond != nil {
			return otcv1alpha1.ProviderConfig{}, fmt.Errorf(
				"referenced ProviderConfig '%s' is not ready: %s",
				pc.Name,
				cond.Message,
			)
		}
		return otcv1alpha1.ProviderConfig{}, fmt.Errorf(
			"referenced ProviderConfig '%s' is not ready",
			pc.Name,
		)
	}

	return pc, nil
}

// resolveByRef fetches a single Kubernetes resource by its name and namespace.
func resolveByRef(
	ctx context.Context,
	c client.Client,
	ref *corev1.LocalObjectReference,
	ns string,
	obj client.Object,
) error {
	objKey := client.ObjectKey{Name: ref.Name, Namespace: ns}
	return c.Get(ctx, objKey, obj)
}

// resolveBySelector lists resources matching labels and ensures exactly one is found.
func resolveBySelector(
	ctx context.Context,
	c client.Client,
	selector *metav1.LabelSelector,
	ns string,
	listObj ObjectListWithItems,
) (client.Object, error) {
	if len(selector.MatchLabels) == 0 {
		return nil, fmt.Errorf("matchLabels cannot be empty for selector")
	}

	labelSelector, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return nil, fmt.Errorf("invalid label selector: %w", err)
	}

	opts := []client.ListOption{
		client.InNamespace(ns),
		client.MatchingLabelsSelector{Selector: labelSelector},
	}

	if err := c.List(ctx, listObj, opts...); err != nil {
		return nil, fmt.Errorf(
			"failed to list resources with selector %v: %w",
			selector.MatchLabels,
			err,
		)
	}

	items := listObj.GetItems()
	if len(items) == 0 {
		return nil, fmt.Errorf(
			"no resources found matching selector %v in namespace %s",
			selector.MatchLabels,
			ns,
		)
	}
	if len(items) > 1 {
		return nil, fmt.Errorf(
			"expected exactly one resource to match selector %v, but found %d",
			selector.MatchLabels,
			len(items),
		)
	}

	return items[0], nil
}

// checkReadinessAndGetID inspects a resolved Kubernetes object for its Ready
// condition and ExternalID.
func checkReadinessAndGetID(obj client.Object, kind string) (string, error) {
	var externalID string
	var conditions []metav1.Condition

	switch o := obj.(type) {
	case *otcv1alpha1.Network:
		externalID = o.Status.ExternalID
		conditions = o.Status.Conditions
	case *otcv1alpha1.Subnet:
		externalID = o.Status.ExternalID
		conditions = o.Status.Conditions
	case *otcv1alpha1.SecurityGroup:
		externalID = o.Status.ExternalID
		conditions = o.Status.Conditions
	default:
		return "", fmt.Errorf("unhandled dependency type for kind %s", kind)
	}

	if externalID == "" {
		return "", fmt.Errorf(
			"%s dependency '%s' is not ready: external ID is not yet set",
			kind,
			obj.GetName(),
		)
	}

	if !meta.IsStatusConditionTrue(conditions, condReady) {
		return "", fmt.Errorf(
			"%s dependency '%s' is not ready: 'Ready' condition is not true",
			kind,
			obj.GetName(),
		)
	}

	return externalID, nil
}
