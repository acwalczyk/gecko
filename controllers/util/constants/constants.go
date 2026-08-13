package constants

// HyperFleet Kubernetes Resource Annotations and Labels
// These constants define standard annotations and labels used across HyperFleet resources
// for tracking, management, and identification purposes.

const (
	// AnnotationGeneration is the annotation key for tracking resource generation.
	// This is used to track changes and ensure resources are updated with the correct generation.
	// Format: "hyperfleet.io/generation"
	// Example value: "5" (integer as string)
	AnnotationGeneration = "hyperfleet.io/generation"

	// LabelGeneration is the label key for tracking resource generation.
	// Used for label-based filtering and selection of resources by generation.
	// Format: "hyperfleet.io/generation"
	// Example value: "5" (integer as string)
	LabelGeneration = "hyperfleet.io/generation"

	// AnnotationClusterID is the annotation key for cluster identification.
	// Links resources to their target cluster.
	// Format: "hyperfleet.io/cluster-id"
	AnnotationClusterID = "hyperfleet.io/cluster-id"

	// LabelClusterID is the label key for cluster identification.
	// Used for label-based filtering and selection of resources by cluster.
	// Format: "hyperfleet.io/cluster-id"
	LabelClusterID = "hyperfleet.io/cluster-id"

	// AnnotationController is the annotation key for controller identification.
	// Identifies which controller created or manages the resource.
	// Format: "hyperfleet.io/controller"
	AnnotationController = "hyperfleet.io/controller"

	// LabelController is the label key for controller identification.
	// Used for label-based filtering and selection of resources by controller.
	// Format: "hyperfleet.io/controller"
	LabelController = "hyperfleet.io/controller"

	// AnnotationManagedBy identifies the entity managing the resource.
	// Format: "hyperfleet.io/managed-by"
	// Example value: "gecko-controllers"
	AnnotationManagedBy = "hyperfleet.io/managed-by"

	// LabelManagedBy is the label key for managed-by identification.
	// Format: "hyperfleet.io/managed-by"
	LabelManagedBy = "hyperfleet.io/managed-by"

	// AnnotationCreatedBy identifies the entity that created the resource.
	// Format: "hyperfleet.io/created-by"
	// Example value: "gecko-controllers"
	AnnotationCreatedBy = "hyperfleet.io/created-by"
)

// Finalizer constants for deletion handling.
const (
	// FinalizerCluster is the finalizer added by the hc-controller to ensure
	// management-cluster resources are cleaned up before a Cluster CR is deleted.
	FinalizerCluster = "hyperfleet.io/cluster-finalizer"

	// FinalizerNodePool is the finalizer added by the nodepool-controller to ensure
	// management-cluster resources are cleaned up before a NodePool CR is deleted.
	FinalizerNodePool = "hyperfleet.io/nodepool-finalizer"
)

// HyperShift API constants
const (
	// HyperShiftGroup is the API group for HyperShift resources (HostedCluster, NodePool, etc.).
	HyperShiftGroup = "hypershift.openshift.io"

	// HyperShiftVersion is the API version for HyperShift resources.
	HyperShiftVersion = "v1beta1"
)

// OCM ManifestWork GVK constants
const (
	// ManifestWorkGroup is the API group for OCM ManifestWork resources.
	ManifestWorkGroup = "work.open-cluster-management.io"

	// ManifestWorkVersion is the API version for OCM ManifestWork resources.
	ManifestWorkVersion = "v1"

	// ManifestWorkKind is the Kind for OCM ManifestWork resources.
	ManifestWorkKind = "ManifestWork"
)
