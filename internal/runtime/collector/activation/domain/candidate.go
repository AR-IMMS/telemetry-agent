package activationdomain

// PendingActivation records the recovery artifact for a candidate that may
// have reached the active config path but is not yet committed in Agent state.
type PendingActivation struct {
	// Generation identifies the desired revision being activated.
	Generation uint64 `json:"generation"`
	// RecoveryPath identifies the config artifact to restore after interruption.
	RecoveryPath string `json:"recoveryPath"`
	// Candidate maps this pending lifecycle generation to its desired target.
	Candidate *CandidateMapping `json:"candidate,omitempty"`
}

// CandidateMapping records configuration identities associated with an
// admitted Runtime Collector candidate.
type CandidateMapping struct {
	DesiredSetRevision    uint64 `json:"desiredSetRevision"`
	BaselineFingerprint   string `json:"baselineFingerprint"`
	CandidateDigest       string `json:"candidateDigest"`
	EffectiveConfigDigest string `json:"effectiveConfigDigest"`
	CollectorVersion      string `json:"collectorVersion"`
	ManifestVersion       string `json:"manifestVersion"`
	Platform              string `json:"platform"`
	Outcome               string `json:"outcome,omitempty"`
	Stage                 string `json:"stage,omitempty"`
	RecoveryResult        string `json:"recoveryResult,omitempty"`
}
