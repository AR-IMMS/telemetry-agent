package agentstate

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

type candidateJournalHistoryEntry struct {
	Outcome        string `json:"outcome"`
	Stage          string `json:"stage,omitempty"`
	RecoveryResult string `json:"recovery_result,omitempty"`
	RecordedAt     string `json:"recorded_at"`
}

func migrateCandidateAttemptHistory(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin candidate attempt history migration: %w", err)
	}
	defer tx.Rollback()
	var applied int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=8`).Scan(&applied); err != nil {
		return fmt.Errorf("check candidate attempt history schema version: %w", err)
	}
	if applied != 0 {
		return tx.Commit()
	}
	if _, err := tx.Exec(`ALTER TABLE activation_journal ADD COLUMN history_json TEXT NOT NULL DEFAULT '[]'`); err != nil {
		return fmt.Errorf("add candidate attempt history to activation journal: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(8,CURRENT_TIMESTAMP)`); err != nil {
		return fmt.Errorf("record candidate attempt history schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit candidate attempt history migration: %w", err)
	}
	return nil
}

func candidateJournalHistory(tx *sql.Tx, generation uint64, mapping CandidateMapping) (string, error) {
	var previous candidateJournalHistoryEntry
	var previousUpdatedAt string
	var historyJSON string
	err := tx.QueryRow(`SELECT outcome,COALESCE(stage,''),COALESCE(recovery_result,''),updated_at,history_json FROM activation_journal WHERE generation=?`, generation).Scan(
		&previous.Outcome,
		&previous.Stage,
		&previous.RecoveryResult,
		&previousUpdatedAt,
		&historyJSON,
	)
	if err == sql.ErrNoRows {
		return "[]", nil
	}
	if err != nil {
		return "", fmt.Errorf("read prior candidate activation evidence: %w", err)
	}
	var history []candidateJournalHistoryEntry
	if err := json.Unmarshal([]byte(historyJSON), &history); err != nil {
		return "", fmt.Errorf("decode prior candidate activation history: %w", err)
	}
	if previous.Outcome != "" && (previous.Outcome != mapping.Outcome || previous.Stage != mapping.Stage || previous.RecoveryResult != mapping.RecoveryResult) {
		previous.RecordedAt = previousUpdatedAt
		history = append(history, previous)
	}
	encoded, err := json.Marshal(history)
	if err != nil {
		return "", fmt.Errorf("encode candidate activation history: %w", err)
	}
	return string(encoded), nil
}
