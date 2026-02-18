# Quickstart Guide

This guide walks you through creating your first Kubernetes cluster on NVIDIA BMM using Cluster API.

## Prerequisites

Before you begin, ensure you have:

1. **Management Cluster** with Cluster API installed
2. **NVIDIA BMM Site** deployed and healthy
3. **NVIDIA BMM API Credentials** (JWT token, org name, endpoint)
4. **Instance Types** available in your NVIDIA BMM site
5. **SSH Key Groups** configured in NVIDIA BMM

## Step 1: Install Cluster API

If you haven't already, install Cluster API on your management cluster:

```bash
# Install clusterctl
curl -L https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.12.1/clusterctl-linux-amd64 -o clusterctl
chmod +x clusterctl
sudo mv clusterctl /usr/local/bin/

# Initialize Cluster API
clusterctl init
```

## Step 2: Install NVIDIA BMM Provider

```bash
# Clone the repository
git clone https://github.com/NVIDIA/cluster-api-provider-nvidia-bmm
cd cluster-api-provider-nvidia-bmm

# Build and push the provider image
export IMG=<your-registry>/cluster-api-provider-nvidia-bmm:v0.1.0
make docker-build docker-push IMG=$IMG

# Install CRDs
make install

# Deploy the controller
make deploy IMG=$IMG
```

Verify the controller is running:

```bash
kubectl get pods -n cluster-api-provider-nvidia-bmm-system
```

## Step 3: Create Credentials Secret

Create a secret with your NVIDIA BMM API credentials:

```bash
kubectl create secret generic nvidia-bmm-credentials \
  --from-literal=endpoint="https://api.carbide.nvidia.com" \
  --from-literal=orgName="your-org-name" \
  --from-literal=token="your-jwt-token" \
  -n default
```

## Step 4: Get Site and Instance Information

Find your Site UUID:

```bash
# List available Sites
kubectl get sites -A

# Get Site details
kubectl get site <site-name> -n <namespace> -o yaml
```

Find available instance types in NVIDIA BMM:

```bash
# Use NVIDIA BMM API or UI to list instance types
# Note the instance type UUIDs
```

## Step 5: Configure Cluster Template

Edit `config/samples/cluster-template.yaml`:

```yaml
# Update these values:
spec:
  siteRef:
    name: your-site-name  # Your Site CRD name
  tenantID: "your-tenant-uuid"

# In NvidiaBMMMachineTemplate:
spec:
  template:
    spec:
      instanceType:
        id: "your-instance-type-uuid"
      sshKeyGroups:
        - "your-ssh-key-group-uuid"
```

## Step 6: Create the Cluster

Apply the cluster configuration:

```bash
kubectl apply -f config/samples/cluster-template.yaml
```

## Step 7: Monitor Cluster Creation

Watch the cluster creation progress:

```bash
# Watch cluster status
kubectl get clusters -w

# Watch machines being created
kubectl get machines -w

# View detailed status
kubectl describe nvidiabmmcluster nvidia-bmm-cluster-example
kubectl describe nvidiabmmmachine -l cluster.x-k8s.io/cluster-name=nvidia-bmm-cluster-example
```

**Note:** Bare-metal instance provisioning typically takes 5-15 minutes.

## Step 8: Access the Workload Cluster

Once the cluster is ready, get the kubeconfig:

```bash
# Get kubeconfig
clusterctl get kubeconfig nvidia-bmm-cluster-example > nvidia-bmm-cluster.kubeconfig

# Verify access
kubectl --kubeconfig=nvidia-bmm-cluster.kubeconfig get nodes
```

## Step 9: Install Cloud Controller Manager (Optional)

For node lifecycle management, install the NVIDIA BMM Cloud Controller Manager in the workload cluster:

```bash
# Edit CCM configuration with your credentials
vim config/openshift/ccm-deployment.yaml

# Deploy to workload cluster
kubectl --kubeconfig=nvidia-bmm-cluster.kubeconfig apply -f config/openshift/ccm-deployment.yaml
```

## Step 10: Install CNI Plugin

Install a CNI plugin in your workload cluster (Calico example):

