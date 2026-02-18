package controller

import (
	"context"
	"fmt"
	"net/http"
	"time"

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
	// NvidiaBMMMachineFinalizer allows cleanup of NVIDIA BMM resources before deletion
	NvidiaBMMMachineFinalizer = "nvidiabmmmachine.infrastructure.cluster.x-k8s.io"
)

// Condition types
const (
	InstanceProvisionedCondition clusterv1.ConditionType = "InstanceProvisioned"
	NetworkConfiguredCondition   clusterv1.ConditionType = "NetworkConfigured"
)

// NvidiaBMMMachineReconciler reconciles a NvidiaBMMMachine object
type NvidiaBMMMachineReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// NvidiaBMMClient can be set for testing to inject a mock client
	NvidiaBMMClient *restclient.ClientWithResponses
	// OrgName can be set for testing
	OrgName string
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=nvidiabmmmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=nvidiabmmmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=nvidiabmmmachines/finalizers,verbs=update
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile handles NvidiaBMMMachine reconciliation
func (r *NvidiaBMMMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the NvidiaBMMMachine instance
	nvidiaBmmMachine := &infrastructurev1.NvidiaBMMMachine{}
	if err := r.Get(ctx, req.NamespacedName, nvidiaBmmMachine); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Fetch the owner Machine
	machine, err := util.GetOwnerMachine(ctx, r.Client, nvidiaBmmMachine.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machine == nil {
		logger.Info("Waiting for Machine Controller to set OwnerRef on NvidiaBMMMachine")
		return ctrl.Result{}, nil
	}

	// Fetch the owner Cluster
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if cluster == nil {
		logger.Info("Waiting for Cluster to be set on Machine")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Fetch the NvidiaBMMCluster
	nvidiaBmmCluster := &infrastructurev1.NvidiaBMMCluster{}
	nvidiaBmmClusterKey := client.ObjectKey{
		Namespace: cluster.Namespace,
		Name:      cluster.Spec.InfrastructureRef.Name,
	}
	if err := r.Get(ctx, nvidiaBmmClusterKey, nvidiaBmmCluster); err != nil {
		return ctrl.Result{}, err
	}

	// Check if cluster is paused
	if annotations.IsPaused(cluster, nvidiaBmmMachine) {
		logger.Info("NvidiaBMMMachine or Cluster is marked as paused, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	// Return early if NvidiaBMMCluster is not ready
	if !nvidiaBmmCluster.Status.Ready {
		logger.Info("Waiting for NvidiaBMMCluster to be ready")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Return early if bootstrap data is not ready
	if machine.Spec.Bootstrap.DataSecretName == nil {
		logger.Info("Waiting for bootstrap data to be available")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Initialize patch helper
	patchHelper, err := patch.NewHelper(nvidiaBmmMachine, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Always attempt to patch the object and status after each reconciliation
	defer func() {
		if err := patchHelper.Patch(ctx, nvidiaBmmMachine); err != nil {
			logger.Error(err, "failed to patch NvidiaBMMMachine")
		}
	}()

	// Create cluster scope for credentials
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

	// Create machine scope
	machineScope, err := scope.NewMachineScope(scope.MachineScopeParams{
		Client:           r.Client,
		Cluster:          cluster,
		Machine:          machine,
		NvidiaBMMCluster: nvidiaBmmCluster,
		NvidiaBMMMachine: nvidiaBmmMachine,
		NvidiaBMMClient:  clusterScope.NvidiaBMMClient,
		OrgName:          clusterScope.OrgName,
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create machine scope: %w", err)
	}

	// Handle deletion
	if !nvidiaBmmMachine.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, machineScope)
	}

	// Handle normal reconciliation
	return r.reconcileNormal(ctx, machineScope, clusterScope)
}

func (r *NvidiaBMMMachineReconciler) reconcileNormal(ctx context.Context, machineScope *scope.MachineScope, clusterScope *scope.ClusterScope) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling NvidiaBMMMachine")

	// Add finalizer if it doesn't exist
	if !controllerutil.ContainsFinalizer(machineScope.NvidiaBMMMachine, NvidiaBMMMachineFinalizer) {
		controllerutil.AddFinalizer(machineScope.NvidiaBMMMachine, NvidiaBMMMachineFinalizer)
		return ctrl.Result{Requeue: true}, nil
	}

	// If instance already exists, check its status
	if machineScope.InstanceID() != "" {
		return r.reconcileInstance(ctx, machineScope, clusterScope)
	}

	// Create new instance
	if err := r.createInstance(ctx, machineScope, clusterScope); err != nil {
		conditions.Set(machineScope.NvidiaBMMMachine, metav1.Condition{
			Type:    string(InstanceProvisionedCondition),
			Status:  metav1.ConditionFalse,
			Reason:  "InstanceCreationFailed",
			Message: err.Error(),
		})
		return ctrl.Result{}, err
	}

	conditions.Set(machineScope.NvidiaBMMMachine, metav1.Condition{
		Type:   string(InstanceProvisionedCondition),
		Status: metav1.ConditionTrue,
		Reason: "InstanceCreated",
	})

	// Requeue to check instance status
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *NvidiaBMMMachineReconciler) createInstance(ctx context.Context, machineScope *scope.MachineScope, clusterScope *scope.ClusterScope) error {
	logger := log.FromContext(ctx)

	// Get bootstrap data
	bootstrapData, err := machineScope.GetBootstrapData(ctx)
	if err != nil {
		return fmt.Errorf("failed to get bootstrap data: %w", err)
	}

	// Get subnet ID for primary network interface
	subnetID, err := machineScope.GetSubnetID()
	if err != nil {
		return fmt.Errorf("failed to get subnet ID: %w", err)
	}

	// Parse subnet ID to UUID
	subnetUUID, err := uuid.Parse(subnetID)
	if err != nil {
		return fmt.Errorf("invalid subnet ID %s: %w", subnetID, err)
	}

	// Parse VPC ID to UUID
	vpcUUID, err := uuid.Parse(machineScope.VPCID())
	if err != nil {
		return fmt.Errorf("invalid VPC ID %s: %w", machineScope.VPCID(), err)
	}

	// Get Site ID (as site name for ProviderID)
	siteName, err := clusterScope.SiteID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get site ID: %w", err)
	}

	// Parse tenant ID to UUID
	tenantUUID, err := uuid.Parse(machineScope.TenantID())
	if err != nil {
		return fmt.Errorf("invalid tenant ID %s: %w", machineScope.TenantID(), err)
	}

	// Build primary network interface
	physicalFalse := false
	interfaces := []restclient.InterfaceCreateRequest{
		{
			SubnetId:   &subnetUUID,
			IsPhysical: &physicalFalse,
		},
	}

	// Add additional network interfaces if specified
	for _, additionalIf := range machineScope.NvidiaBMMMachine.Spec.Network.AdditionalInterfaces {
		// Look up subnet ID from cluster status
		additionalSubnetID, ok := clusterScope.NvidiaBMMCluster.Status.NetworkStatus.SubnetIDs[additionalIf.SubnetName]
		if !ok {
			return fmt.Errorf("subnet %s not found in cluster status", additionalIf.SubnetName)
		}

		additionalSubnetUUID, err := uuid.Parse(additionalSubnetID)
		if err != nil {
			return fmt.Errorf("invalid subnet ID %s: %w", additionalSubnetID, err)
		}

		interfaces = append(interfaces, restclient.InterfaceCreateRequest{
			SubnetId:   &additionalSubnetUUID,
			IsPhysical: &additionalIf.IsPhysical,
		})
	}

	// Build instance create request
	instanceReq := restclient.CreateInstanceJSONRequestBody{
		Name:       machineScope.Name(),
		TenantId:   tenantUUID,
		VpcId:      vpcUUID,
		UserData:   &bootstrapData,
		Interfaces: &interfaces,
	}

	// Set SSH key groups if specified (convert string IDs to UUIDs)
	if len(machineScope.NvidiaBMMMachine.Spec.SSHKeyGroups) > 0 {
		sshKeyGroupUUIDs := make([]uuid.UUID, 0, len(machineScope.NvidiaBMMMachine.Spec.SSHKeyGroups))
		for _, keyGroupID := range machineScope.NvidiaBMMMachine.Spec.SSHKeyGroups {
			keyGroupUUID, err := uuid.Parse(keyGroupID)
			if err != nil {
				return fmt.Errorf("invalid SSH key group ID %s: %w", keyGroupID, err)
			}
			sshKeyGroupUUIDs = append(sshKeyGroupUUIDs, keyGroupUUID)
		}
		instanceReq.SshKeyGroupIds = &sshKeyGroupUUIDs
	}

	// Set labels if specified
	if len(machineScope.NvidiaBMMMachine.Spec.Labels) > 0 {
		labels := restclient.Labels(machineScope.NvidiaBMMMachine.Spec.Labels)
		instanceReq.Labels = &labels
	}

	// Set instance type or specific machine ID
	if machineScope.NvidiaBMMMachine.Spec.InstanceType.ID != "" {
		instanceTypeUUID, err := uuid.Parse(machineScope.NvidiaBMMMachine.Spec.InstanceType.ID)
		if err != nil {
			return fmt.Errorf("invalid instance type ID %s: %w", machineScope.NvidiaBMMMachine.Spec.InstanceType.ID, err)
		}
		instanceReq.InstanceTypeId = &instanceTypeUUID
	}
	if machineScope.NvidiaBMMMachine.Spec.InstanceType.MachineID != "" {
		instanceReq.MachineId = &machineScope.NvidiaBMMMachine.Spec.InstanceType.MachineID
	}

	// Enable phone home for bootstrap communication
	phoneHome := true
	instanceReq.PhoneHomeEnabled = &phoneHome

	logger.Info("Creating NVIDIA BMM instance",
		"name", machineScope.Name(),
		"vpcID", vpcUUID.String(),
		"subnetID", subnetUUID.String(),
		"role", machineScope.Role())

	// Create instance via NVIDIA BMM API
	resp, err := machineScope.NvidiaBMMClient.CreateInstanceWithResponse(ctx, machineScope.OrgName, instanceReq)
	if err != nil {
		return fmt.Errorf("failed to create instance: %w", err)
	}

	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusCreated {
		return fmt.Errorf("failed to create instance, status %d", resp.StatusCode())
	}

	// Extract instance data from response
	if resp.JSON201 == nil {
		return fmt.Errorf("unexpected response: no instance data")
	}

	instance := resp.JSON201
	if instance.Id == nil {
		return fmt.Errorf("instance ID missing in response")
	}

	instanceID := instance.Id.String()
	machineID := ""
	if instance.MachineId != nil {
		machineID = *instance.MachineId
	}

	status := ""
	if instance.Status != nil {
		status = string(*instance.Status) // Convert enum to string
	}

	// Update machine scope with instance details
	machineScope.SetInstanceID(instanceID)
	machineScope.SetMachineID(machineID)
	machineScope.SetInstanceState(status)
	if err := machineScope.SetProviderID(clusterScope.TenantID(), siteName, instanceID); err != nil {
		return fmt.Errorf("failed to set provider ID: %w", err)
	}

	logger.Info("Successfully created NVIDIA BMM instance",
		"instanceID", instanceID,
		"machineID", machineID,
		"status", status)

	return nil
}

func (r *NvidiaBMMMachineReconciler) reconcileInstance(ctx context.Context, machineScope *scope.MachineScope, clusterScope *scope.ClusterScope) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Parse instance ID to UUID
	instanceUUID, err := uuid.Parse(machineScope.InstanceID())
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("invalid instance ID %s: %w", machineScope.InstanceID(), err)
	}

	// Get instance status from NVIDIA BMM
	resp, err := machineScope.NvidiaBMMClient.GetInstanceWithResponse(ctx, machineScope.OrgName, instanceUUID, nil)
	if err != nil {
		logger.Error(err, "failed to get instance status", "instanceID", machineScope.InstanceID())
		return ctrl.Result{}, err
	}

	if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
		logger.Error(nil, "unexpected response getting instance", "instanceID", machineScope.InstanceID(), "status", resp.StatusCode())
		return ctrl.Result{}, fmt.Errorf("failed to get instance, status %d", resp.StatusCode())
	}

	instance := resp.JSON200

	// Update instance state
	if instance.Status != nil {
		machineScope.SetInstanceState(string(*instance.Status))
	}
	if instance.MachineId != nil {
		machineScope.SetMachineID(*instance.MachineId)
	}

	// Extract IP addresses from interfaces
	addresses := []clusterv1.MachineAddress{}
	if instance.Interfaces != nil {
		for _, iface := range *instance.Interfaces {
			if iface.IpAddresses != nil {
				for _, ipAddr := range *iface.IpAddresses {
					addresses = append(addresses, clusterv1.MachineAddress{
						Type:    clusterv1.MachineInternalIP,
						Address: ipAddr,
					})
				}
			}
		}
	}

	if len(addresses) > 0 {
		machineScope.SetAddresses(addresses)
		conditions.Set(machineScope.NvidiaBMMMachine, metav1.Condition{
			Type:   string(NetworkConfiguredCondition),
			Status: metav1.ConditionTrue,
			Reason: "NetworkReady",
		})
	}

	// Check if instance is ready
	if instance.Status != nil && *instance.Status == "Ready" {
		machineScope.SetReady(true)
		conditions.Set(machineScope.NvidiaBMMMachine, metav1.Condition{
			Type:   string(clusterv1.ReadyCondition),
			Status: metav1.ConditionTrue,
			Reason: "NvidiaBMMMachineReady",
		})

		// For first control plane machine, update cluster endpoint if not set
		if machineScope.IsControlPlane() && (clusterScope.NvidiaBMMCluster.Spec.ControlPlaneEndpoint == nil || clusterScope.NvidiaBMMCluster.Spec.ControlPlaneEndpoint.Host == "") {
			if len(addresses) > 0 {
				clusterScope.NvidiaBMMCluster.Spec.ControlPlaneEndpoint = &clusterv1.APIEndpoint{
					Host: addresses[0].Address,
					Port: 6443,
				}
				logger.Info("Updated control plane endpoint", "host", addresses[0].Address)
			}
		}

		instanceIDStr := ""
		if instance.Id != nil {
			instanceIDStr = instance.Id.String()
		}
		logger.Info("NvidiaBMMMachine is ready", "instanceID", instanceIDStr, "status", *instance.Status)
		return ctrl.Result{}, nil
	}

	// Instance is still provisioning, requeue
	instanceIDStr := ""
	statusStr := ""
	if instance.Id != nil {
		instanceIDStr = instance.Id.String()
	}
	if instance.Status != nil {
		statusStr = string(*instance.Status)
	}
	logger.Info("Waiting for instance to be ready",
		"instanceID", instanceIDStr,
		"status", statusStr)

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

//nolint:unparam // ctrl.Result is part of the reconciler interface contract
func (r *NvidiaBMMMachineReconciler) reconcileDelete(ctx context.Context, machineScope *scope.MachineScope) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Deleting NvidiaBMMMachine")

	// Delete instance if it exists
	if machineScope.InstanceID() != "" {
		logger.Info("Deleting NVIDIA BMM instance", "instanceID", machineScope.InstanceID())

		// Parse instance ID to UUID
		instanceUUID, err := uuid.Parse(machineScope.InstanceID())
		if err != nil {
			logger.Error(err, "invalid instance ID", "instanceID", machineScope.InstanceID())
			return ctrl.Result{}, fmt.Errorf("invalid instance ID %s: %w", machineScope.InstanceID(), err)
		}

		// Create delete request body (empty for normal delete, not repair)
		deleteReq := restclient.DeleteInstanceJSONRequestBody{}
		resp, err := machineScope.NvidiaBMMClient.DeleteInstanceWithResponse(ctx, machineScope.OrgName, instanceUUID, deleteReq)
		if err != nil {
			logger.Error(err, "failed to delete instance", "instanceID", machineScope.InstanceID())
			return ctrl.Result{}, err
		}

		if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusNoContent {
			logger.Error(nil, "failed to delete instance", "instanceID", machineScope.InstanceID(), "status", resp.StatusCode())
			return ctrl.Result{}, fmt.Errorf("failed to delete instance, status %d", resp.StatusCode())
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(machineScope.NvidiaBMMMachine, NvidiaBMMMachineFinalizer)

	logger.Info("Successfully deleted NvidiaBMMMachine")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NvidiaBMMMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1.NvidiaBMMMachine{}).
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(util.MachineToInfrastructureMapFunc(infrastructurev1.GroupVersion.WithKind("NvidiaBMMMachine"))),
		).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(mgr.GetScheme(), ctrl.Log.WithName("nvidiabmmmachine"), "")).
		Named("nvidiabmmmachine").
		Complete(r)
}
