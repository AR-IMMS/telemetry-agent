package identity

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	installationid "github.com/ar-imms/telemetry-agent/internal/identity/installation/domain"
)

// HostType is the display classification derived from compute and deployment evidence.
type HostType string

const (
	// HostTypeBareMetal identifies a host with positive physical hardware evidence.
	HostTypeBareMetal HostType = "BARE_METAL"
	// HostTypeVM identifies a virtual machine.
	HostTypeVM HostType = "VM"
	// HostTypeKubernetes identifies a verified Kubernetes node Agent.
	HostTypeKubernetes HostType = "KUBERNETES"
	// HostTypeUnknown means available evidence cannot support a host classification.
	HostTypeUnknown HostType = "UNKNOWN"
)

// ComputeType describes physical or virtual execution independently of orchestration.
type ComputeType string

const (
	// ComputeTypeBareMetal indicates positive physical hardware evidence.
	ComputeTypeBareMetal ComputeType = "BARE_METAL"
	// ComputeTypeVM indicates virtualization evidence.
	ComputeTypeVM ComputeType = "VM"
	// ComputeTypeUnknown means compute evidence is absent or conflicting.
	ComputeTypeUnknown ComputeType = "UNKNOWN"
)

// Orchestrator describes the orchestration environment around the Agent.
type Orchestrator string

const (
	// OrchestratorNone means a supported detector found no orchestrator marker.
	OrchestratorNone Orchestrator = "NONE"
	// OrchestratorKubernetes means Kubernetes environment evidence was observed.
	OrchestratorKubernetes Orchestrator = "KUBERNETES"
	// OrchestratorUnknown means orchestration evidence is unavailable or ambiguous.
	OrchestratorUnknown Orchestrator = "UNKNOWN"
)

// DeploymentScope describes what the Agent installation represents.
type DeploymentScope string

const (
	// DeploymentScopeHostService identifies an Agent installed as a host service.
	DeploymentScopeHostService DeploymentScope = "HOST_SERVICE"
	// DeploymentScopeK8sNodeAgent identifies a verified Kubernetes node Agent.
	DeploymentScopeK8sNodeAgent DeploymentScope = "K8S_NODE_AGENT"
	// DeploymentScopeK8sPod identifies an Agent running inside a Kubernetes Pod.
	DeploymentScopeK8sPod DeploymentScope = "K8S_POD"
	// DeploymentScopeUnknown means deployment scope cannot be safely determined.
	DeploymentScopeUnknown DeploymentScope = "UNKNOWN"
)

// EvidenceOutcome records whether a detector observed, missed, or conflicted on evidence.
type EvidenceOutcome string

const (
	// EvidenceOutcomeObserved means a detector returned a usable observation.
	EvidenceOutcomeObserved EvidenceOutcome = "OBSERVED"
	// EvidenceOutcomeUnavailable means a source could not be read or was absent.
	EvidenceOutcomeUnavailable EvidenceOutcome = "UNAVAILABLE"
	// EvidenceOutcomeConflict means sources imply incompatible classifications.
	EvidenceOutcomeConflict EvidenceOutcome = "CONFLICT"
)

// EvidenceConfidence describes how strongly the source supports its observation.
type EvidenceConfidence string

const (
	// EvidenceConfidenceVerified is reserved for direct, authoritative evidence.
	EvidenceConfidenceVerified EvidenceConfidence = "VERIFIED"
	// EvidenceConfidenceInferred identifies platform hints that may be cloned or spoofed.
	EvidenceConfidenceInferred EvidenceConfidence = "INFERRED"
	// EvidenceConfidenceUnknown indicates that the source cannot establish confidence.
	EvidenceConfidenceUnknown EvidenceConfidence = "UNKNOWN"
)

// EnvironmentEvidence is a local, non-authoritative snapshot of host and deployment evidence.
// It does not identify a business asset or authenticate the Agent.
type EnvironmentEvidence struct {
	AgentID            string            `json:"agent_id,omitempty"`
	OS                 string            `json:"os"`
	OSVersion          string            `json:"os_version,omitempty"`
	Architecture       string            `json:"architecture"`
	Hostname           string            `json:"hostname,omitempty"`
	HostID             string            `json:"host_id,omitempty"`
	HostIDSource       string            `json:"host_id_source,omitempty"`
	HostType           HostType          `json:"host_type"`
	ComputeType        ComputeType       `json:"compute_type"`
	Orchestrator       Orchestrator      `json:"orchestrator"`
	DeploymentScope    DeploymentScope   `json:"deployment_scope"`
	AgentVersion       string            `json:"agent_version,omitempty"`
	CollectorVersion   string            `json:"collector_version,omitempty"`
	SupervisorVersion  string            `json:"supervisor_version,omitempty"`
	ObservedAt         time.Time         `json:"observed_at"`
	Evidence           []EnvironmentFact `json:"evidence"`
	ClassificationNote string            `json:"classification_note,omitempty"`
}

