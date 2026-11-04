//go:build windows

package credentialstore

import (
	"bytes"
	"testing"
)

func TestWindowsMachineDPAPIProtectsAndRecoversSecret(t *testing.T) {
	secret := []byte("service-scoped-sensitive-token")
	protected, err := protectSecret(secret)
	if err != nil {
		t.Fatalf("protectSecret() error = %v", err)
	}
	if bytes.Equal(secret, protected) {
		t.Fatal("DPAPI output equals plaintext")
	}
	got, err := unprotectSecret(protected)
	if err != nil {
		t.Fatalf("unprotectSecret() error = %v", err)
	}
	if !bytes.Equal(got, secret) {
		t.Fatalf("unprotected secret = %q, want original secret", got)
	}
}
