package model

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

func TestDemoFixtureRepresentsDistinctResearchOutcomes(t *testing.T) {
	t.Parallel()
	data := readDemoFixture(t)

	terminations := make(map[string]string)
	messageRoles := make(map[string]int)
	decisionKeys := make(map[string]string)
	failureClasses := make(map[string]string)
	toolEvents := 0
	graderEvents := 0
	rewardEvents := 0
	artifacts := 0
	records := 0

	err := Decode(bytes.NewReader(data), func(record *Record) error {
		records++
		switch value := record.Value.(type) {
		case *Trajectory:
			terminations[value.ID] = value.Status + "/" + value.Termination
		case *Event:
			switch value.Kind {
			case "message":
				if input, ok := value.Input.(map[string]any); ok {
					if role, ok := input["role"].(string); ok {
						messageRoles[role]++
					}
				}
			case "generation":
				if output, ok := value.Output.(map[string]any); ok {
					if role, ok := output["role"].(string); ok {
						messageRoles[role]++
					}
				}
			case "tool":
				toolEvents++
				if value.Input == nil || value.Output == nil {
					t.Errorf("tool event %q does not preserve both call and result", value.ID)
				}
			case "grader":
				graderEvents++
			case "reward":
				rewardEvents++
			}
			if value.Sequence == 40 {
				decisionKeys[value.TrajectoryID] = value.AlignmentKey
			}
		case *Signal:
			if value.Name == "failure_class" {
				failureClasses[value.TrajectoryID], _ = value.Value.(string)
			}
		case *Artifact:
			artifacts++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if records != 60 {
		t.Fatalf("decoded %d records, want 60 including complete", records)
	}
	wantTerminations := map[string]string{
		"traj-demo-success":        "completed/success",
		"traj-demo-policy-failure": "failed/policy_violation",
		"traj-demo-infra-failure":  "failed/infrastructure_error",
	}
	for id, want := range wantTerminations {
		if got := terminations[id]; got != want {
			t.Errorf("%s outcome = %q, want %q", id, got, want)
		}
	}
	if messageRoles["system"] != 3 || messageRoles["user"] != 3 || messageRoles["assistant"] != 9 {
		t.Errorf("message roles = %#v, want 3 system, 3 user, and 9 assistant", messageRoles)
	}
	if toolEvents != 8 || graderEvents != 3 || rewardEvents != 3 || artifacts != 2 {
		t.Errorf("fixture coverage tools=%d graders=%d rewards=%d artifacts=%d", toolEvents, graderEvents, rewardEvents, artifacts)
	}
	if decisionKeys["traj-demo-success"] != "decision:edit-allowlist" ||
		decisionKeys["traj-demo-infra-failure"] != "decision:edit-allowlist" ||
		decisionKeys["traj-demo-policy-failure"] != "decision:probe-registry" {
		t.Errorf("decision alignment keys = %#v", decisionKeys)
	}
	if failureClasses["traj-demo-policy-failure"] != "policy" || failureClasses["traj-demo-infra-failure"] != "infrastructure" {
		t.Errorf("failure class signals = %#v", failureClasses)
	}
}

func TestDemoFixtureMatchesCanonicalSchema(t *testing.T) {
	t.Parallel()
	schemaData, err := os.ReadFile(filepath.Join("..", "..", "schemas", "v1alpha1", "canonical-record.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaData))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	location := "https://rlviz.dev/schemas/v1alpha1/canonical-record.schema.json"
	if err := compiler.AddResource(location, document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(location)
	if err != nil {
		t.Fatal(err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(readDemoFixture(t)))
	for line := 1; scanner.Scan(); line++ {
		value, err := jsonschema.UnmarshalJSON(bytes.NewReader(scanner.Bytes()))
		if err != nil {
			t.Fatalf("line %d: %v", line, err)
		}
		if err := schema.Validate(value); err != nil {
			t.Fatalf("line %d: %v", line, err)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
}

func readDemoFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "canonical", "demo.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
