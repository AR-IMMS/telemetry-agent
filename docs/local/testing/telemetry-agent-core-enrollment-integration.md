# Agent ↔ Core Enrollment Integration Test

`TestCoreEnrollmentLifecycle` in `internal/coreapi` exercises enrollment over
HTTPS against a deployed Core test instance. It issues bootstrap tokens and
revokes bindings, so the configured Node and administrator credential must be
dedicated to an isolated test deployment.

The test requires Core enrollment migrations to be applied and MongoDB to run
as a replica set. The test Node must exist, be active, have an active placement,
and begin without an active Agent binding. The Core origin must use a trusted
TLS certificate and a loopback/localhost or reserved `.test` hostname. The test
uses the published Core bootstrap-token and binding-revocation endpoints; it
does not create Core data through private APIs.

PowerShell opt-in:

```powershell
$env:AR_IMMS_CORE_ENROLLMENT_TEST = '1'
$env:AR_IMMS_CORE_ENROLLMENT_TEST_CONFIRM_ISOLATED = 'yes'
$env:AR_IMMS_CORE_ENROLLMENT_TEST_ORIGIN = 'https://core.test'
$env:AR_IMMS_CORE_ENROLLMENT_TEST_NODE_ID = '<test-node-uuid>'
$env:AR_IMMS_CORE_ENROLLMENT_TEST_ADMIN_TOKEN = '<authorized-test-admin-token>'
go test ./internal/coreapi -run '^TestCoreEnrollmentLifecycle$' -count=1 -v
```

The test creates local state under `t.TempDir()`, simulates response loss after
Core returns a successful enrollment response, resumes through the same durable
attempt, verifies request equality and recovery status, and exercises attempt
conflict, revocation, permanent recovery failure, and explicit re-enrollment.
It revokes the latest known test binding during cleanup. Do not run it against
production or a shared Node. On Windows, run from an elevated process because
the platform CredentialStore ACL allows SYSTEM and Administrators.