// ManagedRuntimeVersions carries versions observed by the Runtime composition layer.
// The identity package does not discover or manage these binaries.
type ManagedRuntimeVersions struct {
	// Collector is the managed OpenTelemetry Collector version, when available.
	Collector string
	// Supervisor is the managed OpAMP Supervisor version, when available.
	Supervisor string
}

// EnvironmentFact records source provenance without retaining raw machine identifiers.
type EnvironmentFact struct {
	Source     string             `json:"source"`
	Outcome    EvidenceOutcome    `json:"outcome"`
	Confidence EvidenceConfidence `json:"confidence"`
	ObservedAt time.Time          `json:"observed_at"`
	Reason     string             `json:"reason,omitempty"`
}

type platformEnvironmentSignals struct {
	VirtualizationVendor     string
	PhysicalHardwareHint     string
	KubernetesDetected       bool
	KubernetesPodIdentity    bool
	KubernetesDetectionKnown bool
	KubernetesNodeAgent      bool
	ContainerRuntimeDetected bool
}

type environmentObservation struct {
	AgentID                  string
	OS                       string
	OSVersion                string
	Architecture             string
	Hostname                 string
	HostID                   string
	HostIDSource             string
	ObservedAt               time.Time
	VirtualizationVendor     string
	PhysicalHardwareHint     string
	KubernetesDetected       bool
	KubernetesDetectionKnown bool
	KubernetesPodIdentity    bool
	KubernetesNodeAgent      bool
	ContainerRuntimeDetected bool
	AgentVersion             string
	CollectorVersion         string
	SupervisorVersion        string
}

// CollectEnvironmentEvidence returns a best-effort local environment snapshot.
// agentID must be the existing persisted installation UUID; this function does
// not create or persist identity. Missing host evidence does not block startup.
func CollectEnvironmentEvidence(agentID installationid.InstallationID) EnvironmentEvidence {
	return CollectEnvironmentEvidenceWithVersions(agentID, ManagedRuntimeVersions{})
}

// CollectEnvironmentEvidenceWithVersions returns a local environment snapshot with
// managed binary versions supplied by the caller. agentID must be the existing
// persisted installation UUID; this function does not create or persist identity.
func CollectEnvironmentEvidenceWithVersions(agentID installationid.InstallationID, versions ManagedRuntimeVersions) EnvironmentEvidence {
	observedAt := time.Now().UTC()
	hostname, hostnameErr := readHostNameForEvidence()
	rawMachineID, machineErr := readRawMachineID()
	machine := MachineIdentity{HostName: hostname}
	if machineErr == nil {
		machine.HostID, machineErr = machineIDFingerprint(runtime.GOOS, rawMachineID)
	}
	details, detailsErr := collectEnvironmentPlatformDetails()
	result := collectEnvironmentEvidence(agentID.String(), hostname, hostnameErr, machine, machineErr, details, detailsErr, readPlatformEnvironmentSignals(), observedAt)
	result.CollectorVersion = strings.TrimSpace(versions.Collector)
	result.SupervisorVersion = strings.TrimSpace(versions.Supervisor)
	return result
}

