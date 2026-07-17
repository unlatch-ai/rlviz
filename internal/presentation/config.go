// Package presentation defines the non-executable viewer customization contract.
package presentation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

const (
	APIVersion = "rlviz.dev/v1alpha1"
	MaxBytes   = 64 * 1024
)

var fieldID = regexp.MustCompile(`^(reward|pass|status|termination|events|errors|tokens|latency|signal:[A-Za-z0-9][A-Za-z0-9._-]{0,127})$`)
var scalarFieldID = regexp.MustCompile(`^(reward|events|errors|tokens|latency|signal:[A-Za-z0-9][A-Za-z0-9._-]{0,127})$`)

type Config struct {
	APIVersion string                  `json:"api_version"`
	Fields     map[string]Field        `json:"fields,omitempty"`
	Scalars    map[string]ScalarFormat `json:"scalars,omitempty"`
	Group      GroupDefaults           `json:"group,omitempty"`
	Theme      map[string]string       `json:"theme,omitempty"`
}

type Field struct {
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
}

type ScalarFormat struct {
	Format    string `json:"format"`
	Precision *int   `json:"precision,omitempty"`
	Unit      string `json:"unit,omitempty"`
}

type GroupDefaults struct {
	Columns []string `json:"columns,omitempty"`
}

var themeDefaults = map[string]string{
	"surface_canvas": "#090b0e", "surface_panel": "#0d1114", "surface_raised": "#12161a", "surface_overlay": "#191e23",
	"border_subtle": "#22282f", "border_strong": "#303840", "text_primary": "#dce2ea", "text_secondary": "#a2adb9",
	"text_muted": "#7f8b98", "text_faint": "#606b77", "focus": "#8be6d0", "selection": "#54d4b5",
	"success": "#54d4b5", "info": "#78adff", "warning": "#e8b968", "danger": "#ff7580", "context_change": "#b49cff",
}

