package firestore

import (
	"encoding/json"
	"fmt"

	"github.com/openshift-online/kube-applier-gcp/pkg/api/kubeapplier"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-online/gecko/controllers/util/constants"
)

// aggregateConditions derives a single "Applied" metav1.Condition from the
// Successful conditions on a slice of ApplyDesire status documents.
// Applied=True only when every desire has Successful=True.
// Applied=False if any desire has Successful=False, or if the slice is empty,
// or if any desire has no Successful condition (still pending).
func aggregateConditions(desires []kubeapplier.ApplyDesire) []metav1.Condition {
	if len(desires) == 0 {
		return []metav1.Condition{{
			Type:    "Applied",
			Status:  metav1.ConditionFalse,
			Reason:  "NoApplyDesires",
			Message: "No ApplyDesire documents found for this ManifestWork",
		}}
	}

	allTrue := true
	for _, d := range desires {
		found := false
		for _, c := range d.Status.Conditions {
			if c.Type == kubeapplier.ConditionTypeSuccessful {
				found = true
				if c.Status != metav1.ConditionTrue {
					allTrue = false
				}
				break
			}
		}
		if !found {
			// kube-applier-gcp has not yet processed this desire
			allTrue = false
		}
	}

	if allTrue {
		return []metav1.Condition{{
			Type:    "Applied",
			Status:  metav1.ConditionTrue,
			Reason:  "AllResourcesApplied",
			Message: fmt.Sprintf("All %d resources applied successfully", len(desires)),
		}}
	}

	// Check if any desire has no Successful condition (still pending)
	for _, d := range desires {
		hasCond := false
		for _, c := range d.Status.Conditions {
			if c.Type == kubeapplier.ConditionTypeSuccessful {
				hasCond = true
				break
			}
		}
		if !hasCond {
			return []metav1.Condition{{
				Type:    "Applied",
				Status:  metav1.ConditionFalse,
				Reason:  "Pending",
				Message: "One or more resources not yet processed by kube-applier-gcp",
			}}
		}
	}

	return []metav1.Condition{{
		Type:    "Applied",
		Status:  metav1.ConditionFalse,
		Reason:  "ApplyFailed",
		Message: "One or more resources failed to apply",
	}}
}

// extractResourceStatuses parses ReadDesire status documents and returns
// per-resource status fields keyed by resource identity string.
// For HostedCluster resources it extracts: availableCondition, controlPlaneEndpoint, version.
// For NodePool resources it extracts: readyCondition, allNodesHealthyCondition.
// Other resources: empty inner map (no known fields to extract).
func extractResourceStatuses(reads []kubeapplier.ReadDesire) map[string]map[string]string {
	result := make(map[string]map[string]string, len(reads))
	for _, rd := range reads {
		key := resourceKey(rd.Spec.TargetItem)
		fields := map[string]string{}

		if rd.Status.KubeContent == nil || len(rd.Status.KubeContent.Raw) == 0 {
			result[key] = fields
			continue
		}

		ref := rd.Spec.TargetItem
		switch {
		case ref.Resource == "hostedclusters" && ref.Group == constants.HyperShiftGroup:
			fields = extractHCFields(rd.Status.KubeContent.Raw)
		case ref.Resource == "nodepools" && ref.Group == constants.HyperShiftGroup:
			fields = extractNPFields(rd.Status.KubeContent.Raw)
		}

		result[key] = fields
	}
	return result
}

// extractHCFields extracts HostedCluster status fields from raw live-object JSON.
//   - availableCondition: .status.conditions[type=Available].status
//   - controlPlaneEndpoint: .status.controlPlaneEndpoint.host
//   - version: .status.version.history[0].version
func extractHCFields(raw []byte) map[string]string {
	fields := map[string]string{}

	var obj struct {
		Status struct {
			Conditions []struct {
				Type   string `json:"type"`
				Status string `json:"status"`
			} `json:"conditions"`
			ControlPlaneEndpoint struct {
				Host string `json:"host"`
			} `json:"controlPlaneEndpoint"`
			Version struct {
				History []struct {
					Version string `json:"version"`
					State   string `json:"state"`
				} `json:"history"`
			} `json:"version"`
		} `json:"status"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return fields
	}

	for _, c := range obj.Status.Conditions {
		if c.Type == "Available" {
			fields["availableCondition"] = c.Status
			break
		}
	}

	if host := obj.Status.ControlPlaneEndpoint.Host; host != "" {
		fields["controlPlaneEndpoint"] = host
	}

	if len(obj.Status.Version.History) > 0 {
		fields["version"] = obj.Status.Version.History[0].Version
	}

	return fields
}

// extractNPFields extracts NodePool status fields from raw live-object JSON.
//   - readyCondition: .status.conditions[type=Ready].status
//   - allNodesHealthyCondition: .status.conditions[type=AllNodesHealthy].status
func extractNPFields(raw []byte) map[string]string {
	fields := map[string]string{}

	var obj struct {
		Status struct {
			Conditions []struct {
				Type   string `json:"type"`
				Status string `json:"status"`
			} `json:"conditions"`
		} `json:"status"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return fields
	}

	for _, c := range obj.Status.Conditions {
		switch c.Type {
		case "Ready":
			fields["readyCondition"] = c.Status
		case "AllNodesHealthy":
			fields["allNodesHealthyCondition"] = c.Status
		}
	}

	return fields
}
