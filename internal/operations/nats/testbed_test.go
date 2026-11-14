package nats

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const testNATSImage = "nats:2.15.3-alpine@sha256:558394a23dd3c36f2d3a64b51865fd09908010e80a542cea5e75c2af543ed62a"

type natsFixture struct {
	url, caFile, coreUser, corePassword, agentUser, agentPassword string
	container                                                     string
}

func startTestNATSServer(t *testing.T, installation, requestStream, resultStream, agentDurable string) natsFixture {
	t.Helper()
	root := t.TempDir()
	caFile := filepath.Join(root, "ca.pem")
	serverCert := filepath.Join(root, "server.pem")
	serverKey := filepath.Join(root, "server-key.pem")
	createTestCertificate(t, caFile, serverCert, serverKey)
	corePassword := randomFixtureSecret(t)
	agentPassword := randomFixtureSecret(t)
	coreUser, agentUser := "mock-core", "agent"
	config := fmt.Sprintf(`port: 4222
tls { cert_file: "/etc/nats/server.pem", key_file: "/etc/nats/server-key.pem" }
jetstream { store_dir: "/data" }
authorization {
  users = [
    { user: %q, password: %q },
    { user: %q, password: %q, permissions: {
      publish: { allow: [%q, %q, %q, %q, %q] },
      subscribe: { allow: ["_INBOX.>"] }
    } }
  ]
}
`, coreUser, corePassword, agentUser, agentPassword,
		"arimms.ops.v1.result."+installation,
		"$JS.API.CONSUMER.INFO."+requestStream+"."+agentDurable,
		"$JS.API.CONSUMER.MSG.NEXT."+requestStream+"."+agentDurable,
		"$JS.ACK."+requestStream+"."+agentDurable+".>",
		"$JS.API.INFO")
	configPath := filepath.Join(root, "nats.conf")
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	container := "arimms-pr09-nats-" + strings.ReplaceAll(randomFixtureSecret(t)[:12], "-", "")
	command := dockerCommand("create", "--name", container,
		"--env", "PR09_SECRET_FIXTURE=must-not-be-returned",
		"--publish", fmt.Sprintf("127.0.0.1:%d:4222", port),
		testNATSImage, "-c", "/etc/nats/nats.conf")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create isolated real JetStream fixture: %v: %s", err, strings.TrimSpace(string(output)))
	}
	t.Cleanup(func() { _ = dockerCommand("rm", "--force", container).Run() })
	for _, file := range []struct{ source, target string }{
		{serverCert, "/etc/nats/server.pem"}, {serverKey, "/etc/nats/server-key.pem"}, {configPath, "/etc/nats/nats.conf"},
	} {
		if output, err := dockerCommand("cp", file.source, container+":"+file.target).CombinedOutput(); err != nil {
			t.Fatalf("copy isolated JetStream fixture file: %v: %s", err, strings.TrimSpace(string(output)))
		}
	}
	if output, err := dockerCommand("start", container).CombinedOutput(); err != nil {
		t.Fatalf("start isolated real JetStream fixture: %v: %s", err, strings.TrimSpace(string(output)))
	}
	url := fmt.Sprintf("nats://127.0.0.1:%d", port)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		probe, err := Connect(Config{URL: url, Username: coreUser, Password: corePassword, CAFile: caFile,
			InstallationID: installation, BindingID: "fixture", Stream: "FIXTURE", Durable: "FIXTURE"})
		if err == nil {
			probe.Close()
			return natsFixture{url: url, caFile: caFile, coreUser: coreUser, corePassword: corePassword,
				agentUser: agentUser, agentPassword: agentPassword, container: container}
		}
		time.Sleep(250 * time.Millisecond)
	}
	logs, _ := dockerCommand("logs", container).CombinedOutput()
	t.Fatalf("isolated JetStream fixture failed to become ready: %s", strings.TrimSpace(string(logs)))
	return natsFixture{}
}

func dockerCommand(args ...string) *exec.Cmd {
	endpoint := strings.TrimSpace(os.Getenv("AR_IMMS_DOCKER_DISCOVERY_ENDPOINT"))
	if endpoint == "" {
		if runtime.GOOS == "windows" {
			endpoint = "npipe:////./pipe/docker_engine"
		} else {
			endpoint = "unix:///var/run/docker.sock"
		}
	}
	arguments := append([]string{"--host", endpoint}, args...)
	return exec.Command("docker", arguments...)
}

func createTestCertificate(t *testing.T, caPath, certPath, keyPath string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatal(err)
	}
	root := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "AR-IMMS PR-09 test CA"},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(24 * time.Hour), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	rootDER, err := x509.CreateCertificate(rand.Reader, root, root, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	serverSerial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatal(err)
	}
	server := &x509.Certificate{SerialNumber: serverSerial, Subject: pkix.Name{CommonName: "localhost"},
		DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	serverDER, err := x509.CreateCertificate(rand.Reader, server, root, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	writePEM := func(path, kind string, data []byte) {
		t.Helper()
		if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: data}), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writePEM(caPath, "CERTIFICATE", rootDER)
	writePEM(certPath, "CERTIFICATE", serverDER)
	writePEM(keyPath, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key))
}

func randomFixtureSecret(t *testing.T) string {
	t.Helper()
	value := make([]byte, 24)
	if _, err := rand.Read(value); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(value)
}
