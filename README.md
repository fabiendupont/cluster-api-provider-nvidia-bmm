# Cluster API Provider for NVIDIA BMM

A Kubernetes Cluster API infrastructure provider for managing bare-metal clusters on NVIDIA BMM platform.

> **✅ Repository Separation Complete**: This repository now focuses solely on Cluster API (CAPI). OpenShift components have been extracted to separate repositories. See **Related Projects** section below.

## Overview

This project implements a Kubernetes Cluster API (CAPI) infrastructure provider for NVIDIA BMM, using an auto-generated Go client from the NVIDIA BMM REST API OpenAPI specification.

**What This Repository Provides:**
- **Cluster API (CAPI) Infrastructure Provider** - Standard Kubernetes cluster lifecycle management using CAPI v1.12.1 ✅

**Extracted Components** (now in separate repositories):
- **OpenShift Machine API** → `machine-api-provider-nvidia-bmm` ✅
- **Kubernetes Cloud Provider** → `cloud-provider-nvidia-bmm` ✅

## Features

### Cluster API Provider
- ✅ **NvidiaBMMCluster Controller**: Manages VPC, subnets, and network security groups
- ✅ **NvidiaBMMMachine Controller**: Provisions bare-metal instances with full lifecycle management
- ✅ **Multi-tenancy Support**: Tenant-scoped resource isolation
- ✅ **Network Virtualization**: Support for ETHERNET_VIRTUALIZER and FNN
- ✅ **Provider ID**: `nvidia-bmm://org/tenant/site/instance-id` format for node correlation
- ✅ **Bootstrap Integration**: Works with kubeadm and k3s bootstrap providers
- ✅ **IP Block Auto-Management**: Automatic creation and management of IP blocks for subnet allocation (Kubernetes-native CIDR notation)
- ✅ **Type-Safe API Client**: Auto-generated from OpenAPI 3.1 specification (zero maintenance)

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│           Management Kubernetes Cluster                     │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  CAPI Core + NVIDIA BMM Infrastructure Provider       │  │
│  │  • NvidiaBMMCluster/NvidiaBMMMachine controllers      │  │
│  │  • OpenShift MachineSet/Machine controllers           │  │
│  │  • Cloud Controller Manager (CCM)                     │  │
│  └───────────────────────────────────────────────────────┘  │
│                        ↓ REST API (JWT)                     │
└─────────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────────┐
│              NVIDIA BMM Platform (carbide-rest/)            │
│  • VPC/Networking    • Instance Lifecycle                   │
│  • Site Management   • Machine Allocation                   │
│  • Multi-tenancy     • Health Monitoring                    │
└─────────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────────┐
│         Physical Sites (carbide-core-snapshot/)             │
│  • Bare-metal machines with DPUs                            │
│  • Site Agent + Instance provisioning                       │
└─────────────────────────────────────────────────────────────┘
```

## Dependencies

This provider uses the auto-generated Go client from the NVIDIA BMM REST API:

- **`github.com/NVIDIA/carbide-rest/client`** - Type-safe Go client generated from OpenAPI 3.1 specification
  - Zero maintenance (regenerated when API changes)
  - Always in sync with NVIDIA BMM REST API
  - Compiler-enforced type safety

For more details on the client generation, see: `https://github.com/NVIDIA/carbide-rest`

## Getting Started

### Prerequisites

- Go version v1.25+
- Docker version 17.03+
- kubectl version v1.28+
- Kubernetes management cluster with Cluster API v1.12+ installed
- Access to NVIDIA BMM REST API with JWT authentication
- NVIDIA BMM Site CRD deployed (from carbide-rest/site-manager)

### Installation

#### 1. Install Cluster API Core Components

```bash
clusterctl init
```

#### 2. Build and Deploy NVIDIA BMM Provider

```bash
# Build the project
make build

# Build and push Docker image
make docker-build docker-push IMG=<your-registry>/cluster-api-provider-nvidia-bmm:latest

# Install CRDs
make install

# Deploy controller manager
make deploy IMG=<your-registry>/cluster-api-provider-nvidia-bmm:latest
```

#### 3. Create Credentials Secret

```bash
kubectl create secret generic nvidia-bmm-credentials \
  --from-literal=endpoint="https://api.carbide.nvidia.com" \
  --from-literal=orgName="your-org-name" \
  --from-literal=token="your-jwt-token" \
  -n default
```

## Usage Example

Create a complete cluster with control plane and worker nodes:

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: my-cluster
spec:
  clusterNetwork:
    pods:
      cidrBlocks: ["10.244.0.0/16"]
    services:
      cidrBlocks: ["10.96.0.0/12"]
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
    kind: NvidiaBMMCluster
    name: my-cluster
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: NvidiaBMMCluster
metadata:
  name: my-cluster
spec:
  siteRef:
    name: my-site  # Reference to existing Site CRD
  tenantID: "tenant-uuid"
  vpc:
    name: "my-cluster-vpc"
    networkVirtualizationType: "ETHERNET_VIRTUALIZER"
  subnets:
    - name: "control-plane"
      cidr: "10.100.1.0/24"
    - name: "worker"
      cidr: "10.100.2.0/24"
  authentication:
    secretRef:
      name: nvidia-bmm-credentials
