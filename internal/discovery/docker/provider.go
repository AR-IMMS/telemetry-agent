package docker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"runtime"
	"sort"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/discovery"
	"github.com/ar-imms/telemetry-agent/internal/monitoringprofile"
	"github.com/distribution/reference"
	"github.com/moby/moby/api/types/container"
	moby "github.com/moby/moby/client"
)

const (
	providerID    = discovery.ProviderID("docker")
	maxContainers = 10000
)

// engineClient is the narrow Docker API seam used by the provider and its tests.
type engineClient interface {
	ContainerList(context.Context, moby.ContainerListOptions) (moby.ContainerListResult, error)
}

// Provider observes one explicitly configured local Docker Engine.
type Provider struct {
	instance discovery.ProviderInstanceID
	client   engineClient
}

// NewProvider constructs a provider from an explicit local Unix socket or named pipe.
// It never reads DOCKER_HOST or other Docker client environment variables.
func NewProvider(endpoint string) (*Provider, error) {
	return newProvider(endpoint, runtime.GOOS, nil)
}

func newProvider(endpoint, goos string, httpClient *http.Client) (*Provider, error) {
	endpoint = strings.TrimSpace(endpoint)
	if err := validateLocalEndpoint(endpoint, goos); err != nil {
		return nil, err
	}
	options := []moby.Opt{moby.WithHost(endpoint)}
	if httpClient != nil {
		options = append(options, moby.WithHTTPClient(httpClient))
	}
	api, err := moby.New(options...)
	if err != nil {
		return nil, fmt.Errorf("configure Docker Engine client")
	}
	sum := sha256.Sum256([]byte(endpoint))
	return &Provider{instance: discovery.ProviderInstanceID("local-" + hex.EncodeToString(sum[:8])), client: api}, nil
}

// ID returns the provider type.
func (p *Provider) ID() discovery.ProviderID { return providerID }

// InstanceID returns a stable, non-reversible identifier for the configured endpoint.
func (p *Provider) InstanceID() discovery.ProviderInstanceID { return p.instance }

// Discover returns a complete snapshot of running and stopped containers.
func (p *Provider) Discover(ctx context.Context) (discovery.ProviderSnapshot, error) {
	if ctx == nil {
		return discovery.ProviderSnapshot{}, fmt.Errorf("Docker discovery context is required")
	}
	result, err := p.client.ContainerList(ctx, moby.ContainerListOptions{All: true})
	if err != nil {
		return discovery.ProviderSnapshot{}, err
	}
	if len(result.Items) > maxContainers {
		return discovery.ProviderSnapshot{}, fmt.Errorf("Docker container limit exceeded")
	}
	resources := make([]discovery.ResourceObservation, 0, len(result.Items))
	for _, item := range result.Items {
		observation, err := mapContainer(item)
		if err != nil {
			return discovery.ProviderSnapshot{}, err
		}
		resources = append(resources, observation)
	}
	return discovery.ProviderSnapshot{
		ProviderID: providerID, ProviderInstanceID: p.instance,
		Complete: true, Resources: resources,
	}, nil
}

func validateLocalEndpoint(endpoint, goos string) error {
	if endpoint == "" || len(endpoint) > 512 {
		return fmt.Errorf("explicit Docker endpoint is required")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("invalid Docker endpoint")
	}
	switch goos {
	case "linux":
		if parsed.Scheme != "unix" || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") {
			return fmt.Errorf("Linux Docker endpoint must be an absolute Unix socket")
		}
	case "windows":
		if parsed.Scheme != "npipe" || !strings.HasPrefix(strings.ToLower(parsed.Host+parsed.Path), "//./pipe/") {
			return fmt.Errorf("Windows Docker endpoint must use a local named pipe")
		}
	default:
		return fmt.Errorf("Docker discovery is unsupported on this platform")
	}
	return nil
}

