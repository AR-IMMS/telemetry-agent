package librehardwaremonitor

import (
	"encoding/xml"
	"strconv"
	"testing"
)

func TestRenderConfigEnablesMetricsWithWildcardListener(t *testing.T) {
	options := DefaultOptions()

	rendered, err := RenderConfig(options)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}

	var document struct {
		AppSettings struct {
			Entries []struct {
				Key   string `xml:"key,attr"`
				Value string `xml:"value,attr"`
			} `xml:"add"`
		} `xml:"appSettings"`
	}

	if err := xml.Unmarshal(rendered, &document); err != nil {
		t.Fatalf("rendered config is not valid XML: %v", err)
	}

	values := make(map[string]string)

	for _, entry := range document.AppSettings.Entries {
		values[entry.Key] = entry.Value
	}

	if got := values["runWebServerMenuItem"]; got != "true" {
		t.Fatalf(
			"runWebServerMenuItem = %q, want true",
			got,
		)
	}

	// LHM converts 127.0.0.1 to "+" internally. Firewall owns containment.
	if got := values["listenerIp"]; got != "+" {
		t.Fatalf("listenerIp = %q, want +", got)
	}

	if got := values["listenerPort"]; got != strconv.Itoa(options.ListenPort) {
		t.Fatalf(
			"listenerPort = %q, want %d",
			got,
			options.ListenPort,
		)
	}
}
