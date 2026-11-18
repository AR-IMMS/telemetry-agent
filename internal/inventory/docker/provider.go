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

	"github.com/ar-imms/telemetry-agent/internal/contracts/monitoring"
	"github.com/ar-imms/telemetry-agent/internal/inventory/observation/application"
	"github.com/ar-imms/telemetry-agent/internal/inventory/observation/domain"
	"github.com/distribution/reference"
	"github.com/moby/moby/api/types/container"
	moby "github.com/moby/moby/client"
)

const (
	providerID    = domain.ProviderID("docker")
	maxContainers = 10000
)

// engineClient is the narrow Docker API seam used by the provider and its tests.
type engineClient interface {
	ContainerList(context.Context, moby.ContainerListOptions) (moby.ContainerListResult, error)
}

var _ application.DiscoveryProvider = (*Provider)(nil)

type containerInspector interface {
	ContainerInspect(context.Context, string, moby.ContainerInspectOptions) (moby.ContainerInspectResult, error)
}

// ContainerInspection contains the explicit allowlist returned by Docker inspection.
type ContainerInspection struct {
	ID           string `json:"container_id"`
	Name         string `json:"name"`
	Image        string `json:"image"`
	State        string `json:"state"`
	Health       string `json:"health,omitempty"`
	RestartCount int    `json:"restart_count"`
	StartedAt    string `json:"started_at,omitempty"`
}

// Provider observes one explicitly configured local Docker Engine.
type Provider struct {
	instance domain.ProviderInstanceID
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
	return &Provider{instance: domain.ProviderInstanceID("local-" + hex.EncodeToString(sum[:8])), client: api}, nil
}

// ID returns the provider type.
func (p *Provider) ID() domain.ProviderID { return providerID }

// InstanceID returns a stable, non-reversible identifier for the configured endpoint.
func (p *Provider) InstanceID() domain.ProviderInstanceID { return p.instance }

// InspectContainer reads only bounded public runtime fields for an exact full Docker ID.
func (p *Provider) InspectContainer(ctx context.Context, containerID string) (ContainerInspection, error) {
	if ctx == nil || p == nil || p.client == nil {
		return ContainerInspection{}, fmt.Errorf("Docker inspection context and provider are required")
	}
	if len(containerID) != 64 {
		return ContainerInspection{}, fmt.Errorf("full Docker container ID is required")
	}
	if _, err := hex.DecodeString(containerID); err != nil {
		return ContainerInspection{}, fmt.Errorf("full Docker container ID is invalid")
	}
	client, ok := p.client.(containerInspector)
	if !ok {
		return ContainerInspection{}, fmt.Errorf("Docker inspection capability is unavailable")
	}
	response, err := client.ContainerInspect(ctx, containerID, moby.ContainerInspectOptions{})
	if err != nil {
		return ContainerInspection{}, err
	}
	item := response.Container
	if item.ID != containerID || item.State == nil || item.Config == nil {
		return ContainerInspection{}, fmt.Errorf("Docker inspection returned incomplete or mismatched identity")
	}
	state := strings.ToLower(string(item.State.Status))
	switch state {
	case "created", "restarting", "running", "removing", "paused", "exited", "dead":
	default:
		return ContainerInspection{}, fmt.Errorf("Docker inspection returned an unsupported state")
	}
	name := strings.TrimPrefix(item.Name, "/")
	image := item.Config.Image
	if len(name) > 256 || len(image) > 512 || item.RestartCount < 0 {
		return ContainerInspection{}, fmt.Errorf("Docker inspection fields exceed bounds")
	}
	health := ""
	if item.State.Health != nil {
		health = strings.ToLower(string(item.State.Health.Status))
		switch health {
		case "none", "starting", "healthy", "unhealthy":
		default:
			return ContainerInspection{}, fmt.Errorf("Docker inspection returned an unsupported health status")
		}
	}
	startedAt := item.State.StartedAt
	if len(startedAt) > 64 {
		return ContainerInspection{}, fmt.Errorf("Docker inspection start time exceeds bounds")
	}
	return ContainerInspection{ID: item.ID, Name: name, Image: image, State: state, Health: health, RestartCount: item.RestartCount, StartedAt: startedAt}, nil
}

