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
	// +orlop:public
	Spec ClusterSpec `json:"spec"`

	// status defines the observed state of Cluster
	// +optional
	// +orlop:public
	Status ClusterStatus `json:"status,omitzero"`
}

// ClusterSpec defines the desired state of a Cluster.
type ClusterSpec struct {
	// infraID is the unique infrastructure identifier for this cluster.
	// +required
	// +orlop:public
	InfraID string `json:"infraID"`

	// issuerURL is the OIDC issuer URL for the cluster.
	// +optional
	// +orlop:public
	IssuerURL string `json:"issuerURL,omitempty"`

	// platform specifies the underlying infrastructure provider configuration.
	// +required
	// +orlop:public
	Platform PlatformSpec `json:"platform"`

	// release specifies the target release for the cluster.
	// +required
	// +orlop:public
	Release ReleaseSpec `json:"release"`

	// networking defines the networking configuration for the cluster.
	// +optional
	// +orlop:public
	Networking *ClusterNetworkingSpec `json:"networking,omitempty"`

	// dns defines the DNS configuration for the cluster.
	// +optional
	// +orlop:public
	DNS *DNSSpec `json:"dns,omitempty"`
}

// ClusterStatus defines the observed state of a Cluster.
type ClusterStatus struct {
	// conditions represent the latest available observations of a cluster's current state.
	// +optional
	// +orlop:public
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// placementResult holds the result of the placement decision.
	// +optional
	// +orlop:public
	PlacementResult *PlacementResult `json:"placementResult,omitempty"`

	// hostedClusterResult holds the result of the hosted cluster provisioning.
	// +optional
	// +orlop:public
	HostedClusterResult *HostedClusterResult `json:"hostedClusterResult,omitempty"`

	// versionResolution holds the resolved version information.
	// +optional
	VersionResolution *VersionResolution `json:"versionResolution,omitempty"`
}

// PlatformSpec specifies the underlying infrastructure provider configuration.
type PlatformSpec struct {
	// type is the infrastructure provider type (e.g. "GCP", "AWS").
	// +required
	// +orlop:public
	Type string `json:"type"`
}

// ReleaseSpec defines the target release for a cluster or node pool.
type ReleaseSpec struct {
	// image is the release image pullspec.
	// +required
	// +orlop:public
	Image string `json:"image"`
}

// ClusterNetworkingSpec defines the networking configuration.
type ClusterNetworkingSpec struct {
	// clusterNetwork is the list of IP address pools for pod IPs.
	// +optional
	// +orlop:public
	ClusterNetwork []NetworkRange `json:"clusterNetwork,omitempty"`

	// serviceNetwork is the list of IP address pools for service IPs.
	// +optional
	// +orlop:public
	ServiceNetwork []NetworkRange `json:"serviceNetwork,omitempty"`

	// networkType is the CNI plugin type (e.g. "OVNKubernetes").
	// +optional
	// +orlop:public
	NetworkType string `json:"networkType,omitempty"`
}

// NetworkRange defines a network CIDR range.
type NetworkRange struct {
	// cidr is the IP address range in CIDR notation.
	// +required
	// +orlop:public
	CIDR string `json:"cidr"`
}

// DNSSpec defines the DNS configuration for the cluster.
type DNSSpec struct {
	// baseDomain is the base DNS domain for the cluster.
	// +required
	// +orlop:public
	BaseDomain string `json:"baseDomain"`
}

// PlacementResult holds the result of the placement decision.
type PlacementResult struct {
	// managementCluster is the name of the management cluster where this cluster is placed.
	// +optional
	// +orlop:public
	ManagementCluster string `json:"managementCluster,omitempty"`
}

// HostedClusterResult holds the result of the hosted cluster provisioning.
type HostedClusterResult struct {
	// kubeconfig is the kubeconfig for accessing the hosted cluster.
	// +optional
	// +orlop:public
	Kubeconfig string `json:"kubeconfig,omitempty"`
}

// VersionResolution holds the resolved version information.
type VersionResolution struct {
	// resolvedImage is the fully-resolved release image pullspec.
	// +optional
	ResolvedImage string `json:"resolvedImage,omitempty"`

	// resolvedVersion is the resolved OCP version string.
	// +optional
	ResolvedVersion string `json:"resolvedVersion,omitempty"`
}

// ClusterList contains a list of Cluster
// +kubebuilder:object:root=true
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	// +orlop:public
	Items []Cluster `json:"items"`
}

func init() { register(&Cluster{}, &ClusterList{}) }
