package bootstrap

import (
	"context"
	"fmt"
	"os"
	"time"
)

// Backoff delays for transient rename failures during publish.
var installationPublishRetryDelays = []time.Duration{
	50 * time.Millisecond,
	100 * time.Millisecond,
	200 * time.Millisecond,
	400 * time.Millisecond,
	800 * time.Millisecond,
}

// renameFunc allows rename behavior to be injected for testing.
type renameFunc func(string, string) error

// publishInstallation publishes a fully prepared staging directory
// as the final Collector installation.
func publishInstallation(
	ctx context.Context,
	stagingDir string,
	targetDir string,
) error {
	return publishWithRename(
		ctx,
		stagingDir,
		targetDir,
		os.Rename,
	)
}

// publishWithRename uses the default retry policy
// with an injected rename function.
func publishWithRename(
	ctx context.Context,
	stagingDir string,
	targetDir string,
	rename renameFunc,
) error {
	return publishWithRenameDelays(
		ctx,
		stagingDir,
		targetDir,
		rename,
		installationPublishRetryDelays,
	)
}

// publishWithRenameDelays retries transient rename failures using backoff.
// This is mainly needed on Windows where files may be temporarily locked. (maybe by antivirus software or something else)
func publishWithRenameDelays(
	ctx context.Context,
	stagingDir string,
	targetDir string,
	rename renameFunc,
	delays []time.Duration,
) error {
	for attempt := 0; ; attempt++ {
		// Stop if the operation was canceled or timed out.
		if err := ctx.Err(); err != nil {
			return fmt.Errorf(
				"publish Collector installation: %w",
				err,
			)
		}

		// Publish the staged installation to the final path.
		err := rename(stagingDir, targetDir)
		if err == nil {
			return nil
		}

		// Retry only known transient errors while retries remain.
		if !isRetryablePublishRenameError(err) ||
			attempt >= len(delays) {
			return fmt.Errorf(
				"publish Collector installation: %w",
				err,
			)
		}

		// Wait before retrying while still respecting cancellation.
		timer := time.NewTimer(delays[attempt])

		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}

			return fmt.Errorf(
				"publish Collector installation: %w",
				ctx.Err(),
			)

		case <-timer.C:
		}
	}
}
