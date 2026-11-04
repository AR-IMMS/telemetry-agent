package collectorruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/integration"
	"gopkg.in/yaml.v3"
)

// Candidate is an immutable complete Collector configuration and the digest of its exact bytes.
type Candidate struct {
	content   []byte
	digest    string
	collector string
	manifest  string
	platform  identity.PlatformInfo
}

// Bytes returns a copy of the candidate configuration.
func (c Candidate) Bytes() []byte { return append([]byte(nil), c.content...) }

// Digest returns the SHA-256 digest of the exact candidate bytes.
func (c Candidate) Digest() string { return c.digest }

// CollectorVersion returns the pinned Collector version used by the catalog.
func (c Candidate) CollectorVersion() string { return c.collector }

// ManifestVersion returns the capability manifest version used to compile the candidate.
func (c Candidate) ManifestVersion() string { return c.manifest }

// Platform returns the target platform associated with the candidate.
func (c Candidate) Platform() identity.PlatformInfo { return c.platform }

// Compiler combines existing rendered layers with resolved Integration contributions.
type Compiler struct{ catalog Catalog }

// NewCompiler creates a compiler for the supplied reviewed capability catalog.
func NewCompiler(catalog Catalog) Compiler { return Compiler{catalog: catalog} }

// Compile renders the existing baseline, adds supported intent components deterministically, and returns complete candidate bytes.
func (c Compiler) Compile(input config.RenderInput, resolved []integration.ResolvedInstance) (Candidate, error) {
	baseline, err := config.Render(input)
	if err != nil {
		return Candidate{}, fmt.Errorf("%w: render baseline: %v", ErrCompilation, err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(baseline, &doc); err != nil {
		return Candidate{}, fmt.Errorf("%w: decode baseline: %v", ErrCompilation, err)
	}
	if doc == nil {
		return Candidate{}, fmt.Errorf("%w: baseline is empty", ErrCompilation)
	}
	components := make([]compiledComponent, 0)
	for _, resolvedInstance := range resolved {
		if err := integration.ValidateInstance(resolvedInstance.Instance); err != nil {
			return Candidate{}, fmt.Errorf("%w: %v", ErrInvalidIntent, err)
		}
		if !resolvedInstance.Instance.Enabled {
			continue
		}
		for _, intent := range resolvedInstance.Contribution.Intents() {
			capability, err := c.catalog.lookup(intent.Capability(), input.Platform.OS)
			if err != nil {
				return Candidate{}, err
			}
			var component map[string]any
			if err := json.Unmarshal(intent.Configuration(), &component); err != nil || component == nil {
				return Candidate{}, fmt.Errorf("%w: capability %s requires object configuration", ErrInvalidIntent, intent.Capability())
			}
			nativeID := capability.nativeName + "/i-" + stableInstanceSuffix(resolvedInstance.Instance.ID)
			components = append(components, compiledComponent{capability, nativeID, component})
		}
	}
	sort.Slice(components, func(i, j int) bool {
		if components[i].capability.id != components[j].capability.id {
			return components[i].capability.id < components[j].capability.id
		}
		return components[i].id < components[j].id
	})
	for _, component := range components {
		if err := mergeComponent(doc, component); err != nil {
			return Candidate{}, err
		}
	}
	encoded, err := config.RenderDocument(doc)
	if err != nil {
		return Candidate{}, fmt.Errorf("%w: render final candidate: %v", ErrCompilation, err)
	}
	digest := sha256.Sum256(encoded)
	return Candidate{content: append([]byte(nil), encoded...), digest: hex.EncodeToString(digest[:]), collector: c.catalog.version, manifest: c.catalog.manifestVersion, platform: input.Platform}, nil
}

type compiledComponent struct {
	capability capability
	id         string
	config     map[string]any
}

func mergeComponent(doc map[string]any, component compiledComponent) error {
	if component.capability.kind == kindProcessor || component.capability.kind == kindConnector {
		return fmt.Errorf("%w: %s wiring requires an explicit ordering or paired-role rule", ErrUnsupportedCapability, component.capability.kind)
	}
	section := map[componentKind]string{kindReceiver: "receivers", kindProcessor: "processors", kindExporter: "exporters", kindConnector: "connectors"}[component.capability.kind]
	if section == "" {
		return fmt.Errorf("%w: unknown component kind", ErrCompilation)
	}
	existing, ok := doc[section].(map[string]any)
	if !ok {
		return fmt.Errorf("%w: baseline section %s missing", ErrCompilation, section)
	}
	if _, exists := existing[component.id]; exists {
		return fmt.Errorf("%w: %s", ErrComponentIDConflict, component.id)
	}
	existing[component.id] = component.config
	service, ok := doc["service"].(map[string]any)
	if !ok {
		return fmt.Errorf("%w: service section missing", ErrCompilation)
	}
	pipelines, ok := service["pipelines"].(map[string]any)
	if !ok {
		return fmt.Errorf("%w: pipelines section missing", ErrCompilation)
	}
	for _, signal := range component.capability.signals {
		pipelineName := component.capability.pipeline
		if pipelineName == "" {
			pipelineName = signal
		}
		raw, exists := pipelines[pipelineName]
		if !exists {
			return fmt.Errorf("%w: target pipeline %s missing", ErrCompilation, pipelineName)
		}
		pipeline, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: pipeline %s malformed", ErrCompilation, pipelineName)
		}
		field := map[componentKind]string{kindReceiver: "receivers", kindExporter: "exporters"}[component.capability.kind]
		if field == "" {
			return fmt.Errorf("%w: component kind %s has no pipeline mapping", ErrUnsupportedCapability, component.capability.kind)
		}
		list, ok := pipeline[field].([]any)
		if !ok {
			return fmt.Errorf("%w: pipeline %s.%s malformed", ErrCompilation, pipelineName, field)
		}
		for _, member := range list {
			if _, ok := member.(string); !ok {
				return fmt.Errorf("%w: pipeline %s.%s contains a non-string component reference", ErrCompilation, pipelineName, field)
			}
		}
		list = append(list, component.id)
		sort.Slice(list, func(i, j int) bool { return fmt.Sprint(list[i]) < fmt.Sprint(list[j]) })
		pipeline[field] = dedupe(list)
	}
	return nil
}

func dedupe(values []any) []any {
	out := make([]any, 0, len(values))
	previous := ""
	for _, value := range values {
		current := fmt.Sprint(value)
		if len(out) == 0 || current != previous {
			out = append(out, value)
			previous = current
		}
	}
	return out
}
func stableInstanceSuffix(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])
}
