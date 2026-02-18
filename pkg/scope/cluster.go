package scope

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	restclient "github.com/NVIDIA/carbide-rest/client"
	infrastructurev1 "github.com/fabiendupont/cluster-api-provider-nvidia-bmm/api/v1beta1"
)

// ClusterScopeParams defines parameters for creating a cluster scope
type ClusterScopeParams struct {
	Client           client.Client
	Cluster          *clusterv1.Cluster
	NvidiaBMMCluster *infrastructurev1.NvidiaBMMCluster
	NvidiaBMMClient  *restclient.ClientWithResponses // Optional: skip creating new client
	OrgName          string                          // Optional: org name
}

// ClusterScope defines the scope for cluster operations
type ClusterScope struct {
	client.Client

	Cluster          *clusterv1.Cluster
	NvidiaBMMCluster *infrastructurev1.NvidiaBMMCluster
	NvidiaBMMClient  *restclient.ClientWithResponses
	OrgName          string // Organization name for API calls
}

// NewClusterScope creates a new cluster scope
func NewClusterScope(ctx context.Context, params ClusterScopeParams) (*ClusterScope, error) {
	if params.Client == nil {
		return nil, fmt.Errorf("client is required")
	}
	if params.Cluster == nil {
		return nil, fmt.Errorf("cluster is required")
	}
	if params.NvidiaBMMCluster == nil {
		return nil, fmt.Errorf("nvidia bmm cluster is required")
	}

	var nvidiaBmmClient *restclient.ClientWithResponses
	var orgName string

	// Use provided client if available (for testing), otherwise create a new one
	if params.NvidiaBMMClient != nil {
		nvidiaBmmClient = params.NvidiaBMMClient
		orgName = params.OrgName
	} else {
		// Fetch credentials from secret
		secret := &corev1.Secret{}
		secretKey := types.NamespacedName{
			Name:      params.NvidiaBMMCluster.Spec.Authentication.SecretRef.Name,
			Namespace: params.NvidiaBMMCluster.Spec.Authentication.SecretRef.Namespace,
		}
		if secretKey.Namespace == "" {
			secretKey.Namespace = params.NvidiaBMMCluster.Namespace
		}

		if err := params.Client.Get(ctx, secretKey, secret); err != nil {
			return nil, fmt.Errorf("failed to get credentials secret: %w", err)
		}

		// Validate secret contains required fields
		endpoint, ok := secret.Data["endpoint"]
		if !ok {
			return nil, fmt.Errorf("secret %s is missing 'endpoint' field", secretKey.Name)
		}
		orgNameBytes, ok := secret.Data["orgName"]
		if !ok {
			return nil, fmt.Errorf("secret %s is missing 'orgName' field", secretKey.Name)
		}
		token, ok := secret.Data["token"]
		if !ok {
			return nil, fmt.Errorf("secret %s is missing 'token' field", secretKey.Name)
		}

		orgName = string(orgNameBytes)

		// Create NVIDIA BMM API client with authentication
		var err error
		nvidiaBmmClient, err = restclient.NewClientWithAuth(
			string(endpoint),
			string(token),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create NVIDIA BMM client: %w", err)
		}
	}

	return &ClusterScope{
		Client:           params.Client,
		Cluster:          params.Cluster,
		NvidiaBMMCluster: params.NvidiaBMMCluster,
		NvidiaBMMClient:  nvidiaBmmClient,
		OrgName:          orgName,
	}, nil
}

// SiteID returns the Site ID from the site reference
func (s *ClusterScope) SiteID(ctx context.Context) (string, error) {
	// If ID is directly specified, use it
	if s.NvidiaBMMCluster.Spec.SiteRef.ID != "" {
		return s.NvidiaBMMCluster.Spec.SiteRef.ID, nil
	}

	// TODO: Fetch Site CRD and extract UUID
	// This requires importing the Site CRD type from carbide-rest/site-manager
	// For now, return an error if name-based reference is used
	if s.NvidiaBMMCluster.Spec.SiteRef.Name != "" {
		return "", fmt.Errorf("site name reference not yet implemented, please use direct ID")
	}

	return "", fmt.Errorf("site reference is empty")
}

// Name returns the cluster name
func (s *ClusterScope) Name() string {
	return s.Cluster.Name
}

// Namespace returns the cluster namespace
func (s *ClusterScope) Namespace() string {
	return s.Cluster.Namespace
}

// TenantID returns the tenant ID
func (s *ClusterScope) TenantID() string {
	return s.NvidiaBMMCluster.Spec.TenantID
}

// VPCID returns the VPC ID from status
func (s *ClusterScope) VPCID() string {
	return s.NvidiaBMMCluster.Status.VPCID
}

// SetVPCID sets the VPC ID in status
func (s *ClusterScope) SetVPCID(vpcID string) {
	s.NvidiaBMMCluster.Status.VPCID = vpcID
}

// SetReady sets the ready status
func (s *ClusterScope) SetReady(ready bool) {
	s.NvidiaBMMCluster.Status.Ready = ready
}

// IsReady returns whether the cluster is ready
func (s *ClusterScope) IsReady() bool {
	return s.NvidiaBMMCluster.Status.Ready
}

// SubnetIDs returns the subnet IDs from status
func (s *ClusterScope) SubnetIDs() map[string]string {
	if s.NvidiaBMMCluster.Status.NetworkStatus.SubnetIDs == nil {
		s.NvidiaBMMCluster.Status.NetworkStatus.SubnetIDs = make(map[string]string)
	}
	return s.NvidiaBMMCluster.Status.NetworkStatus.SubnetIDs
}

// SetSubnetID sets a subnet ID in status
func (s *ClusterScope) SetSubnetID(name, id string) {
	if s.NvidiaBMMCluster.Status.NetworkStatus.SubnetIDs == nil {
		s.NvidiaBMMCluster.Status.NetworkStatus.SubnetIDs = make(map[string]string)
	}
	s.NvidiaBMMCluster.Status.NetworkStatus.SubnetIDs[name] = id
}

// NSGID returns the network security group ID from status
func (s *ClusterScope) NSGID() string {
	return s.NvidiaBMMCluster.Status.NetworkStatus.NSGID
}

// SetNSGID sets the network security group ID in status
func (s *ClusterScope) SetNSGID(nsgID string) {
	s.NvidiaBMMCluster.Status.NetworkStatus.NSGID = nsgID
}

// IPBlockID returns the IP block ID from status
func (s *ClusterScope) IPBlockID() string {
	return s.NvidiaBMMCluster.Status.NetworkStatus.IPBlockID
}

// SetIPBlockID sets the IP block ID in status
func (s *ClusterScope) SetIPBlockID(ipBlockID string) {
	s.NvidiaBMMCluster.Status.NetworkStatus.IPBlockID = ipBlockID
}

// PatchObject persists the cluster status
func (s *ClusterScope) PatchObject(ctx context.Context) error {
	return s.Client.Status().Update(ctx, s.NvidiaBMMCluster)
}

// Close closes the scope
func (s *ClusterScope) Close(ctx context.Context) error {
	return s.PatchObject(ctx)
}
