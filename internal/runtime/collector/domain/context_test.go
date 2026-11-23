package collectordomain

import (
	"encoding/json"
	"testing"
)

func TestCollectorContextKeepsPersistedJSONShape(t *testing.T) {
	context := CollectorContext{
		ConfigRoot:      `C:\Agent\config`,
		BinaryPath:      `C:\Agent\otelcol-contrib.exe`,
		ConfigPath:      `C:\Agent\config\collector.yaml`,
		GatewayEndpoint: "gateway.example:4317",
		HealthEndpoint:  "127.0.0.1:13133",
	}

	encoded, err := json.Marshal(context)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	const want = `{"configRoot":"C:\\Agent\\config","binaryPath":"C:\\Agent\\otelcol-contrib.exe","configPath":"C:\\Agent\\config\\collector.yaml","gatewayEndpoint":"gateway.example:4317","healthEndpoint":"127.0.0.1:13133"}`
	if string(encoded) != want {
		t.Fatalf("CollectorContext JSON = %s, want %s", encoded, want)
	}
}
