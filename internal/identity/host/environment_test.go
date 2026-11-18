package identity

import (
	"errors"
	"strings"
	"testing"
	"time"

	installationid "github.com/ar-imms/telemetry-agent/internal/identity/installation/domain"
)

var errTestMachineIDUnavailable = errors.New("machine ID unreadable")

func hasEvidenceOutcome(evidence []EnvironmentFact, outcome EvidenceOutcome) bool {
	for _, fact := range evidence {
		if fact.Outcome == outcome {
			return true
		}
	}
	return false
}

func TestClassifyEnvironmentKeepsKubernetesSeparateFromComputeType(t *testing.T) {
	observedAt := time.Date(2026, 11, 18, 4, 0, 0, 0, time.UTC)
	got := classifyEnvironment(environmentObservation{
		OS:                       "linux",
		Architecture:             "amd64",
		Hostname:                 "worker-01",
		HostID:                   "linux-opaque",
		HostIDSource:             "linux-machine-id",
		ObservedAt:               observedAt,
		VirtualizationVendor:     "KVM",
		KubernetesDetected:       true,
		KubernetesPodIdentity:    true,
		ContainerRuntimeDetected: true,
	})

	if got.ComputeType != ComputeTypeVM {
		t.Fatalf("compute type = %q, want %q", got.ComputeType, ComputeTypeVM)
	}
	if got.Orchestrator != OrchestratorKubernetes {
		t.Fatalf("orchestrator = %q, want %q", got.Orchestrator, OrchestratorKubernetes)
	}
	if got.DeploymentScope != DeploymentScopeK8sPod {
		t.Fatalf("deployment scope = %q, want %q", got.DeploymentScope, DeploymentScopeK8sPod)
	}
	if got.HostType != HostTypeUnknown {
		t.Fatalf("host type = %q, want %q for a Pod that is not proven to be a node agent", got.HostType, HostTypeUnknown)
	}
	if got.HostIDSource != "linux-machine-id" || got.ObservedAt != observedAt {
		t.Fatalf("identity provenance missing: %+v", got)
	}
}

func TestClassifyEnvironmentLeavesConflictingComputeEvidenceUnknown(t *testing.T) {
	got := classifyEnvironment(environmentObservation{
		OS:                   "windows",
		Architecture:         "amd64",
		VirtualizationVendor: "Microsoft Corporation",
		PhysicalHardwareHint: "Dell Inc.",
		ObservedAt:           time.Now().UTC(),
	})

	if got.ComputeType != ComputeTypeUnknown || got.HostType != HostTypeUnknown {
		t.Fatalf("conflicting evidence must remain unknown, got compute=%q host=%q", got.ComputeType, got.HostType)
	}
	if !hasEvidenceOutcome(got.Evidence, EvidenceOutcomeConflict) {
		t.Fatalf("conflicting evidence should be retained: %+v", got.Evidence)
	}
}

func TestClassifyEnvironmentDoesNotPromoteContainerToHostIdentity(t *testing.T) {
	got := classifyEnvironment(environmentObservation{
		OS:                       "linux",
		Architecture:             "amd64",
		ContainerRuntimeDetected: true,
		ObservedAt:               time.Now().UTC(),
	})

	if got.HostID != "" {
		t.Fatalf("missing host identity must stay empty, got %q", got.HostID)
	}
	if !hasEvidenceOutcome(got.Evidence, EvidenceOutcomeObserved) {
		t.Fatalf("container evidence source was not preserved: %+v", got.Evidence)
	}
	if got.HostType != HostTypeUnknown || got.ComputeType != ComputeTypeUnknown ||
		got.Orchestrator != OrchestratorUnknown || got.DeploymentScope != DeploymentScopeUnknown {
		t.Fatalf("container evidence must not imply host topology: %+v", got)
	}
}

func TestCollectEnvironmentEvidenceKeepsPartialSnapshotWhenHostIDUnavailable(t *testing.T) {
	observedAt := time.Date(2026, 11, 18, 5, 0, 0, 0, time.UTC)
	got := collectEnvironmentEvidence("11111111-2222-4333-8444-555555555555", "agent-host", nil, MachineIdentity{}, errTestMachineIDUnavailable, platformDetails{}, errTestMachineIDUnavailable, platformEnvironmentSignals{}, observedAt)

	if got.Hostname != "agent-host" || got.HostID != "" {
		t.Fatalf("partial host facts were lost or fabricated: %+v", got)
	}
	if got.OS == "" || got.Architecture == "" || got.ObservedAt != observedAt {
		t.Fatalf("platform facts or timestamp missing: %+v", got)
	}
	if got.AgentID != "11111111-2222-4333-8444-555555555555" {
		t.Fatalf("persisted Agent ID was not included: %+v", got)
	}
	if !hasEvidenceOutcome(got.Evidence, EvidenceOutcomeUnavailable) {
		t.Fatalf("missing host ID source should be explicit: %+v", got.Evidence)
	}
}