// Discover returns a complete snapshot of running and stopped containers.
func (p *Provider) Discover(ctx context.Context) (domain.ProviderSnapshot, error) {
	if ctx == nil {
		return domain.ProviderSnapshot{}, fmt.Errorf("Docker discovery context is required")
	}
	result, err := p.client.ContainerList(ctx, moby.ContainerListOptions{All: true})
	if err != nil {
		return domain.ProviderSnapshot{}, err
	}
	if len(result.Items) > maxContainers {
		return domain.ProviderSnapshot{}, fmt.Errorf("Docker container limit exceeded")
	}
	resources := make([]domain.ResourceObservation, 0, len(result.Items))
	for _, item := range result.Items {
		observation, err := mapContainer(item)
		if err != nil {
			return domain.ProviderSnapshot{}, err
		}
		resources = append(resources, observation)
	}
	return domain.ProviderSnapshot{
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

func mapContainer(item container.Summary) (domain.ResourceObservation, error) {
	if item.ID == "" || len(item.ID) > 256 {
		return domain.ResourceObservation{}, fmt.Errorf("Docker returned an invalid container ID")
	}
	state := strings.ToLower(string(item.State))
	switch state {
	case "created", "restarting", "running", "removing", "paused", "exited", "dead":
	default:
		return domain.ResourceObservation{}, fmt.Errorf("Docker returned an unsupported container state")
	}
	name := ""
	names := append([]string(nil), item.Names...)
	sort.Strings(names)
	if len(names) > 0 {
		name = strings.TrimPrefix(names[0], "/")
		if len(name) > 256 {
			return domain.ResourceObservation{}, fmt.Errorf("Docker container name exceeds limit")
		}
	}
	attributes := make(map[string]string)
	if len(item.Labels) > 256 {
		return domain.ResourceObservation{}, fmt.Errorf("Docker label count exceeds limit")
	}
	for key, value := range item.Labels {
		if !allowedLabel(key) {
			continue
		}
		if len(value) > 1024 {
			return domain.ResourceObservation{}, fmt.Errorf("allowlisted Docker label exceeds limit")
		}
		attributes["label."+key] = value
	}
	imageAttributes, err := normalizedImageAttributes(item.Image)
	if err != nil {
		return domain.ResourceObservation{}, err
	}
	for key, value := range imageAttributes {
		attributes[key] = value
	}
	endpoints, err := mapEndpoints(item)
	if err != nil {
		return domain.ResourceObservation{}, err
	}
	return domain.ResourceObservation{
		ProviderResourceID: item.ID, Kind: domain.ResourceContainer,
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
		monitoring.DockerLabelEnabled, monitoring.DockerLabelProfile, monitoring.DockerLabelWorkloadID,
		monitoring.DockerLabelPort:
		return true
	default:
		return false
	}
}

func mapEndpoints(item container.Summary) ([]domain.Endpoint, error) {
	endpoints := make([]domain.Endpoint, 0, len(item.Ports)*2)
	type networkEndpoint struct {
		name, address string
		priority      int
	}
	networkAddresses := make([]networkEndpoint, 0)
	if item.NetworkSettings != nil {
		for name, settings := range item.NetworkSettings.Networks {
			if settings != nil && settings.IPAddress.IsValid() {
				networkAddresses = append(networkAddresses, networkEndpoint{name: name, address: settings.IPAddress.String(), priority: settings.GwPriority})
			}
		}
	}
	sort.Slice(networkAddresses, func(i, j int) bool {
		if networkAddresses[i].priority != networkAddresses[j].priority {
			return networkAddresses[i].priority > networkAddresses[j].priority
		}
		if networkAddresses[i].name == networkAddresses[j].name {
			return networkAddresses[i].address < networkAddresses[j].address
		}
		return networkAddresses[i].name < networkAddresses[j].name
	})
	for _, port := range item.Ports {
		protocol := strings.ToLower(port.Type)
		if port.PrivatePort == 0 || (protocol != "tcp" && protocol != "udp" && protocol != "sctp") {
			return nil, fmt.Errorf("Docker returned an invalid container port")
		}
		for _, network := range networkAddresses {
			endpoints = append(endpoints, domain.Endpoint{
				Address: network.address, NetworkName: network.name, NetworkPriority: network.priority, Port: port.PrivatePort, ContainerPort: port.PrivatePort, Protocol: protocol,
				Scope: domain.EndpointContainer, Provenance: "container_network",
			})
		}
		if len(networkAddresses) == 0 {
			endpoints = append(endpoints, domain.Endpoint{
				Port: port.PrivatePort, ContainerPort: port.PrivatePort, Protocol: protocol,
				Scope: domain.EndpointContainer, Provenance: "exposed_port",
			})
		}
		if port.PublicPort > 0 && port.IP.IsValid() {
			endpoints = append(endpoints, domain.Endpoint{
				Address: port.IP.String(), Port: port.PublicPort, ContainerPort: port.PrivatePort, Protocol: protocol,
				Scope: domain.EndpointHost, Provenance: "published_port",
			})
		}
	}
	if len(item.Ports) == 0 {
		for _, network := range networkAddresses {
			endpoints = append(endpoints, domain.Endpoint{
				Address: network.address, NetworkName: network.name, NetworkPriority: network.priority, Scope: domain.EndpointContainer, Provenance: "network_address",
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
