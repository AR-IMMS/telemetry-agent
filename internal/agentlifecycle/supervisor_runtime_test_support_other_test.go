//go:build !windows

package agentlifecycle

func canRunSupervisorRuntimeE2E() bool {
	return true
}
