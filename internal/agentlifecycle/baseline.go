package agentlifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

// CleanBaselineSnapshot contains the ordered M2 render inputs and their
// freshness identity. It intentionally excludes lifecycle readiness overlays
// and any previously generated Integration candidate.
type CleanBaselineSnapshot struct {
	Input             config.RenderInput
	DesiredGeneration uint64
	Fingerprint       string
}

type baselineLayerFingerprint struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	ContentHash string `json:"content_hash"`
}

type baselineFingerprintInput struct {
	Platform identity.PlatformInfo      `json:"platform"`
	Layers   []baselineLayerFingerprint `json:"layers"`
	Inline   []config.InlineLayer       `json:"inline_layers"`
	Values   map[string]string          `json:"values"`
}

// BuildCleanBaselineSnapshot assembles the current M2 layer and managed
// receiver inputs and fingerprints every deterministic render input.
func BuildCleanBaselineSnapshot(
	platform identity.PlatformInfo,
	catalog dependency.Catalog,
	layers []config.Layer,
	state agentstate.State,
	values map[string]string,
) (CleanBaselineSnapshot, error) {
	return BuildCleanBaselineSnapshotWithAdditionalInputs(platform, catalog, layers, state, values, nil, nil)
}

// BuildCleanBaselineSnapshotWithAdditionalInputs assembles the clean baseline
// with reviewed in-memory layers and receiver names supplied by the composition
// root. Additional inputs are fingerprinted so candidate freshness includes
// native monitoring policy.
func BuildCleanBaselineSnapshotWithAdditionalInputs(
	platform identity.PlatformInfo,
	catalog dependency.Catalog,
	layers []config.Layer,
	state agentstate.State,
	values map[string]string,
	additionalLayers []config.InlineLayer,
	additionalMetricsReceivers []string,
) (CleanBaselineSnapshot, error) {
	if len(layers) == 0 {
		return CleanBaselineSnapshot{}, fmt.Errorf("clean configuration baseline requires at least one layer")
	}
	input, err := cleanBaselineInput(platform, catalog, layers, state, values, additionalLayers, additionalMetricsReceivers)
	if err != nil {
		return CleanBaselineSnapshot{}, err
	}
	fingerprintInput := baselineFingerprintInput{
		Platform: platform,
		Layers:   make([]baselineLayerFingerprint, 0, len(layers)),
		Inline:   input.InlineLayers,
		Values:   input.Values,
	}
	for _, layer := range layers {
		file, err := os.Open(layer.Path)
		if err != nil {
			return CleanBaselineSnapshot{}, fmt.Errorf("open %s baseline layer %q: %w", layer.Name, layer.Path, err)
		}
		content, readErr := io.ReadAll(io.LimitReader(file, maxBaselineLayerBytes+1))
		closeErr := file.Close()
		if readErr != nil {
			return CleanBaselineSnapshot{}, fmt.Errorf("read %s baseline layer %q: %w", layer.Name, layer.Path, readErr)
		}
		if closeErr != nil {
			return CleanBaselineSnapshot{}, fmt.Errorf("close %s baseline layer %q: %w", layer.Name, layer.Path, closeErr)
		}
		if len(content) > maxBaselineLayerBytes {
			return CleanBaselineSnapshot{}, fmt.Errorf("read %s baseline layer %q: exceeds %d bytes", layer.Name, layer.Path, maxBaselineLayerBytes)
		}
		digest := sha256.Sum256(content)
		fingerprintInput.Layers = append(fingerprintInput.Layers, baselineLayerFingerprint{
			Name: layer.Name, Path: layer.Path, ContentHash: hex.EncodeToString(digest[:]),
		})
	}
	encoded, err := json.Marshal(fingerprintInput)
	if err != nil {
		return CleanBaselineSnapshot{}, fmt.Errorf("encode clean baseline fingerprint: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return CleanBaselineSnapshot{
		Input:             input,
		DesiredGeneration: state.DesiredGeneration,
		Fingerprint:       hex.EncodeToString(digest[:]),
	}, nil
}

func cleanBaselineInput(platform identity.PlatformInfo, catalog dependency.Catalog, layers []config.Layer, state agentstate.State, values map[string]string, additionalLayers []config.InlineLayer, additionalMetricsReceivers []string) (config.RenderInput, error) {
	if len(layers) == 0 {
		return config.RenderInput{}, fmt.Errorf("clean configuration baseline requires at least one layer")
	}
	receivers, err := catalog.CollectorReceivers(platform.OS, state)
	if err != nil {
		return config.RenderInput{}, fmt.Errorf("resolve managed dependency receivers: %w", err)
	}
	receivers = append([]string{"otlp", "hostmetrics"}, receivers...)
	receivers = append(receivers, additionalMetricsReceivers...)
	inlineLayers := []config.InlineLayer{config.MetricsReceiverLayer(receivers)}
	inlineLayers = append(inlineLayers, additionalLayers...)
	return config.RenderInput{
		Platform:     platform,
		Layers:       append([]config.Layer(nil), layers...),
		InlineLayers: inlineLayers,
		Values:       cloneStringMap(values),
	}, nil
}

const maxBaselineLayerBytes = 1 << 20

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	copy := make(map[string]string, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return copy
}
