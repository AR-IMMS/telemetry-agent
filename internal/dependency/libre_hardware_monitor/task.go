package librehardwaremonitor

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"path/filepath"
	"unicode/utf16"
)

// RenderTaskXML creates the LocalSystem startup task that keeps LHM running.
func RenderTaskXML(options Options) ([]byte, error) {
	if err := options.Validate(); err != nil {
		return nil, fmt.Errorf(
			"validate Libre Hardware Monitor task options: %w",
			err,
		)
	}

	executablePath := filepath.Join(
		options.InstallDir,
		"LibreHardwareMonitor.exe",
	)

	var escapedCommand bytes.Buffer

	if err := xml.EscapeText(
		&escapedCommand,
		[]byte(executablePath),
	); err != nil {
		return nil, fmt.Errorf(
			"escape Libre Hardware Monitor executable path: %w",
			err,
		)
	}

	taskXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <Triggers>
    <BootTrigger>
      <Enabled>true</Enabled>
    </BootTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>S-1-5-18</UserId>
      <RunLevel>HighestAvailable</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <RestartOnFailure>
      <Interval>PT1M</Interval>
      <Count>3</Count>
    </RestartOnFailure>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>%s</Command>
    </Exec>
  </Actions>
</Task>
`, escapedCommand.String())

	return encodeUTF16LE(taskXML), nil
}

// encodeUTF16LE produces the BOM-prefixed encoding expected by schtasks.exe.
func encodeUTF16LE(content string) []byte {
	codeUnits := utf16.Encode([]rune(content))

	result := make([]byte, 2+len(codeUnits)*2)
	result[0] = 0xff
	result[1] = 0xfe

	for index, codeUnit := range codeUnits {
		binary.LittleEndian.PutUint16(
			result[2+index*2:],
			codeUnit,
		)
	}

	return result
}
