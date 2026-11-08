package agentlifecycle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
)

func TestCandidateRollbackUsesExistingLKGAndRecordsFailure(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "otel.yaml")
	oldConfig := []byte("old-known-good")
	if err := os.WriteFile(configPath, oldConfig, 0o600); err != nil {
		t.Fatal(err)
	}
	recoveryPath, err := bootstrap.PrepareConfigRecoveryArtifact(configPath, 5)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("failed-candidate"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := agentstate.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if _, err := store.Update(func(state *agentstate.State) error {
		state.DesiredGeneration, state.ActivatedGeneration, state.AppliedGeneration = 5, 5, 4
		state.PendingActivation = &agentstate.PendingActivation{Generation: 5, RecoveryPath: recoveryPath, Candidate: &agentstate.CandidateMapping{
			DesiredSetRevision: 12, BaselineFingerprint: "baseline", CandidateDigest: "candidate", EffectiveConfigDigest: "effective",
		}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := rollbackUnappliedConfiguration(context.Background(), store, configPath, 5); err != nil {
		t.Fatalf("rollbackUnappliedConfiguration() error = %v", err)
	}
	active, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(active) != string(oldConfig) {
		t.Fatalf("restored config = %q, want %q", active, oldConfig)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.DesiredGeneration != 5 || state.ActivatedGeneration != 4 || state.AppliedGeneration != 4 || state.PendingActivation != nil {
		t.Fatalf("rollback state = %#v", state)
	}
}

func TestCandidateCrashRecoveryPreservesAppliedGeneration(t *testing.T) {
	for _, activated := range []uint64{4, 5} {
		t.Run(map[uint64]string{4: "before-promotion", 5: "after-promotion"}[activated], func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "otel.yaml")
			oldConfig := []byte("old-known-good")
			if err := os.WriteFile(configPath, oldConfig, 0o600); err != nil {
				t.Fatal(err)
			}
			recoveryPath, err := bootstrap.PrepareConfigRecoveryArtifact(configPath, 5)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(configPath, []byte("pending-candidate"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(configPath+".last-known-good", oldConfig, 0o600); err != nil {
				t.Fatal(err)
			}
			store := agentstate.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
			if _, err := store.Update(func(state *agentstate.State) error {
				state.DesiredGeneration, state.ActivatedGeneration, state.AppliedGeneration = 5, activated, 4
				state.PendingActivation = &agentstate.PendingActivation{Generation: 5, RecoveryPath: recoveryPath, Candidate: &agentstate.CandidateMapping{
					DesiredSetRevision: 12, BaselineFingerprint: "baseline", CandidateDigest: "candidate", EffectiveConfigDigest: "effective",
				}}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			recovery := pendingActivationRecovery{
				store:                store,
				lockPath:             configPath + ".lifecycle",
				recover:              func(path string) error { return bootstrap.RestoreConfigArtifact(configPath, path) },
				restoreLastKnownGood: func(string) error { return bootstrap.RestoreLastKnownGoodConfig(configPath) },
			}
			if err := recovery.Recover(context.Background()); err != nil {
				t.Fatalf("Recover() error = %v", err)
			}
			state, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if state.AppliedGeneration != 4 || state.ActivatedGeneration != 4 || state.PendingActivation != nil || state.DesiredGeneration != 5 {
				t.Fatalf("recovered state = %#v", state)
			}
			active, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(active) != string(oldConfig) {
				t.Fatalf("recovered config = %q, want %q", active, oldConfig)
			}
		})
	}
}
