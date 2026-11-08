# Docker Resource Discovery Operations

## Enable

Set the Agent Service environment variable `AR_IMMS_DOCKER_DISCOVERY_ENDPOINT` to the local Docker endpoint.

- Windows service: `npipe:////./pipe/docker_engine`
- Linux service: `unix:///var/run/docker.sock`

Restart the Agent Service after changing the variable. Discovery is opt-in and uses local endpoints only. Docker API access grants powerful host control; use a service account with the narrowest practical socket or named-pipe ACL.

## Inspect inventory

```text
agentctl inventory status
agentctl inventory list
agentctl inventory inspect docker/<provider-instance>/<full-container-id>
```

Use `--state-path <path>` before the resource key when inspecting a non-default database. Commands read SQLite only and never contact Docker.

## Interpret observations

- `present` means the last successful complete Docker snapshot included the container.
- `missing` means two successful complete snapshots omitted it. The record remains stored.
- `fresh` means the last observation is no more than two minutes old; `stale` does not imply deletion.
- Endpoints are evidence, not a reachability assertion. Container-scoped addresses may not be reachable from the Agent or Collector.
- A provider error leaves previous resource state intact. Check the service account's Docker permissions and the configured local endpoint. Diagnostics store safe error codes, not raw Docker error strings.

Unset `AR_IMMS_DOCKER_DISCOVERY_ENDPOINT` and restart the service to disable discovery. This does not disable Collector monitoring.

Matching, Integration proposals, approvals, process discovery and Docker event notifications are not implemented.
