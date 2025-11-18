package provider

import (
	"context"
	"fmt"
	"net/http"

	gophercloud "github.com/opentelekomcloud/gophertelekomcloud"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack/identity/v3/regions"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack/networking/v1/subnets"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack/networking/v1/vpcs"
)

const (
	defaultMaxRetryAttempts = 60
)

var (
	ErrNotFound       = fmt.Errorf("not found")
	ErrFailedToCreate = fmt.Errorf("failed to create")
)

type Provider interface {
	Validate(ctx context.Context) error

	CreateNetwork(ctx context.Context, r CreateNetworkRequest) (CreateNetworkResponse, error)
	GetNetwork(ctx context.Context, id string) (*NetworkInfo, error)
	UpdateNetwork(ctx context.Context, id string, r UpdateNetworkRequest) error
	DeleteNetwork(ctx context.Context, id string) error

	CreateSubnet(ctx context.Context, r CreateSubnetRequest) (CreateSubnetResponse, error)
	GetSubnet(ctx context.Context, id string) (*SubnetInfo, error)
	UpdateSubnet(ctx context.Context, networkID, id string, r UpdateSubnetRequest) error
	DeleteSubnet(ctx context.Context, networkID, id string) error

	CreateSecurityGroup(
		ctx context.Context,
		r CreateSecurityGroupRequest,
	) (CreateSecurityGroupResponse, error)
	GetSecurityGroup(ctx context.Context, id string) (*SecurityGroupInfo, error)
	UpdateSecurityGroup(ctx context.Context, id string, r UpdateSecurityGroupRequest) error
	DeleteSecurityGroup(ctx context.Context, id string) error

	CreateSecurityGroupRule(
		ctx context.Context,
		r CreateSecurityGroupRuleRequest,
	) (CreateSecurityGroupRuleResponse, error)
	GetSecurityGroupRule(ctx context.Context, id string) (*SecurityGroupRuleInfo, error)
	DeleteSecurityGroupRule(ctx context.Context, id string) error

	CreatePublicIP(
		ctx context.Context,
		r CreatePublicIPRequest,
	) (CreatePublicIPResponse, error)
	GetPublicIP(ctx context.Context, id string) (*PublicIPInfo, error)
	DeletePublicIP(ctx context.Context, id string) error

	CreateNATGateway(
		ctx context.Context,
		r CreateNATGatewayRequest,
	) (CreateNATGatewayResponse, error)
	GetNATGateway(ctx context.Context, id string) (*NATGatewayInfo, error)
	UpdateNATGateway(ctx context.Context, id string, r UpdateNATGatewayRequest) error
	DeleteNATGateway(ctx context.Context, id string) error

	CreateSNATRule(
		ctx context.Context,
		r CreateSNATRuleRequest,
	) (CreateSNATRuleResponse, error)
	GetSNATRule(ctx context.Context, id string) (*SNATRuleInfo, error)
	DeleteSNATRule(ctx context.Context, id string) error
}

func New(opts ...Option) (Provider, error) {
	var options Options
	for _, opt := range opts {
		opt(&options)
	}

	client, err := openstack.NewClient(options.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create new client: %w", err)
	}

	// Configure the HTTP client to handle redirects with AK/SK resigning.
	client.HTTPClient = http.Client{
		Transport: client.HTTPClient.Transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Only re-sign the request if we are using AK/SK authentication.
			if options.AccessKey != "" && options.SecretKey != "" {
				gophercloud.ReSign(req, gophercloud.SignOptions{
					AccessKey: options.AccessKey,
					SecretKey: options.SecretKey,
				})
			}
			return nil
		},
	}

	var authProvider gophercloud.AuthOptionsProvider
	if options.AccessKey != "" && options.SecretKey != "" {
		// Use Access Key / Secret Key authentication
		authProvider = gophercloud.AKSKAuthOptions{
			IdentityEndpoint: options.Endpoint,
			AccessKey:        options.AccessKey,
			SecretKey:        options.SecretKey,
			ProjectId:        options.Project,
		}
	} else {
		// Fall back to Username/Password or Token authentication
		ao := gophercloud.AuthOptions{
			IdentityEndpoint: options.Endpoint,
			Username:         options.User,
			Password:         options.Password,
			DomainName:       options.Domain,
			TokenID:          options.Token,
			TenantID:         options.Project,
		}
		ao.AllowReauth = true
		authProvider = ao
	}

	if err := openstack.Authenticate(client, authProvider); err != nil {
		return nil, fmt.Errorf("failed to authenticate: %w", err)
	}

	identityV3, err := openstack.NewIdentityV3(
		client,
		gophercloud.EndpointOpts{
			Region: options.Region,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create identity client: %w", err)
	}

	networkv1, err := openstack.NewNetworkV1(
		client,
		gophercloud.EndpointOpts{
			Region: options.Region,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create network client: %w", err)
	}

	p := &provider{
		client:         client,
		identityClient: identityV3,
		networkClient:  networkv1,
	}

	return p, nil
}

type provider struct {
	client         *gophercloud.ProviderClient
	identityClient *gophercloud.ServiceClient
	networkClient  *gophercloud.ServiceClient
}

// Validate validates the connection and permissions.
func (p *provider) Validate(ctx context.Context) error {
	// General (IAM) permissions
	if _, err := regions.List(p.identityClient, nil).AllPages(); err != nil {
		return fmt.Errorf("identity validation failed: failed to list regions: %w", err)
	}

	// Validate Network (VPC) permissions
	if _, err := vpcs.List(p.networkClient, vpcs.ListOpts{}); err != nil {
		return fmt.Errorf(
			"network validation failed: could not list VPCs (check permissions): %w",
			err,
		)
	}

	// Validate Network (VPC) permissions
	if _, err := subnets.List(p.networkClient, subnets.ListOpts{}); err != nil {
		return fmt.Errorf(
			"network validation failed: could not list VPCs (check permissions): %w",
			err,
		)
	}

	return nil
}
