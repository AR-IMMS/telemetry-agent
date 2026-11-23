package application

import (
	"testing"
	"time"
)

func TestProposalManagerRejectsMissingRepository(t *testing.T) {
	manager := NewProposalManager(nil)
	if err := manager.Refresh(nil, time.Now().UTC()); err == nil {
		t.Fatal("Refresh() with no repository returned nil")
	}
}
