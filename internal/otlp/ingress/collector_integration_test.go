//go:build integration

package ingress

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPinnedCollectorExportsAuthenticatedMetrics exercises the actual pinned
// Collector's JSON exporter, file-backed bearer, TLS, and ingress identity fence.
func TestPinnedCollectorExportsAuthenticatedMetrics(t *testing.T) {
	binary := os.Getenv("OTELCOL_CONTRIB_BINARY")
	if binary == "" {
		t.Skip("set OTELCOL_CONTRIB_BINARY to the pinned v0.160.0 Collector")
	}
	root := t.TempDir()
	token := strings.Repeat("t", 48)
	tokenFile := filepath.Join(root, "otlp.token")
	if err := os.WriteFile(tokenFile, []byte(token), 0o600); err != nil {
		t.Fatal(err)
	}
	forwarded := make(chan []byte, 2)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		forwarded <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer gateway.Close()
	handler, err := NewHandler(verifierFunc(func(_ context.Context, got string) (Principal, error) {
		if got != token {
			return Principal{}, nil
		}
		return Principal{AgentID: "11111111-1111-4111-8111-111111111111", Scope: "otlp.write"}, nil
	}), gateway.URL, gateway.Client())
	if err != nil {
		t.Fatal(err)
	}
	ingress := httptest.NewTLSServer(handler)
	defer ingress.Close()
	caFile := filepath.Join(root, "ingress-ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ingress.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := x509.ParseCertificate(ingress.Certificate().Raw); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	collectorAddress := listener.Addr().String()
	_ = listener.Close()
	configuration := fmt.Sprintf(`extensions:
  bearertokenauth/otlp:
    filename: %q
receivers:
  otlp:
    protocols:
      http:
        endpoint: %q
exporters:
  otlphttp/gateway:
    endpoint: %q
    encoding: json
    compression: none
    auth:
      authenticator: bearertokenauth/otlp
    tls:
      ca_file: %q
service:
  extensions: [bearertokenauth/otlp]
  pipelines:
    metrics:
      receivers: [otlp]
      exporters: [otlphttp/gateway]
`, tokenFile, collectorAddress, ingress.URL, caFile)
	configFile := filepath.Join(root, "collector.yaml")
	if err := os.WriteFile(configFile, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "--config="+configFile)
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
	for {
		connection, err := net.DialTimeout("tcp", collectorAddress, 100*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			break
		}
		if ctx.Err() != nil {
			t.Fatalf("Collector did not become ready: %s", output.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
	metric := `{"resourceMetrics":[{"resource":{"attributes":[{"key":"agent_id","value":{"stringValue":"spoofed"}}]},"scopeMetrics":[{"metrics":[{"name":"test.counter","sum":{"aggregationTemporality":2,"isMonotonic":true,"dataPoints":[{"asInt":"1"}]}}]}]}]}`
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+collectorAddress+"/v1/metrics", strings.NewReader(metric))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Collector receiver status %d; output: %s", response.StatusCode, output.String())
	}
	select {
	case body := <-forwarded:
		if bytes.Contains(body, []byte("spoofed")) || !bytes.Contains(body, []byte("11111111-1111-4111-8111-111111111111")) {
			t.Fatalf("forwarded identity was not authenticated: %s", body)
		}
	case <-ctx.Done():
		t.Fatalf("Collector did not export authenticated metrics: %s", output.String())
	}
}
