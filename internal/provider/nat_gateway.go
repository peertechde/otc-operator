package provider

import (
	"context"
	"fmt"
	"time"

	gophercloud "github.com/opentelekomcloud/gophertelekomcloud"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack/networking/v2/extensions/natgateways"

	otcv1alpha1 "github.com/peertech.de/otc-operator/api/v1alpha1"
	"github.com/peertech.de/otc-operator/internal/retry"
)

// NOTE: Possible statuses:
// - ACTIVE - The resource status is normal.
// - PENDING_CREATE - The resource is being created.
// - PENDING_UPDATE - The resource is being updated.
// - PENDING_DELETE - The resource is being deleted.
// - EIP_FREEZED - The EIP of the resource is frozen.
// - INACTIVE - The resource status is abnormal.

type CreateNATGatewayRequest struct {
	Name        string
	Description string
	Type        otcv1alpha1.NATGatewayType

	// dependencies
	NetworkID string
	SubnetID  string
}

type UpdateNATGatewayRequest struct {
	Description string
	Type        string
}

type CreateNATGatewayResponse struct {
	ID string
}

// TODO: what about AdminStateUp?
type NATGatewayInfo struct {
	ID          string
	Name        string
	Description string
	Type        string
	Status      string

	// dependencies
	NetworkID string
	SubnetID  string
}

// NOTE: The documentation in the documentation is unclear. Referring to the
// terraform-provider-opentelekomcloud implementation
func (i *NATGatewayInfo) State() State {
	switch i.Status {
	case "ACTIVE":
		return Ready
	case "INACTIVE",
		"DOWN",
		"ERROR":
		return Failed
	case "PENDING_CREATE",
		"PENDING_UPDATE",
		"PENDING_DELETE":
		return Provisioning
	default:
		return Unknown
	}
}

func (i *NATGatewayInfo) Message() string {
	switch i.State() {
	case Ready:
		return "NAT gateway is active"
	case Failed:
		return fmt.Sprintf("NAT gateway is in a failed state: %s", i.Status)
	case Provisioning:
		return fmt.Sprintf("NAT gateway busy with status: %s", i.Status)
	default:
		return fmt.Sprintf("NAT gateway is in an unhandled state: %s", i.Status)
	}
}

func (p *provider) CreateNATGateway(
	ctx context.Context,
	r CreateNATGatewayRequest,
) (CreateNATGatewayResponse, error) {
	var natType string
	switch r.Type {
	case otcv1alpha1.TypeMicro:
		natType = "0"
	case otcv1alpha1.TypeSmall:
		natType = "1"
	case otcv1alpha1.TypeMedium:
		natType = "2"
	case otcv1alpha1.TypeLarge:
		natType = "3"
	case otcv1alpha1.TypeExtraLarge:
		natType = "4"
	default:
		return CreateNATGatewayResponse{}, fmt.Errorf("unknown NAT gateway type: %s", r.Type)
	}

	createOpts := natgateways.CreateOpts{
		Name:        r.Name,
		Description: r.Description,
		Spec:        natType,

		// dependencies
		RouterID:          r.NetworkID,
		InternalNetworkID: r.SubnetID,
	}

	natGateway, err := natgateways.Create(p.natClient, createOpts).Extract()
	if err != nil {
		return CreateNATGatewayResponse{}, fmt.Errorf("failed to create nat gateway: %w", err)
	}

	if err := p.waitForNATGateway(ctx, natGateway.ID); err != nil {
		return CreateNATGatewayResponse{}, fmt.Errorf(
			"failed to wait for nat gateway creation: %w",
			err,
		)
	}

	return CreateNATGatewayResponse{ID: natGateway.ID}, nil
}

func (p *provider) GetNATGateway(ctx context.Context, id string) (*NATGatewayInfo, error) {
	natGateway, err := natgateways.Get(p.natClient, id).Extract()
	if err != nil {
		if _, ok := err.(gophercloud.ErrDefault404); ok {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get nat gateway: %w", err)
	}

	natGatewayInfo := &NATGatewayInfo{
		ID:          natGateway.ID,
		Name:        natGateway.Name,
		Description: natGateway.Description,
		Type:        natGateway.Spec,
		Status:      natGateway.Status,

		// dependencies
		NetworkID: natGateway.RouterID,
		SubnetID:  natGateway.InternalNetworkID,
	}

	return natGatewayInfo, nil
}

func (p *provider) UpdateNATGateway(
	ctx context.Context,
	id string,
	r UpdateNATGatewayRequest,
) error {
	updateOpts := natgateways.UpdateOpts{
		Description: r.Description,
		Spec:        r.Type,
	}

	_, err := natgateways.Update(p.natClient, id, updateOpts).Extract()
	if err != nil {
		return fmt.Errorf("failed to update nat gateway %s: %w", id, err)
	}
	return nil
}

func (p *provider) DeleteNATGateway(ctx context.Context, id string) error {
	err := natgateways.Delete(p.natClient, id).ExtractErr()
	if err != nil {
		if _, ok := err.(gophercloud.ErrDefault404); ok {
			return nil
		}
		return fmt.Errorf("failed to delete nat gateway: %w", err)
	}

	return nil
}

func (p *provider) waitForNATGateway(ctx context.Context, id string) error {
	err := retry.Do(ctx, func() (bool, error) {
		info, err := p.GetNATGateway(ctx, id)
		if err != nil {
			return true, err
		}

		switch info.State() {
		case Ready:
			return false, nil
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
		return fmt.Errorf("failed to wait for nat gateway creation: %w", err)
	}

	return nil
}
