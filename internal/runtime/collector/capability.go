package collectorruntime

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	// ErrUnsupportedCapability identifies an absent or platform-incompatible capability mapping.
	ErrUnsupportedCapability = errors.New("unsupported Collector capability")
	// ErrInvalidIntent identifies malformed or untrusted component intent data.
	ErrInvalidIntent = errors.New("invalid Collector intent")
	// ErrInvalidCatalog identifies an incomplete, malformed, or conflicting manifest.
	ErrInvalidCatalog = errors.New("invalid Collector capability catalog")
	// ErrComponentIDConflict identifies a stable native component ID collision.
	ErrComponentIDConflict = errors.New("Collector component ID conflict")
	// ErrCompilation identifies baseline or candidate construction failures.
	ErrCompilation = errors.New("Collector configuration compilation failed")
	// ErrNativeValidation identifies a candidate rejected by the pinned Collector.
	ErrNativeValidation = errors.New("Collector native validation failed")
)

type componentKind string

const (
	kindReceiver  componentKind = "receiver"
	kindProcessor componentKind = "processor"
	kindExporter  componentKind = "exporter"
	kindConnector componentKind = "connector"
)

type capability struct {
	id, nativeName string
	kind           componentKind
	os             []string
	signals        []string
	pipeline       string
}

// Catalog is an immutable exact capability lookup for one pinned Collector build.
type Catalog struct {
	version, manifestVersion string
	entries                  map[string]capability
}

// ArtifactVersion returns the pinned Collector version represented by the catalog.
func (c Catalog) ArtifactVersion() string { return c.version }

// ManifestVersion returns the reviewed static manifest version represented by the catalog.
func (c Catalog) ManifestVersion() string { return c.manifestVersion }

// NewCatalog creates a catalog from reviewed manifest entries and rejects duplicates or incomplete wiring.
func NewCatalog(version, manifestVersion string, entries []Capability) (Catalog, error) {
	if strings.TrimSpace(version) == "" || strings.TrimSpace(manifestVersion) == "" {
		return Catalog{}, fmt.Errorf("%w: catalog and manifest versions are required", ErrInvalidCatalog)
	}
	c := Catalog{version: version, manifestVersion: manifestVersion, entries: make(map[string]capability, len(entries))}
	for _, e := range entries {
		if !validCapabilityID(e.ID) || !validNativeName(e.NativeName) || len(e.OS) == 0 || len(e.Signals) != 1 || strings.TrimSpace(e.Pipeline) == "" {
			return Catalog{}, fmt.Errorf("%w: incomplete or ambiguous manifest entry %q", ErrInvalidCatalog, e.ID)
		}
		kind := componentKind(e.Kind)
		if kind != kindReceiver && kind != kindProcessor && kind != kindExporter && kind != kindConnector {
			return Catalog{}, fmt.Errorf("%w: invalid component kind %q for capability %s", ErrInvalidCatalog, e.Kind, e.ID)
		}
		if _, ok := c.entries[e.ID]; ok {
			return Catalog{}, fmt.Errorf("%w: duplicate capability %q", ErrInvalidCatalog, e.ID)
		}
		for _, platform := range e.OS {
			if platform != "linux" && platform != "windows" {
				return Catalog{}, fmt.Errorf("%w: unsupported platform %q for %s", ErrInvalidCatalog, platform, e.ID)
			}
		}
		signal := e.Signals[0]
		if signal != "metrics" && signal != "logs" && signal != "traces" {
			return Catalog{}, fmt.Errorf("%w: invalid signal %q for %s", ErrInvalidCatalog, signal, e.ID)
		}
		if e.Pipeline != signal && !strings.HasPrefix(e.Pipeline, signal+"/") {
			return Catalog{}, fmt.Errorf("%w: pipeline %q does not carry signal %q for %s", ErrInvalidCatalog, e.Pipeline, signal, e.ID)
		}
		c.entries[e.ID] = capability{e.ID, e.NativeName, kind, sortedCopy(e.OS), sortedCopy(e.Signals), e.Pipeline}
	}
	return c, nil
}

func validNativeName(name string) bool {
	if name == "" || strings.TrimSpace(name) != name {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

func validCapabilityID(id string) bool {
	if id == "" || strings.TrimSpace(id) != id {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

// Capability is one reviewed mapping from an Integration capability to a native Collector component.
type Capability struct {
	ID, NativeName string
	Kind           string
	OS, Signals    []string
	Pipeline       string
}

func (c Catalog) lookup(id, osName string) (capability, error) {
	e, ok := c.entries[id]
	if !ok {
		return capability{}, fmt.Errorf("%w: %s", ErrUnsupportedCapability, id)
	}
	for _, supported := range e.os {
		if strings.EqualFold(supported, osName) {
			return e, nil
		}
	}
	return capability{}, fmt.Errorf("%w: %s on %s", ErrUnsupportedCapability, id, osName)
}
func sortedCopy(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

// Manifest returns a defensive, sorted copy of the reviewed static capability manifest.
func (c Catalog) Manifest() []Capability {
	out := make([]Capability, 0, len(c.entries))
	for _, e := range c.entries {
		out = append(out, Capability{e.id, e.nativeName, string(e.kind), append([]string(nil), e.os...), append([]string(nil), e.signals...), e.pipeline})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
