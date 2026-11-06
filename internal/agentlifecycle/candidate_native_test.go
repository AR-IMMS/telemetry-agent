package agentlifecycle

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/supervisor"
)

func TestPinnedCollectorCandidateLifecycleWhenBinaryProvided(t *testing.T) {
	binary := os.Getenv("OTELCOL_CONTRIB_BINARY")
	if binary == "" {
		t.Skip("pinned Collector binary is not available in this test environment")
	}

	store, request, options, activePath, statePath := candidateFixture(t)
	options.BinaryPath = binary
	options.Runner = bootstrap.OSCommandRunner{}
	outcome, err := ApplyCollectorCandidate(context.Background(), store, request, options)
	if err != nil {
		t.Fatalf("apply candidate through M2 lifecycle: %v", err)
	}
	if outcome.Result != "activated" || outcome.Generation == 0 {
		t.Fatalf("candidate activation outcome = %#v", outcome)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	runtimeDone := make(chan error, 1)
	go func() {
		runtimeDone <- (runtimeWatcher{
			store: store,
			run:   supervisor.Run,
			options: supervisor.Options{
				BinaryPath:      binary,
				ConfigPath:      activePath,
				HealthEndpoint:  "http://127.0.0.1:13133",
				GatewayEndpoint: "127.0.0.1:14317",
				StartupTimeout:  30 * time.Second,
				ShutdownTimeout: 10 * time.Second,
			},
			pollInterval:   100 * time.Millisecond,
			lockPath:       activePath + ".lifecycle",
			commitConfig:   bootstrap.CommitLastKnownGoodConfigFrom,
			snapshotConfig: bootstrap.SnapshotRuntimeConfig,
			rollback: func(ctx context.Context, configPath string, generation uint64) error {
				return rollbackUnappliedConfiguration(ctx, store, configPath, generation)
			},
		}).Run(ctx)
	}()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		state, loadErr := store.Load()
		if loadErr != nil {
			t.Fatalf("load candidate lifecycle state: %v", loadErr)
		}
		if state.AppliedGeneration == outcome.Generation {
			break
		}
		select {
		case runtimeErr := <-runtimeDone:
			t.Fatalf("Collector runtime exited before readiness commit: %v", runtimeErr)
		case <-ctx.Done():
			t.Fatalf("candidate generation %d did not become ready: %v", outcome.Generation, ctx.Err())
		case <-ticker.C:
		}
	}

	if _, err := os.Stat(activePath + ".last-known-good"); err != nil {
		t.Fatalf("runtime readiness did not commit last-known-good config: %v", err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.PendingActivation != nil || state.ActivatedGeneration != outcome.Generation {
		t.Fatalf("runtime state after readiness = %#v", state)
	}

	cancel()
	select {
	case runtimeErr := <-runtimeDone:
		if runtimeErr != nil {
			t.Fatalf("stop Collector runtime: %v", runtimeErr)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Collector runtime did not stop after cancellation")
	}

	state, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.AppliedGeneration != outcome.Generation {
		t.Fatalf("AppliedGeneration=%d, want %d", state.AppliedGeneration, outcome.Generation)
	}
	db, err := sql.Open("sqlite", statePath)
	if err != nil {
		t.Fatalf("open lifecycle journal: %v", err)
	}
	defer db.Close()
	var journalOutcome, candidateDigest, effectiveDigest, stage string
	if err := db.QueryRow(`SELECT outcome,candidate_digest,effective_config_digest,stage FROM activation_journal WHERE generation=?`, outcome.Generation).Scan(&journalOutcome, &candidateDigest, &effectiveDigest, &stage); err != nil {
		t.Fatalf("read terminal lifecycle mapping: %v", err)
	}
	if journalOutcome != "applied" || candidateDigest != request.Candidate.Digest() || effectiveDigest != outcome.EffectiveConfigDigest || stage != "readiness" {
		t.Fatalf("terminal mapping = %q/%q/%q/%q", journalOutcome, candidateDigest, effectiveDigest, stage)
	}
}
