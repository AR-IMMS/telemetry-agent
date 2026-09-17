package librehardwaremonitor

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNewInstallationInspectorReadsManagedLHMState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/metrics" {
				t.Fatalf("request path = %q, want /metrics", request.URL.Path)
			}

			writer.WriteHeader(http.StatusOK)
		},
	))
	defer server.Close()

	_, portText, err := net.SplitHostPort(
		strings.TrimPrefix(server.URL, "http://"),
	)
	if err != nil {
		t.Fatalf("SplitHostPort() error = %v", err)
	}

	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("Atoi() error = %v", err)
	}

	options := DefaultOptions()
	options.InstallDir = t.TempDir()
	options.ListenPort = port

	if err := WriteConfig(options); err != nil {
		t.Fatalf("WriteConfig() error = %v", err)
	}

	expectedTaskXML, err := RenderTaskXML(options)
	expectedTaskXMLText := decodeUTF16LE(t, expectedTaskXML)
	if err != nil {
		t.Fatalf("RenderTaskXML() error = %v", err)
	}

	inspector := newInstallationInspector(
		options,
		server.Client(),
		10*time.Millisecond,
		func(
			ctx context.Context,
			command processCommand,
		) ([]byte, error) {
			script := command.Args[len(command.Args)-1]

			switch {
			case strings.Contains(script, "Export-ScheduledTask"):
				return []byte(expectedTaskXMLText), nil

			case strings.Contains(script, "Get-ScheduledTask"),
				strings.Contains(script, "Get-NetFirewallRule"):
				return []byte("true"), nil

			default:
				t.Fatalf("unexpected PowerShell script:\n%s", script)
				return nil, nil
			}
		},
	)

	state, err := inspector(context.Background())
	if err != nil {
		t.Fatalf("installation inspector error = %v", err)
	}

	want := installationState{
		TaskExists:      true,
		Healthy:         true,
		ConfigMatches:   true,
		FirewallMatches: true,
		TaskMatches:     true,
	}

	if state != want {
		t.Fatalf("installation state = %+v, want %+v", state, want)
	}
}