func mapContainer(item container.Summary) (discovery.ResourceObservation, error) {
	if item.ID == "" || len(item.ID) > 256 {
		return discovery.ResourceObservation{}, fmt.Errorf("Docker returned an invalid container ID")
	}
	state := strings.ToLower(string(item.State))
	switch state {
	case "created", "restarting", "running", "removing", "paused", "exited", "dead":
	default:
		return discovery.ResourceObservation{}, fmt.Errorf("Docker returned an unsupported container state")
	}
	name := ""
	names := append([]string(nil), item.Names...)
	sort.Strings(names)
	if len(names) > 0 {
		name = strings.TrimPrefix(names[0], "/")
		if len(name) > 256 {
			return discovery.ResourceObservation{}, fmt.Errorf("Docker container name exceeds limit")
		}
	}
	attributes := make(map[string]string)
	if len(item.Labels) > 256 {
		return discovery.ResourceObservation{}, fmt.Errorf("Docker label count exceeds limit")
	}
	for key, value := range item.Labels {
		if !allowedLabel(key) {
			continue
		}
		if len(value) > 1024 {
			return discovery.ResourceObservation{}, fmt.Errorf("allowlisted Docker label exceeds limit")
		}
		attributes["label."+key] = value
	}
	imageAttributes, err := normalizedImageAttributes(item.Image)
	if err != nil {
		return discovery.ResourceObservation{}, err
	}
	for key, value := range imageAttributes {
		attributes[key] = value
	}
	endpoints, err := mapEndpoints(item)
	if err != nil {
		return discovery.ResourceObservation{}, err
	}
	return discovery.ResourceObservation{
		ProviderResourceID: item.ID, Kind: discovery.ResourceContainer,
		DisplayName: name, LifecycleState: state, Attributes: attributes, Endpoints: endpoints,
	}, nil
}

func normalizedImageAttributes(image string) (map[string]string, error) {
	if image == "" {
		return nil, nil
	}
	if len(image) > 1024 || strings.ContainsAny(image, "\x00\r\n") {
		return nil, fmt.Errorf("Docker image reference exceeds inventory limits")
	}
	attributes := map[string]string{"container.image.reference": image}
	named, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return attributes, nil
	}
	attributes["container.image.repository"] = reference.Domain(named) + "/" + reference.Path(named)
	if tagged, ok := named.(reference.Tagged); ok {
		attributes["container.image.tag"] = tagged.Tag()
	}
	if digested, ok := named.(reference.Digested); ok {
		attributes["container.image.digest"] = digested.Digest().String()
	}
	return attributes, nil
}

func allowedLabel(key string) bool {
	switch key {
	case "com.docker.compose.project", "com.docker.compose.service", "com.docker.compose.version",
		"org.opencontainers.image.title", "org.opencontainers.image.version",
		monitoringprofile.LabelMonitoringEnabled, monitoringprofile.LabelMonitoringProfile, monitoringprofile.LabelWorkloadID:
		return true
	default:
		return false
	}
}

func mapEndpoints(item container.Summary) ([]discovery.Endpoint, error) {
	endpoints := make([]discovery.Endpoint, 0, len(item.Ports)*2)
	networkAddresses := make([]string, 0)
	if item.NetworkSettings != nil {
		for _, settings := range item.NetworkSettings.Networks {
			if settings != nil && settings.IPAddress.IsValid() {
				networkAddresses = append(networkAddresses, settings.IPAddress.String())
			}
		}
	}
	sort.Strings(networkAddresses)
	for _, port := range item.Ports {
		protocol := strings.ToLower(port.Type)
		if port.PrivatePort == 0 || (protocol != "tcp" && protocol != "udp" && protocol != "sctp") {
			return nil, fmt.Errorf("Docker returned an invalid container port")
		}
		for _, address := range networkAddresses {
			endpoints = append(endpoints, discovery.Endpoint{
				Address: address, Port: port.PrivatePort, ContainerPort: port.PrivatePort, Protocol: protocol,
				Scope: discovery.EndpointContainer, Provenance: "container_network",
			})
		}
		if len(networkAddresses) == 0 {
			endpoints = append(endpoints, discovery.Endpoint{
				Port: port.PrivatePort, ContainerPort: port.PrivatePort, Protocol: protocol,
				Scope: discovery.EndpointContainer, Provenance: "exposed_port",
			})
		}
		if port.PublicPort > 0 && port.IP.IsValid() {
			endpoints = append(endpoints, discovery.Endpoint{
				Address: port.IP.String(), Port: port.PublicPort, ContainerPort: port.PrivatePort, Protocol: protocol,
				Scope: discovery.EndpointHost, Provenance: "published_port",
			})
		}
	}
	if len(item.Ports) == 0 {
		for _, address := range networkAddresses {
			endpoints = append(endpoints, discovery.Endpoint{
			Address: address, Scope: discovery.EndpointContainer, Provenance: "network_address",
			})
		}
	}
	if len(endpoints) > 256 {
		return nil, fmt.Errorf("Docker endpoint count exceeds limit")
	}
	return endpoints, nil
}

// endpointIsLocalAddress is kept package-private for endpoint mapping tests.
func endpointIsLocalAddress(address string) bool {
	ip, err := netip.ParseAddr(address)
	return err == nil && ip.IsValid()
}
