package processowner

import (
	"context"
	"fmt"
	"strings"
)

// Options describes one executable and its non-secret command-line arguments.
type Options struct {
	Executable string
	Arguments  []string
}

func validate(options Options) error {
	if strings.TrimSpace(options.Executable) == "" {
		return fmt.Errorf("managed process executable is required")
	}
	if len(options.Arguments) == 0 {
		return fmt.Errorf("managed process arguments are required")
	}
	for _, argument := range options.Arguments {
		if strings.IndexByte(argument, 0) >= 0 {
			return fmt.Errorf("managed process arguments cannot contain NUL")
		}
	}
	return nil
}

// Run starts a process inside an OS-owned process group or Job Object. It
// cleans up contained descendants after cancellation or an unexpected exit.
func Run(ctx context.Context, options Options) error {
	if ctx == nil {
		return fmt.Errorf("managed process context is required")
	}
	if err := validate(options); err != nil {
		return err
	}
	return runContained(ctx, options)
}
