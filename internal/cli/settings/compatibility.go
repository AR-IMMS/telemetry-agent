package settings

import (
	"flag"
	"fmt"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// ParseCompatibilityFlags preserves standard flag parsing while resolving
// explicitly changed values through the CLI settings boundary. Environment
// variables and preference files remain disabled until their names and paths
// are approved.
func ParseCompatibilityFlags(flags *flag.FlagSet, args []string) error {
	if err := flags.Parse(args); err != nil {
		return err
	}

	boundFlags := pflag.NewFlagSet(flags.Name(), pflag.ContinueOnError)
	boundFlags.SetOutput(flags.Output())
	boundFlags.AddGoFlagSet(flags)
	viperSettings := viper.New()
	if err := viperSettings.BindPFlags(boundFlags); err != nil {
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
		got, want := viperSettings.GetString(flag.Name), flag.Value.String()
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
