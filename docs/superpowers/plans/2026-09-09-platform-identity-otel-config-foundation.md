# Platform Identity and OTel Configuration Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Establish deterministic platform identity discovery and validated layered OpenTelemetry Collector configuration for the future bootstrap and runtime agent.

**Architecture:** OpenTelemetry Collector remains the telemetry data plane. This phase does not install, start, or supervise the Collector; it builds the identity and configuration foundation that future `bootstrap` and `supervisor` packages consume. Go discovers platform identity, selects and renders configuration layers, validates agent-owned configuration rules, and returns rendered Collector YAML. Collector-specific semantic validation remains the responsibility of the pinned `otelcol-contrib` binary in a later bootstrap phase.

**Tech Stack:** Go, `gopkg.in/yaml.v3`, Go `testing` package, OpenTelemetry Collector YAML configuration.

**Spec:** `AGENTS.md` and `docs/ARCHITECTURE.md`.

## Global Constraints

* Keep `cmd/agent` composition-only; do not put platform checks, shell commands, transport details, or business logic in `main.go`.

* Keep the process split: Go manages lifecycle and unsupported integrations; the Collector owns receivers, resource detection, processing, batching, retry, persistent delivery queue, and exporters.

* This phase does not download/install/start `otelcol-contrib`, register a service, call a gateway, enroll a node, or issue certificates.

* Preserve the identity hierarchy:

  ```text
  physical_host_id -> compute_node_id -> workload_id -> container_id
  ```

  Container IDs are never the sole historical workload identity.

* Merge configuration layers in this exact order:

  ```text
  base -> profile -> OS -> vendor -> local machine override
  ```

* Initial implementation supports only the first three layers: `base`, `profile`, and `OS`. Vendor and local-machine layers are deferred, but the renderer API must allow explicit future layers.

* Later scalar values replace earlier scalar values.

* Nested maps merge recursively.

* Lists replace the complete earlier list. A layer that overrides an OTel list must declare the complete intended final list; the renderer never appends, deduplicates, or infers list members.

* Do not represent the entire OTel configuration schema as Go structs. Go validates layer mechanics and required document structure only; `otelcol-contrib validate` performs component-level validation later.

* Keep secrets, private keys, tokens, and machine-local endpoint values out of committed configuration.

* Return contextual errors. Unsupported platforms, malformed YAML, missing layers, unresolved required values, and structurally invalid merged configuration must be visible.

* Initial supported platforms are Windows and Linux on `amd64`. Do not add speculative ARM64, macOS, vendor, or cloud abstractions.

---

## Scope

### In Scope

* Platform identity contract and Windows/Linux metadata implementations.
* Linux `/etc/os-release` parsing without shell commands.
* Best-effort Windows metadata using standard library/runtime information only.
* Deterministic layered YAML loading and rendering.
* Explicit scalar/map/list merge behavior.
* Structural validation of the rendered OTel document.
* Minimal base, laptop, Windows, and Linux Collector configuration fragments.
* Pure unit and fixture tests without a live Collector, gateway, privileged OS access, or installed Windows exporter.

### Out of Scope

* Collector download/install, service registration, restart supervision, health endpoint handling, upgrade, rollback, or uninstall.
* Invoking `otelcol-contrib validate`; this belongs to the future bootstrap phase after the pinned Collector binary is available.
* Telemetry transport, gateway connectivity, registration, certificate issuance, or mTLS.
* Docker, process, Windows service, hardware, LibreHardwareMonitor, or vendor-specific adapters.
* Vendor profiles, local machine override files, remote configuration, secrets management, disk spool, and ARM64 support.

---

## File Map

```text
go.mod

internal/
  identity/
    platform.go
    platform_linux.go
    platform_windows.go
    platform_test.go

  config/
    types.go
    merge.go
    renderer.go
    validator.go
    renderer_test.go

configs/
  base/
    otel.yaml
  profiles/
    laptop.yaml
  os/
    windows/
      otel.yaml
    linux/
      otel.yaml

tests/
  fixtures/
    os-release/
      normal
      quoted-values
      malformed
      duplicate-keys
    config/
      invalid.yaml
      malformed.yaml

docs/
  ARCHITECTURE.md
  README.md
  superpowers/
    plans/
      2026-09-10-platform-identity-otel-config-foundation.md
```

