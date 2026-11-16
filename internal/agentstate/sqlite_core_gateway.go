package agentstate

import coreconfig "github.com/ar-imms/telemetry-agent/internal/configuration/remoteconfig"

// AcceptGatewaySnapshot forwards legacy SQLiteStore callers to the
// Configuration-owned Gateway repository. Retain this adapter while callers
// migrate to NewCoreGatewayStore; it contains no persistence logic.
func (s SQLiteStore) AcceptGatewaySnapshot(snapshot coreconfig.GatewaySnapshot) (coreconfig.Acceptance, error) {
	return newCoreGatewayRepository(s).AcceptGatewaySnapshot(snapshot)
}

// GatewayForGeneration forwards legacy SQLiteStore callers to the
// Configuration-owned Gateway repository.
func (s SQLiteStore) GatewayForGeneration(generation uint64) (coreconfig.RuntimeGateway, error) {
	return newCoreGatewayRepository(s).GatewayForGeneration(generation)
}

// MarkGatewayActivated forwards legacy SQLiteStore callers to the
// Configuration-owned Gateway repository.
func (s SQLiteStore) MarkGatewayActivated(generation uint64) error {
	return newCoreGatewayRepository(s).MarkGatewayActivated(generation)
}

// MarkGatewayApplied forwards legacy SQLiteStore callers to the
// Configuration-owned Gateway repository.
func (s SQLiteStore) MarkGatewayApplied(generation uint64, endpoint string) error {
	return newCoreGatewayRepository(s).MarkGatewayApplied(generation, endpoint)
}

// EffectiveGateway forwards legacy SQLiteStore callers to the
// Configuration-owned Gateway repository.
func (s SQLiteStore) EffectiveGateway() (coreconfig.EffectiveGateway, error) {
	return newCoreGatewayRepository(s).EffectiveGateway()
}
