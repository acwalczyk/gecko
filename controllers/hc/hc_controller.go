// Package hc implements the hc-controller reconciler for managing HostedClusters via ManifestWork.
package hc

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	privatev1 "github.com/openshift-online/gecko/platform-api/api/private/v1"

	"github.com/openshift-online/gecko/controllers/client/transport"
	"github.com/openshift-online/gecko/controllers/hc/manifest"
	"github.com/openshift-online/gecko/controllers/util/constants"
	"github.com/openshift-online/gecko/controllers/util/logger"
)

const (
	adapterName = "hc-controller"

	requeuePending = 15 * time.Second
	requeueStable  = 5 * time.Minute
)

// Reconciler implements the hc-controller reconcile loop.
type Reconciler struct {
	transport transport.Client
	log       logger.Logger
	client    client.Client
}

// New creates a new Reconciler.
func New(transport transport.Client, log logger.Logger, c client.Client) *Reconciler {
	return &Reconciler{
		transport: transport,
		log:       log,
		client:    c,
	}
}

// Reconcile runs the hc-controller loop for one cluster event.
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	clusterID := req.Name
	log := r.log.With("controller", adapterName).With("cluster_id", clusterID)

	var cluster privatev1.Cluster
	if err := r.client.Get(ctx, req.NamespacedName, &cluster); err != nil {
		if apierrors.IsNotFound(err) {
			log.Infof(ctx, "cluster not found, skipping")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("%s: get cluster: %w", adapterName, err)
	}

	// Check placement readiness.
	if cluster.Status.PlacementResult == nil || cluster.Status.PlacementResult.ManagementClusterName == "" {
		log.Infof(ctx, "placement not ready, waiting for next event")
		if r.setWaitingConditions(&cluster, "PlacementNotReady", "Waiting for placement to select a management cluster") {
			if err := r.client.Status().Update(ctx, &cluster); err != nil && !apierrors.IsConflict(err) {
				return reconcile.Result{}, fmt.Errorf("%s: update cluster status: %w", adapterName, err)
			}
		}
		return reconcile.Result{}, nil
	}

	// Check version-resolution readiness.
	if cluster.Status.VersionResolution == nil {
		log.Infof(ctx, "version resolution not ready, waiting for next event")
		if r.setWaitingConditions(&cluster, "VersionResolutionNotReady", "Waiting for version resolution") {
			if err := r.client.Status().Update(ctx, &cluster); err != nil && !apierrors.IsConflict(err) {
				return reconcile.Result{}, fmt.Errorf("%s: update cluster status: %w", adapterName, err)
			}
		}
		return reconcile.Result{}, nil
	}

	// Check version match.
	if cluster.Status.VersionResolution.ReleaseVersion != cluster.Spec.Release.Version {
		log.Infof(ctx, "vr version %q does not match spec version %q, waiting for next event",
			cluster.Status.VersionResolution.ReleaseVersion, cluster.Spec.Release.Version)
		msg := fmt.Sprintf("VR version %q does not match spec version %q",
			cluster.Status.VersionResolution.ReleaseVersion, cluster.Spec.Release.Version)
		if r.setWaitingConditions(&cluster, "VersionMismatch", msg) {
			if err := r.client.Status().Update(ctx, &cluster); err != nil && !apierrors.IsConflict(err) {
				return reconcile.Result{}, fmt.Errorf("%s: update cluster status: %w", adapterName, err)
			}
		}
		return reconcile.Result{}, nil
	}

	placement := cluster.Status.PlacementResult
	vr := cluster.Status.VersionResolution

	// Extract platform fields.
	var gcpProjectID, gcpRegion, gcpNetwork, gcpSubnet string
	var wifProjectNumber, wifPoolID, wifProviderID string
	var nodePoolEmail, controlPlaneEmail, cloudControllerEmail string
	var storageEmail, imageRegistryEmail, networkEmail string
	if gcp := cluster.Spec.Platform.GCP; gcp != nil {
		gcpProjectID = gcp.ProjectID
		gcpRegion = gcp.Region
		gcpNetwork = gcp.Network
		gcpSubnet = gcp.Subnet
		wif := gcp.WorkloadIdentity
		wifProjectNumber = wif.ProjectNumber
		wifPoolID = wif.PoolID
		wifProviderID = wif.ProviderID
		if sa := wif.ServiceAccountsRef; sa != nil {
			nodePoolEmail = sa.NodePoolEmail
			controlPlaneEmail = sa.ControlPlaneEmail
			cloudControllerEmail = sa.CloudControllerEmail
			storageEmail = sa.StorageEmail
			imageRegistryEmail = sa.ImageRegistryEmail
			networkEmail = sa.NetworkEmail
		}
	}

	// Build manifests.
	// TODO: ClusterIDUUID and CreatedBy are not yet in the orlop ClusterSpec.
	mwInput := manifest.Input{
		ClusterID:            clusterID,
		ClusterName:          cluster.Name,
		Generation:           cluster.Generation,
		CreatedBy:            "", // TODO: not in types
		InfraID:              cluster.Spec.InfraID,
		IssuerURL:            cluster.Spec.IssuerURL,
		ClusterIDUUID:        string(cluster.UID),
		GCPProjectID:         gcpProjectID,
		GCPRegion:            gcpRegion,
		GCPNetwork:           gcpNetwork,
		GCPSubnet:            gcpSubnet,
		WIFProjectNumber:     wifProjectNumber,
		WIFPoolID:            wifPoolID,
		WIFProviderID:        wifProviderID,
		NodePoolEmail:        nodePoolEmail,
		ControlPlaneEmail:    controlPlaneEmail,
		CloudControllerEmail: cloudControllerEmail,
		StorageEmail:         storageEmail,
		ImageRegistryEmail:   imageRegistryEmail,
		NetworkEmail:         networkEmail,
		ReleaseImage:         vr.ReleaseImage,
		ReleaseChannel:       vr.CincinnatiChannel,
		BaseDomain:           placement.BaseDomain,
	}

	manifests, err := manifest.Build(mwInput)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("%s: build manifests: %w", adapterName, err)
	}

	mwStatus, err := r.transport.Apply(ctx, placement.ManagementClusterName, clusterID, manifests)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("%s: apply resources: %w", adapterName, err)
	}

	// Write status conditions — only update if something changed.
	if r.applyStatusConditions(&cluster, mwStatus) {
		if err := r.client.Status().Update(ctx, &cluster); err != nil {
			if apierrors.IsConflict(err) {
				return reconcile.Result{}, nil
			}
			return reconcile.Result{}, fmt.Errorf("%s: update cluster status: %w", adapterName, err)
		}
	}

	if !meta.IsStatusConditionTrue(cluster.Status.Conditions, "ManifestWorkApplied") {
		log.Infof(ctx, "hc-controller: cluster %s MW not yet applied, requeueing after %s", clusterID, requeuePending)
		return reconcile.Result{RequeueAfter: requeuePending}, nil
	}
	log.Infof(ctx, "hc-controller: cluster %s reconciled, requeueing after %s", clusterID, requeueStable)
	return reconcile.Result{RequeueAfter: requeueStable}, nil
}