Do not create `bootstrap/`, `supervisor/`, `adapters/`, `otlp/`, or `spool/` in this plan. Their boundaries are documented but their behavior belongs to later plans.

---

## Public Contracts

### Platform identity

```go
package identity

type PlatformInfo struct {
    OS           string `json:"os"`
    Architecture string `json:"architecture"`
    Hostname     string `json:"hostname,omitempty"`
    Kernel       string `json:"kernel,omitempty"`
    Release      string `json:"release,omitempty"`
    Distribution string `json:"distribution,omitempty"`
    Version      string `json:"version,omitempty"`
}

func CollectPlatformInfo() (PlatformInfo, error)

func (p PlatformInfo) AsJSON() ([]byte, error)
```

Rules:

* `OS` and `Architecture` always come from the Go runtime.
* `Hostname` is collected through `os.Hostname`; a failure returns a contextual error.
* Linux-specific metadata is best-effort. Missing or malformed `/etc/os-release` does not fail platform collection.
* Windows-specific fields unavailable through portable standard-library APIs remain empty; this is not an error.
* Public JSON uses this fixed struct shape. Do not expose a generic metadata map in this phase.

### Configuration rendering

```go
package config

type Layer struct {
    Name string
    Path string
}

type RenderInput struct {
    Platform identity.PlatformInfo
    Layers   []Layer
    Values   map[string]string
}

func Render(input RenderInput) ([]byte, error)
```

Rules:

* `Layers` are supplied in already-selected order.
* `Render` loads, parses, merges, substitutes approved values, validates structure, and returns YAML bytes.
* `Render` does not create files, start processes, invoke shell commands, or call the Collector.
* `Values` supports only explicitly approved non-secret values. This phase does not substitute certificate paths, tokens, keys, or real gateway endpoints.

### Go-level validation

```go
package config

func ValidateDocument(document map[string]any) error
```

The function validates only:

```text
- receivers exists and is a map
- processors exists and is a map
- exporters exists and is a map
- service exists and is a map
- service.pipelines exists and is a non-empty map
- each pipeline is a map with non-empty receiver and exporter lists
```

It does not validate Collector component names, component settings, TLS settings, or exporter endpoint reachability.

---

## Task 1: Establish the Go module and YAML dependency

**Files:**

* Modify: `go.mod`
* Create: `go.sum`

**Interfaces:**

* Adds `gopkg.in/yaml.v3` as the sole external dependency required by this plan.

* No production package is created in this task.

* [ ] Add the YAML dependency:

  ```bash
  go get gopkg.in/yaml.v3
  ```

* [ ] Confirm the module records an explicit dependency version:

  ```bash
  go list -m gopkg.in/yaml.v3
  ```

* [ ] Confirm the repository still builds before feature code is added:

  ```bash
  go test ./...
  ```

* [ ] Commit:

  ```bash
  git add go.mod go.sum
  git commit -m "chore: add YAML configuration dependency"
  ```

---

## Task 2: Define the platform identity contract with failing tests

**Files:**

* Create: `internal/identity/platform.go`
* Create: `internal/identity/platform_linux.go`
* Create: `internal/identity/platform_windows.go`
* Create: `internal/identity/platform_test.go`
* Create: `tests/fixtures/os-release/normal`
* Create: `tests/fixtures/os-release/quoted-values`
* Create: `tests/fixtures/os-release/malformed`
* Create: `tests/fixtures/os-release/duplicate-keys`

**Interfaces:**

* Produces `PlatformInfo`.

* Produces `CollectPlatformInfo()`.

* Produces `PlatformInfo.AsJSON()`.

* Keeps Linux parsing and OS-specific helpers unexported.

