package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

func verifyPR07AArtifact(path, expectedSHA256 string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("artifact path is required")
	}
	if len(expectedSHA256) != sha256.Size*2 {
		return fmt.Errorf("expected SHA-256 must be 64 hexadecimal characters")
	}
	expected, err := hex.DecodeString(expectedSHA256)
	if err != nil {
		return fmt.Errorf("decode expected SHA-256: %w", err)
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open artifact: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat artifact: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("artifact is not a regular file")
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("hash artifact: %w", err)
	}
	if !strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), hex.EncodeToString(expected)) {
		return fmt.Errorf("artifact SHA-256 mismatch: got %s", hex.EncodeToString(hasher.Sum(nil)))
	}
	return nil
}

func verifyPR07AExecutable(path, argument string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, path, argument)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("run %s %s: %w: %s", path, argument, err, strings.TrimSpace(string(output)))
	}
	if len(strings.TrimSpace(string(output))) == 0 {
		return fmt.Errorf("%s %s returned no help output", path, argument)
	}
	return nil
}
