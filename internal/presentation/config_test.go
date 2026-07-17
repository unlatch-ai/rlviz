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
  "theme":{"focus":"#8be6d0"}
}`))
	if err != nil {
		t.Fatal(err)
	}
	if config.Scalars["signal:grader_score"].Precision == nil || *config.Scalars["signal:grader_score"].Precision != precision {
		t.Fatalf("unexpected config: %#v", config)
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
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(strings.NewReader(document)); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestThemeRequiresReadableSemanticColors(t *testing.T) {
	_, err := Load(strings.NewReader(`{"api_version":"rlviz.dev/v1alpha1","theme":{"text_primary":"#0a0a0a"}}`))
	if err == nil || !strings.Contains(err.Error(), "contrast") {
		t.Fatalf("expected contrast error, got %v", err)
	}
}

func TestLoadIsBounded(t *testing.T) {
	if _, err := Load(strings.NewReader(strings.Repeat(" ", MaxBytes+1))); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected size error, got %v", err)
	}
}

func TestSchemaAcceptsRuntimeValidDocument(t *testing.T) {
	data := []byte(`{"api_version":"rlviz.dev/v1alpha1","fields":{"reward":{"label":"Return"}},"group":{"columns":["reward"]},"theme":{"focus":"#8be6d0"}}`)
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
