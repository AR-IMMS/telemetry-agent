package librehardwaremonitor

import (
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestRenderTaskXMLRunsLHMAsLocalSystemAtStartup(t *testing.T) {
	options := DefaultOptions()

	rendered, err := RenderTaskXML(options)
	if err != nil {
		t.Fatalf("RenderTaskXML() error = %v", err)
	}

	taskXML := decodeUTF16LE(t, rendered)

	for _, want := range []string{
		"<BootTrigger>",
		"<UserId>S-1-5-18</UserId>",
		"<RunLevel>HighestAvailable</RunLevel>",
		"<MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>",
		"<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>",
		"<RestartOnFailure>",
		"<Interval>PT1M</Interval>",
		"<Count>3</Count>",
		"<Command>" +
			windowsExecutablePath(options.InstallDir) +
			"</Command>",
	} {
		if !strings.Contains(taskXML, want) {
			t.Fatalf(
				"task XML does not contain %q:\n%s",
				want,
				taskXML,
			)
		}
	}
}

func TestRenderTaskXMLDeclaresUTF16Encoding(t *testing.T) {
	rendered, err := RenderTaskXML(DefaultOptions())
	if err != nil {
		t.Fatalf("RenderTaskXML() error = %v", err)
	}

	taskXML := decodeUTF16LE(t, rendered)

	if !strings.HasPrefix(
		taskXML,
		`<?xml version="1.0" encoding="UTF-16"?>`,
	) {
		t.Fatalf(
			"task XML declaration = %q, want UTF-16",
			taskXML[:len(`<?xml version="1.0" encoding="UTF-16"?>`)],
		)
	}
}

func TestRenderTaskXMLReturnsUTF16LEWithBOM(t *testing.T) {
	rendered, err := RenderTaskXML(DefaultOptions())
	if err != nil {
		t.Fatalf("RenderTaskXML() error = %v", err)
	}

	if len(rendered) < 2 ||
		rendered[0] != 0xff ||
		rendered[1] != 0xfe {
		t.Fatalf(
			"task XML prefix = % x, want UTF-16LE BOM ff fe",
			rendered[:2],
		)
	}
}

func decodeUTF16LE(t *testing.T, content []byte) string {
	t.Helper()

	if len(content) < 2 || content[0] != 0xff || content[1] != 0xfe {
		t.Fatal("task XML does not start with a UTF-16LE BOM")
	}
	if (len(content)-2)%2 != 0 {
		t.Fatal("task XML has an invalid UTF-16LE byte length")
	}

	codeUnits := make([]uint16, (len(content)-2)/2)

	for index := range codeUnits {
		codeUnits[index] = binary.LittleEndian.Uint16(
			content[2+index*2:],
		)
	}

	return string(utf16.Decode(codeUnits))
}

func TestRenderTaskXMLOmitsLogonTypeForLocalSystem(t *testing.T) {
	rendered, err := RenderTaskXML(DefaultOptions())
	if err != nil {
		t.Fatalf("RenderTaskXML() error = %v", err)
	}

	xml := decodeUTF16LE(t, rendered)

	if strings.Contains(xml, "<LogonType>") {
		t.Fatalf(
			"SYSTEM task must omit LogonType; schtasks rejects ServiceAccount:\n%s",
			xml,
		)
	}
}

func TestRenderTaskXMLUsesWindowsExecutablePath(t *testing.T) {
	rendered, err := RenderTaskXML(DefaultOptions())
	if err != nil {
		t.Fatalf("RenderTaskXML() error = %v", err)
	}

	xml := decodeUTF16LE(t, rendered)

	want := `<Command>C:\Program Files\AR-IMMS\LibreHardwareMonitor\LibreHardwareMonitor.exe</Command>`
	if !strings.Contains(xml, want) {
		t.Fatalf("task XML does not contain %q:\n%s", want, xml)
	}
}
