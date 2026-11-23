//go:build !windows && !linux

package processowner

import (
	"context"
	"fmt"
)

func runContained(context.Context, Options) error {
	return fmt.Errorf("managed process containment is unsupported on this operating system")
}
