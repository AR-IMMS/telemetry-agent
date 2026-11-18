package settings

import (
	"flag"
	"io"
	"testing"
)

func TestParseCompatibilityFlagsBindsExistingFlagsAndPreservesPositionals(t *testing.T) {
	flags := flag.NewFlagSet("example", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	statePath := flags.String("state-path", "default.db", "state path")
	enabled := flags.Bool("enabled", true, "enabled")

	if err := ParseCompatibilityFlags(flags, []string{"--state-path", "custom.db", "--enabled=false"}); err != nil {
		t.Fatalf("ParseCompatibilityFlags() error = %v", err)
	}
	if *statePath != "custom.db" || *enabled {
		t.Fatalf("settings = (%q, %t), want parsed values", *statePath, *enabled)
	}

	positionals := flag.NewFlagSet("positional", flag.ContinueOnError)
	positionals.SetOutput(io.Discard)
	positionals.String("state-path", "default.db", "state path")
	if err := ParseCompatibilityFlags(positionals, []string{"record-id", "--state-path", "custom.db"}); err != nil {
		t.Fatalf("ParseCompatibilityFlags() positional error = %v", err)
	}
	if positionals.NArg() != 3 {
		t.Fatalf("positional count = %d, want 3", positionals.NArg())
	}
}
