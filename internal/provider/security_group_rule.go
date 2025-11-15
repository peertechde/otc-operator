package provider

import (
	"context"
	"fmt"

	gophercloud "github.com/opentelekomcloud/gophertelekomcloud"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack/vpc/v3/security/rules"
)

type CreateSecurityGroupRuleRequest struct {
	Name        string
	Description string
	Direction   string
	Protocol    string
	EtherType   string
	Multiport   string
	Action      string
	Priority    *int

	// dependencies
	SecurityGroupID string
}

type UpdateSecurityGroupRuleRequest struct {
	Description string
}

type CreateSecurityGroupRuleResponse struct {
	ID string
}

type SecurityGroupRuleInfo struct {
	ID              string
	SecurityGroupID string
	Description     string
	Direction       string
	Protocol        string
	EtherType       string
	Multiport       string
	Action          string
	Priority        int
}

// As Security Group Rules have no status field, they are considered Ready if
// they exist.
func (i *SecurityGroupRuleInfo) State() State {
	return Ready
}

func (i *SecurityGroupRuleInfo) Message() string {
	return "Security Group Rule is active"
}

func (p *provider) CreateSecurityGroupRule(
	ctx context.Context,
	r CreateSecurityGroupRuleRequest,
) (CreateSecurityGroupRuleResponse, error) {
	createOpts := rules.CreateOpts{
		SecurityGroupRule: rules.SecurityGroupRuleOptions{
			SecurityGroupID: r.SecurityGroupID,
			Description:     r.Description,
			Direction:       r.Direction,
			Protocol:        r.Protocol,
			Ethertype:       r.EtherType,
			Multiport:       r.Multiport,
			Action:          r.Action,
		},
	}
	if r.Priority != nil {
		createOpts.SecurityGroupRule.Priority = *r.Priority
	}

	securityGroupRule, err := rules.Create(p.networkClient, createOpts)
	if err != nil {
		return CreateSecurityGroupRuleResponse{}, fmt.Errorf(
			"failed to create security group rule: %w",
			err,
		)
	}

	return CreateSecurityGroupRuleResponse{ID: securityGroupRule.ID}, nil
}

func (p *provider) GetSecurityGroupRule(
	ctx context.Context,
	id string,
) (*SecurityGroupRuleInfo, error) {
	rule, err := rules.Get(p.networkClient, id)
	if err != nil {
		if _, ok := err.(gophercloud.ErrDefault404); ok {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get security group rule: %w", err)
	}

	securityGroupRuleInfo := &SecurityGroupRuleInfo{
		ID:              rule.ID,
		SecurityGroupID: rule.SecurityGroupID,
		Description:     rule.Description,
		Direction:       rule.Direction,
		Protocol:        rule.Protocol,
		EtherType:       rule.Ethertype,
		Multiport:       rule.Multiport,
		Action:          rule.Action,
		Priority:        rule.Priority,
	}

	return securityGroupRuleInfo, nil
}

func (p *provider) DeleteSecurityGroupRule(
	ctx context.Context,
	id string,
) error {
	err := rules.Delete(p.networkClient, id)
	if err != nil {
		if _, ok := err.(gophercloud.ErrDefault404); ok {
			return nil
		}
		return fmt.Errorf("failed to delete security group rule: %w", err)
	}

	return nil
}