// setWaitingConditions sets ManifestWorkApplied and HostedClusterAvailable to Unknown.
// Returns true if either condition changed.
func (r *Reconciler) setWaitingConditions(cluster *privatev1.Cluster, reason, message string) bool {
	gen := cluster.Generation
	a := meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               "ManifestWorkApplied",
		Status:             metav1.ConditionUnknown,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: gen,
	})
	b := meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               "HostedClusterAvailable",
		Status:             metav1.ConditionUnknown,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: gen,
	})
	return a || b
}

// applyStatusConditions derives conditions from the resource status and writes them to the cluster.
// Returns true if any condition changed.
func (r *Reconciler) applyStatusConditions(cluster *privatev1.Cluster, mwStatus *transport.Status) bool {
	gen := cluster.Generation

	if mwStatus == nil {
		a := meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               "ManifestWorkApplied",
			Status:             metav1.ConditionFalse,
			Reason:             "ManifestWorkNotFound",
			Message:            "ManifestWork has not been processed yet",
			ObservedGeneration: gen,
		})
		b := meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               "HostedClusterAvailable",
			Status:             metav1.ConditionFalse,
			Reason:             "ManifestWorkNotFound",
			Message:            "ManifestWork has not been processed yet",
			ObservedGeneration: gen,
		})
		return a || b
	}

	// Derive ManifestWorkApplied from top-level MW conditions.
	appliedStatus, appliedReason, appliedMessage := mwCondition(mwStatus.Conditions, "Applied")

	// Derive HostedClusterAvailable and HostedClusterResult fields from HC resource status.
	hcKey := transport.ResourceKey(constants.HyperShiftGroup, constants.HyperShiftVersion, "hostedclusters",
		fmt.Sprintf("clusters-%s", cluster.Name), cluster.Name)
	availableStatus := string(metav1.ConditionFalse)
	apiEndpoint := ""
	version := ""
	if hcFeedback, ok := mwStatus.ResourceStatuses[hcKey]; ok {
		if v, ok := hcFeedback["availableCondition"]; ok {
			availableStatus = v
		}
		if v, ok := hcFeedback["controlPlaneEndpoint"]; ok {
			apiEndpoint = v
		}
		if v, ok := hcFeedback["version"]; ok {
			version = v
		}
	}

	a := meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               "ManifestWorkApplied",
		Status:             metav1.ConditionStatus(appliedStatus),
		Reason:             appliedReason,
		Message:            appliedMessage,
		ObservedGeneration: gen,
	})
	b := meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               "HostedClusterAvailable",
		Status:             metav1.ConditionStatus(availableStatus),
		Reason:             "HostedClusterAvailable",
		ObservedGeneration: gen,
	})

	// Write HostedClusterResult when either field is non-empty.
	c := false
	if apiEndpoint != "" || version != "" {
		desired := &privatev1.HostedClusterResult{
			APIEndpoint: apiEndpoint,
			Version:     version,
		}
		if cluster.Status.HostedClusterResult == nil ||
			cluster.Status.HostedClusterResult.APIEndpoint != desired.APIEndpoint ||
			cluster.Status.HostedClusterResult.Version != desired.Version {
			cluster.Status.HostedClusterResult = desired
			c = true
		}
	}

	return a || b || c
}

// mwCondition returns the status, reason, and message of the first MW condition matching condType.
// Defaults: status="False", reason="Unknown", message="".
func mwCondition(conds []metav1.Condition, condType string) (status, reason, message string) {
	for _, c := range conds {
		if c.Type == condType {
			return string(c.Status), c.Reason, c.Message
		}
	}
	return "False", "Unknown", ""
}
