//go:build windows

package agentlifecycle

import "golang.org/x/sys/windows"

func canRunSupervisorRuntimeE2E() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
