package sqlite

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

// CandidateJournalHistory appends previous terminal evidence when a Runtime
// activation target advances to a different outcome, stage, or recovery result.
func CandidateJournalHistory(tx *sql.Tx, generation uint64, outcome, stage, recovery string) (string, error) {
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
	if previous.Outcome != "" && (previous.Outcome != outcome || previous.Stage != stage || previous.RecoveryResult != recovery) {
		previous.RecordedAt = previousUpdatedAt
		history = append(history, previous)
	}
	encoded, err := json.Marshal(history)
	if err != nil {
		return "", fmt.Errorf("encode candidate activation history: %w", err)
	}
	return string(encoded), nil
}
