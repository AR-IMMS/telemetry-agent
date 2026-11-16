//go:build windows

package bootstrap

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"
)

func TestPublishWithRenameRetriesTransientWindowsLock(t *testing.T) {
	lockErrors := []error{
		syscall.ERROR_ACCESS_DENIED,
		errorSharingViolation,
		errorLockViolation,
	}

	for _, lockError := range lockErrors {
		t.Run(lockError.Error(), func(t *testing.T) {
			attempts := 0

			err := publishWithRenameDelays(
				context.Background(),
				"staging",
				"target",
				func(_, _ string) error {
					attempts++

					if attempts < 3 {
						return lockError
					}

					return nil
				},
				[]time.Duration{0, 0},
			)
			if err != nil {
				t.Fatalf("publishWithRenameDelays() error = %v", err)
			}

			if attempts != 3 {
				t.Fatalf("rename attempts = %d, want 3", attempts)
			}
		})
	}
}

func TestPublishWithRenameDoesNotRetryPermanentFailure(t *testing.T) {
	attempts := 0
	permanentError := errors.New("permanent rename failure")

	err := publishWithRenameDelays(
		context.Background(),
		"staging",
		"target",
		func(_, _ string) error {
			attempts++
			return permanentError
		},
		[]time.Duration{0, 0, 0},
	)

	if !errors.Is(err, permanentError) {
		t.Fatalf(
			"publishWithRenameDelays() error = %v, want %v",
			err,
			permanentError,
		)
	}

	if attempts != 1 {
		t.Fatalf("rename attempts = %d, want 1", attempts)
	}
}

func TestPublishWithRenameStopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	attempts := 0

	err := publishWithRenameDelays(
		ctx,
		"staging",
		"target",
		func(_, _ string) error {
			attempts++
			cancel()

			return errorSharingViolation
		},
		[]time.Duration{time.Second},
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf(
			"publishWithRenameDelays() error = %v, want context.Canceled",
			err,
		)
	}

	if attempts != 1 {
		t.Fatalf("rename attempts = %d, want 1", attempts)
	}
}
