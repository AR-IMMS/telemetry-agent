# Operations runbooks — to finish alongside release implementation

These are required for release readiness but depend on the actual installer/service names and CLI contracts. Do not invent commands before implementation. Required output: native Linux install, native Windows install, troubleshooting (Core/OpAMP, Collector, Docker/receiver_creator, Gateway, exporters), upgrade/uninstall/rollback with dependency ownership checks. Each runbook must be validated on the actual platform and link its verification report.
