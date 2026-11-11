# Integration Developer Guide

This guide describes the Agent's current static Integration extension contract. It does not describe dynamic loading: a new Definition is compiled into the Agent, while Templates for an already shipped Definition are operator data and can be managed without rebuilding.

## Discover shipped support

List the Definitions registered in this Agent build:

```text
agentctl integration catalog list
agentctl integration catalog list --output json
```

Inspect one exact Definition version:

```text
agentctl integration catalog inspect --id redis --version 1.0.0
agentctl integration catalog inspect --id redis --version 1.0.0 --output json
```

The catalog shows matchability, selector metadata, required capability IDs, Template setting names/types/descriptions/defaults/constraints, and the static native Collector mapping where available. Capability status is assessed against the pinned artifact-bound manifest for the current platform. It does not prove that the Collector process is running or telemetry is arriving; verify activation and delivery separately.

Grafana is **not supported by the current production registry**. This command reports the missing exact Definition rather than inventing a receiver:

```text
agentctl integration catalog inspect --id grafana --version 1.0.0
```

The pinned manifest currently includes `otel.receiver.prometheus.metrics` on supported Linux/Windows amd64 artifacts. That makes Prometheus scraping a possible building block only. It does not establish that a particular Grafana image exposes a reachable, correctly configured `/metrics` endpoint or that its metrics are delivered. No Grafana Definition, settings or live test are shipped.

## Definition contract and registration

The current production registry is static in `cmd/agentctl/integration_registry.go`, where `productionIntegrationRegistry` registers `internal/integration/redis.Definition()`. A new Integration follows the same pattern:

1. Add a package such as `internal/integration/grafana/` with a `Definition() integration.IntegrationDefinition` constructor.
2. Implement `Validate` for the Integration's bounded, secret-free settings. Reject unknown fields and malformed endpoints. Never accept plaintext credentials in settings.
3. Implement `Contribute` as a deterministic function from a validated `IntegrationInstance` to `Contribution` and immutable `ComponentIntent` values. Use runtime-neutral capability IDs; do not emit Collector YAML from Integration code.
4. Add optional `MatchingDefinition` only when the supported resource evidence is sufficient. Use a declarative `Selector` for exact resource evidence or a bounded `Match` function for genuinely Integration-specific checks. `BuildCandidate` must return only non-secret settings/evidence or explicit required inputs; ambiguity must remain unresolved.
5. Declare `TemplateSettingRules` for every setting Templates may provide or bind. The catalog metadata includes type, description, required status, constraints, default and whether overrides/resource bindings are allowed. Add a per-setting `Validate` function when a value has semantic constraints that must be checked before Template persistence. The Definition's `Validate` function remains the final complete-configuration check after bindings resolve.
6. Register the Definition in `cmd/agentctl/integration_registry.go`. `integration.Registry` performs exact ID/version lookup and is the common static source used by CLI and runtime composition.
7. Add focused tests for validation, secret rejection, exact version lookup, deterministic contributions, selector matching/rejection, candidate building and Template setting/binding rules.

The main reference is `internal/integration/redis/definition.go`. Its reusable public contracts are declared in:

- `internal/integration/integration.go`: `IntegrationDefinition`, `IntegrationInstance`, `Contributor`, `ValidateDefinition` and `ValidateInstance`.
- `internal/integration/matching.go`: `MatchingDefinition`, `MatchSelector`, `CandidateBuilder`, evidence and required-input values.
- `internal/integration/template.go`: `TemplateSettingRule`, `IntegrationTemplate`, selector/binding input and `ResolveTemplateSettings`.
- `internal/integration/registry.go`: static exact-version `Registry` and ordered `Definitions` enumeration.

## Collector capability verification

An Integration requests a capability ID such as `otel.receiver.prometheus.metrics`; `internal/collectorruntime/manifest.go` maps that ID to the native component name, kind, signal/pipeline, supported OS, pinned Collector version and exact artifact checksums. `internal/collectorruntime/compiler.go` owns component ID generation and full configuration assembly. Keep Integration Core independent of native names and YAML.

Before claiming an Integration is supported:

1. Confirm the pinned artifact manifest contains the required capability for each claimed platform. Add a manifest mapping only when the exact shipped binary proves it; do not infer support from an upstream component name.
2. Run the manifest-to-binary inventory verification and candidate native validation against the exact pinned artifact. The `OTELCOL_CONTRIB_BINARY` test input is for verification only and does not change the manifest.
3. Exercise the Definition through reconciliation and the existing M2 activation lifecycle, preserving host metrics, Gateway configuration, readiness and rollback.
4. For a product support claim, verify the actual service endpoint and observe the expected signal at the OTLP Gateway. A successful native config check alone proves neither reachability nor delivery.

Relevant verification code lives in `internal/collectorruntime/manifest_test.go`, `internal/collectorruntime/compiler_test.go`, the Integration package tests, and the gated live Template/runtime E2E tests in `cmd/agentctl/`.

## Operator configuration without rebuilding

Once a Definition is registered and shipped, an operator can create or update a local JSON Template with `agentctl integration template --file`, inspect/export the full stored Template with `inspect --output json`, and manage its enabled state through revision-checked CLI commands. See [Monitoring Proposal Operations](../runbooks/monitoring-proposals.md) and [the Redis Template example](../examples/redis-monitoring.json).

Templates can set only fields declared by the Definition. The current schema supports resource kind/lifecycle/exact-attribute selectors, allowlisted attribute or unambiguous endpoint bindings, and Definition-declared settings. The CLI does not accept arbitrary Collector YAML, free-form expressions, Core ownership, or secret values. Creating a new Definition requires a new Agent build; changing a Template for an existing Definition does not.

## Grafana extension walkthrough (not implemented)

For a proposed Grafana Definition, first verify the selected image's actual metrics endpoint, port, authentication needs and exposition format. If it is compatible with the verified Prometheus receiver, add a Definition that validates a non-secret scrape endpoint and interval, contributes `otel.receiver.prometheus.metrics`, and declares explicit template/matching metadata. If authentication is needed, stop until a separately approved secret reference and secure Collector expansion contract exists; do not place credentials in Template settings.

Register the Definition statically, add tests at the Definition/Registry/Template levels, verify the candidate against the pinned native Collector, then run an isolated service-to-Gateway E2E. The existing generic Resolver, Compiler, Reconciliation Controller and M2 Lifecycle should not need Integration-specific branches when the intent and capability contracts suffice. If the required Collector component or signal wiring is absent, fail closed and extend the reviewed artifact manifest only after binary evidence. Grafana monitoring remains unsupported until this full evidence chain is implemented and tested.
