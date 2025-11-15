package provider

import (
	"context"
	"fmt"
	"time"

	gophercloud "github.com/opentelekomcloud/gophertelekomcloud"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack/networking/v1/subnets"

	"github.com/peertech.de/otc-operator/internal/retry"
)

// NOTE: Possible statuses:
// - ACTIVE - indicates that the subnet has been associated with a VPC.
// - UNKNOWN - indicates that the subnet has not been associated with a VPC.
// - ERROR - indicates that the subnet is abnormal.

// TODO: Add support for:
// - DNSList          []string       `json:"dnsList,omitempty"`
// - EnableIpv6       *bool          `json:"ipv6_enable,omitempty"`
// - EnableDHCP       *bool          `json:"dhcp_enable,omitempty"`
// - PrimaryDNS       string         `json:"primary_dns,omitempty"`
// - SecondaryDNS     string         `json:"secondary_dns,omitempty"`
// - ExtraDHCPOpts    []ExtraDHCPOpt `json:"extra_dhcp_opts,omitempty"`
type CreateSubnetRequest struct {
	Name        string
	Description string
	Cidr        string
	GatewayIP   string

	// dependencies
	NetworkID string
}

type UpdateSubnetRequest struct {
	Description string
}

type CreateSubnetResponse struct {
	ID string
}

type SubnetInfo struct {
	ID          string
	Name        string
	NetworkID   string
	Description string
	Cidr        string
	GatewayIP   string
	Status      string
}

func (i *SubnetInfo) State() State {
	switch i.Status {
	case "ACTIVE", "OK":
		return Ready
	case "DOWN", "ERROR", "error":
		return Failed
	case "CREATING", "UNKNOWN":
		return Provisioning
	default:
		return Unknown
	}
}

func (i *SubnetInfo) Message() string {
	switch i.State() {
	case Ready:
		return "Subnet is active"
	case Failed:
		return fmt.Sprintf("Subnet is in a failed state: %s", i.Status)
	case Provisioning:
		return fmt.Sprintf("Subnet busy with status: %s", i.Status)
	default:
		return fmt.Sprintf("Subnet is in an unhandled state: %s", i.Status)
	}
}

func (p *provider) CreateSubnet(
	ctx context.Context,
	r CreateSubnetRequest,
) (CreateSubnetResponse, error) {
	createOpts := subnets.CreateOpts{
		Name:        r.Name,
		Description: r.Description,
		CIDR:        r.Cidr,
		GatewayIP:   r.GatewayIP,

		// dependencies
		VpcID: r.NetworkID,
	}

	subnet, err := subnets.Create(p.networkClient, createOpts).Extract()
	if err != nil {
		return CreateSubnetResponse{}, fmt.Errorf("failed to create subnet: %w", err)
	}

	if err := p.waitForSubnet(ctx, subnet.ID); err != nil {
		return CreateSubnetResponse{}, fmt.Errorf("failed to wait for subnet creation: %w", err)
	}

	return CreateSubnetResponse{ID: subnet.ID}, nil
}

func (p *provider) GetSubnet(ctx context.Context, id string) (*SubnetInfo, error) {
	subnet, err := subnets.Get(p.networkClient, id).Extract()
	if err != nil {
		if _, ok := err.(gophercloud.ErrDefault404); ok {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get subnet: %w", err)
	}

	subnetInfo := &SubnetInfo{
		ID:          subnet.ID,
		Name:        subnet.Name,
		Description: subnet.Description,
		Cidr:        subnet.CIDR,
		GatewayIP:   subnet.GatewayIP,
		Status:      subnet.Status,

		// dependencies
		NetworkID: subnet.VpcID,
	}

	return subnetInfo, nil
}

func (p *provider) UpdateSubnet(
	ctx context.Context,
	networkID string,
	id string,
	r UpdateSubnetRequest,
) error {
	updateOpts := subnets.UpdateOpts{
		Description: &r.Description,
	}

	_, err := subnets.Update(p.networkClient, networkID, id, updateOpts).Extract()
	if err != nil {
		return fmt.Errorf("failed to update subnet %s: %w", id, err)
	}
	return nil
}

func (p *provider) DeleteSubnet(ctx context.Context, networkID, id string) error {
	err := subnets.Delete(p.networkClient, networkID, id).ExtractErr()
	if err != nil {
		if _, ok := err.(gophercloud.ErrDefault404); ok {
			return nil
		}
		return fmt.Errorf("failed to delete subnet: %w", err)
	}

	return nil
}

func (p *provider) findSubnetByName(networkID, name string) (*SubnetInfo, error) {
	listOpts := subnets.ListOpts{
		Name:  name,
		VpcID: networkID,
	}
	list, err := subnets.List(p.networkClient, listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list subnets: %w", err)
	}

	for _, subnet := range list {
		if subnet.Name == name {
			return &SubnetInfo{
				ID:          subnet.ID,
				Name:        subnet.Name,
				NetworkID:   subnet.VpcID,
				Description: subnet.Description,
				Cidr:        subnet.CIDR,
				Status:      subnet.Status,
			}, nil
		}
	}

	return nil, fmt.Errorf("subnet with name %s not found", name)
}

func (p *provider) waitForSubnet(ctx context.Context, id string) error {
	err := retry.Do(ctx, func() (bool, error) {
		info, err := p.GetSubnet(ctx, id)
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
		return fmt.Errorf("failed to wait for subnet creation: %w", err)
	}

	return nil
}
