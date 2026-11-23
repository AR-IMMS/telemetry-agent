package agentlifecycle

import (
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	coreconfig "github.com/ar-imms/telemetry-agent/internal/configuration/remoteconfig"
	enrollment "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/application"
	fleetenrollment "github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/application"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
)

func testRuntimeStoreFactory(path string) (agentstate.Repository, coreconfig.GatewayRuntimeRepository, enrollment.AttemptRepository, fleetenrollment.AttemptRepository) {
	return sqlitefixture.NewStore(path), sqlitefixture.NewCoreGatewayStore(path), sqlitefixture.NewEnrollmentStore(path), sqlitefixture.NewFleetEnrollmentStore(path)
}
