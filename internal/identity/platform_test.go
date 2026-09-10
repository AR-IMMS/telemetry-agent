package identity

import (
	"runtime"
	"testing"
)

func TestCollectPlatformInfoIncludesRuntimeIdentity(t *testing.T) {
	info, err := CollectPlatformInfo()
	if err != nil {
		t.Fatalf("CollectPlatformInfo() error = %v", err)
	}
	if info.OS != runtime.GOOS || info.Architecture != runtime.GOARCH || info.Hostname == "" {
		t.Fatalf("unexpected identity: %+v", info)
	}
}

func TestPlatformInfoAsJSONUsesStableFieldNames(t *testing.T) {
	info := PlatformInfo{OS: "linux", Architecture: "amd64", Hostname: "node-1", Kernel: "Linux", Release: "6.0.0"}
	payload, err := info.AsJSON()
	if err != nil {
		t.Fatal(err)
	}
	want := `{"os":"linux","architecture":"amd64","hostname":"node-1","kernel":"Linux","release":"6.0.0"}`
	if string(payload) != want {
		t.Fatalf("AsJSON() = %s, want %s", payload, want)
	}
}

func TestParseOSRelease(t *testing.T) {
	tests := []struct{ name, input, distribution, version string }{
		{"normal", "ID=ubuntu\nVERSION_ID=24.04\n", "ubuntu", "24.04"},
		{"quoted", "ID=\"ubuntu\"\nVERSION_ID=\"24.04 LTS\"\n", "ubuntu", "24.04 LTS"},
		{"malformed", "ID=fedora\nnot-a-pair\nVERSION_ID=40\n", "fedora", "40"},
		{"duplicate", "ID=debian\nID=ubuntu\nVERSION_ID=24.04\n", "ubuntu", "24.04"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseOSRelease([]byte(tt.input))
			if got.Distribution != tt.distribution || got.Version != tt.version {
				t.Fatalf("got %+v", got)
			}
		})
	}
}
