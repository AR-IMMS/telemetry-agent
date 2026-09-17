# Libre Hardware Monitor runbook

## Install

Run PowerShell as Administrator from the repository root:

```powershell
go run ./cmd/agentctl dependency install libre-hardware-monitor
```

The command installs the pinned LHM release, configures metrics on TCP `9190`,
blocks remote inbound traffic to that port, starts the `LocalSystem` task, and
waits for `/metrics`.

## Verify

```powershell
curl.exe -i http://127.0.0.1:9190/metrics
```

Expected: `HTTP/1.1 200 OK`.

```powershell
schtasks.exe /Query `
  /TN AR-IMMS-LibreHardwareMonitor `
  /V /FO LIST
```

Expected: task is `Running`, `Enabled`, runs as `SYSTEM`, and starts at system
startup.

```powershell
Get-NetFirewallRule `
  -DisplayName 'AR-IMMS Libre Hardware Monitor metrics' |
  Format-List DisplayName, Enabled, Direction, Action, Profile

Get-NetFirewallRule `
  -DisplayName 'AR-IMMS Libre Hardware Monitor metrics' |
  Get-NetFirewallPortFilter
```

Expected: enabled `Inbound` `Block` rule for TCP `9190`.

## Connect the OTel Agent

Render the current Windows Collector configuration, then run the Agent with the
normal gateway endpoint. The rendered config must contain
`prometheus/libre_hardware_monitor` with target `127.0.0.1:9190`.

After one scrape interval, verify in Prometheus:

```promql
count({exported_job="libre_hardware_monitor"})
```

A result greater than zero confirms LHM → OTel Agent → Gateway → Prometheus.

## Diagnose

| Symptom                       | Check                                                                                                 |
| ----------------------------- | ----------------------------------------------------------------------------------------------------- |
| Install requires elevation    | Start PowerShell as Administrator.                                                                    |
| Task is not running           | `schtasks.exe /Query /TN AR-IMMS-LibreHardwareMonitor /V /FO LIST`                                    |
| Metrics endpoint fails        | `curl.exe -i http://127.0.0.1:9190/metrics`                                                           |
| Remote host reaches port 9190 | Inspect the managed firewall rule; it must be inbound block on TCP `9190`.                            |
| Existing task is unhealthy    | The installer intentionally refuses overwrite. Inspect task, config, and LHM process before recovery. |

## Scoped recovery

Use only for a failed development installation. This removes the Agent-owned
task, firewall rule, and installation directory:

```powershell
$installDir = 'C:\Program Files\AR-IMMS\LibreHardwareMonitor'
$exePath = Join-Path $installDir 'LibreHardwareMonitor.exe'

schtasks.exe /End /TN AR-IMMS-LibreHardwareMonitor 2>$null
schtasks.exe /Delete /TN AR-IMMS-LibreHardwareMonitor /F 2>$null

Get-CimInstance Win32_Process -Filter "Name = 'LibreHardwareMonitor.exe'" |
  Where-Object { $_.ExecutablePath -eq $exePath } |
  ForEach-Object { Stop-Process -Id $_.ProcessId -Force }

Remove-NetFirewallRule `
  -DisplayName 'AR-IMMS Libre Hardware Monitor metrics' `
  -ErrorAction SilentlyContinue

Remove-Item -LiteralPath $installDir -Recurse -Force
```
