package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/coreapi"
	"github.com/ar-imms/telemetry-agent/internal/credentialstore"
	"github.com/ar-imms/telemetry-agent/internal/enrollment"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/charmbracelet/x/term"
)

func runEnrollment(ctx context.Context, args []string, stdout, stderr io.Writer, deps dependencies) int {
	flags := flag.NewFlagSet("enroll", flag.ContinueOnError)
	flags.SetOutput(stderr)
	coreOrigin := flags.String("core-origin", "", "trusted HTTPS AR-IMMS Core origin")
	statePath := flags.String("state-path", agentstate.DefaultPath(), "Agent operational state database path")
	resume := flags.Bool("resume", false, "resume the durable pending enrollment attempt")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: agentctl enroll --core-origin https://core.example [--state-path state.db]")
		fmt.Fprintln(stderr, "       agentctl enroll --resume [--state-path state.db]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || strings.TrimSpace(*statePath) == "" {
		fmt.Fprintln(stderr, "usage error: enrollment accepts only its documented flags")
		flags.Usage()
		return 2
	}
	if !*resume && strings.TrimSpace(*coreOrigin) == "" {
		fmt.Fprintln(stderr, "usage error: --core-origin is required for a new enrollment")
		flags.Usage()
		return 2
	}
	if deps.runEnrollment == nil {
		fmt.Fprintln(stderr, "enroll: enrollment service is not configured")
		return 1
	}
	token := ""
	if !*resume {
		readToken := deps.readBootstrapToken
		if readToken == nil {
			readToken = readBootstrapToken
		}
		var err error
		token, err = readToken(stderr)
		if err != nil {
			fmt.Fprintf(stderr, "enroll: %v\n", err)
			return 1
		}
	}
	result, err := deps.runEnrollment(ctx, *statePath, strings.TrimSpace(*coreOrigin), token, *resume)
	if err != nil {
		fmt.Fprintf(stderr, "enroll: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Enrollment active: enrollment=%s installation=%s node=%s\n",
		result.EnrollmentID, result.InstallationID, result.Binding.NodeID)
	return 0
}

func readBootstrapToken(output io.Writer) (string, error) {
	if !term.IsTerminal(os.Stdin.Fd()) {
		return "", fmt.Errorf("bootstrap token must be entered from an interactive terminal")
	}
	if _, err := fmt.Fprint(output, "Bootstrap token (input hidden): "); err != nil {
		return "", fmt.Errorf("write enrollment prompt: %w", err)
	}
	secret, err := term.ReadPassword(os.Stdin.Fd())
	_, _ = fmt.Fprintln(output)
	if err != nil {
		return "", fmt.Errorf("read bootstrap token: %w", err)
	}
	defer clear(secret)
	if strings.TrimSpace(string(secret)) == "" {
		return "", fmt.Errorf("bootstrap token is required")
	}
	return string(secret), nil
}

func runAgentEnrollment(
	ctx context.Context,
	statePath string,
	coreOriginValue string,
	bootstrapToken string,
	resume bool,
) (enrollment.EnrollmentResult, error) {
	service := enrollment.Service{
		Repository:  agentstate.NewEnrollmentStore(statePath),
		Credentials: credentialstore.New(statePath),
		Client:      coreapi.NewEnrollmentClient(),
	}
	if resume {
		return service.Resume(ctx)
	}
	origin, err := enrollment.ParseCoreOrigin(coreOriginValue)
	if err != nil {
		return enrollment.EnrollmentResult{}, err
	}
	installationID, err := identity.EnsureInstallationIdentity(
		agentstate.NewIdentityStore(statePath),
		rand.Reader,
	)
	if err != nil {
		return enrollment.EnrollmentResult{}, fmt.Errorf("load installation identity: %w", err)
	}
	hostname, _ := os.Hostname()
	result, err := service.Start(ctx, origin, enrollment.EnrollmentPayload{
		Installation: enrollment.Installation{ID: installationID.String()},
		Agent:        enrollment.Agent{Version: agentVersion()},
		Environment:  enrollment.Environment{OS: normalizeOS(runtime.GOOS), Architecture: normalizeArchitecture(runtime.GOARCH), Hostname: hostname},
	}, bootstrapToken)
	if err != nil {
		return enrollment.EnrollmentResult{}, err
	}
	return result, nil
}

func agentVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && strings.TrimSpace(info.Main.Version) != "" {
		return info.Main.Version
	}
	return "development"
}

func normalizeOS(value string) string {
	switch value {
	case "linux", "windows", "darwin":
		return value
	default:
		return "other"
	}
}

func normalizeArchitecture(value string) string {
	switch value {
	case "amd64", "arm64", "386":
		return value
	default:
		return "other"
	}
}
