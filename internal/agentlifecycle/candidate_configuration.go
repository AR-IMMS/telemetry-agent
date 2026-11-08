package agentlifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/collectorruntime"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/coreconfig"
)

// CandidateApplyRequest binds an S2 candidate to the desired Integration and
// clean-baseline revisions against which it was compiled.
type CandidateApplyRequest struct {
	DesiredSetRevision  uint64
	ExpectedGeneration  uint64
	BaselineFingerprint string
	Candidate           collectorruntime.Candidate
}

// CandidateActivationOutcome reports the lifecycle stage reached by one
// admitted candidate. Activated is distinct from runtime-applied readiness.
type CandidateActivationOutcome struct {
	Generation            uint64
	CandidateDigest       string
	EffectiveConfigDigest string
	Stage                 string
	Result                string
	RecoveryResult        string
	AlreadyApplied        bool
}

// ApplyCollectorCandidate submits a complete S2 candidate to the existing M2
// coordinator. Slice 1 deliberately does not wire production reconciliation.
func ApplyCollectorCandidate(
	ctx context.Context,
	store agentstate.Repository,
	request CandidateApplyRequest,
	options CollectorConfigurationOptions,
) (CandidateActivationOutcome, error) {
	if ctx == nil {
		return CandidateActivationOutcome{}, fmt.Errorf("candidate activation context is required")
	}
	if store == nil || request.Candidate.Digest() == "" || request.BaselineFingerprint == "" {
		return CandidateActivationOutcome{}, fmt.Errorf("candidate, baseline fingerprint, and Agent state repository are required")
	}
	if options.Runner == nil || options.ConfigPath == "" || options.BinaryPath == "" {
		return CandidateActivationOutcome{}, fmt.Errorf("Collector runner, binary path, and configuration path are required")
	}
	repository, ok := store.(candidateLifecycleRepository)
	if !ok {
		return CandidateActivationOutcome{}, fmt.Errorf("SQLite candidate lifecycle repository is required")
	}
	coordinator := configurationCoordinator{
		store:    store,
		lockPath: options.ConfigPath + ".lifecycle",
		prepare: func(state agentstate.State) (string, error) {
			if state.DesiredGeneration == ^uint64(0) {
				return "", fmt.Errorf("M2 desired generation overflow")
			}
			return bootstrap.PrepareConfigRecoveryArtifact(options.ConfigPath, state.DesiredGeneration+1)
		},
		recover: func(path string) error { return bootstrap.RestoreConfigArtifact(options.ConfigPath, path) },
	}
	build := func(state agentstate.State) (candidateActivationPlan, error) {
		values, validationEnvironment, err := candidateGatewayValues(options)
		if err != nil {
			return candidateActivationPlan{}, err
		}
		for _, layer := range options.Layers {
			if filepath.Clean(layer.Path) == filepath.Clean(options.ConfigPath) {
				return candidateActivationPlan{}, fmt.Errorf("active Collector configuration cannot be used as a clean baseline layer")
			}
		}
		fresh, err := BuildCleanBaselineSnapshot(options.Platform, options.Catalog, options.Layers, state, values)
		if err != nil {
			return candidateActivationPlan{}, fmt.Errorf("refresh clean Collector baseline: %w", err)
		}
		if fresh.Fingerprint != request.BaselineFingerprint || fresh.DesiredGeneration != request.ExpectedGeneration {
			if fresh.Fingerprint != request.BaselineFingerprint {
				return candidateActivationPlan{}, agentstate.ErrStaleBaseline
			}
			return candidateActivationPlan{}, agentstate.ErrStaleM2Generation
		}
		if request.Candidate.Platform() != options.Platform {
			return candidateActivationPlan{}, fmt.Errorf("candidate platform does not match current baseline platform")
		}
		content := request.Candidate.Bytes()
		if len(content) == 0 || !candidateDigestMatches(content, request.Candidate.Digest()) {
			return candidateActivationPlan{}, fmt.Errorf("candidate bytes do not match their immutable digest")
		}
		readinessToken, err := config.NewReadinessToken()
		if err != nil {
			return candidateActivationPlan{}, err
		}
		effective, err := config.WithReadinessToken(content, readinessToken)
		if err != nil {
			return candidateActivationPlan{}, fmt.Errorf("apply lifecycle readiness overlay: %w", err)
		}
		if _, err := os.Stat(options.ConfigPath); err != nil && !os.IsNotExist(err) {
			return candidateActivationPlan{}, fmt.Errorf("inspect active Collector configuration: %w", err)
		}
		effectiveIdentity, err := json.Marshal(struct {
			Configuration   []byte `json:"configuration"`
			GatewayEndpoint string `json:"gateway_endpoint"`
		}{effective, environmentValue(validationEnvironment, "OTEL_GATEWAY_ENDPOINT")})
		if err != nil {
			return candidateActivationPlan{}, fmt.Errorf("encode effective Collector identity: %w", err)
		}
		effectiveHash := sha256.Sum256(effectiveIdentity)
		return candidateActivationPlan{
			Mapping: agentstate.CandidateMapping{
				DesiredSetRevision:    request.DesiredSetRevision,
				BaselineFingerprint:   request.BaselineFingerprint,
				CandidateDigest:       request.Candidate.Digest(),
				EffectiveConfigDigest: hex.EncodeToString(effectiveHash[:]),
				CollectorVersion:      request.Candidate.CollectorVersion(),
				ManifestVersion:       request.Candidate.ManifestVersion(),
				Platform:              options.Platform.OS + "/" + options.Platform.Architecture,
			},
			Content:     effective,
			Environment: validationEnvironment,
		}, nil
	}
	activate := func(ctx context.Context, content []byte, environment []string) error {
		return bootstrap.ActivateRenderedConfig(ctx, options.Runner, options.BinaryPath, options.ConfigPath, content, environment)
	}
	return coordinator.ApplyCandidate(ctx, request, build, activate, repository)
}

func candidateDigestMatches(content []byte, expected string) bool {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:]) == expected
}

func candidateGatewayValues(options CollectorConfigurationOptions) (map[string]string, []string, error) {
	environment := append([]string(nil), options.ValidationEnvironment...)
	values := cloneStringMap(options.RenderValues)
	if options.GatewayRepository == nil {
		if endpoint := environmentValue(environment, "OTEL_GATEWAY_ENDPOINT"); endpoint != "" {
			if values == nil {
				values = make(map[string]string, 1)
			}
			values["OTEL_GATEWAY_ENDPOINT"] = endpoint
		}
		return values, environment, nil
	}
	gateway, err := options.GatewayRepository.EffectiveGateway()
	if err != nil {
		return nil, nil, fmt.Errorf("resolve effective Gateway endpoint: %w", err)
	}
	if err := coreconfig.ValidateGatewayEndpoint(gateway.Endpoint); err != nil {
		return nil, nil, fmt.Errorf("validate effective Gateway endpoint: %w", err)
	}
	if values == nil {
		values = make(map[string]string, 1)
	}
	values["OTEL_GATEWAY_ENDPOINT"] = gateway.Endpoint
	return values, replaceEnvironmentValue(environment, "OTEL_GATEWAY_ENDPOINT", gateway.Endpoint), nil
}

func environmentValue(environment []string, key string) string {
	prefix := key + "="
	value := ""
	for _, entry := range environment {
		if len(entry) >= len(prefix) && entry[:len(prefix)] == prefix {
			value = entry[len(prefix):]
		}
	}
	return value
}
