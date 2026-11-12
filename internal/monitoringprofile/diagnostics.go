package monitoringprofile

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/discovery"
)

// EndpointState classifies endpoint evidence for one opted-in resource.
type EndpointState string

const (
	// EndpointMissing means no supported endpoint strategy matched inventory evidence.
	EndpointMissing EndpointState = "endpoint_missing"
	// EndpointAmbiguous means a strategy matched multiple endpoints and failed closed.
	EndpointAmbiguous EndpointState = "endpoint_ambiguous"
	// EndpointInvalid means an explicit profile target-port override is malformed.
	EndpointInvalid EndpointState = "endpoint_invalid"
	// EndpointResolved means one candidate is known, but reachability is unverified.
	EndpointResolved EndpointState = "endpoint_resolved"
	// EndpointUnreachable means the matching OTel receiver reported a network scrape error.
	EndpointUnreachable EndpointState = "endpoint_unreachable"

	// EvidenceAgentInventory identifies endpoint candidates sourced from persisted inventory.
	EvidenceAgentInventory = "agent_inventory"
	// EvidenceCollectorScrapeError identifies a matching OTel receiver scrape error.
	EvidenceCollectorScrapeError = "otel_receiver_scrape_error"
)

// CollectorScrapeEvidence contains only safe fields extracted from one OTel log record.
type CollectorScrapeEvidence struct {
	ReceiverID  string
	Endpoint    string
	FailureCode string
	ObservedAt  time.Time
}

// EndpointDiagnostic reports endpoint strategy selection and its evidence source.
type EndpointDiagnostic struct {
	ProfileID      string
	State          EndpointState
	CandidateCount int
	Endpoint       discovery.Endpoint
	EvidenceSource string
	EvidenceCode   string
	EvidenceTime   time.Time
}

// DiagnoseEndpoint resolves one endpoint using the profile's ordered strategies.
// A resolved candidate is not considered reachable without matching OTel error evidence.
func DiagnoseEndpoint(profile Profile, osName string, endpoints []discovery.Endpoint, otelEvidence []CollectorScrapeEvidence) EndpointDiagnostic {
	return DiagnoseEndpointWithAttributes(profile, osName, endpoints, otelEvidence, nil)
}

// DiagnoseEndpointWithAttributes resolves one profile endpoint using provider metadata such as port overrides.
func DiagnoseEndpointWithAttributes(profile Profile, osName string, endpoints []discovery.Endpoint, otelEvidence []CollectorScrapeEvidence, attributes map[string]string) EndpointDiagnostic {
	result := EndpointDiagnostic{ProfileID: profile.ID, State: EndpointMissing, EvidenceSource: EvidenceAgentInventory}
	strategies := make([]EndpointStrategy, 0, len(profile.EndpointStrategies))
	for _, strategy := range profile.EndpointStrategies {
		if strategy.OS == osName {
			strategies = append(strategies, strategy)
		}
	}
	if len(strategies) > 1 {
		for i := 0; i < len(strategies)-1; i++ {
			for j := i + 1; j < len(strategies); j++ {
				if strategies[j].Priority > strategies[i].Priority {
					strategies[i], strategies[j] = strategies[j], strategies[i]
				}
			}
		}
	}
	for _, strategy := range strategies {
		targetPort, err := strategyTargetPort(strategy, attributes)
		if err != nil {
			result.State = EndpointInvalid
			result.EvidenceCode = "invalid_target_port"
			return result
		}
		candidates := make([]discovery.Endpoint, 0, len(endpoints))
		for _, endpoint := range endpoints {
			if endpoint.Scope != strategy.Scope || endpoint.Provenance != strategy.Provenance || !strings.EqualFold(endpoint.Protocol, "tcp") || strings.TrimSpace(endpoint.Address) == "" {
				continue
			}
			if targetPort != 0 {
				containerPort := endpoint.ContainerPort
				if containerPort == 0 && endpoint.Scope == discovery.EndpointContainer {
					containerPort = endpoint.Port
				}
				if containerPort != targetPort {
					continue
				}
			}
			candidates = append(candidates, endpoint)
		}
		if len(candidates) == 0 {
			continue
		}
		result.CandidateCount = len(candidates)
		if len(candidates) > 1 {
			if strategy.Scope != discovery.EndpointContainer || strategy.Provenance != "container_network" {
				result.State = EndpointAmbiguous
				return result
			}
			for _, candidate := range candidates {
				if candidate.NetworkName == "" {
					result.State = EndpointAmbiguous
					return result
				}
			}
			sort.Slice(candidates, func(i, j int) bool {
				if candidates[i].NetworkPriority != candidates[j].NetworkPriority {
					return candidates[i].NetworkPriority > candidates[j].NetworkPriority
				}
				if candidates[i].NetworkName == candidates[j].NetworkName {
					return candidates[i].Address < candidates[j].Address
				}
				return candidates[i].NetworkName < candidates[j].NetworkName
			})
		}
		result.Endpoint = candidates[0]
		result.State = EndpointResolved
		receiverID := profile.ReceiverType + "/" + strategy.ID
		for _, evidence := range otelEvidence {
			if evidence.ReceiverID == receiverID && evidence.FailureCode == "connection_failed" && evidenceMatchesEndpoint(evidence.Endpoint, candidates[0]) {
				result.State = EndpointUnreachable
				result.EvidenceSource = EvidenceCollectorScrapeError
				result.EvidenceCode = evidence.FailureCode
				result.EvidenceTime = evidence.ObservedAt
				return result
			}
		}
		return result
	}
	return result
}

