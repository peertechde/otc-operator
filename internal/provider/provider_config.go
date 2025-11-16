package provider

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	otcv1alpha1 "github.com/peertech.de/otc-operator/api/v1alpha1"
)

const (
	secretKeyUsername  = "username"
	secretKeyPassword  = "password"
	secretKeyToken     = "token"
	secretKeyAccessKey = "accessKey"
	secretKeySecretKey = "secretKey"
)

// NewFromProviderConfig is a helper function that constructs a new Provider
// client by loading the specified ProviderConfig and its referenced credentials
// secret.
func NewFromProviderConfig(
	ctx context.Context,
	c client.Client,
	ref otcv1alpha1.ProviderConfigReference,
	defaultNamespace string,
) (Provider, error) {
	// Get the ProviderConfig object
	ns := ref.Namespace
	if ns == "" {
		ns = defaultNamespace
	}

	var pc otcv1alpha1.ProviderConfig
	err := c.Get(
		ctx,
		client.ObjectKey{
			Namespace: ns,
			Name:      ref.Name,
		},
		&pc,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get ProviderConfig %s/%s: %w", ns, ref.Name, err)
	}

	opts := []Option{
		WithEndpoint(pc.Spec.IdentityEndpoint),
		WithRegion(pc.Spec.Region),
		WithDomain(pc.Spec.DomainName),
	}

	if pc.Spec.ProjectID != "" {
		opts = append(opts, WithProject(pc.Spec.ProjectID))
	}

	// Resolve and add credential options
	credOpts, err := resolveSecretCredentials(ctx, c, &pc)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to resolve credentials for ProviderConfig %s: %w",
			pc.Name,
			err,
		)
	}
	opts = append(opts, credOpts...)

	return New(opts...)
}

// resolveSecretCredentials fetches the secret and extracts credentials
func resolveSecretCredentials(
	ctx context.Context,
	c client.Client,
	pc *otcv1alpha1.ProviderConfig,
) ([]Option, error) {
	ref := pc.Spec.CredentialsSecretRef
	ns := pc.Namespace

	var secret corev1.Secret
	err := c.Get(
		ctx,
		client.ObjectKey{
			Namespace: ns,
			Name:      ref.Name,
		},
		&secret,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to get credentials secret %s/%s: %w",
			ns,
			ref.Name,
			err,
		)
	}

	opts := []Option{}
	foundAuth := false

	// Check for username/password auth
	if username, ok := secret.Data[secretKeyUsername]; ok {
		if password, ok := secret.Data[secretKeyPassword]; ok {
			opts = append(opts, WithUser(string(username)), WithPassword(string(password)))
			foundAuth = true
		}
	}

	// Check for AK/SK auth
	if !foundAuth {
		if accessKey, ok := secret.Data[secretKeyAccessKey]; ok {
			if secretKey, ok := secret.Data[secretKeySecretKey]; ok {
				opts = append(
					opts,
					WithAccessKey(string(accessKey)),
					WithSecretKey(string(secretKey)),
				)
				foundAuth = true
			}
		}
	}

	// Check for token auth
	if !foundAuth {
		if token, ok := secret.Data[secretKeyToken]; ok {
			opts = append(opts, WithToken(string(token)))
			foundAuth = true
		}
	}

	if !foundAuth {
		return nil, fmt.Errorf(
			"secret %s must contain one of the following combinations: ('%s' and '%s'), ('%s' and '%s') or '%s'",
			ref.Name,
			secretKeyUsername,
			secretKeyPassword,
			secretKeyAccessKey,
			secretKeySecretKey,
			secretKeyToken,
		)
	}

	return opts, nil
}
