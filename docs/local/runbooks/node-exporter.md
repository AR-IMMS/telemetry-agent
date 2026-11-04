# Runbook: Node Exporter on Linux

## Install

Run from an Administrator-equivalent Linux terminal:

```bash
sudo agentctl dependency install node-exporter
```

## Verify
```bash
systemctl status ar-imms-node-exporter.service --no-pager
curl -fsS http://127.0.0.1:9100/metrics | head -n 20
```

Expected:
- service state: active (running);
- process binds only 127.0.0.1:9100;
- response contains node_* metrics.

## Verify through the telemetry path
Bootstrap and run the local OTel Agent with the configured gateway endpoint.
Then query Prometheus or Grafana:
```promql
group by (host_id, host_name) (
  node_uname_info
  or
  windows_cpu_time_total
)
```
The result should show one `linux-*` host and one windows-* host when both nodes are running.

## Failure handling
| Symptom                        | Action                                                                          |
| ------------------------------ | ------------------------------------------------------------------------------- |
| `requires a root terminal`     | Run the install command with `sudo`.                                            |
| Existing service is unhealthy  | Inspect `systemctl status`; do not rerun install until the state is understood. |
| No `node_*` metrics            | Check `curl 127.0.0.1:9100/metrics`, then Agent health at `127.0.0.1:13133`.    |
| `/proc/*/io permission denied` | Ensure `hostmetrics.process` is not enabled in the baseline Linux config.       |


