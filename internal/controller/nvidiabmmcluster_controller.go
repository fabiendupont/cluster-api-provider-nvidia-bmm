package controller

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	restclient "github.com/NVIDIA/carbide-rest/client"
	infrastructurev1 "github.com/fabiendupont/cluster-api-provider-nvidia-bmm/api/v1beta1"
	"github.com/fabiendupont/cluster-api-provider-nvidia-bmm/pkg/scope"
)

const (
	// NvidiaBMMClusterFinalizer allows cleanup of NVIDIA BMM resources before deletion
	NvidiaBMMClusterFinalizer = "nvidiabmmcluster.infrastructure.cluster.x-k8s.io"
)

// Condition types
const (
	VPCReadyCondition     clusterv1.ConditionType = "VPCReady"
	SubnetsReadyCondition clusterv1.ConditionType = "SubnetsReady"
	NSGReadyCondition     clusterv1.ConditionType = "NSGReady"
)

// NvidiaBMMClusterReconciler reconciles a NvidiaBMMCluster object
type NvidiaBMMClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// NvidiaBMMClient can be set for testing to inject a mock client
	NvidiaBMMClient *restclient.ClientWithResponses
	// OrgName can be set for testing
	OrgName string
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=nvidiabmmclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=nvidiabmmclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=nvidiabmmclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile handles NvidiaBMMCluster reconciliation
func (r *NvidiaBMMClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the NvidiaBMMCluster instance
	nvidiaBmmCluster := &infrastructurev1.NvidiaBMMCluster{}
	if err := r.Get(ctx, req.NamespacedName, nvidiaBmmCluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Fetch the owner Cluster
	cluster, err := util.GetOwnerCluster(ctx, r.Client, nvidiaBmmCluster.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if cluster == nil {
		logger.Info("Waiting for Cluster Controller to set OwnerRef on NvidiaBMMCluster")
		return ctrl.Result{}, nil
	}

	// Check if cluster is paused
	if annotations.IsPaused(cluster, nvidiaBmmCluster) {
		logger.Info("NvidiaBMMCluster or Cluster is marked as paused, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	// Initialize patch helper
	patchHelper, err := patch.NewHelper(nvidiaBmmCluster, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Always attempt to patch the object and status after each reconciliation
	defer func() {
		if err := patchHelper.Patch(ctx, nvidiaBmmCluster); err != nil {
			logger.Error(err, "failed to patch NvidiaBMMCluster")
		}
	}()

	// Create cluster scope
	clusterScope, err := scope.NewClusterScope(ctx, scope.ClusterScopeParams{
		Client:           r.Client,
		Cluster:          cluster,
		NvidiaBMMCluster: nvidiaBmmCluster,
		NvidiaBMMClient:  r.NvidiaBMMClient, // Will be nil in production, set for tests
		OrgName:          r.OrgName,         // Will be empty in production (fetched from secret), set for tests
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create cluster scope: %w", err)
	}

	// Handle deletion
	if !nvidiaBmmCluster.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, clusterScope)
	}

	// Handle normal reconciliation
	return r.reconcileNormal(ctx, clusterScope)
}

func (r *NvidiaBMMClusterReconciler) reconcileNormal(ctx context.Context, clusterScope *scope.ClusterScope) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling NvidiaBMMCluster")

	// Add finalizer if it doesn't exist
	if !controllerutil.ContainsFinalizer(clusterScope.NvidiaBMMCluster, NvidiaBMMClusterFinalizer) {
		controllerutil.AddFinalizer(clusterScope.NvidiaBMMCluster, NvidiaBMMClusterFinalizer)
		return ctrl.Result{Requeue: true}, nil
	}

	// Get Site ID
	siteID, err := clusterScope.SiteID(ctx)
	if err != nil {
		conditions.Set(clusterScope.NvidiaBMMCluster, metav1.Condition{
			Type:    string(VPCReadyCondition),
			Status:  metav1.ConditionFalse,
			Reason:  "SiteNotFound",
			Message: err.Error(),
		})
		return ctrl.Result{}, err
	}

	// Reconcile VPC
	if err := r.reconcileVPC(ctx, clusterScope, siteID); err != nil {
		conditions.Set(clusterScope.NvidiaBMMCluster, metav1.Condition{
			Type:    string(VPCReadyCondition),
			Status:  metav1.ConditionFalse,
			Reason:  "VPCReconcileFailed",
			Message: err.Error(),
		})
		return ctrl.Result{}, err
	}
	conditions.Set(clusterScope.NvidiaBMMCluster, metav1.Condition{
		Type:   string(VPCReadyCondition),
		Status: metav1.ConditionTrue,
		Reason: "VPCReady",
	})

	// Reconcile Subnets
	if err := r.reconcileSubnets(ctx, clusterScope, siteID); err != nil {
		conditions.Set(clusterScope.NvidiaBMMCluster, metav1.Condition{
			Type:    string(SubnetsReadyCondition),
			Status:  metav1.ConditionFalse,
			Reason:  "SubnetReconcileFailed",
			Message: err.Error(),
		})
		return ctrl.Result{}, err
	}
	conditions.Set(clusterScope.NvidiaBMMCluster, metav1.Condition{
		Type:   string(SubnetsReadyCondition),
		Status: metav1.ConditionTrue,
		Reason: "SubnetsReady",
	})

	// Reconcile Network Security Group (if specified)
	if clusterScope.NvidiaBMMCluster.Spec.VPC.NetworkSecurityGroup != nil {
		if err := r.reconcileNSG(ctx, clusterScope, siteID); err != nil {
			conditions.Set(clusterScope.NvidiaBMMCluster, metav1.Condition{
				Type:    string(NSGReadyCondition),
				Status:  metav1.ConditionFalse,
				Reason:  "NSGReconcileFailed",
				Message: err.Error(),
			})
			return ctrl.Result{}, err
		}
		conditions.Set(clusterScope.NvidiaBMMCluster, metav1.Condition{
			Type:   string(NSGReadyCondition),
			Status: metav1.ConditionTrue,
			Reason: "NSGReady",
		})
	}

	// Mark cluster as ready
	clusterScope.SetReady(true)
	conditions.Set(clusterScope.NvidiaBMMCluster, metav1.Condition{
		Type:   string(clusterv1.ReadyCondition),
		Status: metav1.ConditionTrue,
		Reason: "NvidiaBMMClusterReady",
	})

	logger.Info("Successfully reconciled NvidiaBMMCluster")
	return ctrl.Result{}, nil
}

func (r *NvidiaBMMClusterReconciler) reconcileVPC(ctx context.Context, clusterScope *scope.ClusterScope, siteID string) error {
	logger := log.FromContext(ctx)

	// Check if VPC already exists
	if clusterScope.VPCID() != "" {
		// Verify VPC still exists in NVIDIA BMM
		vpcUUID, err := uuid.Parse(clusterScope.VPCID())
		if err != nil {
			return fmt.Errorf("invalid VPC ID %s: %w", clusterScope.VPCID(), err)
		}

		resp, err := clusterScope.NvidiaBMMClient.GetVpcWithResponse(ctx, clusterScope.OrgName, vpcUUID, nil)
		if err != nil {
			logger.Error(err, "VPC not found in NVIDIA BMM, will recreate", "vpcID", clusterScope.VPCID())
			clusterScope.SetVPCID("")
		} else if resp.StatusCode() == http.StatusOK && resp.JSON200 != nil {
			logger.V(1).Info("VPC already exists", "vpcID", clusterScope.VPCID())
			return nil
		} else {
			logger.Info("VPC not found (status %d), will recreate", resp.StatusCode(), "vpcID", clusterScope.VPCID())
			clusterScope.SetVPCID("")
		}
	}

	// Create VPC
	vpcSpec := clusterScope.NvidiaBMMCluster.Spec.VPC
	siteUUID, err := uuid.Parse(siteID)
	if err != nil {
		return fmt.Errorf("invalid site ID %s: %w", siteID, err)
	}

	// Convert network virtualization type to enum
	netVirtType := restclient.VpcCreateRequestNetworkVirtualizationType(vpcSpec.NetworkVirtualizationType)

	vpcReq := restclient.CreateVpcJSONRequestBody{
		Name:                      vpcSpec.Name,
		SiteId:                    siteUUID,
		NetworkVirtualizationType: &netVirtType,
	}
	if len(vpcSpec.Labels) > 0 {
		labels := restclient.Labels(vpcSpec.Labels)
		vpcReq.Labels = &labels
	}

	logger.Info("Creating VPC", "name", vpcSpec.Name, "siteID", siteID)
	resp, err := clusterScope.NvidiaBMMClient.CreateVpcWithResponse(ctx, clusterScope.OrgName, vpcReq)
	if err != nil {
		return fmt.Errorf("failed to create VPC: %w", err)
	}

	if resp.StatusCode() != http.StatusCreated {
		return fmt.Errorf("failed to create VPC, status %d", resp.StatusCode())
	}

	if resp.JSON201 == nil {
		return fmt.Errorf("unexpected response: no VPC data")
	}

	vpc := resp.JSON201
	if vpc.Id == nil {
		return fmt.Errorf("VPC ID missing in response")
	}

	vpcID := vpc.Id.String()

	clusterScope.SetVPCID(vpcID)
	logger.Info("Successfully created VPC", "vpcID", vpcID)

	return nil
}

// parseCIDR parses a CIDR string and returns the network address and prefix length
func parseCIDR(cidr string) (network string, prefixLength int, err error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid CIDR %s: %w", cidr, err)
	}

	// Get prefix length
	ones, _ := ipNet.Mask.Size()

	// Return the network address (not the host part)
	networkAddr := ipNet.IP.String()

	return networkAddr, ones, nil
}

// ensureIPBlock ensures an IP block exists for subnet allocation
// This creates a shared IP block for all subnets in the cluster
func (r *NvidiaBMMClusterReconciler) ensureIPBlock(ctx context.Context, clusterScope *scope.ClusterScope, siteID string) (uuid.UUID, error) {
	logger := log.FromContext(ctx)

	// Check if we already have an IP block
	if clusterScope.IPBlockID() != "" {
		ipBlockUUID, err := uuid.Parse(clusterScope.IPBlockID())
		if err == nil {
			// Verify it still exists
			resp, err := clusterScope.NvidiaBMMClient.GetIpblockWithResponse(ctx, clusterScope.OrgName, clusterScope.IPBlockID(), nil)
			if err == nil && resp.StatusCode() == http.StatusOK && resp.JSON200 != nil {
				logger.V(1).Info("IP block already exists", "ipBlockID", clusterScope.IPBlockID())
				return ipBlockUUID, nil
			}
		}
		logger.Info("Existing IP block not found, will create new one", "oldIPBlockID", clusterScope.IPBlockID())
	}

	// Parse site ID
	siteUUID, err := uuid.Parse(siteID)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("invalid site ID %s: %w", siteID, err)
	}

	// Create a new IP block for this cluster's subnets
	// Use a large /16 block to accommodate all cluster subnets
	ipBlockName := fmt.Sprintf("%s-ipblock", clusterScope.NvidiaBMMCluster.Name)
	ipBlockReq := restclient.CreateIpblockJSONRequestBody{
		Name:            ipBlockName,
		Prefix:          "10.0.0.0", // Default network for cluster
		PrefixLength:    16,         // /16 gives us 65536 IPs
		ProtocolVersion: restclient.Ipv4,
		RoutingType:     restclient.IpBlockCreateRequestRoutingTypeDatacenterOnly,
		SiteId:          siteUUID,
	}

	logger.Info("Creating IP block", "name", ipBlockName, "prefix", "10.0.0.0/16", "siteID", siteID)
	resp, err := clusterScope.NvidiaBMMClient.CreateIpblockWithResponse(ctx, clusterScope.OrgName, ipBlockReq)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("failed to create IP block: %w", err)
	}

	if resp.StatusCode() != http.StatusCreated {
		return uuid.UUID{}, fmt.Errorf("failed to create IP block, status %d", resp.StatusCode())
	}

	if resp.JSON201 == nil || resp.JSON201.Id == nil {
		return uuid.UUID{}, fmt.Errorf("IP block ID missing in response")
	}

	ipBlockID := *resp.JSON201.Id
	clusterScope.SetIPBlockID(ipBlockID.String())
	logger.Info("Successfully created IP block", "ipBlockID", ipBlockID.String())

	return ipBlockID, nil
}

