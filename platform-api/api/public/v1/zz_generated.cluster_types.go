package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// Cluster is the Schema for the clusters API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type Cluster struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Cluster
	// +required

	Spec ClusterSpec `json:"spec"`

	// status defines the observed state of Cluster
	// +optional

	Status ClusterStatus `json:"status,omitzero"`
}

// ClusterSpec defines the desired state of a Cluster.
type ClusterSpec struct {
	// infraID is the unique infrastructure identifier for this cluster.
	// +required

	InfraID string `json:"infraID"`

	// issuerURL is the OIDC issuer URL for the cluster.
	// +optional

	IssuerURL string `json:"issuerURL,omitempty"`

	// platform specifies the underlying infrastructure provider configuration.
	// +required

	Platform PlatformSpec `json:"platform"`

	// release specifies the target release for the cluster.
	// +required

	Release ReleaseSpec `json:"release"`

	// networking defines the networking configuration for the cluster.
	// +optional

	Networking *ClusterNetworkingSpec `json:"networking,omitempty"`

	// dns defines the DNS configuration for the cluster.
	// +optional

	DNS *DNSSpec `json:"dns,omitempty"`
}

// ClusterStatus defines the observed state of a Cluster.
type ClusterStatus struct {
	// conditions represent the latest available observations of a cluster's current state.
	// +optional

	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// placementResult holds the result of the placement decision.
	// +optional

	PlacementResult *PlacementResult `json:"placementResult,omitempty"`

	// hostedClusterResult holds the result of the hosted cluster provisioning.
	// +optional

	HostedClusterResult *HostedClusterResult `json:"hostedClusterResult,omitempty"`
}

// PlatformSpec specifies the underlying infrastructure provider configuration.
type PlatformSpec struct {
	// type is the infrastructure provider type (e.g. "GCP", "AWS").
	// +required

	Type string `json:"type"`
}

// ReleaseSpec defines the target release for a cluster or node pool.
type ReleaseSpec struct {
	// image is the release image pullspec.
	// +required

	Image string `json:"image"`
}

// ClusterNetworkingSpec defines the networking configuration.
type ClusterNetworkingSpec struct {
	// clusterNetwork is the list of IP address pools for pod IPs.
	// +optional

	ClusterNetwork []NetworkRange `json:"clusterNetwork,omitempty"`

	// serviceNetwork is the list of IP address pools for service IPs.
	// +optional

	ServiceNetwork []NetworkRange `json:"serviceNetwork,omitempty"`

	// networkType is the CNI plugin type (e.g. "OVNKubernetes").
	// +optional

	NetworkType string `json:"networkType,omitempty"`
}

// NetworkRange defines a network CIDR range.
type NetworkRange struct {
	// cidr is the IP address range in CIDR notation.
	// +required

	CIDR string `json:"cidr"`
}

// DNSSpec defines the DNS configuration for the cluster.
type DNSSpec struct {
	// baseDomain is the base DNS domain for the cluster.
	// +required

	BaseDomain string `json:"baseDomain"`
}

// PlacementResult holds the result of the placement decision.
type PlacementResult struct {
	// managementCluster is the name of the management cluster where this cluster is placed.
	// +optional

	ManagementCluster string `json:"managementCluster,omitempty"`
}

// HostedClusterResult holds the result of the hosted cluster provisioning.
type HostedClusterResult struct {
	// kubeconfig is the kubeconfig for accessing the hosted cluster.
	// +optional

	Kubeconfig string `json:"kubeconfig,omitempty"`
}

// ClusterList contains a list of Cluster
// +kubebuilder:object:root=true
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`

	Items []Cluster `json:"items"`
}

func init() { register(&Cluster{}, &ClusterList{}) }
