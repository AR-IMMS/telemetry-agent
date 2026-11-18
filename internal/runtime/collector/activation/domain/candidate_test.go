package activationdomain

import (
	"encoding/json"
	"testing"
)

func TestPendingActivationKeepsPersistedJSONShape(t *testing.T) {
	pending := PendingActivation{
		Generation:   5,
		RecoveryPath: "C:/agent/last-good.yaml",
		Candidate: &CandidateMapping{
			DesiredSetRevision:    4,
			BaselineFingerprint:   "baseline",
			CandidateDigest:       "candidate",
			EffectiveConfigDigest: "effective",
			CollectorVersion:      "0.160.0",
			ManifestVersion:       "v1",
			Platform:              "windows",
		},
	}

	encoded, err := json.Marshal(pending)
	if err != nil {
		t.Fatalf("marshal pending activation: %v", err)
	}

	const want = `{"generation":5,"recoveryPath":"C:/agent/last-good.yaml","candidate":{"desiredSetRevision":4,"baselineFingerprint":"baseline","candidateDigest":"candidate","effectiveConfigDigest":"effective","collectorVersion":"0.160.0","manifestVersion":"v1","platform":"windows"}}`
	if string(encoded) != want {
		t.Fatalf("pending activation JSON = %s, want %s", encoded, want)
	}
}