```

See the full example in `config/samples/` directory.

## Configuration

### NvidiaBMMCluster Options

| Field | Description |
|-------|-------------|
| `siteRef` | Reference to NVIDIA BMM Site (name or ID) |
| `tenantID` | NVIDIA BMM tenant ID for multi-tenancy |
| `vpc.networkVirtualizationType` | `ETHERNET_VIRTUALIZER` or `FNN` |
| `subnets` | List of subnets (use Kubernetes-native CIDR notation) |
| `subnets[].cidr` | Subnet CIDR (e.g., `10.0.1.0/24`) - IP blocks are auto-managed |
| `vpc.networkSecurityGroup` | Optional NSG configuration |

### IP Block Auto-Management

The controller automatically creates and manages IP blocks for subnet allocation:

- **User experience**: Specify subnets using Kubernetes-native CIDR notation
- **Controller behavior**: Creates one /16 IP block per cluster (e.g., `10.0.0.0/16`)
- **Subnet allocation**: Subnets are allocated from the cluster's IP block
- **Status tracking**: IP block ID stored in `status.networkStatus.ipBlockID`

**Example:**
```yaml
spec:
  subnets:
  - name: control-plane
    cidr: 10.0.1.0/24  # Controller handles IP block creation
  - name: worker
    cidr: 10.0.2.0/24  # Allocated from same IP block
```

**Behind the scenes:**
1. Controller creates IP block: `my-cluster-ipblock` (10.0.0.0/16)
2. Subnet `control-plane` allocated with prefix length 24 from IP block
3. Subnet `worker` allocated with prefix length 24 from same IP block

### NvidiaBMMMachine Options

| Field | Description |
|-------|-------------|
| `instanceType.id` | Instance type UUID (or use `machineID` for specific machine) |
| `network.subnetName` | Subnet to attach the machine to |
| `network.additionalInterfaces` | Additional NICs for multi-network configurations |
| `sshKeyGroups` | SSH key group IDs |

## Development

### Building

```bash
# Generate manifests and code
make manifests generate

# Build binary
make build

# Run tests
make test

# Build Docker image
make docker-build IMG=<registry>/cluster-api-provider-nvidia-bmm:tag
```

### Project Structure

```
cluster-api-provider-nvidia-bmm/
├── api/v1beta1/              # CAPI CRD definitions
├── internal/controller/      # CAPI controllers
│   ├── nvidiabmmcluster_controller.go  # VPC, subnets, NSG, IP blocks
│   └── nvidiabmmmachine_controller.go  # Instance lifecycle
├── pkg/scope/               # Controller scopes
│   ├── cluster.go           # ClusterScope with generated client
│   └── machine.go           # MachineScope with generated client
├── cmd/main.go              # CAPI controller manager
├── config/                  # Deployment manifests
├── PHASE2-CHANGES.md        # Phase 2 completion summary
└── TESTING.md               # Test migration guide
```

**Code Reorganization:**
- **Removed in Phase 2:**
  - `pkg/cloud/` - Replaced by `github.com/NVIDIA/carbide-rest/client`
  - `pkg/util/` - Moved to `github.com/NVIDIA/carbide-rest/client`
- **Extracted in Phase 3-4:**
  - `openshift/machine/` → `machine-api-provider-nvidia-bmm` repository
  - `openshift/cloudprovider/` → `cloud-provider-nvidia-bmm` repository
  - `cmd/openshift-ccm/` → `cloud-provider-nvidia-bmm` repository

## Troubleshooting

### Check Controller Logs

```bash
kubectl logs -n cluster-api-provider-nvidia-bmm-system \
  deployment/cluster-api-provider-nvidia-bmm-controller-manager
```

### Verify Cluster Status

```bash
kubectl describe nvidiabmmcluster my-cluster
kubectl get machines -w
```

### Common Issues

- **Instances stuck provisioning**: Bare-metal provisioning typically takes 5-15 minutes
- **Authentication errors**: Verify credentials secret contains valid JWT token
- **Network connectivity**: Check VPC and subnet IDs in cluster status

## Related Projects

This repository is part of the NVIDIA BMM Kubernetes integration suite:

- **[carbide-rest](../carbide-rest)** - Auto-generated Go client for NVIDIA BMM REST API (used by all providers)
- **[machine-api-provider-nvidia-bmm](../machine-api-provider-nvidia-bmm)** - OpenShift Machine API provider for NVIDIA BMM
- **[cloud-provider-nvidia-bmm](../cloud-provider-nvidia-bmm)** - Kubernetes Cloud Controller Manager for NVIDIA BMM

## Additional Documentation

- **[PHASE2-CHANGES.md](./PHASE2-CHANGES.md)** - Complete summary of Phase 2 refactoring (generated client migration)
- **[TESTING.md](./TESTING.md)** - Test migration guide and strategies for testing with generated client

## Contributing

Contributions are welcome! Please submit issues and pull requests to the repository.

## License

Apache License 2.0. See [LICENSE](LICENSE) for details.