func collectEnvironmentEvidence(agentID string, hostname string, hostnameErr error, machine MachineIdentity, machineErr error, details platformDetails, detailsErr error, signals platformEnvironmentSignals, observedAt time.Time) EnvironmentEvidence {
	observedAt = observedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	if hostnameErr != nil {
		hostname = ""
	}
	hostname = strings.TrimSpace(hostname)
	if hostname == "" && hostnameErr == nil {
		hostnameErr = fmt.Errorf("hostname is empty")
	}
	hostIDSource := ""
	if machineErr == nil && machine.HostID != "" {
		hostIDSource = machineIDSource(runtime.GOOS)
	}
	version := ""
	if build, ok := debug.ReadBuildInfo(); ok {
		version = strings.TrimSpace(build.Main.Version)
	}
	observation := environmentObservation{
		AgentID:                  strings.TrimSpace(agentID),
		OS:                       runtime.GOOS,
		OSVersion:                details.Version,
		Architecture:             runtime.GOARCH,
		Hostname:                 hostname,
		HostID:                   machine.HostID,
		HostIDSource:             hostIDSource,
		ObservedAt:               observedAt,
		VirtualizationVendor:     signals.VirtualizationVendor,
		PhysicalHardwareHint:     signals.PhysicalHardwareHint,
		KubernetesDetected:       signals.KubernetesDetected,
		KubernetesDetectionKnown: signals.KubernetesDetectionKnown,
		KubernetesPodIdentity:    signals.KubernetesPodIdentity,
		KubernetesNodeAgent:      signals.KubernetesNodeAgent,
		ContainerRuntimeDetected: signals.ContainerRuntimeDetected,
		AgentVersion:             version,
	}
	result := classifyEnvironment(observation)
	if hostnameErr != nil {
		result.Evidence = append(result.Evidence, EnvironmentFact{Source: "os-hostname", Outcome: EvidenceOutcomeUnavailable, Confidence: EvidenceConfidenceUnknown, ObservedAt: observedAt, Reason: "hostname could not be read"})
	} else {
		result.Evidence = append(result.Evidence, EnvironmentFact{Source: "os-hostname", Outcome: EvidenceOutcomeObserved, Confidence: EvidenceConfidenceInferred, ObservedAt: observedAt})
	}
	if detailsErr != nil || strings.TrimSpace(details.Version) == "" {
		result.Evidence = append(result.Evidence, EnvironmentFact{Source: "os-version", Outcome: EvidenceOutcomeUnavailable, Confidence: EvidenceConfidenceUnknown, ObservedAt: observedAt, Reason: "operating system version is unavailable"})
	} else {
		result.Evidence = append(result.Evidence, EnvironmentFact{Source: "os-version", Outcome: EvidenceOutcomeObserved, Confidence: EvidenceConfidenceInferred, ObservedAt: observedAt})
	}
	if machineErr != nil || machine.HostID == "" {
		result.HostID = ""
		result.HostIDSource = ""
		result.Evidence = append(result.Evidence, EnvironmentFact{Source: machineIDSource(runtime.GOOS), Outcome: EvidenceOutcomeUnavailable, Confidence: EvidenceConfidenceUnknown, ObservedAt: observedAt, Reason: "host identifier could not be read"})
	}
	return result
}

func readHostNameForEvidence() (string, error) {
	return os.Hostname()
}

func machineIDSource(operatingSystem string) string {
	switch strings.ToLower(operatingSystem) {
	case "linux":
		return "linux-machine-id"
	case "windows":
		return "windows-machine-guid"
	default:
		return "os-machine-id"
	}
}