func (r *NvidiaBMMClusterReconciler) reconcileSubnets(ctx context.Context, clusterScope *scope.ClusterScope, siteID string) error {
	logger := log.FromContext(ctx)

	vpcID := clusterScope.VPCID()
	if vpcID == "" {
		return fmt.Errorf("VPC ID is empty")
	}

	vpcUUID, err := uuid.Parse(vpcID)
	if err != nil {
		return fmt.Errorf("invalid VPC ID %s: %w", vpcID, err)
	}

	// Ensure IP block exists for subnet allocation
	ipBlockID, err := r.ensureIPBlock(ctx, clusterScope, siteID)
	if err != nil {
		return fmt.Errorf("failed to ensure IP block: %w", err)
	}

	subnetIDs := clusterScope.SubnetIDs()

	// Reconcile each subnet
	for _, subnetSpec := range clusterScope.NvidiaBMMCluster.Spec.Subnets {
		// Check if subnet already exists
		if existingID, exists := subnetIDs[subnetSpec.Name]; exists {
			// Verify subnet still exists in NVIDIA BMM
			subnetUUID, err := uuid.Parse(existingID)
			if err != nil {
				logger.Error(err, "Invalid subnet ID, will recreate", "subnetName", subnetSpec.Name, "subnetID", existingID)
				delete(subnetIDs, subnetSpec.Name)
			} else {
				resp, err := clusterScope.NvidiaBMMClient.GetSubnetWithResponse(ctx, clusterScope.OrgName, subnetUUID, nil)
				if err != nil || resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
					logger.Error(err, "Subnet not found in NVIDIA BMM, will recreate", "subnetName", subnetSpec.Name, "subnetID", existingID)
					delete(subnetIDs, subnetSpec.Name)
				} else {
					logger.V(1).Info("Subnet already exists", "subnetName", subnetSpec.Name, "subnetID", existingID)
					continue
				}
			}
		}

		// Parse CIDR to get prefix length
		_, prefixLength, err := parseCIDR(subnetSpec.CIDR)
		if err != nil {
			return fmt.Errorf("failed to parse CIDR for subnet %s: %w", subnetSpec.Name, err)
		}

		// Create subnet using IP block
		subnetReq := restclient.CreateSubnetJSONRequestBody{
			Name:         subnetSpec.Name,
			VpcId:        vpcUUID,
			Ipv4BlockId:  &ipBlockID,
			PrefixLength: prefixLength,
		}

		logger.Info("Creating subnet", "name", subnetSpec.Name, "cidr", subnetSpec.CIDR, "prefixLength", prefixLength, "vpcID", vpcID, "ipBlockID", ipBlockID.String())
		resp, err := clusterScope.NvidiaBMMClient.CreateSubnetWithResponse(ctx, clusterScope.OrgName, subnetReq)
		if err != nil {
			return fmt.Errorf("failed to create subnet %s: %w", subnetSpec.Name, err)
		}

		if resp.StatusCode() != http.StatusCreated {
			return fmt.Errorf("failed to create subnet %s, status %d", subnetSpec.Name, resp.StatusCode())
		}

		if resp.JSON201 == nil || resp.JSON201.Id == nil {
			return fmt.Errorf("subnet ID missing in response for %s", subnetSpec.Name)
		}

		subnetID := resp.JSON201.Id.String()
		clusterScope.SetSubnetID(subnetSpec.Name, subnetID)
		logger.Info("Successfully created subnet", "subnetName", subnetSpec.Name, "subnetID", subnetID)
	}

	return nil
}