func TestCollectEnvironmentEvidenceNeverRetainsRawMachineID(t *testing.T) {
	const raw = "sensitive-raw-machine-guid"
	machine := MachineIdentity{HostName: "agent-host", HostID: "windows-opaque"}
	got := collectEnvironmentEvidence("", "agent-host", nil, machine, nil, platformDetails{}, nil, platformEnvironmentSignals{}, time.Now().UTC())
	for _, fact := range got.Evidence {
		if strings.Contains(fact.Source, raw) || strings.Contains(fact.Reason, raw) {
			t.Fatalf("raw machine ID leaked into evidence: %+v", fact)
		}
	}
}

func TestClonedVMHostFingerprintDoesNotReplaceInstallationIdentity(t *testing.T) {
	observedAt := time.Date(2026, 11, 18, 6, 0, 0, 0, time.UTC)
	first := collectEnvironmentEvidence("11111111-2222-4333-8444-555555555555", "clone", nil, MachineIdentity{HostID: "linux-shared-fingerprint"}, nil, platformDetails{}, nil, platformEnvironmentSignals{VirtualizationVendor: "kvm"}, observedAt)
	second := collectEnvironmentEvidence("aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee", "clone", nil, MachineIdentity{HostID: "linux-shared-fingerprint"}, nil, platformDetails{}, nil, platformEnvironmentSignals{VirtualizationVendor: "kvm"}, observedAt)

	if first.HostID != second.HostID {
		t.Fatalf("test setup expected cloned host IDs to match: %q != %q", first.HostID, second.HostID)
	}
	if first.AgentID == "" || second.AgentID == "" || first.AgentID == second.AgentID {
		t.Fatalf("installation identity must remain distinct across cloned host fingerprints: first=%q second=%q", first.AgentID, second.AgentID)
	}
	if first.ComputeType != ComputeTypeVM || second.ComputeType != ComputeTypeVM {
		t.Fatalf("VM evidence was lost: first=%q second=%q", first.ComputeType, second.ComputeType)
	}
}

func TestCollectEnvironmentEvidenceUsesPersistedInstallationID(t *testing.T) {
	const persistedID = "11111111-2222-4333-8444-555555555555"
	id, err := installationid.ParseInstallationID(persistedID)
	if err != nil {
		t.Fatal(err)
	}
	got := CollectEnvironmentEvidence(id)
	if got.AgentID != persistedID {
		t.Fatalf("agent ID = %q, want persisted installation UUID %q", got.AgentID, persistedID)
	}
}

func TestNormalizeHardwareEvidenceDistinguishesVirtualAndPhysicalHints(t *testing.T) {
	tests := []struct {
		name         string
		manufacturer string
		product      string
		wantVirtual  string
		wantPhysical string
	}{
		{name: "vmware product", manufacturer: "VMware, Inc.", product: "VMware Virtual Platform", wantVirtual: "vmware"},
		{name: "kvm product", manufacturer: "QEMU", product: "KVM", wantVirtual: "kvm"},
		{name: "known physical host", manufacturer: "Dell Inc.", product: "PowerEdge R750", wantPhysical: "dell"},
		{name: "unknown vendor", manufacturer: "Example Corp", product: "Model 1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			virtual, physical := normalizeHardwareEvidence(test.manufacturer, test.product)
			if virtual != test.wantVirtual || physical != test.wantPhysical {
				t.Fatalf("normalizeHardwareEvidence() = (%q, %q), want (%q, %q)", virtual, physical, test.wantVirtual, test.wantPhysical)
			}
		})
	}
}

func TestKubernetesEnvironmentSignalsRequirePositiveEvidence(t *testing.T) {
	t.Run("pod environment", func(t *testing.T) {
		got := detectKubernetesSignals(map[string]string{"KUBERNETES_SERVICE_HOST": "10.0.0.1", "POD_UID": "pod-123"})
		if !got.KubernetesDetected || !got.KubernetesPodIdentity || !got.KubernetesDetectionKnown {
			t.Fatalf("Kubernetes pod signals = %+v", got)
		}
	})

	t.Run("no marker remains unknown", func(t *testing.T) {
		got := detectKubernetesSignals(map[string]string{})
		if got.KubernetesDetected || got.KubernetesDetectionKnown {
			t.Fatalf("absence of pod env must not assert no orchestrator: %+v", got)
		}
	})
}

func TestJoinVersionPartsOmitsUnavailableParts(t *testing.T) {
	if got := joinVersionParts(" Windows Server 2025 ", "", "26100"); got != "Windows Server 2025 26100" {
		t.Fatalf("joinVersionParts() = %q", got)
	}
}
