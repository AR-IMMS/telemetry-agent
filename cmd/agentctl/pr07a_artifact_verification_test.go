package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

const (
	pr07aSupervisorWindowsSHA256 = "df2dfae96d6b711db31675d2deb8493d3dbada8561b25fe575fd90bd7cac8449"
	pr07aSupervisorLinuxSHA256   = "7967e3210e0252f25f41a96b15758dd2e03feb0aecbe85a945f1e2e6e71865c9"
)

func TestPR07ASupervisorArtifactVerificationRejectsWrongDigest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-supervisor")
	if err := os.WriteFile(path, []byte("not the official Supervisor"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyPR07AArtifact(path, pr07aSupervisorWindowsSHA256); err == nil {
		t.Fatal("artifact with a different SHA-256 was accepted")
	}
}

func TestPR07AOfficialSupervisorArtifactsMatchPublishedDigests(t *testing.T) {
	for _, artifact := range []struct {
		name   string
		env    string
		digest string
	}{
		{name: "windows amd64", env: "AR_IMMS_PR07A_SUPERVISOR_WINDOWS_BINARY", digest: pr07aSupervisorWindowsSHA256},
		{name: "linux amd64", env: "AR_IMMS_PR07A_SUPERVISOR_LINUX_BINARY", digest: pr07aSupervisorLinuxSHA256},
	} {
		t.Run(artifact.name, func(t *testing.T) {
			path := os.Getenv(artifact.env)
			if path == "" {
				t.Skipf("set %s to the downloaded official v0.160.0 artifact", artifact.env)
			}
			if err := verifyPR07AArtifact(path, artifact.digest); err != nil {
				t.Fatalf("verify official Supervisor %s artifact: %v", artifact.name, err)
			}
			if runtime.GOOS == "windows" && artifact.name == "windows amd64" {
				if err := verifyPR07AExecutable(path, "--help"); err != nil {
					t.Fatalf("run downloaded Windows Supervisor: %v", err)
				}
			}
		})
	}
}