func strategyTargetPort(strategy EndpointStrategy, attributes map[string]string) (uint16, error) {
	if strategy.TargetPortLabel == "" {
		return strategy.TargetPort, nil
	}
	value := attributes["label."+strategy.TargetPortLabel]
	if value == "" {
		return strategy.TargetPort, nil
	}
	if len(value) > 5 {
		return 0, fmt.Errorf("target port exceeds valid TCP range")
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return 0, fmt.Errorf("target port must contain ASCII decimal digits")
		}
	}
	port, err := strconv.ParseUint(value, 10, 16)
	if err != nil || port == 0 {
		return 0, fmt.Errorf("target port is outside the valid TCP range")
	}
	return uint16(port), nil
}

// ParseCollectorLog extracts a per-endpoint failure only from OTel's Redis scrape error record.
func ParseCollectorLog(line string) (CollectorScrapeEvidence, bool) {
	if !strings.Contains(line, "Error scraping metrics") {
		return CollectorScrapeEvidence{}, false
	}
	jsonStart := strings.IndexByte(line, '{')
	if jsonStart < 0 {
		return CollectorScrapeEvidence{}, false
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(line[jsonStart:]), &fields); err != nil {
		return CollectorScrapeEvidence{}, false
	}
	componentID, _ := fields["otelcol.component.id"].(string)
	receiverID := componentID
	if componentID == "receiver_creator" {
		dynamicName, _ := fields["name"].(string)
		const receiverCreatorMarker = "/receiver_creator{"
		marker := strings.Index(dynamicName, receiverCreatorMarker)
		if marker <= 0 {
			return CollectorScrapeEvidence{}, false
		}
		receiverID = dynamicName[:marker]
	}
	signal, _ := fields["otelcol.signal"].(string)
	message, _ := fields["error"].(string)
	if signal != "metrics" || !strings.Contains(receiverID, "/") || !isNetworkScrapeFailure(message) {
		return CollectorScrapeEvidence{}, false
	}
	endpoint, ok := scrapeFailureEndpoint(message)
	if !ok {
		return CollectorScrapeEvidence{}, false
	}
	return CollectorScrapeEvidence{ReceiverID: receiverID, Endpoint: endpoint, FailureCode: "connection_failed"}, true
}

func isNetworkScrapeFailure(message string) bool {
	return strings.Contains(strings.ToLower(message), "dial tcp ")
}

func scrapeFailureEndpoint(message string) (string, bool) {
	const marker = "dial tcp "
	start := strings.Index(strings.ToLower(message), marker)
	if start < 0 {
		return "", false
	}
	start += len(marker)
	rest := message[start:]
	end := strings.Index(rest, ": ")
	if end < 0 {
		return "", false
	}
	endpoint := rest[:end]
	if _, _, err := net.SplitHostPort(endpoint); err != nil {
		return "", false
	}
	return endpoint, true
}

func evidenceMatchesEndpoint(target string, candidate discovery.Endpoint) bool {
	host, portText, err := net.SplitHostPort(target)
	if err != nil {
		return false
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || uint16(port) != candidate.Port {
		return false
	}
	if candidate.Scope == discovery.EndpointHost {
		// Docker Desktop rules intentionally dial loopback regardless of the host bind address.
		return host == "127.0.0.1" || host == "localhost" || host == candidate.Address
	}
	return host == candidate.Address
}
