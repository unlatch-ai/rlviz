package daemon

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	ErrNoMetadata       = errors.New("daemon metadata does not exist")
	ErrInsecureMetadata = errors.New("daemon metadata permissions are not user-only")
)

const tokenBytes = 32

const maxMetadataBytes = 64 << 10

// Metadata is the private rendezvous record written by a running daemon.
type Metadata struct {
	PID     int    `json:"pid"`
	Address string `json:"address"`
	Token   string `json:"token"`
	Version string `json:"version"`
}

// GenerateToken returns a URL-safe 256-bit daemon secret.
func GenerateToken() (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate daemon token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// Validate rejects incomplete records and addresses that are not numeric
// loopback endpoints. Hostnames are intentionally not resolved.
func (metadata Metadata) Validate() error {
	if metadata.PID <= 0 {
		return fmt.Errorf("daemon pid must be positive")
	}
	if err := ValidateLoopbackAddress(metadata.Address); err != nil {
		return err
	}
	raw, err := base64.RawURLEncoding.DecodeString(metadata.Token)
	if err != nil || len(raw) < tokenBytes {
		return fmt.Errorf("daemon token must contain at least %d random bytes", tokenBytes)
	}
	if strings.TrimSpace(metadata.Version) == "" {
		return fmt.Errorf("daemon version is empty")
	}
	return nil
}

// ValidateLoopbackAddress accepts only an IP literal and port on a loopback
// interface. This prevents stale or tampered metadata from exfiltrating tokens.
func ValidateLoopbackAddress(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid daemon address: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("daemon address must use a numeric loopback IP")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("daemon address must use a numeric port between 1 and 65535")
	}
	return nil
}

// WriteMetadata atomically publishes metadata with user-only permissions.
func WriteMetadata(paths Paths, metadata Metadata) error {
	if err := metadata.Validate(); err != nil {
		return fmt.Errorf("validate daemon metadata: %w", err)
	}
	if err := paths.EnsureRuntimeDir(); err != nil {
		return err
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode daemon metadata: %w", err)
	}
	payload = append(payload, '\n')

	temporary, err := os.CreateTemp(paths.RuntimeDir, ".daemon-*.json")
	if err != nil {
		return fmt.Errorf("create temporary daemon metadata: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure temporary daemon metadata: %w", err)
	}
	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary daemon metadata: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary daemon metadata: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary daemon metadata: %w", err)
	}
	if err := os.Rename(temporaryName, paths.MetadataFile); err != nil {
		return fmt.Errorf("publish daemon metadata: %w", err)
	}
	if err := os.Chmod(paths.MetadataFile, 0o600); err != nil {
		return fmt.Errorf("secure daemon metadata: %w", err)
	}
	return nil
}

// ReadMetadata reads and validates a private daemon rendezvous record.
func ReadMetadata(paths Paths) (Metadata, error) {
	info, err := os.Lstat(paths.MetadataFile)
	if errors.Is(err, os.ErrNotExist) {
		return Metadata{}, ErrNoMetadata
	}
	if err != nil {
		return Metadata{}, fmt.Errorf("inspect daemon metadata: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Metadata{}, fmt.Errorf("daemon metadata is not a regular file")
	}
	if !userOnlyFile(info.Mode()) {
		return Metadata{}, ErrInsecureMetadata
	}
	if info.Size() <= 0 || info.Size() > maxMetadataBytes {
		return Metadata{}, fmt.Errorf("daemon metadata size is invalid")
	}
	payload, err := os.ReadFile(paths.MetadataFile)
	if err != nil {
		return Metadata{}, fmt.Errorf("read daemon metadata: %w", err)
	}
	var metadata Metadata
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&metadata); err != nil {
		return Metadata{}, fmt.Errorf("decode daemon metadata: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Metadata{}, fmt.Errorf("decode daemon metadata: trailing content")
	}
	if err := metadata.Validate(); err != nil {
		return Metadata{}, fmt.Errorf("validate daemon metadata: %w", err)
	}
	return metadata, nil
}

// RemoveMetadata removes the rendezvous record. Absence is not an error.
func RemoveMetadata(paths Paths) error {
	err := os.Remove(filepath.Clean(paths.MetadataFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("remove daemon metadata: %w", err)
	}
	return nil
}
