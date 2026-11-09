package main

import (
	"strings"
	"testing"
)

func TestParseProposalInputsRejectsSecretSettings(t *testing.T) {
	if _, err := parseProposalInputs("endpoint=127.0.0.1:6379"); err != nil {
		t.Fatalf("non-secret input rejected: %v", err)
	}
	if _, err := parseProposalInputs("api_key=plaintext"); err == nil || strings.Contains(err.Error(), "plaintext") {
		t.Fatalf("secret input error=%v", err)
	}
}
