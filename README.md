# Open Telekom Cloud (OTC) Kubernetes Operator

> **Note:** This project is currently in an **alpha** stage and is part of ongoing research and development. It should be considered experimental. The APIs and features are subject to change without notice. It is not recommended for use in production environments.

The OTC Operator allows you to manage Open Telekom Cloud resources natively from within your Kubernetes cluster using Custom Resource Definitions (CRDs). It bridges the gap between Kubernetes and the OTC API, enabling you to define your cloud infrastructure as code and integrate it seamlessly with GitOps workflows.

## Overview

This operator follows the standard Kubernetes controller pattern. You define the desired state of your OTC resources in YAML manifests and the operator's controllers work to bring the actual state of your cloud environment into alignment with that desired state.

### Supported Resources

The OTC Operator currently supports managing the following resources:
* `ProviderConfig`: Defines credentials and connection details for an OTC project.
* `Network`: Corresponds to an Virtual Private Cloud (VPC).
* `Subnet`: A subnet within a VPC.
* `SecurityGroup`: A collection of access control rules for cloud resources.
* `SecurityGroupRule`: A rule within a Security Group.
* `PublicIP`: An Elastic IP (EIP) address.
* `NATGateway`: A Network Address Translation Gateway.
* `SNATRule`: A Source NAT rule for a NAT Gateway.

## Getting Started

### Prerequisites

*   A running Kubernetes cluster (v1.25+ recommended).
*   `kubectl` installed and configured to communicate with your cluster.
*   An Open Telekom Cloud account with your domain name, project ID and one of the following credential types:
    *   An IAM username and password.
    *   An Access Key (AK) and Secret Key (SK) pair.

### Installation

Follow these steps to deploy the OTC Operator to your cluster.

#### Step 1: Install cert-manager

The OTC Operator uses webhooks to validate its custom resources, which requires `cert-manager` to be installed in your cluster for managing TLS certificates.

Install `cert-manager` using the official manifest.
```sh
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
```

Wait for the cert-manager pods to become ready before proceeding.

#### Step 2: Install the OTC Operator

The recommended way to install the operator is by using the pre-built release.yaml manifest from the [latest GitHub release](https://github.com/peertechde/otc-operator/releases/latest).

Install `otc-operator` using the official manifest.
```sh
kubectl apply -f https://github.com/peertechde/otc-operator/releases/latest/download/release.yaml
```

This command will:
* Create the otc-operator-system namespace. 
* Install all the necessary Custom Resource Definitions (CRDs). 
* Create the operator Deployment. 
* Set up the required RBAC roles and permissions.

After a few moments, the operator pod should be running in the `otc-operator-system` namespace.

## Usage: Creating Your First Network

Let's walk through creating a `ProviderConfig`, a `Network` (VPC) and a `Subnet`.

### 1. Create a Credentials Secret

Choose **one** of the options below and create a Kubernetes Secret containing your credentials.

#### Option A: Using Username and Password

Run the following command, replacing the placeholder values with your actual credentials.

```sh
kubectl create secret generic otc-credentials \
  --from-literal=username='YOUR_USERNAME' \
  --from-literal=password='YOUR_PASSWORD' \
  -n default # Or your target namespace
```

#### Option B: Using Access Key and Secret Key

Run the following command, replacing the placeholders with your actual keys.

```sh
kubectl create secret generic otc-credentials \
  --from-literal=accessKey='YOUR_ACCESS_KEY' \
  --from-literal=secretKey='YOUR_SECRET_KEY' \
  -n default # Or your target namespace
```

### 2. Create a ProviderConfig

Next, create a `ProviderConfig` resource that tells the operator how to connect to your OTC project and references the secret you just created.

**`provider-config.yaml`**
```yaml
apiVersion: otc.peertech.de/v1alpha1
kind: ProviderConfig
metadata:
  name: otc-provider-config
  namespace: default # Or your target namespace
spec:
  identityEndpoint: "https://iam.eu-de.otc.t-systems.com/v3"
  region: "eu-de"
  domainName: "YOUR_OTC_DOMAIN_NAME"  # Your OTC Domain Name
  projectID: "YOUR_OTC_PROJECT_ID"    # Your OTC Project ID
  credentialsSecretRef:
    name: otc-credentials
    namespace: default # Or your target namespace
```
Apply the `ProviderConfig`:
```sh
kubectl apply -f provider-config.yaml
```

### 3. Create a Network (VPC)

Now you can create your first cloud resource.

**`network.yaml`**
```yaml
apiVersion: otc.peertech.de/v1alpha1
kind: Network
metadata:
  name: my-first-vpc
  namespace: default # Or your target namespace
spec:
  providerConfigRef:
    # This must match the ProviderConfig you created
    name: otc-provider-config
  cidr: "10.0.0.0/16"
  description: "VPC created by the OTC Operator"
```

Apply the `Network`:
```sh
kubectl apply -f network.yaml
```

### 4. Create a Subnet

Finally, create a `Subnet` that lives inside the `Network` you just created. Note the `networkRef` which tells the operator about the dependency.

**`subnet.yaml`**
```yaml
apiVersion: otc.peertech.de/v1alpha1
kind: Subnet
metadata:
  name: my-first-subnet
  namespace: default # Or your target namespace
spec:
  providerConfigRef:
    name: otc-provider-config
  network:
    # This tells the operator to find the 'my-first-vpc' Network
    # in the same namespace and use it as a dependency.
    networkRef:
      name: my-first-vpc
  cidr: "10.0.1.0/24"
  gatewayIP: "10.0.1.1"
  description: "Subnet created by the OTC Operator"
```

Apply the `Subnet`:
```sh
kubectl apply -f subnet.yaml
```

### 5. Check the Status

After a few moments, the operator will create the resources in OTC. You can check the status of your Kubernetes resource to see the cloud provider's ID (externalID) and the resource's condition.

```sh
kubectl get subnet my-first-subnet -o yaml
```