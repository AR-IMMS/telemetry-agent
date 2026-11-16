package main

import (
	"flag"
	"fmt"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// parseCompatibilityFlags preserves the standard flag parser while binding its
// existing values to the CLI settings boundary. Environment and preference-file
// sources remain disabled until their names and paths are approved.
func parseCompatibilityFlags(flags *flag.FlagSet, args []string) error {
	if err := flags.Parse(args); err != nil {
		return err
	}

	boundFlags := pflag.NewFlagSet(flags.Name(), pflag.ContinueOnError)
	boundFlags.SetOutput(flags.Output())
	boundFlags.AddGoFlagSet(flags)
	settings := viper.New()
	if err := settings.BindPFlags(boundFlags); err != nil {
		return fmt.Errorf("bind existing CLI flags: %w", err)
	}

	var mismatch error
	flags.Visit(func(flag *flag.Flag) {
		if mismatch != nil {
			return
		}
		bound := boundFlags.Lookup(flag.Name)
		if bound == nil {
			mismatch = fmt.Errorf("CLI setting %q is not bound", flag.Name)
			return
		}
		bound.Changed = true
		got, want := settings.GetString(flag.Name), flag.Value.String()
		if got != want {
			mismatch = fmt.Errorf("CLI setting %q resolved to %q, want parsed flag value %q", flag.Name, got, want)
			return
		}
		if err := flag.Value.Set(got); err != nil {
			mismatch = fmt.Errorf("apply CLI setting %q: %w", flag.Name, err)
		}
	})
	return mismatch
}
