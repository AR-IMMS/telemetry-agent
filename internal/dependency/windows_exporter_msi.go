package dependency

import (
	"fmt"
	"strings"
)

// processCommand describes one operating-system process without starting it.
type processCommand struct {
	Executable string
	Args       []string
}

// buildWindowsExporterMSIInstallCommand creates the non-interactive MSI command.
// Listener settings stay in the Agent-owned YAML because MSI properties override
// configuration-file values.
func buildWindowsExporterMSIInstallCommand(
	msiPath string,
	options WindowsExporterOptions,
) (processCommand, error) {
	if strings.TrimSpace(msiPath) == "" {
		return processCommand{}, fmt.Errorf(
			"Windows Exporter MSI path is required",
		)
	}
	if err := options.Validate(); err != nil {
		return processCommand{}, err
	}

	args := []string{
		"/i",
		msiPath,
		"/qn",
		"/norestart",
		"CONFIG_FILE=" + options.ConfigPath,
	}

	// The default MSI destination already matches the Agent default. Avoid
	// passing a path containing spaces unless a future custom location needs it.
	if !strings.EqualFold(
		strings.TrimSpace(options.InstallDir),
		DefaultWindowsExporterOptions().InstallDir,
	) {
		args = append(
			args,
			"APPLICATIONFOLDER="+options.InstallDir,
		)
	}

	return processCommand{
		Executable: "msiexec.exe",
		Args:       args,
	}, nil
}
