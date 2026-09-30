//go:build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/agentinstallation"
	"golang.org/x/sys/windows/svc"
)

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
		windowsAgentService{
			parent: ctx,
			run: func(serviceContext context.Context) error {
				return runCollectorRuntime(
					serviceContext,
					*statePath,
					deps,
				)
			},
		},
	)
	if err != nil {
		fmt.Fprintf(stderr, "run Windows Agent service: %v\n", err)

		return 1
	}

	return 0
}

type windowsAgentService struct {
	parent context.Context
	run    func(context.Context) error
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
			switch request.Cmd {
			case svc.Interrogate:
				changes <- running

			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{
					State: svc.StopPending,
				}
				cancel()

				err := <-runDone

				changes <- svc.Status{
					State: svc.Stopped,
				}

				return err != nil, exitCode(err)
			}

		case err := <-runDone:
			changes <- svc.Status{
				State: svc.Stopped,
			}

			return err != nil, exitCode(err)

		case <-serviceContext.Done():
			changes <- svc.Status{
				State: svc.StopPending,
			}

			err := <-runDone

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
