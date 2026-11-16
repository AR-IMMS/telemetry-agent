package cli

import (
	"flag"
	"io"
	"testing"
)

func TestParseCompatibilityFlagsKeepsExistingFlagValuesAndPositionals(t *testing.T) {
	flags := flag.NewFlagSet("example", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	statePath := flags.String("state-path", "default.db", "state path")
	enabled := flags.Bool("enabled", true, "enabled")

	if err := ParseCompatibilityFlags(flags, []string{"record-id", "--state-path", "custom.db", "--enabled=false"}); err != nil {
		t.Fatalf("ParseCompatibilityFlags() error = %v", err)
	}
	if *statePath != "default.db" || !*enabled {
		t.Fatalf("settings = (%q, %t), want standard parser to stop at first positional argument", *statePath, *enabled)
	}
	if flags.NArg() != 4 {
		t.Fatalf("positional count = %d, want 4", flags.NArg())
	}
}

func TestParseCompatibilityFlagsBindsParsedFlagValues(t *testing.T) {
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
}