func classifyEnvironment(observation environmentObservation) EnvironmentEvidence {
	hostIDAmbiguous := observation.ContainerRuntimeDetected && !observation.KubernetesNodeAgent
	result := EnvironmentEvidence{
		AgentID:           observation.AgentID,
		OS:                observation.OS,
		OSVersion:         observation.OSVersion,
		Architecture:      observation.Architecture,
		Hostname:          observation.Hostname,
		HostID:            observation.HostID,
		HostIDSource:      observation.HostIDSource,
		HostType:          HostTypeUnknown,
		ComputeType:       ComputeTypeUnknown,
		Orchestrator:      OrchestratorUnknown,
		DeploymentScope:   DeploymentScopeUnknown,
		AgentVersion:      observation.AgentVersion,
		CollectorVersion:  observation.CollectorVersion,
		SupervisorVersion: observation.SupervisorVersion,
		ObservedAt:        observation.ObservedAt.UTC(),
	}
	if result.ObservedAt.IsZero() {
		result.ObservedAt = time.Now().UTC()
	}
	if hostIDAmbiguous {
		result.HostID = ""
		result.HostIDSource = ""
		if observation.HostIDSource != "" {
			result.Evidence = append(result.Evidence, EnvironmentFact{Source: observation.HostIDSource, Outcome: EvidenceOutcomeConflict, Confidence: EvidenceConfidenceUnknown, ObservedAt: result.ObservedAt, Reason: "container machine identifier cannot establish whether it identifies the host or container"})
		} else {
			result.Evidence = append(result.Evidence, EnvironmentFact{Source: "host-id", Outcome: EvidenceOutcomeUnavailable, Confidence: EvidenceConfidenceUnknown, ObservedAt: result.ObservedAt, Reason: "container machine identifier has no verified host source"})
		}
	} else if observation.HostIDSource != "" {
		result.Evidence = append(result.Evidence, EnvironmentFact{Source: observation.HostIDSource, Outcome: EvidenceOutcomeObserved, Confidence: EvidenceConfidenceInferred, ObservedAt: result.ObservedAt})
	} else {
		result.Evidence = append(result.Evidence, EnvironmentFact{Source: "host-id", Outcome: EvidenceOutcomeUnavailable, Confidence: EvidenceConfidenceUnknown, ObservedAt: result.ObservedAt, Reason: "stable host identifier unavailable"})
	}
	if observation.ContainerRuntimeDetected {
		result.Evidence = append(result.Evidence, EnvironmentFact{Source: "container-runtime", Outcome: EvidenceOutcomeObserved, Confidence: EvidenceConfidenceInferred, ObservedAt: result.ObservedAt})
	}

	virtualized := observation.VirtualizationVendor != ""
	physical := observation.PhysicalHardwareHint != ""
	switch {
	case virtualized && physical:
		result.Evidence = append(result.Evidence, EnvironmentFact{Source: "compute-classification", Outcome: EvidenceOutcomeConflict, Confidence: EvidenceConfidenceUnknown, ObservedAt: result.ObservedAt, Reason: "virtualization and physical hardware hints conflict"})
		result.ClassificationNote = "compute type is unknown because evidence conflicts"
	case virtualized:
		result.ComputeType = ComputeTypeVM
		result.Evidence = append(result.Evidence, EnvironmentFact{Source: "virtualization:" + observation.VirtualizationVendor, Outcome: EvidenceOutcomeObserved, Confidence: EvidenceConfidenceInferred, ObservedAt: result.ObservedAt})
	case physical:
		result.ComputeType = ComputeTypeBareMetal
		result.Evidence = append(result.Evidence, EnvironmentFact{Source: "hardware:" + observation.PhysicalHardwareHint, Outcome: EvidenceOutcomeObserved, Confidence: EvidenceConfidenceInferred, ObservedAt: result.ObservedAt})
	default:
		result.Evidence = append(result.Evidence, EnvironmentFact{Source: "compute-classification", Outcome: EvidenceOutcomeUnavailable, Confidence: EvidenceConfidenceUnknown, ObservedAt: result.ObservedAt, Reason: "no reliable compute type evidence"})
	}

	switch {
	case observation.KubernetesDetected:
		result.Orchestrator = OrchestratorKubernetes
		source := "kubernetes-environment"
		if observation.KubernetesNodeAgent {
			source = "deployment-scope-setting"
		}
		result.Evidence = append(result.Evidence, EnvironmentFact{Source: source, Outcome: EvidenceOutcomeObserved, Confidence: EvidenceConfidenceInferred, ObservedAt: result.ObservedAt})
		if observation.KubernetesNodeAgent {
			result.DeploymentScope = DeploymentScopeK8sNodeAgent
			result.HostType = HostTypeKubernetes
		} else if observation.KubernetesPodIdentity {
			result.DeploymentScope = DeploymentScopeK8sPod
		}
	case observation.KubernetesDetectionKnown:
		result.Orchestrator = OrchestratorNone
		result.Evidence = append(result.Evidence, EnvironmentFact{Source: "orchestrator-detection", Outcome: EvidenceOutcomeObserved, Confidence: EvidenceConfidenceInferred, ObservedAt: result.ObservedAt, Reason: "explicit environment setting reports no orchestrator"})
		if !observation.ContainerRuntimeDetected {
			result.DeploymentScope = DeploymentScopeHostService
			result.HostType = hostTypeForCompute(result.ComputeType)
		}
	default:
		result.Evidence = append(result.Evidence, EnvironmentFact{Source: "orchestrator-detection", Outcome: EvidenceOutcomeUnavailable, Confidence: EvidenceConfidenceUnknown, ObservedAt: result.ObservedAt, Reason: "orchestrator detection is incomplete"})
	}
	if observation.ContainerRuntimeDetected && result.DeploymentScope == DeploymentScopeHostService {
		result.DeploymentScope = DeploymentScopeUnknown
		result.HostType = HostTypeUnknown
		result.ClassificationNote = "container context prevents asserting host service identity"
	}
	if hostIDAmbiguous {
		result.ClassificationNote = "container context prevents asserting host identity from the machine identifier"
	}
	return result
}

func hostTypeForCompute(computeType ComputeType) HostType {
	switch computeType {
	case ComputeTypeBareMetal:
		return HostTypeBareMetal
	case ComputeTypeVM:
		return HostTypeVM
	default:
		return HostTypeUnknown
	}
}
