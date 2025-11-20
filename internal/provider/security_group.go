package provider

import (
	"context"
	"fmt"

	gophercloud "github.com/opentelekomcloud/gophertelekomcloud"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack/vpc/v3/security/group"
)

type CreateSecurityGroupRequest struct {
	Name        string
	Description string
}

type UpdateSecurityGroupRequest struct {
	Description string
}

type CreateSecurityGroupResponse struct {
	ID string
}

type SecurityGroupInfo struct {
	ID          string
	Name        string
	Description string
}

// As Security Groups have no status field, they are considered Ready if they
// exist.
func (i *SecurityGroupInfo) State() State {
	return Ready
}

func (i *SecurityGroupInfo) Message() string {
	return "Security Group is active"
}

func (p *provider) CreateSecurityGroup(
	ctx context.Context,
	r CreateSecurityGroupRequest,
) (CreateSecurityGroupResponse, error) {
	createOpts := group.CreateOpts{
		SecurityGroup: group.SecurityGroupOptions{
			Name:        r.Name,
			Description: r.Description,
		},
	}

	securityGroup, err := group.Create(p.networkv3Client, createOpts)
	if err != nil {
		return CreateSecurityGroupResponse{}, fmt.Errorf(
			"failed to create security group: %w",
			err,
		)
	}

	return CreateSecurityGroupResponse{ID: securityGroup.ID}, nil
}

func (p *provider) GetSecurityGroup(ctx context.Context, id string) (*SecurityGroupInfo, error) {
	securityGroup, err := group.Get(p.networkv3Client, id)
	if err != nil {
		if _, ok := err.(gophercloud.ErrDefault404); ok {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get security group: %w", err)
	}

	securityGroupInfo := &SecurityGroupInfo{
		ID:          securityGroup.ID,
		Name:        securityGroup.Name,
		Description: securityGroup.Description,
	}

	return securityGroupInfo, nil
}

func (p *provider) UpdateSecurityGroup(
	ctx context.Context,
	id string,
	r UpdateSecurityGroupRequest,
) error {
	updateOpts := group.UpdateOpts{
		SecurityGroup: group.SecurityGroupUpdateOptions{
			Description: r.Description,
		},
	}

	_, err := group.Update(p.networkv3Client, id, updateOpts)
	if err != nil {
		return fmt.Errorf("failed to update security group %s: %w", id, err)
	}
	return nil
}

func (p *provider) DeleteSecurityGroup(ctx context.Context, id string) error {
	err := group.Delete(p.networkv3Client, id)
	if err != nil {
		if _, ok := err.(gophercloud.ErrDefault404); ok {
			return nil
		}
		return fmt.Errorf("failed to delete security group: %w", err)
	}

	return nil
}
