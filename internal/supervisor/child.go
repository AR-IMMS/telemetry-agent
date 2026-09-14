package supervisor

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// LaunchOptions contains the verified Collector executable, final config, and
// runtime environment used for one process launch.
type LaunchOptions struct {
	BinaryPath  string
	ConfigPath  string
	Environment []string
}

func (o LaunchOptions) validate() error {
	// Reject blank paths before process creation so failures are deterministic and
	// cannot accidentally invoke a different executable or configuration.
	if strings.TrimSpace(o.BinaryPath) == "" {
		return fmt.Errorf("Collector binary path is required")
	}
	if strings.TrimSpace(o.ConfigPath) == "" {
		return fmt.Errorf("Collector configuration path is required")
	}

	return nil
}

// ExitResult reports the Collector process exit code and wait error.
type ExitResult struct {
	Code int
	Err  error
}

// Child is the lifecycle boundary for a running Collector process.
type Child interface {
	// Wait returns the process result once and then closes the channel.
	Wait() <-chan ExitResult
	// RequestStop asks the Collector to perform its graceful shutdown path.
	RequestStop() error
	// Kill forcibly terminates the Collector after graceful shutdown fails.
	Kill() error
}

// Starter launches a Collector process with the supplied runtime options.
type Starter interface {
	// Start returns a child handle after the operating-system process is started.
	Start(LaunchOptions) (Child, error)
}

type commandFactory func(string, ...string) *exec.Cmd

type execStarter struct {
	newCommand commandFactory
}

func newExecStarter() *execStarter {
	// Keep command construction injectable so process arguments can be tested without
	// starting the real Collector binary.
	return &execStarter{
		newCommand: exec.Command,
	}
}

func (s *execStarter) Start(options LaunchOptions) (Child, error) {
	if err := options.validate(); err != nil {
		return nil, err
	}

	command := s.newCommand(
		options.BinaryPath,
		"--config",
		options.ConfigPath,
	)
	// Keep Collector diagnostics visible to the operator instead of buffering
	// them in the supervisor.
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Env = mergeEnvironment(os.Environ(), options.Environment)

	// Platform-specific process-group setup lets graceful stop reach descendants.
	configureChildCommand(command)

	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start Collector process: %w", err)
	}

	child := &execChild{
		command: command,
		exited:  make(chan ExitResult, 1),
	}

	go child.waitForExit()

	return child, nil
}

type execChild struct {
	command *exec.Cmd
	exited  chan ExitResult
}

func (c *execChild) Wait() <-chan ExitResult {
	return c.exited
}

func (c *execChild) RequestStop() error {
	// A stop request is valid only after Start has populated Process.
	if c.command.Process == nil {
		return fmt.Errorf("Collector process has not started")
	}

	if err := requestGracefulStop(c.command); err != nil {
		return fmt.Errorf("request graceful Collector shutdown: %w", err)
	}

	return nil
}

func (c *execChild) Kill() error {
	// Kill is the final escalation when the Collector ignores graceful shutdown.
	if c.command.Process == nil {
		return fmt.Errorf("Collector process has not started")
	}

	if err := c.command.Process.Kill(); err != nil {
		return fmt.Errorf("force-kill Collector process: %w", err)
	}

	return nil
}

func (c *execChild) waitForExit() {
	err := c.command.Wait()

	result := ExitResult{
		Code: -1,
		Err:  err,
	}
	if c.command.ProcessState != nil {
		result.Code = c.command.ProcessState.ExitCode()
	}

	// Buffer one result so the wait goroutine never blocks during cancellation.
	c.exited <- result
	close(c.exited)
}

func mergeEnvironment(
	base []string,
	overrides []string,
) []string {
	// Copy the base slice so launch-specific overrides cannot mutate process state
	// shared by later starts.
	merged := append([]string(nil), base...)

	for _, override := range overrides {
		key, _, found := strings.Cut(override, "=")
		if !found || key == "" {
			// Ignore malformed entries rather than passing ambiguous environment data
			// to the child process.
			continue
		}

		replaced := false

		for index, existing := range merged {
			existingKey, _, _ := strings.Cut(existing, "=")

			if existingKey == key {
				// Replace by key so a runtime endpoint cannot be shadowed by the host value.
				merged[index] = override
				replaced = true
				break
			}
		}

		if !replaced {
			// Preserve new variables while keeping the inherited environment intact.
			merged = append(merged, override)
		}
	}

	return merged
}
