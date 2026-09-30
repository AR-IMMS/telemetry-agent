package agentinstallation

import (
	"context"
	"fmt"
	"strings"
)

type windowsServiceDefinition struct {
	Name           string
	DisplayName    string
	ExecutablePath string
	Args           []string
}

type windowsServiceManager interface {
	Ensure(context.Context, windowsServiceDefinition) error
	Restart(context.Context, string) error
}

type windowsServiceManagerFunc struct {
	ensure  func(context.Context, windowsServiceDefinition) error
	restart func(context.Context, string) error
}

func (f windowsServiceManagerFunc) Ensure(
	ctx context.Context,
	definition windowsServiceDefinition,
) error {
	if f.ensure == nil {
		return fmt.Errorf("Windows service ensure function is required")
	}

	return f.ensure(ctx, definition)
}

func (f windowsServiceManagerFunc) Restart(
	ctx context.Context,
	serviceName string,
) error {
	if f.restart == nil {
		return fmt.Errorf("Windows service restart function is required")
	}

	return f.restart(ctx, serviceName)
}

type windowsServiceInstaller struct {
	manager windowsServiceManager
}

// Install ensures the Windows service points to the staged Agent binary, then
// restarts it so a reinstallation uses the new binary and state path.
func (i windowsServiceInstaller) Install(
	ctx context.Context,
	layout Layout,
) error {
	if ctx == nil {
		return fmt.Errorf("Windows service installation context is required")
	}
	if !strings.EqualFold(strings.TrimSpace(layout.Platform), "windows") {
		return fmt.Errorf(
			"Windows service requires Windows layout, got %q",
			layout.Platform,
		)
	}
	if i.manager == nil {
		return fmt.Errorf("Windows service manager is required")
	}
	if strings.TrimSpace(layout.ServiceName) == "" {
		return fmt.Errorf("Windows service name is required")
	}
	if strings.TrimSpace(layout.AgentBinaryPath) == "" {
		return fmt.Errorf("Windows Agent binary path is required")
	}
	if strings.TrimSpace(layout.StatePath) == "" {
		return fmt.Errorf("Windows Agent state path is required")
	}

	definition := windowsServiceDefinition{
		Name:           layout.ServiceName,
		DisplayName:    "AR-IMMS Telemetry Agent",
		ExecutablePath: layout.AgentBinaryPath,
		Args: []string{
			"service",
			"--state-path",
			layout.StatePath,
		},
	}

	if err := i.manager.Ensure(ctx, definition); err != nil {
		return fmt.Errorf(
			"ensure Windows service %q: %w",
			definition.Name,
			err,
		)
	}

	if err := i.manager.Restart(ctx, definition.Name); err != nil {
		return fmt.Errorf(
			"restart Windows service %q: %w",
			definition.Name,
			err,
		)
	}

	return nil
}

func (d windowsServiceDefinition) commandLine() string {
	arguments := make([]string, 0, len(d.Args)+1)
	arguments = append(arguments, d.ExecutablePath)
	arguments = append(arguments, d.Args...)

	for index, argument := range arguments {
		arguments[index] = quoteWindowsArgument(argument)
	}

	return strings.Join(arguments, " ")
}

func quoteWindowsArgument(argument string) string {
	if argument == "" {
		return `""`
	}
	if !strings.ContainsAny(argument, " \t\n\v\"") {
		return argument
	}

	var builder strings.Builder

	builder.WriteByte('"')

	backslashes := 0

	for index := 0; index < len(argument); index++ {
		switch argument[index] {
		case '\\':
			backslashes++

		case '"':
			builder.WriteString(
				strings.Repeat(`\`, backslashes*2+1),
			)
			builder.WriteByte('"')
			backslashes = 0

		default:
			builder.WriteString(strings.Repeat(`\`, backslashes))
			builder.WriteByte(argument[index])
			backslashes = 0
		}
	}

	// A trailing backslash must be doubled before the closing quote.
	builder.WriteString(strings.Repeat(`\`, backslashes*2))
	builder.WriteByte('"')

	return builder.String()
}
