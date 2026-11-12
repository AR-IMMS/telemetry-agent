package docker

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/discovery"
	"github.com/moby/moby/api/types/container"
	moby "github.com/moby/moby/client"
)

type fakeEngine struct {
	result moby.ContainerListResult
	err    error
	all    bool
}

func (f *fakeEngine) ContainerList(ctx context.Context, options moby.ContainerListOptions) (moby.ContainerListResult, error) {
	if err := ctx.Err(); err != nil {
		return moby.ContainerListResult{}, err
	}
	f.all = options.All
	return f.result, f.err
}

func TestProviderRequiresExplicitPlatformLocalEndpoint(t *testing.T) {
	for _, tc := range []struct {
		endpoint, goos string
		wantErr        bool
	}{
		{"unix:///var/run/docker.sock", "linux", false},
		{"tcp://127.0.0.1:2375", "linux", true},
		{"npipe:////./pipe/docker_engine", "windows", false},
		{"npipe:////remote/pipe/docker_engine", "windows", true},
		{"unix:///var/run/docker.sock", "windows", true},
		{"unix:///var/run/docker.sock", "darwin", true},
	} {
		err := validateLocalEndpoint(tc.endpoint, tc.goos)
		if (err != nil) != tc.wantErr {
			t.Fatalf("validateLocalEndpoint(%q,%q) error=%v", tc.endpoint, tc.goos, err)
		}
	}
}

func TestProviderMapsCompleteContainerInventoryWithoutSensitiveMetadata(t *testing.T) {
	fake := &fakeEngine{result: moby.ContainerListResult{Items: []container.Summary{{
		ID: strings.Repeat("a", 64), Names: []string{"/redis-fixture"},
		State: "running", Labels: map[string]string{
			"com.docker.compose.service": "redis", "secret": "must-not-persist",
			"org.opencontainers.image.version": "7.4",
			"ar-imms.monitoring.enabled":       "true", "ar-imms.monitoring.profile": "redis",
			"ar-imms.workload.id": "cache-service",
		},
		Ports: []container.PortSummary{{IP: netip.MustParseAddr("127.0.0.1"), PrivatePort: 6379, PublicPort: 16379, Type: "tcp"}},
	}}}}
	provider := &Provider{instance: "local-test", client: fake}
	snapshot, err := provider.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Complete || !fake.all {
		t.Fatalf("snapshot=%+v all=%v", snapshot, fake.all)
	}
	if len(snapshot.Resources) != 1 {
		t.Fatalf("resources=%d, want 1", len(snapshot.Resources))
	}
	resource := snapshot.Resources[0]
	if resource.ProviderResourceID != strings.Repeat("a", 64) || resource.DisplayName != "redis-fixture" || resource.LifecycleState != "running" {
		t.Fatalf("resource identity/name/state = %+v", resource)
	}
	if resource.Attributes["secret"] != "" || resource.Attributes["label.com.docker.compose.service"] != "redis" || resource.Attributes["label.org.opencontainers.image.version"] != "7.4" {
		t.Fatalf("attributes leaked or missing: %+v", resource.Attributes)
	}
	for key, value := range map[string]string{
		"label.ar-imms.monitoring.enabled": "true", "label.ar-imms.monitoring.profile": "redis",
		"label.ar-imms.workload.id": "cache-service",
	} {
		if resource.Attributes[key] != value {
			t.Errorf("allowlisted monitoring attribute %q=%q, want %q", key, resource.Attributes[key], value)
		}
	}
	if len(resource.Endpoints) != 2 {
		t.Fatalf("endpoints=%+v", resource.Endpoints)
	}
	if resource.Endpoints[0].ContainerPort != 6379 || resource.Endpoints[1].ContainerPort != 6379 {
		t.Fatalf("endpoint container-port mapping=%+v, want private port 6379 on both network and published evidence", resource.Endpoints)
	}
}

func TestMapContainerPreservesBoundedImageEvidenceWithoutUsingImageID(t *testing.T) {
	resource, err := mapContainer(container.Summary{
		ID: "container-id", State: "running",
		Image:   "registry.example:5000/team/redis:7.4",
		ImageID: "sha256:must-not-be-used-as-registry-digest",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"container.image.reference":  "registry.example:5000/team/redis:7.4",
		"container.image.repository": "registry.example:5000/team/redis",
		"container.image.tag":        "7.4",
	}
	for key, value := range want {
		if resource.Attributes[key] != value {
			t.Errorf("attribute %q=%q, want %q; all=%+v", key, resource.Attributes[key], value, resource.Attributes)
		}
	}
	if _, ok := resource.Attributes["container.image.digest"]; ok {
		t.Fatalf("Docker ImageID was treated as an image digest: %+v", resource.Attributes)
	}
}

func TestMapContainerKeepsUnknownImageReferenceWithoutGuessingRepository(t *testing.T) {
	resource, err := mapContainer(container.Summary{ID: "container-id", State: "running", Image: "not a valid image ref"})
	if err != nil {
		t.Fatal(err)
	}
	if got := resource.Attributes["container.image.reference"]; got != "not a valid image ref" {
		t.Fatalf("raw image reference=%q", got)
	}
	if got := resource.Attributes["container.image.repository"]; got != "" {
		t.Fatalf("malformed image reference received guessed repository %q", got)
	}
}

func TestMapContainerPreservesExplicitImageDigest(t *testing.T) {
	resource, err := mapContainer(container.Summary{
		ID: "container-id", State: "running",
		Image: "redis@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := resource.Attributes["container.image.repository"]; got != "docker.io/library/redis" {
		t.Fatalf("repository=%q", got)
	}
	if got := resource.Attributes["container.image.digest"]; got != "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("digest=%q", got)
	}
}

func TestProviderReturnsDaemonErrorsAndHonorsCancellation(t *testing.T) {
	failure := errors.New("daemon unavailable")
	provider := &Provider{instance: "local-test", client: &fakeEngine{err: failure}}
	if _, err := provider.Discover(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("Discover error=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := provider.Discover(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Discover cancellation error=%v", err)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestMobyClientAcceptsInjectedHTTPTransport(t *testing.T) {
	called := false
	transport := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: 500, Body: http.NoBody, Header: make(http.Header), Request: request}, nil
	})
	provider, err := newProvider("unix:///var/run/docker.sock", "linux", &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Discover(context.Background()); err == nil {
		t.Fatal("expected fake server response error")
	}
	if !called {
		t.Fatal("injected HTTP transport was not used")
	}
}

func TestEndpointEvidenceDoesNotClaimReachability(t *testing.T) {
	resource, err := mapContainer(container.Summary{
		ID: "id", State: "exited",
		Ports: []container.PortSummary{{PrivatePort: 6379, PublicPort: 16379, Type: "tcp"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range resource.Endpoints {
		if endpoint.Scope != discovery.EndpointContainer && endpoint.Scope != discovery.EndpointHost {
			t.Fatalf("unexpected endpoint scope %q", endpoint.Scope)
		}
	}
	if endpointIsLocalAddress("not-an-ip") {
		t.Fatal("invalid address accepted as IP")
	}
}
