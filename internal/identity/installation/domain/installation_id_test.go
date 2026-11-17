package domain

import (
	"bytes"
	"testing"
)

func TestInstallationIDRoundTripAndCanonicalization(t *testing.T) {
	id, err := ParseInstallationID("11111111-2222-4333-8444-555555555555")
	if err != nil {
		t.Fatalf("ParseInstallationID() error = %v", err)
	}
	if got, want := id.String(), "11111111-2222-4333-8444-555555555555"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}

	generated, err := NewInstallationID(bytes.NewReader(bytes.Repeat([]byte{0x11}, 16)))
	if err != nil {
		t.Fatalf("NewInstallationID() error = %v", err)
	}
	if parsed, err := ParseInstallationID(generated.String()); err != nil || parsed != generated {
		t.Fatalf("generated ID round trip = %q, %v", generated.String(), err)
	}
}

func TestNewInstallationIDRejectsMissingEntropy(t *testing.T) {
	if _, err := NewInstallationID(nil); err == nil {
		t.Fatal("NewInstallationID(nil) succeeded, want error")
	}
}
