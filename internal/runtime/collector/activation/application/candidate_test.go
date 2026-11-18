package application

import (
	"errors"
	"testing"
)

func TestCandidateStalenessErrorsShareSentinel(t *testing.T) {
	for name, check := range map[string]struct {
		err  error
		text string
	}{
		"desired revision":   {ErrStaleDesiredRevision, "candidate is stale: desired Integration revision changed"},
		"runtime generation": {ErrStaleM2Generation, "candidate is stale: M2 desired generation changed"},
		"baseline":           {ErrStaleBaseline, "candidate is stale: clean baseline changed"},
	} {
		t.Run(name, func(t *testing.T) {
			if !errors.Is(check.err, ErrStaleCandidate) {
				t.Fatalf("error %v does not wrap ErrStaleCandidate", check.err)
			}
			if got := check.err.Error(); got != check.text {
				t.Fatalf("error text = %q, want %q", got, check.text)
			}
		})
	}
}
