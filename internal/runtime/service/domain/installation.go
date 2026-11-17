package domain

// AgentInstallation records the service, binary, and owned resources created
// by one Agent installation so removal never has to infer ownership.
type AgentInstallation struct {
	// Platform identifies the operating system on which this installation runs.
	Platform string `json:"platform"`
	// ServiceName is the OS service registered for the Agent process.
	ServiceName string `json:"serviceName"`
	// BinaryPath is the durable Agent executable path.
	BinaryPath string `json:"binaryPath"`
	// Ownership lists resources this installation may safely remove.
	Ownership OwnershipRecord `json:"ownership"`
}

// OwnedResource identifies one host resource created and managed by the Agent.
type OwnedResource struct {
	Kind       string `json:"kind"`
	Identifier string `json:"identifier"`
}

// OwnershipRecord records resources that an Agent-managed installation owns.
type OwnershipRecord struct {
	Resources []OwnedResource `json:"resources"`
}
