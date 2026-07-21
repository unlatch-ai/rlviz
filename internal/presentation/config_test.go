package presentation

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

func TestLoadValidPresentation(t *testing.T) {
	precision := 2
	config, err := Load(strings.NewReader(`{
  "api_version":"rlviz.dev/v1alpha1",
  "fields":{"signal:grader_score":{"label":"Grader","description":"Customer verifier score"}},
  "scalars":{"signal:grader_score":{"format":"percent_fraction","precision":2,"unit":"pass"}},
  "group":{"columns":["pass","reward","signal:grader_score"]},
  "inspector":{"sections":["properties","analysis","source"]},
  "keymap":{"bindings":{"trajectory.next":["j","ArrowDown"],"trajectory.previous":["k","ArrowUp"]}},
  "theme":{"focus":"#8be6d0"}
}`))
	if err != nil {
		t.Fatal(err)
	}
	if config.Scalars["signal:grader_score"].Precision == nil || *config.Scalars["signal:grader_score"].Precision != precision {
		t.Fatalf("unexpected config: %#v", config)
	}
	if config.Inspector == nil || strings.Join(config.Inspector.Sections, ",") != "properties,analysis,source" {
		t.Fatalf("unexpected inspector order: %#v", config.Inspector.Sections)
	}
}

func TestLoadRejectsExecutableOrUnboundedSurfaces(t *testing.T) {
	for name, document := range map[string]string{
		"unknown top-level HTML": `{"api_version":"rlviz.dev/v1alpha1","html":"<b>x</b>"}`,
		"CSS token":              `{"api_version":"rlviz.dev/v1alpha1","theme":{"--anything":"red"}}`,
		"CSS value":              `{"api_version":"rlviz.dev/v1alpha1","theme":{"focus":"var(--danger)"}}`,
		"URL-like field":         `{"api_version":"rlviz.dev/v1alpha1","fields":{"https://example.com":{"label":"x"}}}`,
		"URL-like signal":        `{"api_version":"rlviz.dev/v1alpha1","fields":{"signal:https://example.com":{"label":"x"}}}`,
		"control character":      "{\"api_version\":\"rlviz.dev/v1alpha1\",\"fields\":{\"reward\":{\"label\":\"return\\u007fhidden\"}}}",
		"duplicate column":       `{"api_version":"rlviz.dev/v1alpha1","group":{"columns":["reward","reward"]}}`,
		"nonnumeric scalar":      `{"api_version":"rlviz.dev/v1alpha1","scalars":{"pass":{"format":"integer"}}}`,
		"unknown format":         `{"api_version":"rlviz.dev/v1alpha1","scalars":{"reward":{"format":"template"}}}`,
		"empty inspector":        `{"api_version":"rlviz.dev/v1alpha1","inspector":{"sections":[]}}`,
		"unknown inspector":      `{"api_version":"rlviz.dev/v1alpha1","inspector":{"sections":["properties","html"]}}`,
		"duplicate inspector":    `{"api_version":"rlviz.dev/v1alpha1","inspector":{"sections":["source","source"]}}`,
		"too many inspector":     `{"api_version":"rlviz.dev/v1alpha1","inspector":{"sections":["properties","context","source","input","output","content","metadata","linked_artifacts","analysis","other_artifacts","properties"]}}`,
		"unknown command":        `{"api_version":"rlviz.dev/v1alpha1","keymap":{"bindings":{"trajectory.execute":["q"]}}}`,
		"empty command binding":  `{"api_version":"rlviz.dev/v1alpha1","keymap":{"bindings":{"trajectory.next":[]}}}`,
		"unknown modifier":       `{"api_version":"rlviz.dev/v1alpha1","keymap":{"bindings":{"trajectory.next":["Hyper+j"]}}}`,
		"duplicate modifier":     `{"api_version":"rlviz.dev/v1alpha1","keymap":{"bindings":{"trajectory.next":["Ctrl+Ctrl+j"]}}}`,
		"command conflict":       `{"api_version":"rlviz.dev/v1alpha1","keymap":{"bindings":{"trajectory.next":["k"]}}}`,
		"portable mod conflict":  `{"api_version":"rlviz.dev/v1alpha1","keymap":{"bindings":{"trajectory.next":["Mod+k"],"trajectory.previous":["Ctrl+k"]}}}`,
		"normalized duplicate":   `{"api_version":"rlviz.dev/v1alpha1","keymap":{"bindings":{"trajectory.next":["j","J"]}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(strings.NewReader(document)); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestNormalizePreservesInspectorOrder(t *testing.T) {
	config, err := Load(strings.NewReader(`{"api_version":"rlviz.dev/v1alpha1","inspector":{"sections":["analysis","source","properties"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	data, err := Normalize(config)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"sections":["analysis","source","properties"]`)) {
		t.Fatalf("inspector order was not preserved: %s", data)
	}
}

func TestThemeRequiresReadableSemanticColors(t *testing.T) {
	_, err := Load(strings.NewReader(`{"api_version":"rlviz.dev/v1alpha1","theme":{"text_primary":"#0a0a0a"}}`))
	if err == nil || !strings.Contains(err.Error(), "contrast") {
		t.Fatalf("expected contrast error, got %v", err)
	}
}

func TestPaletteResolvesPartialOverridesForBothModes(t *testing.T) {
	config, err := Load(strings.NewReader(`{"api_version":"rlviz.dev/v1alpha1","palette":{"light":{"ctx":"#ABC"},"dark":{"good":"#123456"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if config.Palette == nil || config.Palette.Light["ctx"] != "#aabbcc" || config.Palette.Dark["good"] != "#123456" {
		t.Fatalf("palette overrides were not normalized: %#v", config.Palette)
	}
	if config.Palette.Light["page"] != "#f9f9f7" || config.Palette.Dark["page"] != "#0d0d0d" {
		t.Fatalf("palette defaults were not filled: %#v", config.Palette)
	}
}

func TestHighContrastPaletteResolvesEndToEnd(t *testing.T) {
	config, err := Load(strings.NewReader(`{"api_version":"rlviz.dev/v1alpha1","palette":{"name":"high-contrast"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if config.Palette == nil || config.Palette.Name != "high-contrast" || config.Palette.Light["ctx"] != "#005fcc" || config.Palette.Dark["ink"] != "#ffffff" {
		t.Fatalf("unexpected high-contrast palette: %#v", config.Palette)
	}
	data, err := Normalize(config)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"name":"high-contrast"`)) || !bytes.Contains(data, []byte(`"failInfra":"#ff9a6c"`)) {
		t.Fatalf("normalized palette is incomplete: %s", data)
	}
}

func TestInvalidPaletteColorFallsBackWithNotice(t *testing.T) {
	config, err := Load(strings.NewReader(`{"api_version":"rlviz.dev/v1alpha1","palette":{"light":{"ctx":"blue"}}}`))
	if err != nil {
		t.Fatalf("invalid palette colors must not hard fail: %v", err)
	}
	if config.Palette != nil || len(config.Notices) != 1 {
		t.Fatalf("expected default fallback and notice, got palette=%#v notices=%#v", config.Palette, config.Notices)
	}
	data, err := Normalize(config)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(`"palette"`)) || !bytes.Contains(data, []byte(`"notices":["Palette ignored`)) {
		t.Fatalf("fallback was not surfaced in normalized output: %s", data)
	}
	roundTrip, err := NormalizeJSON(data)
	if err != nil {
		t.Fatalf("daemon-side independent validation rejected the normalized notice: %v", err)
	}
	if !bytes.Contains(roundTrip, []byte(`"notices":["Palette ignored`)) {
		t.Fatalf("daemon-side normalization lost the notice: %s", roundTrip)
	}
}

func TestLoadIsBounded(t *testing.T) {
	if _, err := Load(strings.NewReader(strings.Repeat(" ", MaxBytes+1))); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected size error, got %v", err)
	}
}

func TestSchemaAcceptsRuntimeValidDocument(t *testing.T) {
	data := []byte(`{"api_version":"rlviz.dev/v1alpha1","fields":{"pass":{"label":"Passed"}},"group":{"columns":["reward"]},"inspector":{"sections":["analysis","properties"]},"theme":{"focus":"#8be6d0"},"palette":{"name":"high-contrast","light":{"ctx":"#005fcc"},"dark":{"ctx":"#66aaff"}}}`)
	schemaData, err := os.ReadFile(filepath.Join("..", "..", "schemas", "v1alpha1", "presentation-config.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaData))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	location := "https://rlviz.dev/schemas/v1alpha1/presentation-config.schema.json"
	if err := compiler.AddResource(location, document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(location)
	if err != nil {
		t.Fatal(err)
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(instance); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaAndRuntimeRejectNonnumericScalar(t *testing.T) {
	data := []byte(`{"api_version":"rlviz.dev/v1alpha1","scalars":{"pass":{"format":"integer"}}}`)
	schemaData, err := os.ReadFile(filepath.Join("..", "..", "schemas", "v1alpha1", "presentation-config.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaData))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	location := "https://rlviz.dev/schemas/v1alpha1/presentation-config.schema.json"
	if err := compiler.AddResource(location, document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(location)
	if err != nil {
		t.Fatal(err)
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(instance); err == nil {
		t.Fatal("schema accepted nonnumeric scalar")
	}
	if _, err := Load(bytes.NewReader(data)); err == nil {
		t.Fatal("runtime accepted nonnumeric scalar")
	}
}

func TestPublicExampleRemainsValid(t *testing.T) {
	file, err := os.Open(filepath.Join("..", "..", "examples", "presentation", "research.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	config, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	if config.Fields["signal:reward.policy_compliance"].Label != "Policy compliance" {
		t.Fatalf("unexpected example: %#v", config)
	}
}