func (r *NvidiaBMMClusterReconciler) reconcileNSG(ctx context.Context, clusterScope *scope.ClusterScope, siteID string) error {
	logger := log.FromContext(ctx)

	siteUUID, err := uuid.Parse(siteID)
	if err != nil {
		return fmt.Errorf("invalid site ID %s: %w", siteID, err)
	}

	nsgSpec := clusterScope.NvidiaBMMCluster.Spec.VPC.NetworkSecurityGroup

	// Check if NSG already exists
	if clusterScope.NSGID() != "" {
		// Verify NSG still exists in NVIDIA BMM
		resp, err := clusterScope.NvidiaBMMClient.GetNetworkSecurityGroupWithResponse(ctx, clusterScope.OrgName, clusterScope.NSGID(), nil)
		if err != nil || resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
			logger.Error(err, "NSG not found in NVIDIA BMM, will recreate", "nsgID", clusterScope.NSGID())
			clusterScope.SetNSGID("")
		} else {
			logger.V(1).Info("NSG already exists", "nsgID", clusterScope.NSGID())
			return nil
		}
	}

	// Convert NSG rules from CRD types to API types
	rules := make([]restclient.NetworkSecurityGroupRule, 0, len(nsgSpec.Rules))
	for _, rule := range nsgSpec.Rules {
		// Convert string enums to API enum types
		direction := restclient.NetworkSecurityGroupRuleDirection(strings.ToLower(rule.Direction))
		protocol := restclient.NetworkSecurityGroupRuleProtocol(strings.ToLower(rule.Protocol))
		action := restclient.NetworkSecurityGroupRuleAction(strings.ToLower(rule.Action))

		// API requires both source and destination prefixes
		// Use "0.0.0.0/0" as default (any) if not specified
		sourcePrefix := rule.SourceCIDR
		if sourcePrefix == "" {
			sourcePrefix = "0.0.0.0/0"
		}
		destPrefix := "0.0.0.0/0" // Default to any destination

		nsgRule := restclient.NetworkSecurityGroupRule{
			Name:              &rule.Name,
			Direction:         direction,
			Protocol:          protocol,
			Action:            action,
			SourcePrefix:      sourcePrefix,
			DestinationPrefix: destPrefix,
		}

		// Map port range to destination port range
		if rule.PortRange != "" {
			nsgRule.DestinationPortRange = &rule.PortRange
		}

		rules = append(rules, nsgRule)
	}

	// Create NSG
	nsgReq := restclient.CreateNetworkSecurityGroupJSONRequestBody{
		Name:   nsgSpec.Name,
		SiteId: siteUUID,
	}
	if len(rules) > 0 {
		nsgReq.Rules = &rules
	}

	logger.Info("Creating NSG", "name", nsgSpec.Name, "siteID", siteID)
	resp, err := clusterScope.NvidiaBMMClient.CreateNetworkSecurityGroupWithResponse(ctx, clusterScope.OrgName, nsgReq)
	if err != nil {
		return fmt.Errorf("failed to create NSG: %w", err)
	}

	if resp.StatusCode() != http.StatusCreated {
		return fmt.Errorf("failed to create NSG, status %d", resp.StatusCode())
	}

	if resp.JSON201 == nil || resp.JSON201.Id == nil {
		return fmt.Errorf("NSG ID missing in response")
	}

	nsgID := *resp.JSON201.Id

	clusterScope.SetNSGID(nsgID)
	logger.Info("Successfully created NSG", "nsgID", nsgID)

	return nil
}