* [ ] Write a failing test that defines the required runtime identity behavior:

  ```go
  func TestCollectPlatformInfoIncludesRuntimeIdentity(t *testing.T) {
      info, err := CollectPlatformInfo()
      if err != nil {
          t.Fatalf("CollectPlatformInfo() error = %v", err)
      }

      if info.OS != runtime.GOOS {
          t.Fatalf("OS = %q, want %q", info.OS, runtime.GOOS)
      }
      if info.Architecture != runtime.GOARCH {
          t.Fatalf("Architecture = %q, want %q", info.Architecture, runtime.GOARCH)
      }
      if info.Hostname == "" {
          t.Fatal("Hostname is empty")
      }
  }
  ```

* [ ] Write a failing test for stable JSON shape:

  ```go
  func TestPlatformInfoAsJSONUsesStableFieldNames(t *testing.T) {
      info := PlatformInfo{
          OS:           "linux",
          Architecture: "amd64",
          Hostname:     "node-1",
          Kernel:       "Linux",
          Release:      "6.0.0",
      }

      payload, err := info.AsJSON()
      if err != nil {
          t.Fatalf("AsJSON() error = %v", err)
      }

      want := `{"os":"linux","architecture":"amd64","hostname":"node-1","kernel":"Linux","release":"6.0.0"}`
      if string(payload) != want {
          t.Fatalf("AsJSON() = %s, want %s", payload, want)
      }
  }
  ```

* [ ] Write table-driven failing tests for `parseOSRelease`:

  ```go
  func TestParseOSRelease(t *testing.T) {
      tests := []struct {
          name             string
          input            string
          wantDistribution string
          wantVersion      string
      }{
          {
              name:             "normal",
              input:            "ID=ubuntu\nVERSION_ID=24.04\n",
              wantDistribution: "ubuntu",
              wantVersion:      "24.04",
          },
          {
              name:             "quoted values",
              input:            "ID=\"ubuntu\"\nVERSION_ID=\"24.04 LTS\"\n",
              wantDistribution: "ubuntu",
              wantVersion:      "24.04 LTS",
          },
          {
              name:             "malformed lines are ignored",
              input:            "ID=fedora\nnot-a-pair\nVERSION_ID=40\n",
              wantDistribution: "fedora",
              wantVersion:      "40",
          },
          {
              name:             "last duplicate key wins",
              input:            "ID=debian\nID=ubuntu\nVERSION_ID=24.04\n",
              wantDistribution: "ubuntu",
              wantVersion:      "24.04",
          },
      }

      for _, tt := range tests {
          t.Run(tt.name, func(t *testing.T) {
              got := parseOSRelease([]byte(tt.input))
              if got.Distribution != tt.wantDistribution {
                  t.Fatalf("Distribution = %q, want %q", got.Distribution, tt.wantDistribution)
              }
              if got.Version != tt.wantVersion {
                  t.Fatalf("Version = %q, want %q", got.Version, tt.wantVersion)
              }
          })
      }
  }
  ```

* [ ] Run the tests and verify they fail because the contract is not implemented:

  ```bash
  go test ./internal/identity -run Test -v
  ```

  Expected: compilation failure for missing `PlatformInfo`, `CollectPlatformInfo`, `AsJSON`, or `parseOSRelease`.

* [ ] Commit the failing tests:

  ```bash
  git add internal/identity tests/fixtures/os-release
  git commit -m "test: define platform identity contract"
  ```

---

## Task 3: Implement platform identity collection

**Files:**

* Modify: `internal/identity/platform.go`
* Modify: `internal/identity/platform_linux.go`
* Modify: `internal/identity/platform_windows.go`
* Modify: `internal/identity/platform_test.go`

**Interfaces:**

* `CollectPlatformInfo()` calls the common runtime collector and the OS-specific detail collector.

* `parseOSRelease([]byte)` is available only to the Linux implementation and package tests.

* Shared consumers receive only `PlatformInfo`.