```bash
kubectl --kubeconfig=nvidia-bmm-cluster.kubeconfig apply -f https://raw.githubusercontent.com/projectcalico/calico/v3.26.1/manifests/calico.yaml
```

## Verify the Cluster

Check that all nodes are ready:

```bash
kubectl --kubeconfig=nvidia-bmm-cluster.kubeconfig get nodes
```

Expected output:
```
NAME                                         STATUS   ROLES           AGE   VERSION
nvidia-bmm-cluster-example-control-plane-xxx    Ready    control-plane   10m   v1.28.0
nvidia-bmm-cluster-example-control-plane-yyy    Ready    control-plane   9m    v1.28.0
nvidia-bmm-cluster-example-control-plane-zzz    Ready    control-plane   8m    v1.28.0
nvidia-bmm-cluster-example-workers-aaa          Ready    <none>          7m    v1.28.0
nvidia-bmm-cluster-example-workers-bbb          Ready    <none>          7m    v1.28.0
nvidia-bmm-cluster-example-workers-cbb          Ready    <none>          7m    v1.28.0
```

## Scaling the Cluster

### Scale Workers

```bash
kubectl scale machinedeployment nvidia-bmm-cluster-example-workers --replicas=5
```

### Scale Control Plane

```bash
kubectl patch kubeadmcontrolplane nvidia-bmm-cluster-example-control-plane --type=merge -p '{"spec":{"replicas":5}}'
```

## Deleting the Cluster

To delete the cluster and all resources:

```bash
kubectl delete cluster nvidia-bmm-cluster-example
```

This will:
1. Delete all Machines
2. Deprovision all NVIDIA BMM instances
3. Delete subnets
4. Delete NSG
5. Delete VPC
6. Remove the Cluster resource

## Troubleshooting

### Cluster stuck in Provisioning

```bash
# Check NvidiaBMMCluster status
kubectl describe nvidiabmmcluster nvidia-bmm-cluster-example

# Check controller logs
kubectl logs -n cluster-api-provider-nvidia-bmm-system deployment/cluster-api-provider-nvidia-bmm-controller-manager -f
```

### Machines not provisioning

```bash
# Check machine status
kubectl get machines
kubectl describe machine <machine-name>

# Check NVIDIA BMM instance status via API
# Instances typically take 5-15 minutes to provision
```

### Network issues

```bash
# Verify VPC created
kubectl get nvidiabmmcluster nvidia-bmm-cluster-example -o jsonpath='{.status.vpcID}'

# Check subnet IDs
kubectl get nvidiabmmcluster nvidia-bmm-cluster-example -o jsonpath='{.status.networkStatus}'
```

### Can't access cluster

```bash
# Verify control plane endpoint is set
kubectl get nvidiabmmcluster nvidia-bmm-cluster-example -o jsonpath='{.spec.controlPlaneEndpoint}'

# Check if bootstrap data is available
kubectl get secrets | grep bootstrap
```

## Next Steps

- [Architecture Documentation](architecture.md)
- [Configuration Reference](../README.md#configuration-options)
- [OpenShift Integration](openshift-integration.md)
- [Advanced Networking](advanced-networking.md)

## Common Configuration Patterns

### High Availability Control Plane

Use 3 or 5 control plane nodes for production:

```yaml
spec:
  replicas: 5  # Must be odd number
```

### Multi-NIC for DPU Connectivity

```yaml
spec:
  template:
    spec:
      network:
        subnetName: "primary"
        additionalInterfaces:
          - subnetName: "dpu-network"
            isPhysical: true
```

### Specific Machine Targeting

For predictable placement or GPU/DPU-enabled nodes:

```yaml
spec:
  template:
    spec:
      instanceType:
        machineID: "specific-machine-uuid"
        allowUnhealthyMachine: false
```

### Custom Network Security

```yaml
vpc:
  networkSecurityGroup:
    name: "custom-nsg"
    rules:
      - name: "allow-custom-port"
        direction: "ingress"
        protocol: "tcp"
        portRange: "8080"
        sourceCIDR: "10.0.0.0/8"
        action: "allow"
```
