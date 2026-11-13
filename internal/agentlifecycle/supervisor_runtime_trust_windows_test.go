//go:build windows

package agentlifecycle

import (
	"crypto/x509"
	"encoding/pem"
	"testing"

	"golang.org/x/sys/windows"
)

func trustManagedRuntimeCertificate(t *testing.T, certificate []byte) {
	t.Helper()
	block, _ := pem.Decode(certificate)
	if block == nil {
		t.Fatal("decode temporary Supervisor trust certificate")
	}
	parsed, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	storeName := windows.StringToUTF16Ptr("ROOT")
	store, err := windows.CertOpenSystemStore(0, storeName)
	if err != nil {
		t.Fatalf("open current-user ROOT certificate store: %v", err)
	}
	encoded, err := windows.CertCreateCertificateContext(
		windows.X509_ASN_ENCODING|windows.PKCS_7_ASN_ENCODING,
		&parsed.Raw[0], uint32(len(parsed.Raw)),
	)
	if err != nil {
		_ = windows.CertCloseStore(store, 0)
		t.Fatalf("create test certificate context: %v", err)
	}
	var stored *windows.CertContext
	if err := windows.CertAddCertificateContextToStore(store, encoded, windows.CERT_STORE_ADD_REPLACE_EXISTING, &stored); err != nil {
		_ = windows.CertFreeCertificateContext(encoded)
		_ = windows.CertCloseStore(store, 0)
		t.Fatalf("add temporary test certificate to current-user ROOT store: %v", err)
	}
	if err := windows.CertFreeCertificateContext(encoded); err != nil {
		t.Errorf("free temporary certificate context: %v", err)
	}
	t.Cleanup(func() {
		if err := windows.CertDeleteCertificateFromStore(stored); err != nil {
			t.Errorf("remove temporary test certificate from current-user ROOT store: %v", err)
		}
		if err := windows.CertCloseStore(store, 0); err != nil {
			t.Errorf("close current-user ROOT certificate store: %v", err)
		}
	})
}
