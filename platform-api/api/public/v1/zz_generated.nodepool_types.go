package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// NodePool is the Schema for the nodepools API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type NodePool struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of NodePool
	// +required

	Spec NodePoolSpec `json:"spec"`

	// status defines the observed state of NodePool
	// +optional

	Status NodePoolStatus `json:"status,omitzero"`
}

// NodePoolSpec defines the desired state of a NodePool.
type NodePoolSpec struct {
	// clusterID is the ID of the parent cluster this node pool belongs to.
	// +required

	ClusterID string `json:"clusterID"`

	// platform specifies the infrastructure provider configuration for nodes.
	// +required

	Platform NodePoolPlatformSpec `json:"platform"`

	// release specifies the target release for the node pool.
	// +required

	Release ReleaseSpec `json:"release"`

	// nodeCount is the desired number of nodes. Mutually exclusive with autoscaling.
	// +optional

	NodeCount *int32 `json:"nodeCount,omitempty"`

	// autoscaling defines autoscaling configuration for the node pool.
	// +optional

	Autoscaling *NodePoolAutoscaling `json:"autoscaling,omitempty"`

	// nodeLabels are labels applied to all nodes in the pool.
	// +optional

	NodeLabels map[string]string `json:"nodeLabels,omitempty"`

	// taints are taints applied to all nodes in the pool.
	// +optional

	Taints []Taint `json:"taints,omitempty"`
}

// NodePoolStatus defines the observed state of a NodePool.
type NodePoolStatus struct {
	// conditions represent the latest available observations of a node pool's current state.
	// +optional

	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// NodePoolPlatformSpec specifies the platform-specific configuration for nodes.
type NodePoolPlatformSpec struct {
	// type is the infrastructure provider type (e.g. "GCP", "AWS").
	// +required

	Type string `json:"type"`

	// gcp is the GCP-specific configuration. Required when type is "GCP".
	// +optional

	GCP *GCPNodePoolSpec `json:"gcp,omitempty"`
}

// GCPNodePoolSpec defines GCP-specific node pool configuration.
type GCPNodePoolSpec struct {
	// instanceType is the GCP machine type (e.g. "n2-standard-4").
	// +required

	InstanceType string `json:"instanceType"`
}

// NodePoolAutoscaling defines autoscaling parameters for a node pool.
type NodePoolAutoscaling struct {
	// min is the minimum number of nodes.
	// +required

	Min int32 `json:"min"`

	// max is the maximum number of nodes.
	// +required

	Max int32 `json:"max"`
}

// Taint defines a Kubernetes taint.
type Taint struct {
	// key is the taint key.
	// +required

	Key string `json:"key"`

	// value is the taint value.
	// +optional

	Value string `json:"value,omitempty"`

	// effect is the taint effect (NoSchedule, PreferNoSchedule, NoExecute).
	// +required

	Effect string `json:"effect"`
}

// NodePoolList contains a list of NodePool
// +kubebuilder:object:root=true
type NodePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`

	Items []NodePool `json:"items"`
}

func init() { register(&NodePool{}, &NodePoolList{}) }
