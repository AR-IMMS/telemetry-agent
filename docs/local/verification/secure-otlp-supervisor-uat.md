# Secure OTLP ingestion: integrated Supervisor UAT

Date: 2026-11-25

Result: **PASS**

The opt-in `TestSecureOTLPSupervisorUAT` ran against the task-scoped Core,
Fleet, TLS ingress, and Gateway services using the official Linux OpAMP
Supervisor v0.160.0 and pinned Collector v0.160.0. The Go test passed in
80.98 seconds.

| Check | Observed result |
| --- | --- |
| Core invitation and Agent enrollment | Passed; OpAMP and `otlp.write` credentials were stored separately in the Agent protected store. |
| Supervisor control connection | Passed over authenticated WSS to Fleet. |
| Collector export | Passed; pinned Collector sent host metrics as OTLP/HTTP JSON over TLS. |
| Gateway identity | Authenticated Agent ID appeared in Gateway metrics; client supplied spoofed identity did not. |
| Anonymous OTLP | Rejected with HTTP 401. |
| Core binding synchronization and revocation | Passed; Core synchronized the Fleet enrollment and revoked the resulting binding. |
| Same-token post-revocation probe | Rejected with HTTP 401 immediately after revocation completed. |

The Linux container used a test-local transparent TCP relay for the registered
loopback OpAMP endpoint. It preserved the endpoint stored during enrollment
and forwarded the TLS stream to Fleet without terminating TLS. The Agent
identity and invitation values, credentials, and private keys are intentionally
omitted from this record. Temporary UAT files and service resources were
removed after evidence capture.

The live test is opt-in and skipped by normal CI because it needs real Core and
Fleet services, TLS identities, pinned runtime binaries, and a reachable
Gateway. The existing CI Go gates still run the unit and service integration
tests. A hosted exact-head CI run, cross-service contract owner approval, and a
Windows live Supervisor run remain open.
