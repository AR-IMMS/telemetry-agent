package opampdomain

// SupervisorContext records the pinned OpAMP Supervisor runtime owned by the
// Agent. Sensitive credential contents are represented only by protected paths.
type SupervisorContext struct {
	BinaryPath        string `json:"binaryPath,omitempty"`
	ConfigurationPath string `json:"configurationPath,omitempty"`
	StateDirectory    string `json:"stateDirectory,omitempty"`
	OpAMPServerURL    string `json:"opampServerUrl,omitempty"`
	ServerCAFile      string `json:"serverCaFile,omitempty"`
	Enabled           bool   `json:"enabled,omitempty"`
}
