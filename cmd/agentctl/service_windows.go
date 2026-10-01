//go:build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentinstallation"
	"golang.org/x/sys/windows/svc"
)

const windowsServiceDebugLogPath = `C:\ProgramData\AR-IMMS\Telemetry Agent\service-debug.log`

func runService(
	ctx context.Context,
	args []string,
	_ io.Writer,
	stderr io.Writer,
	deps dependencies,
) int {
	flags := flag.NewFlagSet("service", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(
			stderr,
			"usage: agentctl service --state-path <path>",
		)
	}

	statePath := flags.String(
		"state-path",
		"",
		"persistent Agent state path",
	)

	if err := flags.Parse(args); err != nil {
		flags.Usage()

		return 2
	}
	if flags.NArg() != 0 {
		return usageError(
			stderr,
			flags,
			"service does not accept positional arguments",
		)
	}
	if strings.TrimSpace(*statePath) == "" {
		return usageError(
			stderr,
			flags,
			"--state-path is required",
		)
	}

	err := svc.Run(
		agentinstallation.DefaultAgentServiceName,
		newWindowsAgentService(
			ctx,
			func(serviceContext context.Context) error {
				return runCollectorRuntime(
					serviceContext,
					*statePath,
					deps,
				)
			},
		),
	)
	if err != nil {
		fmt.Fprintf(stderr, "run Windows Agent service: %v\n", err)

		return 1
	}

	return 0
}

func newWindowsAgentService(
	_ context.Context,
	run func(context.Context) error,
) windowsAgentService {
	return windowsAgentService{
		parent: context.Background(),
		run:    run,
	}
}

type windowsAgentService struct {
	parent context.Context
	run    func(context.Context) error
}

func recordWindowsServiceDebug(format string, args ...any) {
	file, err := os.OpenFile(
		windowsServiceDebugLogPath,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0600,
	)
	if err != nil {
		return
	}
	defer file.Close()

	_, _ = fmt.Fprintf(
		file,
		"%s %s\n",
		time.Now().UTC().Format(time.RFC3339Nano),
		fmt.Sprintf(format, args...),
	)
}

func (s windowsAgentService) Execute(
	_ []string,
	requests <-chan svc.ChangeRequest,
	changes chan<- svc.Status,
) (bool, uint32) {
	parent := s.parent
	if parent == nil {
		parent = context.Background()
	}

	serviceContext, cancel := context.WithCancel(parent)
	defer cancel()

	recordWindowsServiceDebug("service Execute started")

	changes <- svc.Status{
		State: svc.StartPending,
	}

	runDone := make(chan error, 1)

	go func() {
		runDone <- s.run(serviceContext)
	}()

	running := svc.Status{
		State:   svc.Running,
		Accepts: svc.AcceptStop | svc.AcceptShutdown,
	}
	changes <- running

	for {
		select {
		case request := <-requests:
			recordWindowsServiceDebug(
				"received SCM control command=%d",
				request.Cmd,
			)

			switch request.Cmd {
			case svc.Interrogate:
				changes <- running

			case svc.Stop, svc.Shutdown:
				recordWindowsServiceDebug(
					"handling SCM stop command=%d",
					request.Cmd,
				)

				changes <- svc.Status{
					State: svc.StopPending,
				}
				cancel()

				err := <-runDone
				recordWindowsServiceDebug(
					"runtime ended after SCM stop: err=%v",
					err,
				)

				changes <- svc.Status{
					State: svc.Stopped,
				}

				return err != nil, exitCode(err)
			}

		case err := <-runDone:
			recordWindowsServiceDebug(
				"runtime ended: err=%v service_context_err=%v",
				err,
				serviceContext.Err(),
			)

			if err == nil && serviceContext.Err() == nil {
				recordWindowsServiceDebug(
					"relaunching cleanly exited runtime",
				)

				go func() {
					runDone <- s.run(serviceContext)
				}()

				continue
			}

			changes <- svc.Status{
				State: svc.Stopped,
			}

			return err != nil, exitCode(err)

		case <-serviceContext.Done():
			recordWindowsServiceDebug(
				"service context cancelled: err=%v",
				serviceContext.Err(),
			)

			changes <- svc.Status{
				State: svc.StopPending,
			}

			err := <-runDone
			recordWindowsServiceDebug(
				"runtime ended after service context cancellation: err=%v",
				err,
			)

			changes <- svc.Status{
				State: svc.Stopped,
			}

			return err != nil, exitCode(err)
		}
	}
}

func exitCode(err error) uint32 {
	if err != nil {
		return 1
	}

	return 0
}
