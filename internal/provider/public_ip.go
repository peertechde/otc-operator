package provider

import (
	"context"
	"fmt"
	"time"

	gophercloud "github.com/opentelekomcloud/gophertelekomcloud"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack/networking/v1/eips"

	otcv1alpha1 "github.com/peertech.de/otc-operator/api/v1alpha1"
	"github.com/peertech.de/otc-operator/internal/retry"
)

// NOTE: Possible statuses:
// - FREEZED (Frozen)
// - BIND_ERROR (Binding failed)
// - BINDING (Binding)
// - PENDING_DELETE (Releasing)
// - PENDING_CREATE (Assigning)
// - PENDING_UPDATE (Updating)
// - NOTIFYING (Assigning)
// - NOTIFY_DELETE (Releasing)
// - DOWN (Unbound)
// - ACTIVE (Bound)
// - ELB (Bound to a load balancer)
// - VPN (Bound to a VPN)
// - ERROR (Exceptions)

// TODO: When creating the EIP, should we use a prefix for the bandwidth name?
// - like bandwidth-$NAME
// TODO: Should we support using a already existing bandwidth?
type CreatePublicIPRequest struct {
	Name               string
	Type               otcv1alpha1.PublicIPType
	BandwidthName      string
	BandwidthSize      int
	BandwidthShareType otcv1alpha1.PublicIPBandwidthShareType
}

type UpdatePublicIPRequest struct{}

type CreatePublicIPResponse struct {
	ID string
}

type PublicIPInfo struct {
	ID                 string
	Name               string
	PublicAddress      string
	PrivateAddress     string
	Type               string
	BandwidthSize      int
	BandwidthName      string
	BandwidthShareType string
	Status             string
}

func (i *PublicIPInfo) State() State {
	switch i.Status {
	case "ACTIVE",
		"OK",
		"ELB", // NOTE: we don't support ELB yet
		"VPN": // NOTE: we don't support VPN yet
		return Ready
	case "DOWN", "FREEZED": // EIP is unbound or frozen
		return Stopped
	case "ERROR", "error":
		return Failed
	case "BINDING",
		"NOTIFYING",
		"NOTIFY_DELETE",
		"PENDING_CREATE",
		"PENDING_UPDATE",
		"PENDING_DELETE":
		return Provisioning
	default:
		return Unknown
	}
}

func (i *PublicIPInfo) Message() string {
	switch i.State() {
	case Ready:
		return "Public IP is active"
	case Stopped:
		return "Public IP is inactive."
	case Failed:
		return fmt.Sprintf("Public IP is in a failed state: %s", i.Status)
	case Provisioning:
		return fmt.Sprintf("Public IP busy with status: %s", i.Status)
	default:
		return fmt.Sprintf("Public IP is in an unhandled state: %s", i.Status)
	}
}

func (p *provider) CreatePublicIP(
	ctx context.Context,
	r CreatePublicIPRequest,
) (CreatePublicIPResponse, error) {
	var providerType string
	switch r.Type {
	case otcv1alpha1.PublicIPBGP:
		providerType = "5_bgp"
	case otcv1alpha1.PublicIPMail:
		providerType = "5_mailbgp"
	default:
		return CreatePublicIPResponse{}, fmt.Errorf("unknown public IP type: %s", r.Type)
	}

	var providerShareType string
	switch r.BandwidthShareType {
	case otcv1alpha1.PublicIPBandwidthDedicated:
		providerShareType = "PER"
	case otcv1alpha1.PublicIPBandwidthShared:
		providerShareType = "WHOLE"
	default:
		return CreatePublicIPResponse{}, fmt.Errorf(
			"unknown bandwidth share type: %s",
			r.BandwidthShareType,
		)
	}

	createOpts := eips.ApplyOpts{
		IP: eips.PublicIpOpts{
			Type: providerType,
			Name: r.Name,
		},
		Bandwidth: eips.BandwidthOpts{
			Name:      r.Name,
			Size:      r.BandwidthSize,
			ShareType: providerShareType,
		},
	}

	publicIP, err := eips.Apply(p.networkClient, createOpts).Extract()
	if err != nil {
		return CreatePublicIPResponse{}, fmt.Errorf("failed to create public IP: %w", err)
	}

	if err := p.waitForPublicIP(ctx, publicIP.ID); err != nil {
		return CreatePublicIPResponse{}, fmt.Errorf(
			"failed to wait for public IP creation: %w",
			err,
		)
	}

	return CreatePublicIPResponse{ID: publicIP.ID}, nil
}

func (p *provider) GetPublicIP(ctx context.Context, id string) (*PublicIPInfo, error) {
	publicIP, err := eips.Get(p.networkClient, id).Extract()
	if err != nil {
		if _, ok := err.(gophercloud.ErrDefault404); ok {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get public IP: %w", err)
	}

	publicIPInfo := &PublicIPInfo{
		ID:                 publicIP.ID,
		Name:               publicIP.Name,
		PublicAddress:      publicIP.PublicAddress,
		PrivateAddress:     publicIP.PrivateAddress,
		Type:               publicIP.Type,
		BandwidthSize:      publicIP.BandwidthSize,
		BandwidthShareType: publicIP.BandwidthShareType,
		Status:             publicIP.Status,
	}

	return publicIPInfo, nil
}

func (p *provider) DeletePublicIP(ctx context.Context, id string) error {
	err := eips.Delete(p.networkClient, id).ExtractErr()
	if err != nil {
		if _, ok := err.(gophercloud.ErrDefault404); ok {
			return nil
		}
		return fmt.Errorf("failed to delete public IP: %w", err)
	}

	return nil
}

func (p *provider) waitForPublicIP(ctx context.Context, id string) error {
	err := retry.Do(ctx, func() (bool, error) {
		info, err := p.GetPublicIP(ctx, id)
		if err != nil {
			return true, err
		}

		switch info.State() {
		case Ready:
			return false, nil
		case Stopped:
			return false, fmt.Errorf(
				"public IP entered a non-ready terminal state: %s",
				info.Status,
			)
		case Failed:
			return false, ErrFailedToCreate
		default: // Provisioning or Unknown
			return true, nil
		}

	},
		retry.WithMaxAttempts(defaultMaxRetryAttempts),
		retry.WithDelay(5*time.Second),
	)

	if err != nil {
		return fmt.Errorf("failed to wait for public IP creation: %w", err)
	}

	return nil
}
