package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/otlp/ingress"
)

type config struct {
	listenAddress  string
	serverCertFile string
	serverKeyFile  string
	fleetOrigin    string
	fleetCAFile    string
	fleetCertFile  string
	fleetKeyFile   string
	gatewayOrigin  string
}

func loadConfig(getenv func(string) string) (config, error) {
	if getenv == nil {
		return config{}, fmt.Errorf("environment reader is required")
	}
	c := config{
		listenAddress:  strings.TrimSpace(getenv("OTLP_INGRESS_LISTEN")),
		serverCertFile: strings.TrimSpace(getenv("OTLP_INGRESS_TLS_CERT_FILE")),
		serverKeyFile:  strings.TrimSpace(getenv("OTLP_INGRESS_TLS_KEY_FILE")),
		fleetOrigin:    strings.TrimSpace(getenv("OTLP_INGRESS_FLEET_ORIGIN")),
		fleetCAFile:    strings.TrimSpace(getenv("OTLP_INGRESS_FLEET_CA_FILE")),
		fleetCertFile:  strings.TrimSpace(getenv("OTLP_INGRESS_FLEET_CLIENT_CERT_FILE")),
		fleetKeyFile:   strings.TrimSpace(getenv("OTLP_INGRESS_FLEET_CLIENT_KEY_FILE")),
		gatewayOrigin:  strings.TrimSpace(getenv("OTLP_INGRESS_GATEWAY_ORIGIN")),
	}
	for _, value := range []string{c.listenAddress, c.serverCertFile, c.serverKeyFile, c.fleetOrigin, c.fleetCAFile, c.fleetCertFile, c.fleetKeyFile, c.gatewayOrigin} {
		if value == "" {
			return config{}, fmt.Errorf("all OTLP ingress listener, TLS, Fleet, and Gateway settings are required")
		}
	}
	return c, nil
}

func (c config) handler() (http.Handler, error) {
	caPEM, err := os.ReadFile(c.fleetCAFile)
	if err != nil {
		return nil, fmt.Errorf("read Fleet trust root: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("Fleet trust root is invalid")
	}
	clientCertificate, err := tls.LoadX509KeyPair(c.fleetCertFile, c.fleetKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load Fleet client certificate: %w", err)
	}
	fleetClient := &http.Client{Timeout: 4 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, Certificates: []tls.Certificate{clientCertificate}},
	}}
	verifier, err := ingress.NewFleetVerifier(c.fleetOrigin, fleetClient)
	if err != nil {
		return nil, err
	}
	return ingress.NewHandler(verifier, c.gatewayOrigin, &http.Client{Timeout: 4 * time.Second})
}
