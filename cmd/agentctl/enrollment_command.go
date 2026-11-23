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

	"github.com/ar-imms/telemetry-agent/internal/coreapi"
	credentialstore "github.com/ar-imms/telemetry-agent/internal/identity/credentials"
	enrollment "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/application"
	installationapp "github.com/ar-imms/telemetry-agent/internal/identity/installation/application"
	agentservice "github.com/ar-imms/telemetry-agent/internal/runtime/service"

	"github.com/charmbracelet/x/term"
)

func runEnrollment(ctx context.Context, args []string, stdout, stderr io.Writer, deps dependencies) int {
	flags := flag.NewFlagSet("enroll", flag.ContinueOnError)
	flags.SetOutput(stderr)
	coreOrigin := flags.String("core-origin", "", "trusted HTTPS AR-IMMS Core origin")
	fleetOrigin := flags.String("fleet-origin", "", "trusted HTTPS Fleet origin; selects Fleet-owned registration")
	statePath := flags.String("state-path", agentservice.DefaultStatePath(), "Agent operational state database path")
	resume := flags.Bool("resume", false, "resume the durable pending enrollment attempt")
	reEnroll := flags.Bool("re-enroll", false, "start a new enrollment after an authorized Core revocation")
	provisionSupervisor := flags.Bool("provision-supervisor", false, "explicitly set the installed Supervisor endpoint to Fleet after enrollment")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: agentctl enroll --core-origin https://core.example [--state-path state.db]")
		fmt.Fprintln(stderr, "       agentctl enroll --fleet-origin https://fleet.example [--provision-supervisor] [--state-path state.db]")
		fmt.Fprintln(stderr, "       agentctl enroll --resume [--core-origin https://core.example | --fleet-origin https://fleet.example]")
		fmt.Fprintln(stderr, "       agentctl enroll --re-enroll --core-origin https://core.example [--state-path state.db]")
		flags.PrintDefaults()
	}
	if err := parseCompatibilityFlags(flags, args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || strings.TrimSpace(*statePath) == "" {
		fmt.Fprintln(stderr, "usage error: enrollment accepts only its documented flags")
		flags.Usage()
		return 2
	}
	coreSelected, fleetSelected := strings.TrimSpace(*coreOrigin) != "", strings.TrimSpace(*fleetOrigin) != ""
	if *resume && *reEnroll {
		fmt.Fprintln(stderr, "usage error: --resume and --re-enroll cannot be combined")
		return 2
	}
	if *resume && coreSelected && fleetSelected {
		fmt.Fprintln(stderr, "usage error: select one enrollment authority")
		return 2
	}
	if !*resume && coreSelected == fleetSelected {
		fmt.Fprintln(stderr, "usage error: choose exactly one of --core-origin or --fleet-origin")
		flags.Usage()
		return 2
	}
	if *reEnroll && fleetSelected {
		fmt.Fprintln(stderr, "usage error: Fleet re-enrollment requires a new invitation and is not yet supported by this command")
		return 2
	}
	if *provisionSupervisor && !fleetSelected {
		fmt.Fprintln(stderr, "usage error: --provision-supervisor requires --fleet-origin")
		return 2
	}

	if fleetSelected {
		if deps.runFleetEnrollment == nil {
			fmt.Fprintln(stderr, "enroll: Fleet enrollment service is not configured")
			return 1
		}
		invitation := ""
		if !*resume {
			readToken := deps.readFleetInvitationToken
			if readToken == nil {
				readToken = readFleetInvitationToken
			}
			var err error
			invitation, err = readToken(stderr)
			if err != nil {
				fmt.Fprintf(stderr, "enroll: %v\n", err)
				return 1
			}
		}
		result, err := deps.runFleetEnrollment(ctx, *statePath, strings.TrimSpace(*fleetOrigin), invitation, *resume)
		if err != nil {
			fmt.Fprintf(stderr, "enroll: %v\n", err)
			return 1
		}
		if *provisionSupervisor {
			provision := deps.provisionFleetSupervisor
			if provision == nil {
				provision = provisionFleetSupervisor
			}
			if err := provision(*statePath, result.OpAMPEndpoint); err != nil {
				fmt.Fprintf(stderr, "enroll: %v\n", err)
				return 1
			}
		}
		fmt.Fprintf(stdout, "Fleet enrollment active: agent=%s node=%s credential=%s\n", result.AgentID, result.NodeID, result.CredentialID)
		if *provisionSupervisor {
			fmt.Fprintln(stdout, "Fleet Supervisor endpoint saved; restart the Agent service to apply it")
		}
		return 0
	}

	if !*resume && !coreSelected {
		fmt.Fprintln(stderr, "usage error: --core-origin is required for a new Core enrollment")
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
	result, err := deps.runEnrollment(ctx, *statePath, strings.TrimSpace(*coreOrigin), token, *resume, *reEnroll)
	if err != nil {
		fmt.Fprintf(stderr, "enroll: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Enrollment active: enrollment=%s installation=%s node=%s\n", result.EnrollmentID, result.InstallationID, result.Binding.NodeID)
	return 0
}

func readBootstrapToken(output io.Writer) (string, error) {
	return readHiddenEnrollmentSecret(output, "Bootstrap token")
}
func readFleetInvitationToken(output io.Writer) (string, error) {
	return readHiddenEnrollmentSecret(output, "Fleet invitation token")
}

func readHiddenEnrollmentSecret(output io.Writer, label string) (string, error) {
	if !term.IsTerminal(os.Stdin.Fd()) {
		return "", fmt.Errorf("%s must be entered from an interactive terminal", strings.ToLower(label))
	}
	if _, err := fmt.Fprintf(output, "%s (input hidden): ", label); err != nil {
		return "", fmt.Errorf("write enrollment prompt")
	}
	secret, err := term.ReadPassword(os.Stdin.Fd())
	_, _ = fmt.Fprintln(output)
	if err != nil {
		return "", fmt.Errorf("read enrollment secret")
	}
	defer clear(secret)
	if strings.TrimSpace(string(secret)) == "" {
		return "", fmt.Errorf("%s is required", strings.ToLower(label))
	}
	return string(secret), nil
}

func runAgentEnrollment(ctx context.Context, statePath, coreOriginValue, bootstrapToken string, resume, reEnroll bool) (enrollment.EnrollmentResult, error) {
	service := enrollment.Service{Repository: newAgentEnrollmentStore(statePath), Credentials: credentialstore.New(statePath), Client: coreapi.NewEnrollmentClient()}
	if resume {
		return service.Resume(ctx)
	}
	origin, err := enrollment.ParseCoreOrigin(coreOriginValue)
	if err != nil {
		return enrollment.EnrollmentResult{}, err
	}
	installationID, err := installationapp.EnsureInstallationIdentity(newAgentIdentityStore(statePath), rand.Reader)
	if err != nil {
		return enrollment.EnrollmentResult{}, fmt.Errorf("load installation identity: %w", err)
	}
	hostname, _ := os.Hostname()
	payload := enrollment.EnrollmentPayload{Installation: enrollment.Installation{ID: installationID.String()}, Agent: enrollment.Agent{Version: agentVersion()}, Environment: enrollment.Environment{OS: normalizeOS(runtime.GOOS), Architecture: normalizeArchitecture(runtime.GOARCH), Hostname: hostname}}
	if reEnroll {
		return service.ReEnroll(ctx, origin, payload, bootstrapToken)
	}
	return service.Start(ctx, origin, payload, bootstrapToken)
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
