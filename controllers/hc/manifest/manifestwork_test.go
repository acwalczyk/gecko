package manifest_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/openshift-online/gecko/controllers/hc/manifest"
	"github.com/stretchr/testify/require"
)

func testInput() manifest.Input {
	return manifest.Input{
		ClusterID:                    "cluster-abc",
		ClusterName:                  "my-cluster",
		Generation:                   3,
		CreatedBy:                    "alice@redhat.com",
		InfraID:                      "infra-xyz",
		IssuerURL:                    "https://issuer.example.com",
		ClusterIDUUID:                "550e8400-e29b-41d4-a716-446655440000",
		GCPProjectID:                 "my-gcp-project",
		GCPRegion:                    "us-central1",
		GCPNetwork:                   "my-vpc",
		GCPSubnet:                    "my-subnet",
		GCPEndpointAccess:            "Private",
		WIFProjectNumber:             "123456789",
		WIFPoolID:                    "my-pool",
		WIFProviderID:                "my-provider",
		NodePoolEmail:                "nodepool@project.iam.gserviceaccount.com",
		ControlPlaneEmail:            "cp@project.iam.gserviceaccount.com",
		CloudControllerEmail:         "cc@project.iam.gserviceaccount.com",
		StorageEmail:                 "storage@project.iam.gserviceaccount.com",
		ImageRegistryEmail:           "registry@project.iam.gserviceaccount.com",
		NetworkEmail:                 "network@project.iam.gserviceaccount.com",
		ReleaseImage:                 "quay.io/openshift-release-dev/ocp-release:4.15.0-x86_64",
		ReleaseChannel:               "stable-4.15",
		BaseDomain:                   "example.com",
		PullSecretStoreName:          "gcp-secret-manager",
		PullSecretGCPKey:             "default-openshift-pull-secret",
		ControllerAvailabilityPolicy: "HighlyAvailable",
		CPOImage:                     "",
		CAPGImage:                    "",
		Slug:                         "alice",
	}
}

func TestBuild_ReturnsManifests(t *testing.T) {
	input := testInput()
	manifests, err := manifest.Build(input)
	require.NoError(t, err)
	require.NotNil(t, manifests)
}

func TestBuild_FiveManifests(t *testing.T) {
	input := testInput()
	manifests, err := manifest.Build(input)
	require.NoError(t, err)
	require.Len(t, manifests, 5, "expected 5 manifests: Namespace, ExternalSecret, Certificate, HostedCluster, Job")
}

func TestBuild_ManifestKinds(t *testing.T) {
	input := testInput()
	manifests, err := manifest.Build(input)
	require.NoError(t, err)

	expectedKinds := []string{"Namespace", "ExternalSecret", "Certificate", "HostedCluster", "Job"}
	for i, m := range manifests {
		var obj map[string]any
		require.NoError(t, json.Unmarshal(m, &obj))
		kind, ok := obj["kind"].(string)
		require.True(t, ok, "manifest[%d] missing kind", i)
		require.Equal(t, expectedKinds[i], kind, "manifest[%d] wrong kind", i)
	}
}

func TestBuild_HostedClusterReleaseImage(t *testing.T) {
	input := testInput()
	manifests, err := manifest.Build(input)
	require.NoError(t, err)

	// HostedCluster is at index 3.
	var obj map[string]any
	require.NoError(t, json.Unmarshal(manifests[3], &obj))

	spec, ok := obj["spec"].(map[string]any)
	require.True(t, ok, "HostedCluster missing spec")
	release, ok := spec["release"].(map[string]any)
	require.True(t, ok, "HostedCluster spec missing release")
	require.Equal(t, input.ReleaseImage, release["image"])
}

func TestBuild_JobName(t *testing.T) {
	input := testInput()
	manifests, err := manifest.Build(input)
	require.NoError(t, err)

	var obj map[string]any
	require.NoError(t, json.Unmarshal(manifests[4], &obj))

	meta, ok := obj["metadata"].(map[string]any)
	require.True(t, ok, "Job missing metadata")
	expectedJobName := fmt.Sprintf("rbac-setup-gen-%d", input.Generation)
	require.Equal(t, expectedJobName, meta["name"])
}

func TestBuild_CPOAnnotation(t *testing.T) {
	input := testInput()
	input.CPOImage = "quay.io/openshift/hypershift:latest"

	manifests, err := manifest.Build(input)
	require.NoError(t, err)

	var obj map[string]any
	require.NoError(t, json.Unmarshal(manifests[3], &obj))

	meta := obj["metadata"].(map[string]any)
	annotations := meta["annotations"].(map[string]any)
	require.Equal(t, input.CPOImage, annotations["hypershift.openshift.io/control-plane-operator-image"])
}

func TestBuild_NoCPOAnnotationWhenEmpty(t *testing.T) {
	input := testInput()
	input.CPOImage = "" // empty — should not be set

	manifests, err := manifest.Build(input)
	require.NoError(t, err)

	var obj map[string]any
	require.NoError(t, json.Unmarshal(manifests[3], &obj))

	meta := obj["metadata"].(map[string]any)
	annotations := meta["annotations"].(map[string]any)
	_, hasCPO := annotations["hypershift.openshift.io/control-plane-operator-image"]
	require.False(t, hasCPO, "CPO annotation should not be set when CPOImage is empty")
}

func TestBuild_DefaultEndpointAccess(t *testing.T) {
	input := testInput()
	input.GCPEndpointAccess = "" // should default to "Private"

	manifests, err := manifest.Build(input)
	require.NoError(t, err)

	var obj map[string]any
	require.NoError(t, json.Unmarshal(manifests[3], &obj))

	spec := obj["spec"].(map[string]any)
	platform := spec["platform"].(map[string]any)
	gcp := platform["gcp"].(map[string]any)
	require.Equal(t, "Private", gcp["endpointAccess"])
}

func TestBuild_RequiredFieldValidation(t *testing.T) {
	input := testInput()
	input.ClusterID = ""
	_, err := manifest.Build(input)
	require.Error(t, err)
}

func TestBuild_GenerationValidation(t *testing.T) {
	input := testInput()
	input.Generation = 0
	_, err := manifest.Build(input)
	require.Error(t, err)
}
