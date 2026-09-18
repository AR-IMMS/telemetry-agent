package librehardwaremonitor

import (
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

const testInteractiveUserSID = "S-1-5-21-1000-2000-3000-1001"

func TestRenderTaskXMLRunsLHMForInteractiveUserAtLogon(t *testing.T) {
	options := DefaultOptions()

	rendered, err := RenderTaskXML(options, testInteractiveUserSID)
	if err != nil {
		t.Fatalf("RenderTaskXML() error = %v", err)
	}

	taskXML := decodeUTF16LE(t, rendered)

	for _, want := range []string{
		"<LogonTrigger>",
		"<UserId>" + testInteractiveUserSID + "</UserId>",
		"<LogonType>InteractiveToken</LogonType>",
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

	for _, unwanted := range []string{
		"<BootTrigger>",
		"<UserId>S-1-5-18</UserId>",
	} {
		if strings.Contains(taskXML, unwanted) {
			t.Fatalf(
				"task XML unexpectedly contains %q:\n%s",
				unwanted,
				taskXML,
			)
		}
	}
}

func TestRenderTaskXMLRejectsMissingInteractiveUserSID(t *testing.T) {
	_, err := RenderTaskXML(DefaultOptions(), "")

	if err == nil {
		t.Fatal("RenderTaskXML() error = nil, want missing user SID error")
	}
	if !strings.Contains(err.Error(), "user SID") {
		t.Fatalf(
			"RenderTaskXML() error = %v, want user SID error",
			err,
		)
	}
}

func TestRenderTaskXMLDeclaresUTF16Encoding(t *testing.T) {
	rendered, err := RenderTaskXML(
		DefaultOptions(),
		testInteractiveUserSID,
	)
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
	rendered, err := RenderTaskXML(
		DefaultOptions(),
		testInteractiveUserSID,
	)
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

func TestRenderTaskXMLUsesWindowsExecutablePath(t *testing.T) {
	rendered, err := RenderTaskXML(
		DefaultOptions(),
		testInteractiveUserSID,
	)
	if err != nil {
		t.Fatalf("RenderTaskXML() error = %v", err)
	}

	taskXML := decodeUTF16LE(t, rendered)

	want := `<Command>C:\Program Files\AR-IMMS\LibreHardwareMonitor\LibreHardwareMonitor.exe</Command>`
	if !strings.Contains(taskXML, want) {
		t.Fatalf("task XML does not contain %q:\n%s", want, taskXML)
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
