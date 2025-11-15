package controller

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	otcv1alpha1 "github.com/peertech.de/otc-operator/api/v1alpha1"
)

func NewDependencyResolver(c client.Client, namespace string) *DependencyResolver {
	return &DependencyResolver{
		client:    c,
		namespace: namespace,
	}
}

type DependencyResolver struct {
	client    client.Client
	namespace string
}

// ResolveNetwork resolves a NetworkDependency to its external ID
func (r *DependencyResolver) ResolveNetwork(
	ctx context.Context,
	dep otcv1alpha1.NetworkDependency,
) (string, error) {
	switch {
	case dep.NetworkID != nil && *dep.NetworkID != "":
		return *dep.NetworkID, nil
	case dep.NetworkRef != nil:
		var network otcv1alpha1.Network
		err := resolveByRef(ctx, r.client, dep.NetworkRef, r.namespace, &network)
		if err != nil {
			return "", fmt.Errorf("failed to resolve network by reference: %w", err)
		}
		return checkReadinessAndGetID(&network, "Network")
	case dep.NetworkSelector != nil:
		resolvedObject, err := resolveBySelector(
			ctx,
			r.client,
			dep.NetworkSelector,
			r.namespace,
			&otcv1alpha1.NetworkList{},
		)
		if err != nil {
			return "", fmt.Errorf("failed to resolve network by selector: %w", err)
		}
		return checkReadinessAndGetID(resolvedObject, "Network")
	default:
		return "", fmt.Errorf("no network specified")
	}
}

// ResolveSubnet resolves a SubnetDependency to its external ID
func (r *DependencyResolver) ResolveSubnet(
	ctx context.Context,
	dep otcv1alpha1.SubnetDependency,
) (string, error) {
	switch {
	case dep.SubnetID != nil && *dep.SubnetID != "":
		return *dep.SubnetID, nil
	case dep.SubnetRef != nil:
		var subnet otcv1alpha1.Subnet
		err := resolveByRef(ctx, r.client, dep.SubnetRef, r.namespace, &subnet)
		if err != nil {
			return "", fmt.Errorf("failed to resolve subnet by reference: %w", err)
		}
		return checkReadinessAndGetID(&subnet, "Subnet")
	case dep.SubnetSelector != nil:
		resolvedObject, err := resolveBySelector(
			ctx,
			r.client,
			dep.SubnetSelector,
			r.namespace,
			&otcv1alpha1.SubnetList{},
		)
		if err != nil {
			return "", fmt.Errorf("failed to resolve subnet by selector: %w", err)
		}
		return checkReadinessAndGetID(resolvedObject, "Subnet")
	default:
		return "", fmt.Errorf("no subnet specified")
	}
}

// ResolveSecurityGroup resolves a SecurityGroupDependency to its external ID
func (r *DependencyResolver) ResolveSecurityGroup(
	ctx context.Context,
	dep otcv1alpha1.SecurityGroupDependency,
) (string, error) {
	switch {
	case dep.SecurityGroupID != nil && *dep.SecurityGroupID != "":
		return *dep.SecurityGroupID, nil
	case dep.SecurityGroupRef != nil:
		var sg otcv1alpha1.SecurityGroup
		err := resolveByRef(ctx, r.client, dep.SecurityGroupRef, r.namespace, &sg)
		if err != nil {
			return "", fmt.Errorf("failed to resolve security group by reference: %w", err)
		}
		return checkReadinessAndGetID(&sg, "SecurityGroup")
	case dep.SecurityGroupSelector != nil:
		resolvedObject, err := resolveBySelector(
			ctx,
			r.client,
			dep.SecurityGroupSelector,
			r.namespace,
			&otcv1alpha1.SecurityGroupList{},
		)
		if err != nil {
			return "", fmt.Errorf("failed to resolve security group by selector: %w", err)
		}
		return checkReadinessAndGetID(resolvedObject, "SecurityGroup")
	default:
		return "", fmt.Errorf("no security group specified")
	}
}

