# Node Exporter managed dependency

`node-exporter` là Linux-only dependency do `agentctl` quản lý. Nó bổ sung metric
`node_*`; không thay thế `hostmetrics` baseline của OTel Agent.

```mermaid
flowchart TD
    CLI["sudo agentctl dependency install node-exporter"]
    Check["Root preflight + inspect managed service"]
    Install["Download verified archive<br/>→ /opt/ar-imms/node-exporter"]
    Update["Rewrite managed unit<br/>→ daemon-reload → restart → health check"]
    Reject["Fail safely<br/>do not overwrite"]
    Service["Node Exporter systemd service<br/>127.0.0.1:9100"]
    Agent["OTel Agent<br/>Prometheus receiver"]
    Gateway["OTel Gateway"]
    Stack["Prometheus → Grafana"]

    CLI --> Check
    Check -->|"absent"| Install
    Check -->|"healthy + matching unit"| Service
    Check -->|"healthy + drifted unit"| Update
    Check -->|"unhealthy"| Reject
    Install --> Service
    Update --> Service
    Service --> Agent --> Gateway --> Stack
```
# Ownership and safety
| Resource         | Owner                                               |
| ---------------- | --------------------------------------------------- |
| Binary           | `/opt/ar-imms/node-exporter/node_exporter`          |
| systemd unit     | `/etc/systemd/system/ar-imms-node-exporter.service` |
| Metrics endpoint | `127.0.0.1:9100/metrics`                            |
| Artifact         | pinned Node Exporter `v1.12.1`, SHA-256 verified    |

Reconciliation policy:
| State                                 | Action                                         |
| ------------------------------------- | ---------------------------------------------- |
| Unit absent                           | Download, verify, install, start, health-check |
| Unit exists and `/metrics` is healthy | Reuse                                          |
| Unit exists but unhealthy             | Fail without overwrite                         |

The default Linux `hostmetrics` profile does not enable the `process`` scraper.
Per-process `/proc` access requires elevated runtime privileges and is deferred to an explicit future capability.
