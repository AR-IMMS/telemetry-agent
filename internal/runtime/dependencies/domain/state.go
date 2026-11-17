package dependencydomain

// TeardownAction identifies the host-resource operation permitted after the
// Collector has acknowledged a configuration generation.
type TeardownAction string

const (
	// TeardownActionDisable stops a dependency after its receiver is removed.
	TeardownActionDisable TeardownAction = "disable"
	// TeardownActionUninstall removes a dependency after its receiver is removed.
	TeardownActionUninstall TeardownAction = "uninstall"
)

// PendingTeardown delays host-resource changes until the Collector has applied
// the configuration generation that removes its scrape receiver.
type PendingTeardown struct {
	Action     TeardownAction `json:"action"`
	Generation uint64         `json:"generation"`
}

// DependencyState is the desired lifecycle state of one managed dependency.
type DependencyState struct {
	Enabled         bool             `json:"enabled"`
	PendingTeardown *PendingTeardown `json:"pendingTeardown,omitempty"`
}
