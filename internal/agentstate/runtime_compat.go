package agentstate

import servicedomain "github.com/ar-imms/telemetry-agent/internal/runtime/service/domain"

// The aliases below preserve source compatibility while remaining production
// consumers migrate to Runtime service domain contracts. Remove them after
// all Runtime dependency/service callers and state fixtures import that owner.
type AgentInstallation = servicedomain.AgentInstallation
type OwnedResource = servicedomain.OwnedResource
type OwnershipRecord = servicedomain.OwnershipRecord
