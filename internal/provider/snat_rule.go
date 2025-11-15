package provider

import (
	"context"
	"fmt"
	"time"

	gophercloud "github.com/opentelekomcloud/gophertelekomcloud"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack/networking/v2/extensions/snatrules"

	"github.com/peertech.de/otc-operator/internal/retry"
)

// NOTE: Possible statuses:
// - ACTIVE - The resource status is normal.
// - PENDING_CREATE - The resource is being created.
// - PENDING_UPDATE - The resource is being updated.
// - PENDING_DELETE - The resource is being deleted.
// - EIP_FREEZED - The EIP of the resource is frozen.
// - INACTIVE - The resource status is abnormal.

type CreateSNATRuleRequest struct {
	Description string

	// dependencies
	NATGatewayID string
	SubnetID     string
	PublicIPID   string
}

type CreateSNATRuleResponse struct {
	ID string
}

type UpdateSNATRuleRequest struct{}

type SNATRuleInfo struct {
	ID          string
	Description string
	Status      string

	// dependencies
	NATGatewayID string
	SubnetID     string
	PublicIPID   string
}

func (i *SNATRuleInfo) State() State {
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

func (i *SNATRuleInfo) Message() string {
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

func (p *provider) CreateSNATRule(
	ctx context.Context,
	r CreateSNATRuleRequest,
) (CreateSNATRuleResponse, error) {
	createOpts := snatrules.CreateOpts{
		Description: r.Description,

		// dependencies
		NatGatewayID: r.NATGatewayID,
		NetworkID:    r.SubnetID,
		FloatingIPID: r.PublicIPID,
	}

	snatRule, err := snatrules.Create(p.networkClient, createOpts)
	if err != nil {
		return CreateSNATRuleResponse{}, fmt.Errorf("failed to create snat rule: %w", err)
	}

	if err := p.waitForSNATRule(ctx, snatRule.ID); err != nil {
		return CreateSNATRuleResponse{}, fmt.Errorf(
			"failed to wait for snat rule creation: %w",
			err,
		)
	}

	return CreateSNATRuleResponse{ID: snatRule.ID}, nil
}

func (p *provider) GetSNATRule(ctx context.Context, id string) (*SNATRuleInfo, error) {
	snatRule, err := snatrules.Get(p.networkClient, id)
	if err != nil {
		if _, ok := err.(gophercloud.ErrDefault404); ok {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get snat rule: %w", err)
	}

	snatRuleInfo := &SNATRuleInfo{
		ID: snatRule.ID,
		// NOTE: "github.com/opentelekomcloud/gophertelekomcloud/openstack/networking/v2/extensions/snatrules"
		// is missing Description in the response.
		//Description: snatRule.Description,

		// dependencies
		NATGatewayID: snatRule.NatGatewayID,
		SubnetID:     snatRule.NetworkID,
		PublicIPID:   snatRule.FloatingIPAddress,
	}

	return snatRuleInfo, nil
}

func (p *provider) DeleteSNATRule(ctx context.Context, id string) error {
	err := snatrules.Delete(p.networkClient, id)
	if err != nil {
		if _, ok := err.(gophercloud.ErrDefault404); ok {
			return nil
		}
		return fmt.Errorf("failed to delete snat rule: %w", err)
	}

	return nil
}

func (p *provider) waitForSNATRule(ctx context.Context, id string) error {
	err := retry.Do(ctx, func() (bool, error) {
		snatRule, err := snatrules.Get(p.networkClient, id)
		if err != nil {
			return true, err
		}

		switch snatRule.Status {
		case "ACTIVE", "OK":
			return false, nil
		case "INACTIVE":
			return false, ErrFailedToCreate
		default: // "UNKNOWN"
			return true, nil
		}
	},
		retry.WithMaxAttempts(defaultMaxRetryAttempts),
		retry.WithDelay(5*time.Second),
	)

	if err != nil {
		return fmt.Errorf("failed to wait for snat rule creation: %w", err)
	}

	return nil
}
