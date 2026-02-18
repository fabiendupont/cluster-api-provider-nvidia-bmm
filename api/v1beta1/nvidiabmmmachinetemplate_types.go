package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NvidiaBMMMachineTemplateSpec defines the desired state of NvidiaBMMMachineTemplate
type NvidiaBMMMachineTemplateSpec struct {
	// Template contains the NvidiaBMMMachine template specification
	// +required
	Template NvidiaBMMMachineTemplateResource `json:"template"`
}

// NvidiaBMMMachineTemplateResource describes the data needed to create a NvidiaBMMMachine from a template
type NvidiaBMMMachineTemplateResource struct {
	// Standard object's metadata
	// +optional
	ObjectMeta metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the specification of the desired behavior of the machine
	// +required
	Spec NvidiaBMMMachineSpec `json:"spec"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=nvidiabmmmachinetemplates,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion

// NvidiaBMMMachineTemplate is the Schema for the nvidiabmmmachinetemplates API
type NvidiaBMMMachineTemplate struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of NvidiaBMMMachineTemplate
	// +required
	Spec NvidiaBMMMachineTemplateSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// NvidiaBMMMachineTemplateList contains a list of NvidiaBMMMachineTemplate
type NvidiaBMMMachineTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []NvidiaBMMMachineTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NvidiaBMMMachineTemplate{}, &NvidiaBMMMachineTemplateList{})
}
