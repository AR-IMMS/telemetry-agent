package identity

import "testing"

func TestMachineIDFingerprintNormalizesAndHashesOSMachineID(t *testing.T) {
	got, err := machineIDFingerprint("windows", "  ABC123\n")
	if err != nil {
		t.Fatalf("machineIDFingerprint() error = %v", err)
	}

	const want = "windows-6ca13d52ca70c883e0f0bb10"

	if got != want {
		t.Fatalf(
			"machineIDFingerprint() = %q, want %q",
			got,
			want,
		)
	}
}

func TestCollectMachineIdentityReturnsHostnameAndHashedHostID(t *testing.T) {
	got, err := collectMachineIdentity(
		"windows",
		func() (string, error) {
			return "MSI", nil
		},
		func() (string, error) {
			return "ABC123", nil
		},
	)
	if err != nil {
		t.Fatalf("collectMachineIdentity() error = %v", err)
	}

	want := MachineIdentity{
		HostName: "MSI",
		HostID:   "windows-6ca13d52ca70c883e0f0bb10",
	}

	if got != want {
		t.Fatalf(
			"collectMachineIdentity() = %#v, want %#v",
			got,
			want,
		)
	}
}
