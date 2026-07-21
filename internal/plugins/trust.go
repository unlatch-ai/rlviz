package plugins

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
)

var ErrUntrusted = errors.New("plugin is not trusted")

type TrustStore struct {
	Path string
	mu   sync.Mutex
}
type trustData struct {
	Version int               `json:"version"`
	Plugins map[string]string `json:"plugins"`
}

type TrustEntry struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

func DefaultTrustStore() (*TrustStore, error) {
	if override := os.Getenv("RLVIZ_CONFIG_DIR"); override != "" {
		return &TrustStore{Path: filepath.Join(override, "trusted-plugins.json")}, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return &TrustStore{Path: filepath.Join(dir, "rlviz", "trusted-plugins.json")}, nil
}

// Trust records the plugin's resolved absolute path and current digest.
func (s *TrustStore) Trust(plugin *Plugin) error {
	if plugin == nil {
		return errors.New("plugin is nil")
	}
	current, err := Load(plugin.Path)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.read()
	if err != nil {
		return err
	}
	data.Plugins[current.Path] = current.Digest
	return s.write(data)
}

func (s *TrustStore) Revoke(pluginPath string) error {
	abs, err := filepath.Abs(pluginPath)
	if err != nil {
		return err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(abs); resolveErr == nil {
		abs = resolved
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.read()
	if err != nil {
		return err
	}
	delete(data.Plugins, abs)
	return s.write(data)
}

func (s *TrustStore) List() ([]TrustEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.read()
	if err != nil {
		return nil, err
	}
	entries := make([]TrustEntry, 0, len(data.Plugins))
	for path, digest := range data.Plugins {
		entries = append(entries, TrustEntry{Path: path, Digest: digest})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

func (s *TrustStore) IsTrusted(plugin *Plugin) (bool, error) {
	if plugin == nil {
		return false, errors.New("plugin is nil")
	}
	current, err := Load(plugin.Path)
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.read()
	if err != nil {
		return false, err
	}
	return data.Plugins[current.Path] == current.Digest, nil
}

func (s *TrustStore) Require(plugin *Plugin) error {
	ok, err := s.IsTrusted(plugin)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: %s (%s)", ErrUntrusted, plugin.Path, plugin.Digest)
	}
	return nil
}

func (s *TrustStore) read() (trustData, error) {
	data := trustData{Version: 1, Plugins: map[string]string{}}
	info, err := os.Stat(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return data, nil
	}
	if err != nil {
		return data, err
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return data, fmt.Errorf("trust store %s permissions %o are insecure; want 600", s.Path, info.Mode().Perm())
	}
	raw, err := os.ReadFile(s.Path)
	if err != nil {
		return data, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&data); err != nil {
		return data, fmt.Errorf("read trust store: %w", err)
	}
	if data.Version != 1 {
		return data, fmt.Errorf("unsupported trust store version %d", data.Version)
	}
	if data.Plugins == nil {
		data.Plugins = map[string]string{}
	}
	return data, nil
}

func (s *TrustStore) write(data trustData) error {
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp, err := os.CreateTemp(dir, ".trust-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.Path)
}
