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
	// +orlop:public
	Spec NodePoolSpec `json:"spec"`

	// status defines the observed state of NodePool
	// +optional
	// +orlop:public
	Status NodePoolStatus `json:"status,omitzero"`
}

// NodePoolSpec defines the desired state of a NodePool.
type NodePoolSpec struct {
	// clusterID is the ID of the parent cluster this node pool belongs to.
	// +required
	// +orlop:public
	ClusterID string `json:"clusterID"`

	// platform specifies the infrastructure provider configuration for nodes.
	// +required
	// +orlop:public
	Platform NodePoolPlatformSpec `json:"platform"`

	// release specifies the target release for the node pool.
	// +required
	// +orlop:public
	Release ReleaseSpec `json:"release"`

	// nodeCount is the desired number of nodes. Mutually exclusive with autoscaling.
	// +optional
	// +orlop:public
	NodeCount *int32 `json:"nodeCount,omitempty"`

	// autoscaling defines autoscaling configuration for the node pool.
	// +optional
	// +orlop:public
	Autoscaling *NodePoolAutoscaling `json:"autoscaling,omitempty"`

	// nodeLabels are labels applied to all nodes in the pool.
	// +optional
	// +orlop:public
	NodeLabels map[string]string `json:"nodeLabels,omitempty"`

	// taints are taints applied to all nodes in the pool.
	// +optional
	// +orlop:public
	Taints []Taint `json:"taints,omitempty"`
}

// NodePoolStatus defines the observed state of a NodePool.
type NodePoolStatus struct {
	// conditions represent the latest available observations of a node pool's current state.
	// +optional
	// +orlop:public
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// versionResolution holds the resolved version information.
	// +optional
	VersionResolution *VersionResolution `json:"versionResolution,omitempty"`
}

// NodePoolPlatformSpec specifies the platform-specific configuration for nodes.
type NodePoolPlatformSpec struct {
	// type is the infrastructure provider type (e.g. "GCP", "AWS").
	// +required
	// +orlop:public
	Type string `json:"type"`

	// gcp is the GCP-specific configuration. Required when type is "GCP".
	// +optional
	// +orlop:public
	GCP *GCPNodePoolSpec `json:"gcp,omitempty"`
}

// GCPNodePoolSpec defines GCP-specific node pool configuration.
type GCPNodePoolSpec struct {
	// instanceType is the GCP machine type (e.g. "n2-standard-4").
	// +required
	// +orlop:public
	InstanceType string `json:"instanceType"`
}

// NodePoolAutoscaling defines autoscaling parameters for a node pool.
type NodePoolAutoscaling struct {
	// min is the minimum number of nodes.
	// +required
	// +orlop:public
	Min int32 `json:"min"`

	// max is the maximum number of nodes.
	// +required
	// +orlop:public
	Max int32 `json:"max"`
}

// Taint defines a Kubernetes taint.
type Taint struct {
	// key is the taint key.
	// +required
	// +orlop:public
	Key string `json:"key"`

	// value is the taint value.
	// +optional
	// +orlop:public
	Value string `json:"value,omitempty"`

	// effect is the taint effect (NoSchedule, PreferNoSchedule, NoExecute).
	// +required
	// +orlop:public
	Effect string `json:"effect"`
}

// NodePoolList contains a list of NodePool
// +kubebuilder:object:root=true
type NodePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	// +orlop:public
	Items []NodePool `json:"items"`
}

func init() { register(&NodePool{}, &NodePoolList{}) }
