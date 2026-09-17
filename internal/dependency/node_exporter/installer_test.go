package nodeexporter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOptionsValidateRejectsMissingRequiredValues(t *testing.T) {
	tests := []struct {
		name    string
		options Options
		want    string
	}{
		{
			name: "missing installation directory",
			options: Options{
				ServicePath:   "/etc/systemd/system/ar-imms-node-exporter.service",
				ListenAddress: "127.0.0.1:9100",
			},
			want: "installation directory",
		},
		{
			name: "missing service path",
			options: Options{
				InstallDir:    "/opt/ar-imms/node-exporter",
				ListenAddress: "127.0.0.1:9100",
			},
			want: "service path",
		},
		{
			name: "missing listen address",
			options: Options{
				InstallDir:  "/opt/ar-imms/node-exporter",
				ServicePath: "/etc/systemd/system/ar-imms-node-exporter.service",
			},
			want: "listen address",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.options.Validate()

			if err == nil {
				t.Fatal("Options.Validate() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf(
					"Options.Validate() error = %q, want text containing %q",
					err,
					test.want,
				)
			}
		})
	}
}

func TestDefaultOptionsUseAgentOwnedLoopbackPaths(t *testing.T) {
	options := DefaultOptions()

	if options.InstallDir != "/opt/ar-imms/node-exporter" {
		t.Fatalf(
			"install directory = %q, want %q",
			options.InstallDir,
			"/opt/ar-imms/node-exporter",
		)
	}

	if options.ServicePath !=
		"/etc/systemd/system/ar-imms-node-exporter.service" {
		t.Fatalf(
			"service path = %q, want Agent-owned systemd unit path",
			options.ServicePath,
		)
	}

	if options.ListenAddress != "127.0.0.1:9100" {
		t.Fatalf(
			"listen address = %q, want loopback Node Exporter endpoint",
			options.ListenAddress,
		)
	}
}

func TestRenderSystemdUnitUsesAgentOwnedBinaryAndLoopback(
	t *testing.T,
) {
	rendered, err := RenderSystemdUnit(DefaultOptions())
	if err != nil {
		t.Fatalf("RenderSystemdUnit() error = %v", err)
	}

	unit := string(rendered)

	for _, want := range []string{
		"[Unit]",
		"Description=AR-IMMS Node Exporter",
		"After=network-online.target",
		"[Service]",
		"Type=simple",
		"ExecStart=/opt/ar-imms/node-exporter/node_exporter " +
			"--web.listen-address=127.0.0.1:9100",
		"Restart=on-failure",
		"[Install]",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf(
				"systemd unit does not contain %q:\n%s",
				want,
				unit,
			)
		}
	}
}

func TestWriteSystemdUnitPersistsRenderedUnit(t *testing.T) {
	options := DefaultOptions()
	options.ServicePath = filepath.Join(
		t.TempDir(),
		"ar-imms-node-exporter.service",
	)

	if err := WriteSystemdUnit(options); err != nil {
		t.Fatalf("WriteSystemdUnit() error = %v", err)
	}

	content, err := os.ReadFile(options.ServicePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if !strings.Contains(
		string(content),
		"ExecStart=/opt/ar-imms/node-exporter/node_exporter "+
			"--web.listen-address=127.0.0.1:9100",
	) {
		t.Fatalf(
			"written systemd unit does not contain expected ExecStart:\n%s",
			content,
		)
	}
}

func TestRenderSystemdUnitUsesLeastPrivilegeServiceAccount(t *testing.T) {
	rendered, err := RenderSystemdUnit(Options{
		InstallDir:    "/opt/ar-imms/node-exporter",
		ServicePath:   "/etc/systemd/system/ar-imms-node-exporter.service",
		ListenAddress: "127.0.0.1:9100",
	})
	if err != nil {
		t.Fatalf("RenderSystemdUnit() error = %v", err)
	}

	for _, want := range []string{
		"DynamicUser=yes",
		"NoNewPrivileges=yes",
		"ProtectHome=yes",
	} {
		if !strings.Contains(string(rendered), want) {
			t.Fatalf(
				"rendered unit does not contain %q:\n%s",
				want,
				rendered,
			)
		}
	}
}