//nolint:unparam // ctrl.Result is part of the reconciler interface contract
func (r *NvidiaBMMClusterReconciler) reconcileDelete(ctx context.Context, clusterScope *scope.ClusterScope) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Deleting NvidiaBMMCluster")

	// Delete NSG if it exists
	if clusterScope.NSGID() != "" {
		logger.Info("Deleting NSG", "nsgID", clusterScope.NSGID())
		resp, err := clusterScope.NvidiaBMMClient.DeleteNetworkSecurityGroupWithResponse(ctx, clusterScope.OrgName, clusterScope.NSGID())
		if err != nil {
			logger.Error(err, "failed to delete NSG", "nsgID", clusterScope.NSGID())
			return ctrl.Result{}, err
		}
		if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusNoContent {
			logger.Error(nil, "failed to delete NSG", "nsgID", clusterScope.NSGID(), "status", resp.StatusCode())
			return ctrl.Result{}, fmt.Errorf("failed to delete NSG, status %d", resp.StatusCode())
		}
	}

	// Delete Subnets
	for subnetName, subnetID := range clusterScope.SubnetIDs() {
		logger.Info("Deleting subnet", "subnetName", subnetName, "subnetID", subnetID)
		subnetUUID, err := uuid.Parse(subnetID)
		if err != nil {
			logger.Error(err, "invalid subnet ID", "subnetName", subnetName, "subnetID", subnetID)
			return ctrl.Result{}, fmt.Errorf("invalid subnet ID %s: %w", subnetID, err)
		}
		resp, err := clusterScope.NvidiaBMMClient.DeleteSubnetWithResponse(ctx, clusterScope.OrgName, subnetUUID)
		if err != nil {
			logger.Error(err, "failed to delete subnet", "subnetName", subnetName, "subnetID", subnetID)
			return ctrl.Result{}, err
		}
		if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusNoContent {
			logger.Error(nil, "failed to delete subnet", "subnetName", subnetName, "subnetID", subnetID, "status", resp.StatusCode())
			return ctrl.Result{}, fmt.Errorf("failed to delete subnet %s, status %d", subnetName, resp.StatusCode())
		}
	}

	// Delete VPC
	if clusterScope.VPCID() != "" {
		logger.Info("Deleting VPC", "vpcID", clusterScope.VPCID())
		vpcUUID, err := uuid.Parse(clusterScope.VPCID())
		if err != nil {
			logger.Error(err, "invalid VPC ID", "vpcID", clusterScope.VPCID())
			return ctrl.Result{}, fmt.Errorf("invalid VPC ID %s: %w", clusterScope.VPCID(), err)
		}
		resp, err := clusterScope.NvidiaBMMClient.DeleteVpcWithResponse(ctx, clusterScope.OrgName, vpcUUID)
		if err != nil {
			logger.Error(err, "failed to delete VPC", "vpcID", clusterScope.VPCID())
			return ctrl.Result{}, err
		}
		if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusNoContent {
			logger.Error(nil, "failed to delete VPC", "vpcID", clusterScope.VPCID(), "status", resp.StatusCode())
			return ctrl.Result{}, fmt.Errorf("failed to delete VPC, status %d", resp.StatusCode())
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(clusterScope.NvidiaBMMCluster, NvidiaBMMClusterFinalizer)

	logger.Info("Successfully deleted NvidiaBMMCluster")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NvidiaBMMClusterReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1.NvidiaBMMCluster{}).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(util.ClusterToInfrastructureMapFunc(ctx, infrastructurev1.GroupVersion.WithKind("NvidiaBMMCluster"), mgr.GetClient(), &infrastructurev1.NvidiaBMMCluster{})),
		).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(mgr.GetScheme(), ctrl.Log.WithName("nvidiabmmcluster"), "")).
		Named("nvidiabmmcluster").
		Complete(r)
}
