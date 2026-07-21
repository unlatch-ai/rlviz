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
	"sort"
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
	Inspector  *InspectorDefaults      `json:"inspector,omitempty"`
	Keymap     *KeymapDefaults         `json:"keymap,omitempty"`
	Theme      map[string]string       `json:"theme,omitempty"`
	Palette    *Palette                `json:"palette,omitempty"`
	Notices    []string                `json:"notices,omitempty"`
}

// Palette is the bounded, mode-aware design token override contract. Load
// resolves partial variants against either the default or named built-in
// palette before the configuration crosses a process boundary.
type Palette struct {
	Name  string            `json:"name,omitempty"`
	Light map[string]string `json:"light,omitempty"`
	Dark  map[string]string `json:"dark,omitempty"`
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

type InspectorDefaults struct {
	Sections []string `json:"sections,omitempty"`
}

type KeymapDefaults struct {
	Bindings map[string][]string `json:"bindings,omitempty"`
}

var commandIDs = map[string]bool{
	"trajectory.dismiss": true, "trajectory.search": true, "trajectory.next": true, "trajectory.previous": true,
	"trajectory.nextError": true, "trajectory.nextReward": true, "trajectory.nextContext": true, "trajectory.nextFinding": true,
	"trajectory.nextArtifact": true, "trajectory.toggleRaw": true, "trajectory.openGroup": true, "trajectory.toggleHelp": true,
	"trajectory.toggleExpanded": true, "trajectory.openTranscript": true, "trajectory.openTimeline": true, "trajectory.openOutcome": true,
	"trajectory.nextRollout": true, "trajectory.previousRollout": true, "trajectory.ascend": true, "trajectory.markIn": true, "trajectory.markOut": true,
	"trajectory.goto": true, "trajectory.replay": true, "trajectory.pivotAggregate": true, "trajectory.dropMarker": true, "trajectory.cycleMarkers": true,
	"view.fidelityUp": true, "view.fidelityDown": true, "view.zoomIn": true, "view.zoomOut": true, "view.zoomFit": true, "view.toggleHelp": true,
	"group.back": true, "group.togglePaths": true, "group.search": true, "group.next": true, "group.previous": true,
	"group.open": true, "group.toggleCompare": true, "group.compare": true, "group.best": true, "group.median": true,
	"group.worst": true, "group.rewardOutlier": true, "group.nextFailure": true, "group.nextInfraFailure": true, "group.toggleColumns": true,
	"group.tagVerdict1": true, "group.tagVerdict2": true, "group.tagVerdict3": true, "group.tagVerdict4": true,
	"paths.back": true, "paths.togglePaths": true, "paths.next": true, "paths.previous": true, "paths.open": true,
	"comparison.back": true, "comparison.next": true, "comparison.previous": true, "comparison.firstDivergence": true, "comparison.nextChange": true, "comparison.toggleDivergenceCurve": true,
}

var commandDefaults = map[string][]string{
	"trajectory.dismiss": {"Escape"}, "trajectory.search": {"/"}, "trajectory.next": {"j"}, "trajectory.previous": {"k"},
	"trajectory.nextError": {"e"}, "trajectory.nextReward": {"r"}, "trajectory.nextContext": {"c"}, "trajectory.nextFinding": {"a"},
	"trajectory.nextArtifact": {"o"}, "trajectory.toggleRaw": {"x"}, "trajectory.openGroup": {"g"}, "trajectory.toggleHelp": {"?"},
	"trajectory.toggleExpanded": {"Enter", "Space"}, "trajectory.openTranscript": {"1"}, "trajectory.openTimeline": {"2"}, "trajectory.openOutcome": {"3"},
	"trajectory.nextRollout": {"n"}, "trajectory.previousRollout": {"p"}, "trajectory.ascend": {"Escape"}, "trajectory.markIn": {"i"}, "trajectory.markOut": {"Shift+O"},
	"trajectory.goto": {":"}, "trajectory.replay": {"Shift+R"}, "trajectory.pivotAggregate": {"."}, "trajectory.dropMarker": {"m"}, "trajectory.cycleMarkers": {"Shift+M"},
	"view.fidelityUp": {"]"}, "view.fidelityDown": {"["}, "view.zoomIn": {"+"}, "view.zoomOut": {"-"}, "view.zoomFit": {"0"}, "view.toggleHelp": {"?"},
	"group.back": {"Escape"}, "group.togglePaths": {"p"}, "group.search": {"/"}, "group.next": {"j", "ArrowDown"},
	"group.previous": {"k", "ArrowUp"}, "group.open": {"Enter", "o"}, "group.toggleCompare": {"Space", "c"}, "group.compare": {"v"},
	"group.best": {"b"}, "group.median": {"m"}, "group.worst": {"w"}, "group.rewardOutlier": {"u"}, "group.nextFailure": {"f"},
	"group.nextInfraFailure": {"i"}, "group.toggleColumns": {"Shift+C"},
	"group.tagVerdict1": {"1"}, "group.tagVerdict2": {"2"}, "group.tagVerdict3": {"3"}, "group.tagVerdict4": {"4"},
	"paths.back": {"Escape"}, "paths.togglePaths": {"p"}, "paths.next": {"j", "ArrowDown"}, "paths.previous": {"k", "ArrowUp"}, "paths.open": {"Enter", "o"},
	"comparison.back": {"Escape"}, "comparison.next": {"j", "ArrowDown"}, "comparison.previous": {"k", "ArrowUp"},
	"comparison.firstDivergence": {"d"}, "comparison.nextChange": {"n"}, "comparison.toggleDivergenceCurve": {"Shift+D"},
}

var inspectorSectionIDs = map[string]bool{
	"properties": true, "context": true, "source": true, "input": true, "output": true,
	"content": true, "metadata": true, "linked_artifacts": true, "analysis": true, "other_artifacts": true,
}

var themeDefaults = map[string]string{
	"surface_canvas": "#090b0e", "surface_panel": "#0d1114", "surface_raised": "#12161a", "surface_overlay": "#191e23",
	"border_subtle": "#22282f", "border_strong": "#303840", "text_primary": "#dce2ea", "text_secondary": "#a2adb9",
	"text_muted": "#7f8b98", "text_faint": "#606b77", "focus": "#8be6d0", "selection": "#54d4b5",
	"success": "#54d4b5", "info": "#78adff", "warning": "#e8b968", "danger": "#ff7580", "context_change": "#b49cff",
}

var paletteDefaults = map[string]map[string]string{
	"light": {
		"ctx": "#2a78d6", "failPolicy": "#d03b3b", "failInfra": "#ec835a", "good": "#006300",
		"page": "#f9f9f7", "surface": "#fcfcfb", "ink": "#0b0b0b", "inkSecondary": "#52514e", "muted": "#898781", "hairline": "#e1e0d9",
	},
	"dark": {
		"ctx": "#3987e5", "failPolicy": "#d03b3b", "failInfra": "#ec835a", "good": "#0ca30c",
		"page": "#0d0d0d", "surface": "#1a1a19", "ink": "#ffffff", "inkSecondary": "#c3c2b7", "muted": "#898781", "hairline": "#2c2c2a",
	},
}

var highContrastPalette = map[string]map[string]string{
	"light": {
		"ctx": "#005fcc", "failPolicy": "#b00020", "failInfra": "#b54708", "good": "#005a00",
		"page": "#ffffff", "surface": "#ffffff", "ink": "#000000", "inkSecondary": "#333333", "muted": "#666666", "hairline": "#a0a0a0",
	},
	"dark": {
		"ctx": "#66aaff", "failPolicy": "#ff5c5c", "failInfra": "#ff9a6c", "good": "#42d642",
		"page": "#000000", "surface": "#101010", "ink": "#ffffff", "inkSecondary": "#dddddd", "muted": "#a0a0a0", "hairline": "#666666",
	},
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
	if err := resolvePalette(&config); err != nil {
		return config, err
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
	if err := resolvePalette(&config); err != nil {
		return nil, err
	}
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
	if len(config.Notices) > 4 {
		return errors.New("notices may contain at most four entries")
	}
	for _, notice := range config.Notices {
		if notice == "" || len([]rune(notice)) > 240 || unsafeText(notice) {
			return errors.New("notices must be non-empty, at most 240 characters, and contain no controls")
		}
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
	if config.Inspector != nil {
		if config.Inspector.Sections != nil && len(config.Inspector.Sections) == 0 {
			return errors.New("inspector.sections must contain at least one section")
		}
		if len(config.Inspector.Sections) > len(inspectorSectionIDs) {
			return fmt.Errorf("inspector.sections may contain at most %d entries", len(inspectorSectionIDs))
		}
		seen = map[string]bool{}
		for _, id := range config.Inspector.Sections {
			if !inspectorSectionIDs[id] {
				return fmt.Errorf("inspector.sections contains unsupported section %q", id)
			}
			if seen[id] {
				return fmt.Errorf("inspector.sections contains duplicate %q", id)
			}
			seen[id] = true
		}
	}
	if config.Keymap != nil {
		if len(config.Keymap.Bindings) > len(commandIDs) {
			return fmt.Errorf("keymap.bindings may contain at most %d commands", len(commandIDs))
		}
		for id, bindings := range config.Keymap.Bindings {
			if !commandIDs[id] {
				return fmt.Errorf("keymap.bindings contains unsupported command %q", id)
			}
			if len(bindings) == 0 || len(bindings) > 4 {
				return fmt.Errorf("keymap binding %q must contain between one and four keys", id)
			}
			seenBindings := map[string]bool{}
			for _, binding := range bindings {
				trimmed := strings.TrimSpace(binding)
				if !validKeyBinding(trimmed) {
					return fmt.Errorf("keymap binding %q contains an invalid key", id)
				}
				normalized := normalizeKeyBinding(trimmed)
				if seenBindings[normalized] {
					return fmt.Errorf("keymap binding %q contains duplicate %q", id, trimmed)
				}
				seenBindings[normalized] = true
			}
		}
		resolved := make(map[string][]string, len(commandDefaults))
		for id, bindings := range commandDefaults {
			resolved[id] = bindings
		}
		for id, bindings := range config.Keymap.Bindings {
			resolved[id] = bindings
		}
		occupied := map[string]string{}
		ids := make([]string, 0, len(resolved))
		for id := range resolved {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			bindings := resolved[id]
			for _, scope := range commandScopes(id) {
				for _, binding := range bindings {
					for _, collisionKey := range keyBindingCollisionKeys(binding) {
						key := scope + "\x00" + collisionKey
						if prior := occupied[key]; prior != "" && prior != id {
							return fmt.Errorf("keymap binding %q conflicts between %q and %q", strings.TrimSpace(binding), prior, id)
						}
						occupied[key] = id
					}
				}
			}
		}
	}
	if err := validateTheme(config.Theme); err != nil {
		return err
	}
	if err := validateResolvedPalette(config.Palette); err != nil {
		return err
	}
	return nil
}

var paletteTokenNames = map[string]bool{
	"ctx": true, "failPolicy": true, "failInfra": true, "good": true,
	"page": true, "surface": true, "ink": true, "inkSecondary": true, "muted": true, "hairline": true,
}

func clonePalette(source map[string]map[string]string) *Palette {
	result := &Palette{Light: map[string]string{}, Dark: map[string]string{}}
	for key, value := range source["light"] {
		result.Light[key] = value
	}
	for key, value := range source["dark"] {
		result.Dark[key] = value
	}
	return result
}

func resolvePalette(config *Config) error {
	if config.Palette == nil {
		return nil
	}
	input := config.Palette
	var resolved *Palette
	switch input.Name {
	case "":
		resolved = clonePalette(paletteDefaults)
	case "high-contrast":
		resolved = clonePalette(highContrastPalette)
		resolved.Name = input.Name
	default:
		return fmt.Errorf("unsupported built-in palette %q", input.Name)
	}
	for mode, overrides := range map[string]map[string]string{"light": input.Light, "dark": input.Dark} {
		target := resolved.Light
		if mode == "dark" {
			target = resolved.Dark
		}
		for token, value := range overrides {
			if !paletteTokenNames[token] {
				return fmt.Errorf("palette.%s contains unsupported token %q", mode, token)
			}
			normalized, err := normalizePaletteColor(value)
			if err != nil {
				config.Palette = nil
				config.Notices = append(config.Notices, "Palette ignored because it contains an invalid hex color; built-in defaults are active.")
				return nil
			}
			target[token] = normalized
		}
	}
	config.Palette = resolved
	return nil
}

func normalizePaletteColor(value string) (string, error) {
	if len(value) == 4 && value[0] == '#' {
		value = fmt.Sprintf("#%c%c%c%c%c%c", value[1], value[1], value[2], value[2], value[3], value[3])
	}
	if _, err := rgb(value); err != nil {
		return "", errors.New("must be an opaque three- or six-digit hex color")
	}
	return strings.ToLower(value), nil
}

func validateResolvedPalette(palette *Palette) error {
	if palette == nil {
		return nil
	}
	for mode, values := range map[string]map[string]string{"light": palette.Light, "dark": palette.Dark} {
		if len(values) != len(paletteTokenNames) {
			return fmt.Errorf("palette.%s must resolve all semantic tokens", mode)
		}
		for token, value := range values {
			if !paletteTokenNames[token] {
				return fmt.Errorf("palette.%s contains unsupported token %q", mode, token)
			}
			if _, err := rgb(value); err != nil {
				return fmt.Errorf("palette.%s token %q: %w", mode, token, err)
			}
		}
	}
	return nil
}

func commandScopes(id string) []string {
	if id == "trajectory.dismiss" || id == "trajectory.toggleHelp" {
		return []string{"overlay"}
	}
	if strings.HasPrefix(id, "view.") {
		return []string{"trajectory", "group", "paths", "comparison"}
	}
	return []string{strings.SplitN(id, ".", 2)[0]}
}

func validKeyBinding(binding string) bool {
	if binding == "" || len([]rune(binding)) > 32 || unsafeText(binding) {
		return false
	}
	parts := strings.Split(binding, "+")
	if strings.TrimSpace(parts[len(parts)-1]) == "" {
		return false
	}
	seen := map[string]bool{}
	for _, modifier := range parts[:len(parts)-1] {
		modifier = strings.ToLower(strings.TrimSpace(modifier))
		if seen[modifier] || (modifier != "mod" && modifier != "ctrl" && modifier != "meta" && modifier != "alt" && modifier != "shift") {
			return false
		}
		seen[modifier] = true
	}
	return true
}

func normalizeKeyBinding(binding string) string {
	parts := strings.Split(binding, "+")
	key := strings.TrimSpace(parts[len(parts)-1])
	if strings.EqualFold(key, "esc") || strings.EqualFold(key, "escape") {
		key = "escape"
	} else if key == " " || strings.EqualFold(key, "space") {
		key = "space"
	} else {
		key = strings.ToLower(key)
	}
	modifiers := map[string]bool{}
	for _, modifier := range parts[:len(parts)-1] {
		modifiers[strings.ToLower(strings.TrimSpace(modifier))] = true
	}
	ordered := make([]string, 0, 6)
	for _, modifier := range []string{"mod", "ctrl", "meta", "alt", "shift"} {
		if modifiers[modifier] {
			ordered = append(ordered, modifier)
		}
	}
	return strings.Join(append(ordered, key), "+")
}

func keyBindingCollisionKeys(binding string) []string {
	normalized := normalizeKeyBinding(binding)
	if strings.HasPrefix(normalized, "mod+") {
		return []string{strings.Replace(normalized, "mod+", "ctrl+", 1), strings.Replace(normalized, "mod+", "meta+", 1)}
	}
	return []string{normalized}
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
