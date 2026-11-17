package main

import (
	"flag"

	clisettings "github.com/ar-imms/telemetry-agent/internal/cli/settings"
)

// parseCompatibilityFlags preserves the standard flag parser while binding its
// existing values to the CLI settings boundary. Environment and preference-file
// sources remain disabled until their names and paths are approved.
func parseCompatibilityFlags(flags *flag.FlagSet, args []string) error {
	return clisettings.ParseCompatibilityFlags(flags, args)
}
