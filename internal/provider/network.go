package provider

import (
	"context"
	"fmt"
	"time"

	gophercloud "github.com/opentelekomcloud/gophertelekomcloud"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack/networking/v1/vpcs"

	"github.com/peertech.de/otc-operator/internal/retry"
)

type CreateNetworkRequest struct {
	Name        string
	Description string
	Cidr        string
}

type UpdateNetworkRequest struct {
	Description string
}

type CreateNetworkResponse struct {
	ID string
}

type NetworkInfo struct {
	ID          string
	Name        string
	Description string
	Cidr        string
	Status      string
}

func (i *NetworkInfo) State() State {
	switch i.Status {
	case "ACTIVE", "OK":
		return Ready
	case "DOWN", "ERROR", "error":
		return Failed
	case "CREATING":
		return Provisioning
	default:
		return Unknown
	}
}

func (i *NetworkInfo) Message() string {
	switch i.State() {
	case Ready:
		return "Network is active"
	case Failed:
		return fmt.Sprintf("Network is in a failed state: %s", i.Status)
	case Provisioning:
		return fmt.Sprintf("Network busy with status: %s", i.Status)
	default:
		return fmt.Sprintf("Network is in an unhandled state: %s", i.Status)
	}
}

func (p *provider) CreateNetwork(
	ctx context.Context,
	r CreateNetworkRequest,
) (CreateNetworkResponse, error) {
	createOpts := vpcs.CreateOpts{
		Name:        r.Name,
		Description: r.Description,
		CIDR:        r.Cidr,
	}

	vpc, err := vpcs.Create(p.networkv1Client, createOpts).Extract()
	if err != nil {
		return CreateNetworkResponse{}, fmt.Errorf("failed to create network: %w", err)
	}

	if err := p.waitForVPC(ctx, vpc.ID); err != nil {
		return CreateNetworkResponse{}, fmt.Errorf("failed to wait for network creation: %w", err)
	}

	return CreateNetworkResponse{ID: vpc.ID}, nil
}

func (p *provider) GetNetwork(ctx context.Context, id string) (*NetworkInfo, error) {
	vpc, err := vpcs.Get(p.networkv1Client, id).Extract()
	if err != nil {
		if _, ok := err.(gophercloud.ErrDefault404); ok {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get network: %w", err)
	}

	networkInfo := &NetworkInfo{
		ID:          vpc.ID,
		Name:        vpc.Name,
		Description: vpc.Description,
		Cidr:        vpc.CIDR,
		Status:      vpc.Status,
	}

	return networkInfo, nil
}

func (p *provider) UpdateNetwork(
	ctx context.Context,
	id string,
	r UpdateNetworkRequest,
) error {
	updateOpts := vpcs.UpdateOpts{
		Description: &r.Description,
	}

	_, err := vpcs.Update(p.networkv1Client, id, updateOpts).Extract()
	if err != nil {
		return fmt.Errorf("failed to update network %s: %w", id, err)
	}
	return nil
}

func (p *provider) DeleteNetwork(ctx context.Context, id string) error {
	err := vpcs.Delete(p.networkv1Client, id).ExtractErr()
	if err != nil {
		if _, ok := err.(gophercloud.ErrDefault404); ok {
			return nil
		}
		return fmt.Errorf("failed to delete network: %w", err)
	}

	return nil
}

func (p *provider) waitForVPC(ctx context.Context, id string) error {
	err := retry.Do(ctx, func() (bool, error) {
		info, err := p.GetNetwork(ctx, id)
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
		return fmt.Errorf("failed to wait for vpc creation: %w", err)
	}

	return nil
}
