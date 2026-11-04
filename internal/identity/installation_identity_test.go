package identity

import (
	"bytes"
	"errors"
	"testing"
)

func TestNewInstallationIDGeneratesUUIDv4(t *testing.T) {
	id, err := NewInstallationID(bytes.NewReader(make([]byte, 16)))
	if err != nil {
		t.Fatalf("NewInstallationID() error = %v", err)
	}
	if got, want := id.String(), "00000000-0000-4000-8000-000000000000"; got != want {
		t.Fatalf("installation ID = %q, want %q", got, want)
	}
}

func TestNewInstallationIDReturnsEntropyError(t *testing.T) {
	wantErr := errors.New("entropy unavailable")
	_, err := NewInstallationID(errorReader{err: wantErr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("NewInstallationID() error = %v, want %v", err, wantErr)
	}
}

func TestParseInstallationIDRejectsMalformedValue(t *testing.T) {
	if _, err := ParseInstallationID("not-a-uuid"); err == nil {
		t.Fatal("ParseInstallationID() error = nil, want malformed UUID error")
	}
}

func TestEnsureInstallationIdentityReturnsCommittedRepositoryID(t *testing.T) {
	committed, err := ParseInstallationID("11111111-2222-4333-8444-555555555555")
	if err != nil {
		t.Fatalf("ParseInstallationID() error = %v", err)
	}
	repository := &committedIdentityRepository{committed: committed}

	got, err := EnsureInstallationIdentity(
		repository,
		bytes.NewReader(make([]byte, 16)),
	)
	if err != nil {
		t.Fatalf("EnsureInstallationIdentity() error = %v", err)
	}
	if got != committed {
		t.Fatalf("identity = %q, want repository's committed ID %q", got, committed)
	}
	if repository.calls != 1 {
		t.Fatalf("repository calls = %d, want 1", repository.calls)
	}
}

func TestEnsureInstallationIdentityRejectsInvalidCommittedValue(t *testing.T) {
	repository := &committedIdentityRepository{committed: InstallationID{value: "invalid"}}
	_, err := EnsureInstallationIdentity(
		repository,
		bytes.NewReader(make([]byte, 16)),
	)
	if err == nil {
		t.Fatal("EnsureInstallationIdentity() error = nil, want invalid repository ID error")
	}
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}

type committedIdentityRepository struct {
	committed InstallationID
	calls     int
}

func (r *committedIdentityRepository) EnsureInstallationID(
	InstallationID,
) (InstallationID, error) {
	r.calls++
	return r.committed, nil
}
