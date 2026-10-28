package identity

import (
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

// InstallationID identifies one installed Agent independently of its host.
type InstallationID struct {
	value string
}

// String returns the canonical UUID representation of the installation ID.
func (id InstallationID) String() string {
	return id.value
}

// NewInstallationID creates a random UUIDv4 from the supplied entropy source.
func NewInstallationID(entropy io.Reader) (InstallationID, error) {
	if entropy == nil {
		return InstallationID{}, fmt.Errorf("installation identity entropy source is required")
	}
	var value [16]byte
	if _, err := io.ReadFull(entropy, value[:]); err != nil {
		return InstallationID{}, fmt.Errorf("read installation identity entropy: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return InstallationID{value: formatUUID(value[:])}, nil
}

// ParseInstallationID validates a UUID string and stores its canonical form.
func ParseInstallationID(value string) (InstallationID, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != 36 || value[8] != '-' || value[13] != '-' ||
		value[18] != '-' || value[23] != '-' {
		return InstallationID{}, fmt.Errorf("installation identity must be a UUID")
	}
	compact := strings.ReplaceAll(value, "-", "")
	decoded, err := hex.DecodeString(compact)
	if err != nil || len(decoded) != 16 {
		return InstallationID{}, fmt.Errorf("installation identity must be a UUID")
	}
	return InstallationID{value: value}, nil
}

func formatUUID(value []byte) string {
	encoded := hex.EncodeToString(value)
	return encoded[:8] + "-" + encoded[8:12] + "-" +
		encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}
