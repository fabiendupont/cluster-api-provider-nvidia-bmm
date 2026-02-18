package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NvidiaBMMMachineSpec defines the desired state of NvidiaBMMMachine
type NvidiaBMMMachineSpec struct {
	// ProviderID is the unique identifier for the machine instance
	// Format: nvidia-bmm://org/tenant/site/instance-id
	// +optional
	ProviderID *string `json:"providerID,omitempty"`

	// InstanceType specifies the machine instance configuration
	// +required
	InstanceType InstanceTypeSpec `json:"instanceType"`

	// OperatingSystem configuration for the machine
	// +optional
	OperatingSystem *OSSpec `json:"operatingSystem,omitempty"`

	// Network configuration for the machine
	// +required
	Network NetworkSpec `json:"network"`

	// SSHKeyGroups contains SSH key group IDs for accessing the machine
	// +optional
	SSHKeyGroups []string `json:"sshKeyGroups,omitempty"`

	// Labels to apply to the NVIDIA BMM instance
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// InstanceTypeSpec specifies the instance type or specific machine allocation
type InstanceTypeSpec struct {
	// ID specifies the NVIDIA BMM instance type UUID
	// Mutually exclusive with MachineID
	// +optional
	ID string `json:"id,omitempty"`

	// MachineID specifies a specific machine UUID for targeted provisioning
	// Mutually exclusive with ID
	// +optional
	MachineID string `json:"machineID,omitempty"`

	// AllowUnhealthyMachine allows provisioning on an unhealthy machine
	// +optional
	AllowUnhealthyMachine bool `json:"allowUnhealthyMachine,omitempty"`
}

// OSSpec defines operating system configuration
type OSSpec struct {
	// Type specifies the OS type (e.g., "ubuntu", "rhel")
	// +optional
	Type string `json:"type,omitempty"`

	// Version specifies the OS version
	// +optional
	Version string `json:"version,omitempty"`
}

// NetworkSpec defines network configuration for the machine
type NetworkSpec struct {
	// SubnetName specifies the subnet to attach the machine to
	// +required
	SubnetName string `json:"subnetName"`

	// AdditionalInterfaces for multi-NIC configurations
	// +optional
	AdditionalInterfaces []NetworkInterface `json:"additionalInterfaces,omitempty"`
}

// NetworkInterface defines an additional network interface
type NetworkInterface struct {
	// SubnetName specifies the subnet for this interface
	// +required
	SubnetName string `json:"subnetName"`

	// IsPhysical indicates if this is a physical interface
	// +optional
	IsPhysical bool `json:"isPhysical,omitempty"`
}

// NvidiaBMMMachineStatus defines the observed state of NvidiaBMMMachine.
type NvidiaBMMMachineStatus struct {
	// Ready indicates if the machine is ready and available
	// +optional
	Ready bool `json:"ready"`

	// InstanceID is the NVIDIA BMM instance ID
	// +optional
	InstanceID string `json:"instanceID,omitempty"`

	// MachineID is the physical machine ID
	// +optional
	MachineID string `json:"machineID,omitempty"`

	// InstanceState represents the current state of the instance
	// Possible values: Pending, Provisioning, Ready, Error, Terminating
	// +optional
	InstanceState string `json:"instanceState,omitempty"`

	// Addresses contains the IP addresses assigned to the machine
	// +optional
	Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`

	// Conditions represent the current state of the NvidiaBMMMachine
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// GetConditions returns the conditions from the status
func (m *NvidiaBMMMachine) GetConditions() []metav1.Condition {
	return m.Status.Conditions
}

// SetConditions sets the conditions in the status
func (m *NvidiaBMMMachine) SetConditions(conditions []metav1.Condition) {
	m.Status.Conditions = conditions
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// NvidiaBMMMachine is the Schema for the nvidiabmmmachines API
type NvidiaBMMMachine struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of NvidiaBMMMachine
	// +required
	Spec NvidiaBMMMachineSpec `json:"spec"`

	// status defines the observed state of NvidiaBMMMachine
	// +optional
	Status NvidiaBMMMachineStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// NvidiaBMMMachineList contains a list of NvidiaBMMMachine
type NvidiaBMMMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []NvidiaBMMMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NvidiaBMMMachine{}, &NvidiaBMMMachineList{})
}
