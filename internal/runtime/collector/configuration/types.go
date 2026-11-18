package collectorconfig

import "github.com/ar-imms/telemetry-agent/internal/runtime/collector/contracts"

// Layer identifies one ordered YAML configuration fragment.
type Layer = contracts.Layer

// InlineLayer identifies an in-memory document merged after file-backed layers.
type InlineLayer = contracts.InlineLayer

// RenderInput is the Runtime collector configuration request contract.
type RenderInput = contracts.RenderInput
