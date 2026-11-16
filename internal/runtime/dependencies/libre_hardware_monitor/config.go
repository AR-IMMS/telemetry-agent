package librehardwaremonitor

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ar-imms/telemetry-agent/internal/config"
)

type configDocument struct {
	XMLName     xml.Name    `xml:"configuration"`
	AppSettings appSettings `xml:"appSettings"`
}

type appSettings struct {
	Entries []configEntry `xml:"add"`
}

type configEntry struct {
	Key   string `xml:"key,attr"`
	Value string `xml:"value,attr"`
}

// RenderConfig creates the minimal Agent-owned LHM settings file.
func RenderConfig(options Options) ([]byte, error) {
	if err := options.Validate(); err != nil {
		return nil, fmt.Errorf(
			"validate Libre Hardware Monitor configuration options: %w",
			err,
		)
	}

	document := configDocument{
		AppSettings: appSettings{
			Entries: []configEntry{
				{
					Key:   "runWebServerMenuItem",
					Value: "true",
				},
				{
					// LHM maps 127.0.0.1 to "+"; the firewall is the boundary.
					Key:   "listenerIp",
					Value: "+",
				},
				{
					Key:   "listenerPort",
					Value: strconv.Itoa(options.ListenPort),
				},
			},
		},
	}

	rendered, err := xml.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf(
			"encode Libre Hardware Monitor configuration: %w",
			err,
		)
	}

	return append([]byte(xml.Header), rendered...), nil
}

// WriteConfig atomically persists the Agent-owned LHM configuration.
func WriteConfig(options Options) error {
	rendered, err := RenderConfig(options)
	if err != nil {
		return err
	}

	configPath := filepath.Join(
		options.InstallDir,
		"LibreHardwareMonitor.config",
	)

	if err := config.WriteAtomic(configPath, rendered, 0o600); err != nil {
		return fmt.Errorf(
			"write Libre Hardware Monitor configuration: %w",
			err,
		)
	}

	return nil
}

// ConfigMatches reports whether the required Agent-owned LHM settings match.
// Extra settings are allowed because LHM persists sensor and UI preferences.
func ConfigMatches(options Options) (bool, error) {
	if err := options.Validate(); err != nil {
		return false, fmt.Errorf(
			"validate Libre Hardware Monitor configuration options: %w",
			err,
		)
	}

	configPath := filepath.Join(
		options.InstallDir,
		"LibreHardwareMonitor.config",
	)

	content, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf(
			"read Libre Hardware Monitor configuration: %w",
			err,
		)
	}

	var document configDocument

	if err := xml.Unmarshal(content, &document); err != nil {
		return false, fmt.Errorf(
			"parse Libre Hardware Monitor configuration: %w",
			err,
		)
	}

	values := make(map[string]string)

	for _, entry := range document.AppSettings.Entries {
		values[entry.Key] = entry.Value
	}

	return values["runWebServerMenuItem"] == "true" &&
		values["listenerIp"] == "+" &&
		values["listenerPort"] == strconv.Itoa(options.ListenPort), nil
}
