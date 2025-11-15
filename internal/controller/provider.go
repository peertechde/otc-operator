package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	otcv1alpha1 "github.com/peertech.de/otc-operator/api/v1alpha1"
	provider "github.com/peertech.de/otc-operator/internal/provider"
)

// providerEntry holds a provider client and its creation metadata
type providerEntry struct {
	provider              provider.Provider
	createdAt             time.Time
	configGeneration      int64
	secretResourceVersion string
}

func NewProviderCache(c client.Client, logger zerolog.Logger) *ProviderCache {
	return &ProviderCache{
		client: c,
		logger: logger.With().Str("component", "providers").Logger(),
		cache:  make(map[string]*providerEntry),
	}
}

type ProviderCache struct {
	client client.Client
	logger zerolog.Logger

	mu    sync.RWMutex
	cache map[string]*providerEntry
}

// GetOrCreate retrieves a cached provider or creates a new one
func (p *ProviderCache) GetOrCreate(
	ctx context.Context,
	ref otcv1alpha1.ProviderConfigReference,
	defaultNamespace string,
) (provider.Provider, *otcv1alpha1.ProviderConfig, error) {
	ns := ref.Namespace
	if ns == "" {
		ns = defaultNamespace
	}
	cacheKey := fmt.Sprintf("%s/%s", ns, ref.Name)

	// Load current ProviderConfig to check generation
	var pc otcv1alpha1.ProviderConfig
	err := p.client.Get(
		ctx,
		client.ObjectKey{
			Namespace: ns,
			Name:      ref.Name,
		},
		&pc,
	)
	if err != nil {
		// If not found, clear cache entry
		if apierrors.IsNotFound(err) {
			p.mu.Lock()
			delete(p.cache, cacheKey) // idempotent operation
			p.mu.Unlock()
		}
		return nil, nil, fmt.Errorf("failed to get ProviderConfig %s: %w", cacheKey, err)
	}

	var currentSecretVersion string
	var secret corev1.Secret
	err = p.client.Get(
		ctx,
		client.ObjectKey{
			Namespace: pc.Namespace,
			Name:      pc.Spec.CredentialsSecretRef.Name,
		},
		&secret,
	)
	if err == nil {
		currentSecretVersion = secret.ResourceVersion
	}
	// NOTE: We ignore the error here. If the secret is missing, the factory
	// function below will catch it.

	// Check cache
	p.mu.RLock()
	entry, exists := p.cache[cacheKey]
	p.mu.RUnlock()

	// Check if cached entry is still valid
	if exists && entry.configGeneration == pc.Generation &&
		entry.secretResourceVersion == currentSecretVersion {
		p.logger.Debug().
			Str("providerConfig", cacheKey).
			Int64("generation", pc.Generation).
			Msg("Using cached provider client")

		return entry.provider, &pc, nil
	}

	// Create new provider client
	p.logger.Info().
		Str("providerConfig", cacheKey).
		Msg("Cache miss or invalid, creating new provider client")

	prov, err := provider.NewFromProviderConfig(ctx, p.client, ref, defaultNamespace)
	if err != nil {
		return nil, nil, err
	}

	// Cache the new provider
	p.mu.Lock()
	p.cache[cacheKey] = &providerEntry{
		provider:              prov,
		createdAt:             time.Now(),
		configGeneration:      pc.Generation,
		secretResourceVersion: currentSecretVersion,
	}
	p.mu.Unlock()

	p.logger.Debug().
		Str("providerConfig", cacheKey).
		Int64("generation", pc.Generation).
		Msg("Created and cached new provider client")

	return prov, &pc, nil
}

// Invalidate removes a provider from cache
func (p *ProviderCache) Invalidate(
	ref otcv1alpha1.ProviderConfigReference,
	defaultNamespace string,
) {
	ns := ref.Namespace
	if ns == "" {
		ns = defaultNamespace
	}
	cacheKey := fmt.Sprintf("%s/%s", ns, ref.Name)

	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.cache, cacheKey)

	p.logger.Debug().
		Str("providerConfig", cacheKey).
		Msg("Invalidated provider cache entry")
}