* [ ] Implement the common contract in `internal/identity/platform.go`:

  ```go
  package identity

  import (
      "encoding/json"
      "fmt"
      "os"
      "runtime"
  )

  type PlatformInfo struct {
      OS           string `json:"os"`
      Architecture string `json:"architecture"`
      Hostname     string `json:"hostname,omitempty"`
      Kernel       string `json:"kernel,omitempty"`
      Release      string `json:"release,omitempty"`
      Distribution string `json:"distribution,omitempty"`
      Version      string `json:"version,omitempty"`
  }

  func CollectPlatformInfo() (PlatformInfo, error) {
      hostname, err := os.Hostname()
      if err != nil {
          return PlatformInfo{}, fmt.Errorf("resolve hostname: %w", err)
      }

      info := PlatformInfo{
          OS:           runtime.GOOS,
          Architecture: runtime.GOARCH,
          Hostname:     hostname,
      }

      details, err := collectPlatformDetails()
      if err != nil {
          return PlatformInfo{}, fmt.Errorf("collect %s platform details: %w", runtime.GOOS, err)
      }

      info.Kernel = details.Kernel
      info.Release = details.Release
      info.Distribution = details.Distribution
      info.Version = details.Version

      return info, nil
  }

  func (p PlatformInfo) AsJSON() ([]byte, error) {
      payload, err := json.Marshal(p)
      if err != nil {
          return nil, fmt.Errorf("marshal platform info: %w", err)
      }
      return payload, nil
  }

  type platformDetails struct {
      Kernel       string
      Release      string
      Distribution string
      Version      string
  }
  ```

