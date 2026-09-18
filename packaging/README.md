Tạo file `packaging/README.md` với nội dung này. Đây là file sẽ được copy vào cả bundle Windows và Linux.

# AR-IMMS Telemetry Agent

`agentctl` installs local telemetry exporters, renders an OpenTelemetry
Collector configuration, and forwards telemetry to an OTel Gateway.

## Prerequisites

- Run dependency installation as Administrator on Windows or root on Linux.
- The gateway endpoint must be reachable from this machine.
- Keep exporter metrics ports local. Do not expose them to the LAN.

## Included configuration

The `configs/` directory is an editable input to `agentctl bootstrap`.

```text
configs/
├── base/                 # shared Collector configuration
├── profiles/laptop.yaml  # laptop-specific resource attributes
└── os/
    ├── linux/otel.yaml
    └── windows/otel.yaml
````

Edit the appropriate OS file to add or change a supported Prometheus exporter.
Run `bootstrap` again after editing configuration.

## Windows

Open an Administrator PowerShell in the extracted bundle directory.

```powershell
.\agentctl.exe version

.\agentctl.exe dependency install windows-exporter
.\agentctl.exe dependency install libre-hardware-monitor

.\agentctl.exe bootstrap `
  --config-root .\configs `
  --install-dir 'C:\Program Files\AR-IMMS\OpenTelemetryCollector' `
  --config-path 'C:\ProgramData\AR-IMMS\otelcol\otel.yaml'
```

Copy the `Collector binary path` printed by `bootstrap`, then start the Agent:

```powershell
.\agentctl.exe run `
  --collector-path '<Collector binary path>' `
  --config-path 'C:\ProgramData\AR-IMMS\otelcol\otel.yaml' `
  --gateway-endpoint '<gateway-host>:14317'
```

Windows Exporter listens on `127.0.0.1:9182`.
Libre Hardware Monitor provides metrics on port `9190`; the Agent-owned
Windows Firewall rule blocks remote inbound access.

## Linux

Open a root shell in the extracted bundle directory.

```bash
chmod +x ./agentctl

sudo ./agentctl version
sudo ./agentctl dependency install node-exporter

sudo ./agentctl bootstrap \
  --config-root ./configs \
  --install-dir /opt/ar-imms/otelcol \
  --config-path /etc/ar-imms/otelcol/otel.yaml
```

Copy the `Collector binary path` printed by `bootstrap`, then start the Agent:

```bash
sudo ./agentctl run \
  --collector-path '<Collector binary path>' \
  --config-path /etc/ar-imms/otelcol/otel.yaml \
  --gateway-endpoint '<gateway-host>:14317'
```

Node Exporter listens on `127.0.0.1:9100`.

## Verify

Check the local Collector health endpoint while `agentctl run` is active:

```bash
curl http://127.0.0.1:13133/
```

Stop the foreground Agent with `Ctrl+C`.

## Integrity

Verify the downloaded release archive before extraction:

```bash
sha256sum -c checksums.txt
```

On PowerShell:

```powershell
Get-FileHash .\telemetry-agent_<version>_windows_amd64.zip -Algorithm SHA256
Get-Content .\checksums.txt
```


