package opampserver

import (
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/coreapi"
	"github.com/ar-imms/telemetry-agent/internal/coreconfig"
)

// RenderGatewayConfig creates a partial Collector fragment containing only the
// v1 Core-owned OTLP Gateway endpoint. Agent-owned receivers and pipelines are
// intentionally absent.
func RenderGatewayConfig(policy coreapi.DesiredGatewayPolicy) ([]byte, [32]byte, error) {
	if policy.SchemaVersion != "ar-imms/gateway-policy/v1" || policy.Revision < 1 {
		return nil, [32]byte{}, fmt.Errorf("unsupported Gateway policy schema or revision")
	}
	if err := coreconfig.ValidateGatewayEndpoint(policy.GatewayEndpoint); err != nil {
		return nil, [32]byte{}, fmt.Errorf("invalid Gateway endpoint")
	}
	endpoint := strconv.Quote(policy.GatewayEndpoint)
	if strings.Contains(endpoint, "\n") {
		return nil, [32]byte{}, fmt.Errorf("invalid Gateway endpoint encoding")
	}
	body := []byte("exporters:\n  otlp/gateway:\n    endpoint: " + endpoint + "\n")
	return body, sha256.Sum256(body), nil
}
