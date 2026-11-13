//go:build windows

package processowner

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	processWaitInterval       = 250
	processWaitLimit          = 20_000
	processShutdownGraceLimit = 5_000
)

type jobBasicAccountingInformation struct {
	TotalUserTime            int64
	TotalKernelTime          int64
	ThisPeriodUserTime       int64
	ThisPeriodKernelTime     int64
	TotalPageFaults          uint32
	TotalProcesses           uint32
	ActiveProcesses          uint32
	TotalTerminatedProcesses uint32
}

func runContained(ctx context.Context, options Options) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("create managed process Job Object: %w", err)
	}
	defer windows.CloseHandle(job)
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return fmt.Errorf("configure managed process Job Object: %w", err)
	}

	executable, err := windows.UTF16PtrFromString(options.Executable)
	if err != nil {
		return fmt.Errorf("encode managed process executable: %w", err)
	}
	args := append([]string{options.Executable}, options.Arguments...)
	commandLine, err := windows.UTF16FromString(windows.ComposeCommandLine(args))
	if err != nil {
		return fmt.Errorf("encode managed process command line: %w", err)
	}
	startup := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	var environment *uint16
	var environmentBlock []uint16
	if len(options.Environment) > 0 {
		entries := mergedEnvironment(windows.Environ(), options.Environment)
		environmentBlock = utf16.Encode([]rune(strings.Join(entries, "\x00") + "\x00\x00"))
		environment = &environmentBlock[0]
	}
	var process windows.ProcessInformation
	if err := windows.CreateProcess(executable, &commandLine[0], nil, nil, false, windows.CREATE_SUSPENDED|windows.CREATE_UNICODE_ENVIRONMENT|windows.CREATE_NEW_PROCESS_GROUP, environment, nil, &startup, &process); err != nil {
		return fmt.Errorf("create suspended managed process: %w", err)
	}
	defer windows.CloseHandle(process.Process)
	defer windows.CloseHandle(process.Thread)
	if err := windows.AssignProcessToJobObject(job, process.Process); err != nil {
		_ = windows.TerminateProcess(process.Process, 1)
		return fmt.Errorf("assign managed process to Job Object: %w", err)
	}
	if _, err := windows.ResumeThread(process.Thread); err != nil {
		_ = windows.TerminateJobObject(job, 1)
		return fmt.Errorf("resume managed process in Job Object: %w", err)
	}

	ticker := time.NewTicker(processWaitInterval * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// Services often have no attached console. Try the Supervisor's normal
			// Windows control-break shutdown path first, then fall back to the Job.
			if windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, process.ProcessId) == nil {
				_ = waitForProcessWithin(process.Process, processShutdownGraceLimit)
			}
			if err := windows.TerminateJobObject(job, 1); err != nil {
				return fmt.Errorf("terminate managed process Job Object: %w", err)
			}
			if err := waitForProcess(process.Process); err != nil {
				return fmt.Errorf("wait for terminated managed process: %w", err)
			}
			if err := waitForJobEmpty(job); err != nil {
				return fmt.Errorf("wait for managed process descendants: %w", err)
			}
			return nil
		case <-ticker.C:
			result, err := windows.WaitForSingleObject(process.Process, 0)
			if err != nil {
				return fmt.Errorf("check managed process state: %w", err)
			}
			if result == uint32(windows.WAIT_TIMEOUT) {
				continue
			}
			if result != windows.WAIT_OBJECT_0 {
				return fmt.Errorf("unexpected managed process wait result %d", result)
			}
			var exitCode uint32
			if err := windows.GetExitCodeProcess(process.Process, &exitCode); err != nil {
				return fmt.Errorf("read managed process exit status: %w", err)
			}
			// A Job Object may still contain descendants after its root exits.
			if err := windows.TerminateJobObject(job, exitCode); err != nil {
				return fmt.Errorf("clean up managed process descendants: %w", err)
			}
			if err := waitForJobEmpty(job); err != nil {
				return fmt.Errorf("wait for managed process descendants: %w", err)
			}
			if exitCode != 0 {
				return fmt.Errorf("contained process exited with code %d", exitCode)
			}
			return fmt.Errorf("contained process exited unexpectedly")
		}
	}
}

func waitForJobEmpty(job windows.Handle) error {
	for waited := 0; waited < processWaitLimit; waited += processWaitInterval {
		var accounting jobBasicAccountingInformation
		if err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil); err != nil {
			return err
		}
		if accounting.ActiveProcesses == 0 {
			return nil
		}
		time.Sleep(processWaitInterval * time.Millisecond)
	}
	return fmt.Errorf("Job Object remained populated after %d milliseconds", processWaitLimit)
}

func waitForProcess(process windows.Handle) error {
	return waitForProcessWithin(process, processWaitLimit)
}

func waitForProcessWithin(process windows.Handle, maxWait int) error {
	for waited := 0; waited < maxWait; waited += processWaitInterval {
		result, err := windows.WaitForSingleObject(process, processWaitInterval)
		if err != nil {
			return err
		}
		if result == windows.WAIT_OBJECT_0 {
			return nil
		}
		if result != uint32(windows.WAIT_TIMEOUT) {
			return fmt.Errorf("unexpected managed process wait result %d", result)
		}
		runtime.Gosched()
	}
	return fmt.Errorf("managed process did not terminate within %d milliseconds", maxWait)
}
