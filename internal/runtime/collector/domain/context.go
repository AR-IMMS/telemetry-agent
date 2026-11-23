package collectordomain

// CollectorContext records the persisted Runtime paths and endpoints needed
// to configure and supervise the local OpenTelemetry Collector.
type CollectorContext struct {
	ConfigRoot      string `json:"configRoot"`
	BinaryPath      string `json:"binaryPath"`
	ConfigPath      string `json:"configPath"`
	GatewayEndpoint string `json:"gatewayEndpoint"`
	HealthEndpoint  string `json:"healthEndpoint"`
}
