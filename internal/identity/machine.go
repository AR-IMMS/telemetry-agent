package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"runtime"
	"strings"
)

const machineIDFingerprintLength = 24

type MachineIdentity struct {
	HostName string
	HostID   string
}

type hostNameReader func() (string, error)

type rawMachineIDReader func() (string, error)

// CollectMachineIdentity returns non-secret, stable machine identity attributes
// suitable for telemetry resources. It never returns the raw OS machine ID.
func CollectMachineIdentity() (MachineIdentity, error) {
	return collectMachineIdentity(
		runtime.GOOS,
		os.Hostname,
		readRawMachineID,
	)
}

func collectMachineIdentity(
	operatingSystem string,
	readHostName hostNameReader,
	readRawMachineID rawMachineIDReader,
) (MachineIdentity, error) {
	if readHostName == nil {
		return MachineIdentity{}, fmt.Errorf("machine hostname reader is required")
	}

	if readRawMachineID == nil {
		return MachineIdentity{}, fmt.Errorf("raw machine ID reader is required")
	}

	hostName, err := readHostName()
	if err != nil {
		return MachineIdentity{}, fmt.Errorf(
			"read machine hostname: %w",
			err,
		)
	}

	hostName = strings.TrimSpace(hostName)
	if hostName == "" {
		return MachineIdentity{}, fmt.Errorf(
			"machine hostname is required",
		)
	}

	rawMachineID, err := readRawMachineID()
	if err != nil {
		return MachineIdentity{}, fmt.Errorf(
			"read raw machine ID: %w",
			err,
		)
	}

	hostID, err := machineIDFingerprint(
		operatingSystem,
		rawMachineID,
	)
	if err != nil {
		return MachineIdentity{}, err
	}

	return MachineIdentity{
		HostName: hostName,
		HostID:   hostID,
	}, nil
}

func machineIDFingerprint(
	operatingSystem string,
	rawMachineID string,
) (string, error) {
	operatingSystem = strings.ToLower(
		strings.TrimSpace(operatingSystem),
	)
	rawMachineID = strings.ToLower(
		strings.TrimSpace(rawMachineID),
	)

	if operatingSystem == "" {
		return "", fmt.Errorf("machine operating system is required")
	}

	if rawMachineID == "" {
		return "", fmt.Errorf("raw machine ID is required")
	}

	sum := sha256.Sum256([]byte(rawMachineID))
	hash := hex.EncodeToString(sum[:])

	return operatingSystem + "-" +
		hash[:machineIDFingerprintLength], nil
}