// Load reads one strict, bounded JSON presentation document. It never executes
// code and deliberately does not accept YAML, CSS, selectors, URLs, or HTML.
func Load(reader io.Reader) (Config, error) {
	var config Config
	data, err := io.ReadAll(io.LimitReader(reader, MaxBytes+1))
	if err != nil {
		return config, err
	}
	if len(data) > MaxBytes {
		return config, fmt.Errorf("presentation configuration exceeds %d bytes", MaxBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return config, fmt.Errorf("invalid presentation configuration: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return config, errors.New("presentation configuration contains multiple JSON values")
	} else if !errors.Is(err, io.EOF) {
		return config, fmt.Errorf("invalid trailing JSON: %w", err)
	}
	if err := config.Validate(); err != nil {
		return config, err
	}
	return config, nil
}

// Normalize validates config and returns its deterministic JSON representation.
// Callers use this at process boundaries so only the bounded contract, never
// source file bytes or paths, crosses into the daemon and browser APIs.
func Normalize(config Config) (json.RawMessage, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("normalize presentation configuration: %w", err)
	}
	return json.RawMessage(data), nil
}

// NormalizeJSON independently decodes, validates, and normalizes JSON received
// across a process or HTTP boundary. JSON null means no presentation config.
func NormalizeJSON(data json.RawMessage) (json.RawMessage, error) {
	if len(bytes.TrimSpace(data)) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, nil
	}
	config, err := Load(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return Normalize(config)
}

func (config Config) Validate() error {
	if config.APIVersion != APIVersion {
		return fmt.Errorf("api_version must be %q", APIVersion)
	}
	if len(config.Fields) > 64 || len(config.Scalars) > 64 {
		return errors.New("fields and scalars may each contain at most 64 entries")
	}
	for id, field := range config.Fields {
		if err := validateFieldID(id); err != nil {
			return fmt.Errorf("fields: %w", err)
		}
		if len([]rune(field.Label)) > 48 {
			return fmt.Errorf("field %q label exceeds 48 characters", id)
		}
		if len([]rune(field.Description)) > 240 {
			return fmt.Errorf("field %q description exceeds 240 characters", id)
		}
		if field.Label == "" && field.Description == "" {
			return fmt.Errorf("field %q must declare label or description", id)
		}
		if unsafeText(field.Label) || unsafeText(field.Description) {
			return fmt.Errorf("field %q contains control characters", id)
		}
	}
	for id, scalar := range config.Scalars {
		if !scalarFieldID.MatchString(id) {
			return fmt.Errorf("scalars: invalid scalar field id %q", id)
		}
		if scalar.Format != "number" && scalar.Format != "integer" && scalar.Format != "percent_fraction" && scalar.Format != "duration_ms" && scalar.Format != "bytes" && scalar.Format != "scientific" {
			return fmt.Errorf("scalar %q has unsupported format %q", id, scalar.Format)
		}
		if scalar.Precision != nil && (*scalar.Precision < 0 || *scalar.Precision > 6) {
			return fmt.Errorf("scalar %q precision must be between 0 and 6", id)
		}
		if len([]rune(scalar.Unit)) > 16 || unsafeText(scalar.Unit) {
			return fmt.Errorf("scalar %q unit must be at most 16 characters without controls", id)
		}
	}
	if len(config.Group.Columns) > 32 {
		return errors.New("group.columns may contain at most 32 entries")
	}
	seen := map[string]bool{}
	for _, id := range config.Group.Columns {
		if err := validateFieldID(id); err != nil {
			return fmt.Errorf("group.columns: %w", err)
		}
		if seen[id] {
			return fmt.Errorf("group.columns contains duplicate %q", id)
		}
		seen[id] = true
	}
	if err := validateTheme(config.Theme); err != nil {
		return err
	}
	return nil
}

func validateFieldID(id string) error {
	if !fieldID.MatchString(id) {
		return fmt.Errorf("invalid field id %q", id)
	}
	return nil
}

func unsafeText(value string) bool {
	return strings.ContainsFunc(value, unicode.IsControl)
}

func validateTheme(overrides map[string]string) error {
	if len(overrides) > len(themeDefaults) {
		return errors.New("theme has too many token overrides")
	}
	resolved := make(map[string]string, len(themeDefaults))
	for key, value := range themeDefaults {
		resolved[key] = value
	}
	for key, value := range overrides {
		if _, ok := themeDefaults[key]; !ok {
			return fmt.Errorf("unsupported semantic theme token %q", key)
		}
		if _, err := rgb(value); err != nil {
			return fmt.Errorf("theme token %q: %w", key, err)
		}
		resolved[key] = strings.ToLower(value)
	}
	for _, check := range []struct {
		foreground, background string
		ratio                  float64
	}{
		{"text_primary", "surface_canvas", 4.5}, {"text_primary", "surface_panel", 4.5}, {"text_primary", "surface_raised", 4.5},
		{"text_secondary", "surface_canvas", 4.5}, {"text_secondary", "surface_panel", 4.5}, {"text_secondary", "surface_raised", 4.5},
		{"text_muted", "surface_canvas", 4.5}, {"text_muted", "surface_panel", 4.5},
		{"focus", "surface_canvas", 3}, {"focus", "surface_panel", 3}, {"focus", "surface_raised", 3},
		{"success", "surface_canvas", 3}, {"success", "surface_panel", 3},
		{"warning", "surface_canvas", 3}, {"warning", "surface_panel", 3},
		{"danger", "surface_canvas", 3}, {"danger", "surface_panel", 3},
	} {
		if contrast(resolved[check.foreground], resolved[check.background]) < check.ratio {
			return fmt.Errorf("theme tokens %s and %s do not meet %.1f:1 contrast", check.foreground, check.background, check.ratio)
		}
	}
	return nil
}

func rgb(value string) ([3]float64, error) {
	var result [3]float64
	if len(value) != 7 || value[0] != '#' {
		return result, errors.New("must be an opaque six-digit hex color")
	}
	for index := range 3 {
		component, err := strconv.ParseUint(value[1+index*2:3+index*2], 16, 8)
		if err != nil {
			return result, errors.New("must be an opaque six-digit hex color")
		}
		channel := float64(component) / 255
		if channel <= .04045 {
			result[index] = channel / 12.92
		} else {
			result[index] = math.Pow((channel+.055)/1.055, 2.4)
		}
	}
	return result, nil
}

func contrast(a, b string) float64 {
	left, _ := rgb(a)
	right, _ := rgb(b)
	luminance := func(c [3]float64) float64 { return .2126*c[0] + .7152*c[1] + .0722*c[2] }
	l1, l2 := luminance(left), luminance(right)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + .05) / (l2 + .05)
}
