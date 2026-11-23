package opampdomain

// CredentialSource identifies the issuer of the bearer provisioned to the
// OpAMP Supervisor. It prevents a credential from one control plane being
// sent to another endpoint.
type CredentialSource string

const (
	CredentialSourceCore  CredentialSource = "core"
	CredentialSourceFleet CredentialSource = "fleet"
)

// SupervisorContext records the pinned OpAMP Supervisor runtime owned by the
// Agent. Sensitive credential contents are represented only by protected paths.
type SupervisorContext struct {
	BinaryPath        string           `json:"binaryPath,omitempty"`
	ConfigurationPath string           `json:"configurationPath,omitempty"`
	StateDirectory    string           `json:"stateDirectory,omitempty"`
	OpAMPServerURL    string           `json:"opampServerUrl,omitempty"`
	CredentialSource  CredentialSource `json:"credentialSource,omitempty"`
	ServerCAFile      string           `json:"serverCaFile,omitempty"`
	Enabled           bool             `json:"enabled,omitempty"`
}