// ResolveNATGateway resolves a NATGatewayDependency to its external ID
func (r *DependencyResolver) ResolveNATGateway(
	ctx context.Context,
	dep otcv1alpha1.NATGatewayDependency,
) (string, error) {
	switch {
	case dep.NATGatewayID != nil && *dep.NATGatewayID != "":
		return *dep.NATGatewayID, nil
	case dep.NATGatewayRef != nil:
		var sg otcv1alpha1.SecurityGroup
		err := resolveByRef(ctx, r.client, dep.NATGatewayRef, r.namespace, &sg)
		if err != nil {
			return "", fmt.Errorf("failed to resolve NAT gateway by reference: %w", err)
		}
		return checkReadinessAndGetID(&sg, "NATGateway")
	case dep.NATGatewaySelector != nil:
		resolvedObject, err := resolveBySelector(
			ctx,
			r.client,
			dep.NATGatewaySelector,
			r.namespace,
			&otcv1alpha1.NATGatewayList{},
		)
		if err != nil {
			return "", fmt.Errorf("failed to resolve NAT gateway by selector: %w", err)
		}
		return checkReadinessAndGetID(resolvedObject, "NATGateway")
	default:
		return "", fmt.Errorf("no NAT gateway specified")
	}
}

// ResolvePublicIP resolves a PublicIPDependency to its external ID
func (r *DependencyResolver) ResolvePublicIP(
	ctx context.Context,
	dep otcv1alpha1.PublicIPDependency,
) (string, error) {
	switch {
	case dep.PublicIPID != nil && *dep.PublicIPID != "":
		return *dep.PublicIPID, nil

	case dep.PublicIPRef != nil:
		var sg otcv1alpha1.SecurityGroup
		err := resolveByRef(ctx, r.client, dep.PublicIPRef, r.namespace, &sg)
		if err != nil {
			return "", fmt.Errorf("failed to resolve public IP by reference: %w", err)
		}
		return checkReadinessAndGetID(&sg, "PublicIP")

	case dep.PublicIPSelector != nil:
		resolvedObject, err := resolveBySelector(
			ctx,
			r.client,
			dep.PublicIPSelector,
			r.namespace,
			&otcv1alpha1.PublicIPList{},
		)
		if err != nil {
			return "", fmt.Errorf("failed to resolve public IP by selector: %w", err)
		}
		return checkReadinessAndGetID(resolvedObject, "PublicIP")

	default:
		return "", fmt.Errorf("no public IP specified")
	}
}

// ResolveNATGatewayDependencies resolves all dependencies for a NATGateway resource
func (r *DependencyResolver) ResolveNATGatewayDependencies(
	ctx context.Context,
	spec otcv1alpha1.NATGatewaySpec,
) (networkID, subnetID string, err error) {
	networkID, err = r.ResolveNetwork(ctx, spec.Network)
	if err != nil {
		return "", "", err
	}

	subnetID, err = r.ResolveSubnet(ctx, spec.Subnet)
	if err != nil {
		return "", "", err
	}

	return networkID, subnetID, nil
}

// ResolveSNATRuleDependencies resolves all dependencies for a SNAT rule resource
func (r *DependencyResolver) ResolveSNATRuleDependencies(
	ctx context.Context,
	spec otcv1alpha1.SNATRuleSpec,
) (natGatewayID, subnetID, publicIPID string, err error) {
	natGatewayID, err = r.ResolveNATGateway(ctx, spec.NATGateway)
	if err != nil {
		return "", "", "", err
	}

	subnetID, err = r.ResolveSubnet(ctx, spec.Subnet)
	if err != nil {
		return "", "", "", err
	}

	publicIPID, err = r.ResolvePublicIP(ctx, spec.PublicIP)
	if err != nil {
		return "", "", "", err
	}

	return natGatewayID, subnetID, publicIPID, nil
}
