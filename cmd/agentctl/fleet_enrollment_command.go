package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	credentialstore "github.com/ar-imms/telemetry-agent/internal/identity/credentials"
	"github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/application"
	fleethttp "github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/delivery/http"
	"github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/domain"
	installationapp "github.com/ar-imms/telemetry-agent/internal/identity/installation/application"
	agentservice "github.com/ar-imms/telemetry-agent/internal/runtime/service"
)

const fleetEnrollmentRequestTimeout = 15 * time.Second

type fleetEnrollmentRunFunc func(context.Context, string, string, string, bool) (domain.Result, error)
type fleetSupervisorProvisionFunc func(string, string) error

func runAgentFleetEnrollment(ctx context.Context, statePath, origin, invitationToken string, resume bool) (domain.Result, error) {
	service := application.Service{
		Repository:  newAgentFleetEnrollmentStore(statePath),
		Credentials: credentialstore.New(statePath),
		Client:      fleethttp.NewClient(&http.Client{Timeout: fleetEnrollmentRequestTimeout}),
	}
	if resume {
		return service.Resume(ctx)
	}
	installationID, err := installationapp.EnsureInstallationIdentity(newAgentIdentityStore(statePath), rand.Reader)
	if err != nil {
		return domain.Result{}, fmt.Errorf("load Agent installation identity: %w", err)
	}
	return service.Start(ctx, origin, installationID.String(), invitationToken)
}

func provisionFleetSupervisor(statePath, endpoint string) error {
	if err := agentservice.ValidateOpAMPServerURL(endpoint); err != nil {
		return fmt.Errorf("validate Fleet OpAMP endpoint: %w", err)
	}
	store := newAgentStateStore(statePath)
	_, err := store.Update(func(state *agentstate.State) error {
		if !state.Supervisor.Enabled || strings.TrimSpace(state.Supervisor.BinaryPath) == "" {
			return fmt.Errorf("Fleet Supervisor provisioning requires an installed, enabled OpAMP Supervisor")
		}
		state.Supervisor.OpAMPServerURL = endpoint
		return nil
	})
	if err != nil {
		return fmt.Errorf("persist Fleet OpAMP endpoint for explicit Supervisor provisioning: %w", err)
	}
	return nil
}
