package contracts

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/ar-imms/telemetry-agent/internal/identity/host"
)

// Candidate is an immutable complete Collector configuration produced by Runtime.
type Candidate struct {
	content   []byte
	digest    string
	collector string
	manifest  string
	platform  identity.PlatformInfo
}

// NewCandidate copies the compiled bytes and binds their digest and Runtime provenance.
func NewCandidate(content []byte, collector, manifest string, platform identity.PlatformInfo) Candidate {
	data := append([]byte(nil), content...)
	digest := sha256.Sum256(data)
	return Candidate{content: data, digest: hex.EncodeToString(digest[:]), collector: collector, manifest: manifest, platform: platform}
}

// Bytes returns a copy of the candidate configuration.
func (c Candidate) Bytes() []byte { return append([]byte(nil), c.content...) }

// Digest returns the SHA-256 digest of the exact candidate bytes.
func (c Candidate) Digest() string { return c.digest }

// CollectorVersion returns the pinned Collector version used by the catalog.
func (c Candidate) CollectorVersion() string { return c.collector }

// ManifestVersion returns the capability manifest version used by the catalog.
func (c Candidate) ManifestVersion() string { return c.manifest }

// Platform returns the target platform associated with the candidate.
func (c Candidate) Platform() identity.PlatformInfo { return c.platform }