* [ ] Implement Linux collection in `internal/identity/platform_linux.go` with a Linux build constraint:

  ```go
  //go:build linux

  package identity

  import (
      "os"
      "strings"
  )

  func collectPlatformDetails() (platformDetails, error) {
      release, err := os.ReadFile("/proc/sys/kernel/osrelease")
      if err != nil {
          return platformDetails{}, err
      }

      details := platformDetails{
          Kernel:  "Linux",
          Release: strings.TrimSpace(string(release)),
      }

      osRelease, err := os.ReadFile("/etc/os-release")
      if err == nil {
          parsed := parseOSRelease(osRelease)
          details.Distribution = parsed.Distribution
          details.Version = parsed.Version
      }

      return details, nil
  }

  type osRelease struct {
      Distribution string
      Version      string
  }

  func parseOSRelease(input []byte) osRelease {
      values := make(map[string]string)

      for _, line := range strings.Split(string(input), "\n") {
          key, value, found := strings.Cut(line, "=")
          if !found || key == "" {
              continue
          }

          value = strings.Trim(strings.TrimSpace(value), `"`)
          values[key] = value
      }

      return osRelease{
          Distribution: values["ID"],
          Version:      values["VERSION_ID"],
      }
  }
  ```

* [ ] Implement Windows best-effort details in `internal/identity/platform_windows.go` with a Windows build constraint:

  ```go
  //go:build windows

  package identity

  import "runtime"

  func collectPlatformDetails() (platformDetails, error) {
      return platformDetails{
          Kernel: runtime.GOOS,
      }, nil
  }
  ```

* [ ] Add a non-Windows/non-Linux fallback implementation if the package must be cross-compiled outside supported targets:

  ```go
  //go:build !linux && !windows

  package identity

  import "fmt"

  func collectPlatformDetails() (platformDetails, error) {
      return platformDetails{}, fmt.Errorf("unsupported platform")
  }
  ```

* [ ] Run formatting and identity tests:

  ```bash
  gofmt -w internal/identity/*.go
  go test ./internal/identity -v
  ```

  Expected: PASS.

* [ ] Cross-compile package checks:

  ```bash
  GOOS=linux GOARCH=amd64 go test ./internal/identity
  GOOS=windows GOARCH=amd64 go test ./internal/identity
  ```

* [ ] Commit:

  ```bash
  git add internal/identity
  git commit -m "feat: collect platform identity"
  ```

---

## Task 4: Define renderer behavior with failing tests

**Files:**

* Create: `internal/config/types.go`
* Create: `internal/config/merge.go`
* Create: `internal/config/renderer.go`
* Create: `internal/config/validator.go`
* Create: `internal/config/renderer_test.go`
* Create: `tests/fixtures/config/malformed.yaml`

**Interfaces:**

```go
type Layer struct {
    Name string
    Path string
}

type RenderInput struct {
    Platform identity.PlatformInfo
    Layers   []Layer
    Values   map[string]string
}

func Render(input RenderInput) ([]byte, error)

func ValidateDocument(document map[string]any) error
```

* [ ] Write a failing precedence test:

  ```go
  func TestRenderLaterScalarOverridesEarlierValue(t *testing.T) {
      rendered, err := Render(RenderInput{
          Layers: []Layer{
              {Name: "base", Path: writeLayer(t, `
  ```

exporters:
otlp/gateway:
endpoint: base.example:4317
`)},
              {Name: "profile", Path: writeLayer(t, `
exporters:
otlp/gateway:
endpoint: profile.example:4317
`)},
},
})
if err != nil {
t.Fatalf("Render() error = %v", err)
}

```
  document := decodeYAML(t, rendered)
  endpoint := nestedString(t, document, "exporters", "otlp/gateway", "endpoint")
  if endpoint != "profile.example:4317" {
      t.Fatalf("endpoint = %q, want profile.example:4317", endpoint)
  }
```

}

````

- [ ] Write a failing nested-map merge test:

```go
func TestRenderMergesNestedMaps(t *testing.T) {
    rendered, err := Render(RenderInput{
        Layers: []Layer{
            {Name: "base", Path: writeLayer(t, `
processors:
resource/laptop:
  attributes:
    asset.type: laptop
`)},
            {Name: "os", Path: writeLayer(t, `
processors:
resource/laptop:
  attributes:
    os.family: linux
`)},
        },
    })
    if err != nil {
        t.Fatalf("Render() error = %v", err)
    }

    document := decodeYAML(t, rendered)
    attributes := nestedMap(t, document, "processors", "resource/laptop", "attributes")

    if attributes["asset.type"] != "laptop" {
        t.Fatalf("asset.type = %v, want laptop", attributes["asset.type"])
    }
    if attributes["os.family"] != "linux" {
        t.Fatalf("os.family = %v, want linux", attributes["os.family"])
    }
}
````

* [ ] Write a failing complete-list replacement test:

  ```go
  func TestRenderReplacesListsAsCompleteOverlays(t *testing.T) {
      rendered, err := Render(RenderInput{
          Layers: []Layer{
              {Name: "base", Path: writeLayer(t, `
  ```

service:
pipelines:
metrics:
receivers: [otlp]
exporters: [otlp/gateway]
`)},
              {Name: "linux", Path: writeLayer(t, `
service:
pipelines:
metrics:
receivers: [otlp, hostmetrics]
`)},
},
})
if err != nil {
t.Fatalf("Render() error = %v", err)
}

```
  document := decodeYAML(t, rendered)
  receivers := nestedStringSlice(t, document, "service", "pipelines", "metrics", "receivers")

  want := []string{"otlp", "hostmetrics"}
  if !slices.Equal(receivers, want) {
      t.Fatalf("receivers = %v, want %v", receivers, want)
  }
```

}

````

- [ ] Write failing tests for malformed YAML, missing file, and structurally invalid final document:

```go
func TestRenderReturnsPathAwareErrorForMissingLayer(t *testing.T) {
    _, err := Render(RenderInput{
        Layers: []Layer{{Name: "base", Path: "testdata/does-not-exist.yaml"}},
    })
    if err == nil || !strings.Contains(err.Error(), "base") {
        t.Fatalf("Render() error = %v, want layer name in error", err)
    }
}

func TestValidateDocumentRejectsMissingPipelines(t *testing.T) {
    err := ValidateDocument(map[string]any{
        "receivers":  map[string]any{},
        "processors": map[string]any{},
        "exporters":  map[string]any{},
        "service":    map[string]any{},
    })
    if err == nil || !strings.Contains(err.Error(), "service.pipelines") {
        t.Fatalf("ValidateDocument() error = %v, want service.pipelines error", err)
    }
}
````

* [ ] Run tests and verify they fail because renderer behavior is absent:

  ```bash
  go test ./internal/config -run Test -v
  ```

* [ ] Commit:

  ```bash
  git add internal/config tests/fixtures/config
  git commit -m "test: define layered Collector config rendering"
  ```

---

## Task 5: Implement deterministic rendering and structural validation

**Files:**

* Modify: `internal/config/types.go`
* Modify: `internal/config/merge.go`
* Modify: `internal/config/renderer.go`
* Modify: `internal/config/validator.go`
* Modify: `internal/config/renderer_test.go`

**Interfaces:**

* `Render` returns only validated YAML bytes.

* The internal merge representation is `map[string]any`.

* The implementation does not expose the intermediate map outside `internal/config`.

* [ ] Implement YAML layer loading with bounded reads:

  ```go
  const maxLayerBytes = 1 << 20

  func loadLayer(layer Layer) (map[string]any, error) {
      file, err := os.Open(layer.Path)
      if err != nil {
          return nil, fmt.Errorf("open %s layer %q: %w", layer.Name, layer.Path, err)
      }
      defer file.Close()

      reader := io.LimitReader(file, maxLayerBytes+1)
      content, err := io.ReadAll(reader)
      if err != nil {
          return nil, fmt.Errorf("read %s layer %q: %w", layer.Name, layer.Path, err)
      }
      if len(content) > maxLayerBytes {
          return nil, fmt.Errorf("read %s layer %q: exceeds %d bytes", layer.Name, layer.Path, maxLayerBytes)
      }

      var document map[string]any
      if err := yaml.Unmarshal(content, &document); err != nil {
          return nil, fmt.Errorf("parse %s layer %q: %w", layer.Name, layer.Path, err)
      }

      return document, nil
  }
  ```

* [ ] Implement explicit recursive merge behavior:

  ```go
  func mergeDocument(base, overlay map[string]any) map[string]any {
      result := maps.Clone(base)

      for key, overlayValue := range overlay {
          baseValue, exists := result[key]
          if !exists {
              result[key] = overlayValue
              continue
          }

          baseMap, baseIsMap := baseValue.(map[string]any)
          overlayMap, overlayIsMap := overlayValue.(map[string]any)
          if baseIsMap && overlayIsMap {
              result[key] = mergeDocument(baseMap, overlayMap)
              continue
          }

          result[key] = overlayValue
      }

      return result
  }
  ```

  Lists are not maps and therefore replace the complete earlier list.

* [ ] Implement `ValidateDocument` with field-specific errors:

  ```go
  func ValidateDocument(document map[string]any) error {
      for _, key := range []string{"receivers", "processors", "exporters"} {
          value, ok := document[key].(map[string]any)
          if !ok || len(value) == 0 {
              return fmt.Errorf("%s must be a non-empty map", key)
          }
      }

      service, ok := document["service"].(map[string]any)
      if !ok {
          return fmt.Errorf("service must be a map")
      }

      pipelines, ok := service["pipelines"].(map[string]any)
      if !ok || len(pipelines) == 0 {
          return fmt.Errorf("service.pipelines must be a non-empty map")
      }

      return nil
  }
  ```

* [ ] Implement `Render`:

  ```go
  func Render(input RenderInput) ([]byte, error) {
      if len(input.Layers) == 0 {
          return nil, fmt.Errorf("render configuration: no layers supplied")
      }

      merged := map[string]any{}
      for _, layer := range input.Layers {
          document, err := loadLayer(layer)
          if err != nil {
              return nil, err
          }
          merged = mergeDocument(merged, document)
      }

      if err := ValidateDocument(merged); err != nil {
          return nil, fmt.Errorf("validate rendered configuration: %w", err)
      }

      rendered, err := yaml.Marshal(merged)
      if err != nil {
          return nil, fmt.Errorf("encode rendered configuration: %w", err)
      }

      return rendered, nil
  }
  ```

* [ ] Run formatting and renderer tests:

  ```bash
  gofmt -w internal/config/*.go
  go test ./internal/config -v
  ```

  Expected: PASS.

* [ ] Commit:

  ```bash
  git add internal/config
  git commit -m "feat: render layered Collector configuration"
  ```

---

## Task 6: Add minimal OTel configuration fragments

**Files:**

* Create: `configs/base/otel.yaml`
* Create: `configs/profiles/laptop.yaml`
* Create: `configs/os/windows/otel.yaml`
* Create: `configs/os/linux/otel.yaml`
* Modify: `internal/config/renderer_test.go`

**Interfaces:**

* Fragments are committed input data only.

* The renderer reads and merges fragments; it does not interpret Collector component semantics beyond structural validation.

* The future bootstrap phase invokes `otelcol-contrib validate` against the rendered document.

* [ ] Create the base fragment:

  ```yaml
  extensions:
    health_check:
      endpoint: 127.0.0.1:13133

  receivers:
    otlp:
      protocols:
        grpc:
          endpoint: 127.0.0.1:4317
        http:
          endpoint: 127.0.0.1:4318

  processors:
    memory_limiter:
      check_interval: 5s
      limit_mib: 256
      spike_limit_mib: 64
    batch:
      timeout: 5s

  exporters:
    otlp/gateway:
      endpoint: ${env:OTEL_GATEWAY_ENDPOINT}

  service:
    extensions: [health_check]
    pipelines:
      metrics:
        receivers: [otlp]
        processors: [memory_limiter, batch]
        exporters: [otlp/gateway]
      logs:
        receivers: [otlp]
        processors: [memory_limiter, batch]
        exporters: [otlp/gateway]
      traces:
        receivers: [otlp]
        processors: [memory_limiter, batch]
        exporters: [otlp/gateway]
  ```

* [ ] Create the laptop profile:

  ```yaml
  processors:
    resource/laptop:
      attributes:
        - key: asset.type
          value: laptop
          action: upsert
        - key: deployment.environment
          value: homelab
          action: upsert

  service:
    pipelines:
      metrics:
        processors: [memory_limiter, resource/laptop, batch]
      logs:
        processors: [memory_limiter, resource/laptop, batch]
      traces:
        processors: [memory_limiter, resource/laptop, batch]
  ```

* [ ] Create the Windows fragment. It uses Windows exporter as the canonical host-metric source and declares the complete final metrics receiver list:

  ```yaml
  receivers:
    prometheus/windows_exporter:
      config:
        scrape_configs:
          - job_name: windows_exporter
            scrape_interval: 15s
            static_configs:
              - targets: [127.0.0.1:9182]

  service:
    pipelines:
      metrics:
        receivers: [otlp, prometheus/windows_exporter]
  ```

* [ ] Create the Linux fragment. It uses `hostmetrics` as the canonical host-metric source and declares the complete final metrics receiver list:

  ```yaml
  receivers:
    hostmetrics:
      collection_interval: 15s
      scrapers:
        cpu:
        memory:
        disk:
        filesystem:
        network:
        paging:
        system:

  service:
    pipelines:
      metrics:
        receivers: [otlp, hostmetrics]
  ```

* [ ] Add tests that render base + laptop + Windows and base + laptop + Linux:

  ```go
  func TestRenderWindowsProfileContainsWindowsExporter(t *testing.T) {
      rendered := renderRepositoryLayers(t, "windows")
      document := decodeYAML(t, rendered)

      receivers := nestedMap(t, document, "receivers")
      if _, ok := receivers["prometheus/windows_exporter"]; !ok {
          t.Fatal("rendered Windows config is missing prometheus/windows_exporter")
      }

      metricsReceivers := nestedStringSlice(t, document, "service", "pipelines", "metrics", "receivers")
      want := []string{"otlp", "prometheus/windows_exporter"}
      if !slices.Equal(metricsReceivers, want) {
          t.Fatalf("metrics receivers = %v, want %v", metricsReceivers, want)
      }
  }

  func TestRenderLinuxProfileContainsHostMetrics(t *testing.T) {
      rendered := renderRepositoryLayers(t, "linux")
      document := decodeYAML(t, rendered)

      receivers := nestedMap(t, document, "receivers")
      if _, ok := receivers["hostmetrics"]; !ok {
          t.Fatal("rendered Linux config is missing hostmetrics")
      }

      metricsReceivers := nestedStringSlice(t, document, "service", "pipelines", "metrics", "receivers")
      want := []string{"otlp", "hostmetrics"}
      if !slices.Equal(metricsReceivers, want) {
          t.Fatalf("metrics receivers = %v, want %v", metricsReceivers, want)
      }
  }
  ```

* [ ] Confirm configuration does not contain real credentials, private paths, tokens, or static gateway addresses:

  ```bash
  rg -n "BEGIN (RSA |EC )?PRIVATE KEY|api[_-]?key|token|password" configs/
  ```

  Expected: no matches.

* [ ] Run config tests:

  ```bash
  go test ./internal/config -v
  ```

* [ ] Commit:

  ```bash
  git add configs internal/config/renderer_test.go
  git commit -m "feat: add initial Collector configuration fragments"
  ```

---

## Task 7: Document the foundation and perform final verification

**Files:**

* Modify: `docs/ARCHITECTURE.md`

* Modify: `docs/README.md`

* Modify: `docs/superpowers/plans/2026-09-10-platform-identity-otel-config-foundation.md`

* [ ] Add a short Phase 1 implementation note to `docs/ARCHITECTURE.md`:

  ```md
  ## Phase 1 implementation boundary

  Phase 1 provides platform identity and deterministic Collector configuration
  rendering only. It does not install or run the Collector. Collector-specific
  semantic validation through `otelcol-contrib validate` begins in the bootstrap
  and lifecycle phase.
  ```

* [ ] Add local development commands to `docs/README.md`:

  ````md
  ## Foundation checks

  ```bash
  go test ./...
  go vet ./...
  go build ./...
  ````

  ```
  ```

* [ ] Run final verification:

  ```bash
  gofmt -w internal/identity/*.go internal/config/*.go
  go test ./...
  go vet ./...
  go build ./...
  ```

* [ ] Record verification output, platform limitations, and deferred work in the Completion Summary below.

* [ ] Commit:

  ```bash
  git add docs internal
  git commit -m "docs: record platform and config foundation"
  ```

---

## Risks and Decisions

### Risk: OTel configuration components may be unavailable in the pinned distribution

**Mitigation:** This phase validates structure only. The next bootstrap phase installs the pinned `otelcol-contrib` release and runs:

```bash
otelcol-contrib validate --config <rendered-config-path>
```

before any Collector process starts.

### Risk: YAML merge can produce valid YAML with invalid operational intent

**Mitigation:** lists are complete replacement overlays, and tests assert the complete resulting receiver and processor lists for each OS profile.

### Risk: Linux metadata files differ by distribution

**Mitigation:** `/etc/os-release` parsing is best-effort. Missing/malformed content leaves distribution/version empty and does not prevent collection of runtime identity.

### Risk: Windows portable APIs expose limited metadata

**Mitigation:** Windows returns stable runtime/hostname information. Vendor, model, and additional hardware metadata are deferred to later adapters or platform capabilities.

### Decision: YAML dependency

Use `gopkg.in/yaml.v3` for parsing and rendering because Go standard library does not provide YAML support. No additional configuration framework is introduced.

---

## Verification

* [ ] Unit tests:

  ```bash
  go test ./internal/identity ./internal/config -v
  ```

* [ ] Cross-platform compile checks:

  ```bash
  GOOS=linux GOARCH=amd64 go test ./internal/identity
  GOOS=windows GOARCH=amd64 go test ./internal/identity
  ```

* [ ] Structural configuration checks:

  ```bash
  go test ./internal/config -v
  ```

* [ ] Lint/build checks:

  ```bash
  gofmt -w internal/identity/*.go internal/config/*.go
  go vet ./...
  go build ./...
  ```

* [ ] Manual verification:

  ```text
  Inspect rendered Linux and Windows YAML.
  Confirm the metrics receiver list is complete and canonical for each OS.
  Confirm no secrets or static production endpoints appear in committed fragments.
  ```

---

## Completion Summary

* Changed: Plan only; implementation not started.
* Verification: Plan reviewed against `AGENTS.md` and `docs/ARCHITECTURE.md`.
* Known limitations:

  * No live Collector validation occurs in this phase.
  * Windows exporter is referenced as a future local dependency but is not installed or tested here.
  * Registration, mTLS, service lifecycle, Docker, hardware, and vendor behavior are intentionally deferred.
* Follow-up: After approval, execute Tasks 1–7 in order. The next separate implementation plan covers Collector installation, service lifecycle, local health checks, and `otelcol-contrib validate`.