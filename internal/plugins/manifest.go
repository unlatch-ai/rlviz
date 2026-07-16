// Package plugins implements the external RolloutViz adapter boundary.
package plugins

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	APIVersion   = "rolloutviz.dev/v1alpha1"
	ManifestName = "rolloutviz-plugin.yaml"
)

var (
	pluginName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
	semver     = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
)

type Manifest struct {
	APIVersion   string   `json:"api_version"`
	Kind         string   `json:"kind"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Command      []string `json:"command"`
	Capabilities []string `json:"capabilities"`
	Description  string   `json:"description,omitempty"`
}

type Plugin struct {
	Path         string
	ManifestPath string
	Manifest     Manifest
	Digest       string
}

// Load resolves a plugin directory, parses its manifest, validates it, and
// computes the digest used by the trust store.
func Load(path string) (*Plugin, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("resolve plugin path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	manifestPath := abs
	if info.IsDir() {
		manifestPath, err = findManifest(abs)
		if err != nil {
			return nil, err
		}
		info, err = os.Stat(manifestPath)
		if err != nil {
			return nil, err
		}
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("plugin manifest must be a regular file")
	}
	root := filepath.Dir(manifestPath)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	m, err := ParseManifest(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", manifestPath, err)
	}
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", manifestPath, err)
	}
	for _, argument := range m.Command {
		if !filepath.IsAbs(argument) {
			continue
		}
		resolved, resolveErr := filepath.EvalSymlinks(argument)
		if resolveErr == nil && within(root, resolved) {
			return nil, fmt.Errorf("plugin command paths inside the plugin must be relative so they can be snapshotted: %s", argument)
		}
	}
	digest, err := ContentDigest(root, manifestPath, m.Command)
	if err != nil {
		return nil, err
	}
	return &Plugin{Path: root, ManifestPath: manifestPath, Manifest: m, Digest: digest}, nil
}

func findManifest(root string) (string, error) {
	for _, name := range []string{ManifestName, "rolloutviz-plugin.yml", "rolloutviz-plugin.json", "plugin.yaml", "plugin.yml", "plugin.json"} {
		path := filepath.Join(root, name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path, nil
		}
	}
	return "", fmt.Errorf("no plugin manifest in %s", root)
}

func ParseManifest(data []byte) (Manifest, error) {
	var m Manifest
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return m, errors.New("manifest is empty")
	}
	if trimmed[0] == '{' {
		dec := json.NewDecoder(bytes.NewReader(trimmed))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&m); err != nil {
			return m, fmt.Errorf("invalid JSON manifest: %w", err)
		}
		var extra any
		if err := dec.Decode(&extra); err == nil {
			return m, errors.New("manifest contains multiple JSON values")
		} else if !errors.Is(err, io.EOF) {
			return m, fmt.Errorf("invalid trailing JSON: %w", err)
		}
		return m, nil
	}
	values, err := parseSimpleYAML(string(data))
	if err != nil {
		return m, err
	}
	known := map[string]bool{"api_version": true, "kind": true, "name": true, "version": true, "command": true, "capabilities": true, "description": true}
	for key := range values {
		if !known[key] {
			return m, fmt.Errorf("unknown manifest field %q", key)
		}
	}
	m.APIVersion = scalar(values, "api_version")
	m.Kind = scalar(values, "kind")
	m.Name = scalar(values, "name")
	m.Version = scalar(values, "version")
	m.Description = scalar(values, "description")
	m.Command, err = list(values, "command")
	if err != nil {
		return m, err
	}
	m.Capabilities, err = list(values, "capabilities")
	if err != nil {
		return m, err
	}
	return m, nil
}

// parseSimpleYAML intentionally accepts only the documented manifest subset:
// top-level scalar keys and string lists (block or JSON-style inline lists).
func parseSimpleYAML(input string) (map[string]any, error) {
	result := map[string]any{}
	var active string
	for number, raw := range strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n") {
		line := strings.TrimRight(raw, " \t\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		text := strings.TrimSpace(line)
		if indent > 0 {
			if active == "" || !strings.HasPrefix(text, "- ") {
				return nil, fmt.Errorf("line %d: expected list item", number+1)
			}
			item, err := yamlString(strings.TrimSpace(strings.TrimPrefix(text, "- ")))
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", number+1, err)
			}
			result[active] = append(result[active].([]string), item)
			continue
		}
		active = ""
		parts := strings.SplitN(text, ":", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, fmt.Errorf("line %d: expected key: value", number+1)
		}
		key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("line %d: duplicate field %q", number+1, key)
		}
		if value == "" {
			result[key] = []string{}
			active = key
			continue
		}
		if strings.HasPrefix(value, "[") {
			var items []string
			if err := json.Unmarshal([]byte(value), &items); err != nil {
				return nil, fmt.Errorf("line %d: inline lists must use JSON string syntax", number+1)
			}
			result[key] = items
		} else {
			s, err := yamlString(value)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", number+1, err)
			}
			result[key] = s
		}
	}
	return result, nil
}

func yamlString(value string) (string, error) {
	if value == "" {
		return "", errors.New("empty string")
	}
	if value[0] == '"' {
		var s string
		if err := json.Unmarshal([]byte(value), &s); err != nil {
			return "", err
		}
		return s, nil
	}
	if value[0] == '\'' {
		if len(value) < 2 || value[len(value)-1] != '\'' {
			return "", errors.New("unterminated quoted string")
		}
		return strings.ReplaceAll(value[1:len(value)-1], "''", "'"), nil
	}
	if strings.Contains(value, " #") {
		value = strings.SplitN(value, " #", 2)[0]
	}
	return strings.TrimSpace(value), nil
}

func scalar(values map[string]any, key string) string {
	if value, ok := values[key].(string); ok {
		return value
	}
	return ""
}
func list(values map[string]any, key string) ([]string, error) {
	value, ok := values[key]
	if !ok {
		return nil, nil
	}
	items, ok := value.([]string)
	if !ok {
		return nil, fmt.Errorf("%s must be a list", key)
	}
	return items, nil
}

func (m Manifest) Validate() error {
	if m.APIVersion != APIVersion {
		return fmt.Errorf("api_version must be %q", APIVersion)
	}
	if m.Kind != "Adapter" {
		return errors.New("kind must be Adapter")
	}
	if !pluginName.MatchString(m.Name) {
		return errors.New("name must contain only lowercase letters, digits, '.', '_' or '-'")
	}
	if !semver.MatchString(m.Version) {
		return errors.New("version must be semantic versioning")
	}
	if len(m.Command) == 0 {
		return errors.New("command must not be empty")
	}
	for _, item := range m.Command {
		if item == "" || strings.ContainsRune(item, '\x00') {
			return errors.New("command items must be non-empty strings without NUL")
		}
	}
	want := map[string]bool{"adapter.probe": false, "adapter.stream": false}
	for _, capability := range m.Capabilities {
		if _, ok := want[capability]; !ok {
			return fmt.Errorf("unsupported capability %q", capability)
		}
		if want[capability] {
			return fmt.Errorf("duplicate capability %q", capability)
		}
		want[capability] = true
	}
	for capability, present := range want {
		if !present {
			return fmt.Errorf("required capability %q is missing", capability)
		}
	}
	return nil
}
