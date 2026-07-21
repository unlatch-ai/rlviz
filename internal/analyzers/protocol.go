// Package analyzers defines deterministic analysis over canonical rollout data.
package analyzers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/TheSnakeFang/rlviz/internal/model"
)

var canonicalEventKinds = map[string]struct{}{
	"message": {}, "generation": {}, "tool": {}, "environment_action": {},
	"observation": {}, "state": {}, "reward": {}, "grader": {},
	"artifact": {}, "error": {}, "log": {},
}

const (
	APIVersion       = "rlviz.dev/analyzer/v1alpha1"
	OperationAnalyze = "analyze"

	MaxInputEvents   = 100_000
	MaxInputSignals  = 100_000
	MaxFindings      = 1_000
	MaxOutputSignals = 1_000
	MaxTextBytes     = 16 << 10
	MaxInputBytes    = 64 << 20
	MaxOutputBytes   = 16 << 20
)

type Input struct {
	APIVersion   string         `json:"api_version"`
	Operation    string         `json:"operation"`
	TrajectoryID string         `json:"trajectory_id"`
	Events       []model.Event  `json:"events"`
	Signals      []model.Signal `json:"signals,omitempty"`
}

type Provenance struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Digest      string `json:"digest"`
	InputDigest string `json:"input_digest"`
}

type Finding struct {
	ID           string         `json:"id"`
	TrajectoryID string         `json:"trajectory_id"`
	EventIDs     []string       `json:"event_ids,omitempty"`
	Kind         string         `json:"kind"`
	Severity     string         `json:"severity"`
	Title        string         `json:"title"`
	Summary      string         `json:"summary,omitempty"`
	Fingerprint  string         `json:"fingerprint,omitempty"`
	Metadata     model.Metadata `json:"metadata,omitempty"`
}

type Output struct {
	APIVersion string         `json:"api_version"`
	Provenance Provenance     `json:"provenance"`
	Findings   []Finding      `json:"findings,omitempty"`
	Signals    []model.Signal `json:"signals,omitempty"`
}

// NormalizeInput returns a defensive copy in canonical order. Events are
// ordered by sequence then ID; signals are ordered by ID.
func NormalizeInput(input Input) Input {
	out := input
	out.Events = append([]model.Event(nil), input.Events...)
	out.Signals = append([]model.Signal(nil), input.Signals...)
	sort.SliceStable(out.Events, func(i, j int) bool {
		if out.Events[i].Sequence == out.Events[j].Sequence {
			return out.Events[i].ID < out.Events[j].ID
		}
		return out.Events[i].Sequence < out.Events[j].Sequence
	})
	sort.SliceStable(out.Signals, func(i, j int) bool { return out.Signals[i].ID < out.Signals[j].ID })
	return out
}

func ValidateInput(input Input) error {
	if input.APIVersion != APIVersion {
		return fmt.Errorf("api_version must be %q", APIVersion)
	}
	if input.Operation != OperationAnalyze {
		return fmt.Errorf("operation must be %q", OperationAnalyze)
	}
	if input.TrajectoryID == "" {
		return errors.New("trajectory_id is required")
	}
	if len(input.Events) > MaxInputEvents {
		return fmt.Errorf("events exceed limit %d", MaxInputEvents)
	}
	if len(input.Signals) > MaxInputSignals {
		return fmt.Errorf("signals exceed limit %d", MaxInputSignals)
	}
	ids := make(map[string]struct{}, len(input.Events)+len(input.Signals))
	var previous int64 = -1
	for _, event := range input.Events {
		if event.ID == "" {
			return errors.New("event id is required")
		}
		if event.TrajectoryID != input.TrajectoryID {
			return fmt.Errorf("event %q belongs to another trajectory", event.ID)
		}
		if event.RecordType != model.RecordEvent {
			return fmt.Errorf("event %q record_type must be event", event.ID)
		}
		if _, ok := canonicalEventKinds[event.Kind]; !ok {
			return fmt.Errorf("event %q has unsupported kind %q", event.ID, event.Kind)
		}
		if event.Sequence < 0 || event.Sequence <= previous {
			return fmt.Errorf("event %q sequence must be strictly increasing", event.ID)
		}
		previous = event.Sequence
		if _, exists := ids[event.ID]; exists {
			return fmt.Errorf("duplicate input id %q", event.ID)
		}
		ids[event.ID] = struct{}{}
	}
	for _, signal := range input.Signals {
		if signal.ID == "" || signal.Name == "" {
			return errors.New("signal id and name are required")
		}
		if signal.TrajectoryID != input.TrajectoryID {
			return fmt.Errorf("signal %q belongs to another trajectory", signal.ID)
		}
		if signal.RecordType != model.RecordSignal {
			return fmt.Errorf("signal %q record_type must be signal", signal.ID)
		}
		if !validSignalValue(signal.Value) {
			return fmt.Errorf("signal %q value must be a finite number, string, or boolean", signal.ID)
		}
		if signal.EventID != "" {
			if _, exists := ids[signal.EventID]; !exists {
				return fmt.Errorf("signal %q references unknown event %q", signal.ID, signal.EventID)
			}
		}
		if _, exists := ids[signal.ID]; exists {
			return fmt.Errorf("duplicate input id %q", signal.ID)
		}
		ids[signal.ID] = struct{}{}
	}
	return nil
}

func InputDigest(input Input) (string, error) {
	normalized := NormalizeInput(input)
	if err := ValidateInput(normalized); err != nil {
		return "", err
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("encode analyzer input: %w", err)
	}
	if len(data) > MaxInputBytes {
		return "", fmt.Errorf("encoded input exceeds limit %d bytes", MaxInputBytes)
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func ValidateOutput(output Output, input Input, expected Provenance) error {
	if output.APIVersion != APIVersion {
		return fmt.Errorf("api_version must be %q", APIVersion)
	}
	if output.Provenance.Name == "" || output.Provenance.Version == "" {
		return errors.New("analyzer provenance name and version are required")
	}
	digest, err := InputDigest(input)
	if err != nil {
		return err
	}
	if output.Provenance.Name != expected.Name || output.Provenance.Version != expected.Version || output.Provenance.Digest != expected.Digest {
		return errors.New("analyzer provenance does not match the selected analyzer")
	}
	if output.Provenance.InputDigest != digest {
		return errors.New("analyzer input_digest does not match input")
	}
	if !validDigest(output.Provenance.Digest) {
		return errors.New("analyzer digest must be sha256:<64 lowercase hex>")
	}
	if len(output.Findings) > MaxFindings {
		return fmt.Errorf("findings exceed limit %d", MaxFindings)
	}
	if len(output.Signals) > MaxOutputSignals {
		return fmt.Errorf("signals exceed limit %d", MaxOutputSignals)
	}
	events := make(map[string]struct{}, len(input.Events))
	for _, event := range input.Events {
		events[event.ID] = struct{}{}
	}
	ids := make(map[string]struct{}, len(input.Events)+len(input.Signals)+len(output.Findings)+len(output.Signals))
	for _, event := range input.Events {
		ids[event.ID] = struct{}{}
	}
	for _, signal := range input.Signals {
		ids[signal.ID] = struct{}{}
	}
	for _, finding := range output.Findings {
		if finding.ID == "" || finding.Kind == "" || finding.Title == "" {
			return errors.New("finding id, kind, and title are required")
		}
		if finding.TrajectoryID != input.TrajectoryID {
			return fmt.Errorf("finding %q belongs to another trajectory", finding.ID)
		}
		if finding.Severity != "info" && finding.Severity != "warning" && finding.Severity != "error" {
			return fmt.Errorf("finding %q has unsupported severity", finding.ID)
		}
		if len(finding.Title) > MaxTextBytes || len(finding.Summary) > MaxTextBytes {
			return fmt.Errorf("finding %q text exceeds limit", finding.ID)
		}
		if finding.Fingerprint != "" && !validDigest(finding.Fingerprint) {
			return fmt.Errorf("finding %q fingerprint must be sha256:<64 lowercase hex>", finding.ID)
		}
		if _, exists := ids[finding.ID]; exists {
			return fmt.Errorf("duplicate output id %q", finding.ID)
		}
		ids[finding.ID] = struct{}{}
		seenEvent := map[string]struct{}{}
		for _, eventID := range finding.EventIDs {
			if _, ok := events[eventID]; !ok {
				return fmt.Errorf("finding %q references unknown event %q", finding.ID, eventID)
			}
			if _, ok := seenEvent[eventID]; ok {
				return fmt.Errorf("finding %q repeats event %q", finding.ID, eventID)
			}
			seenEvent[eventID] = struct{}{}
		}
	}
	for _, signal := range output.Signals {
		if signal.ID == "" || signal.Name == "" {
			return errors.New("output signal id and name are required")
		}
		if signal.RecordType != model.RecordSignal {
			return fmt.Errorf("signal %q record_type must be signal", signal.ID)
		}
		if signal.TrajectoryID != input.TrajectoryID {
			return fmt.Errorf("signal %q belongs to another trajectory", signal.ID)
		}
		if !validSignalValue(signal.Value) {
			return fmt.Errorf("signal %q value must be a finite number, string, or boolean", signal.ID)
		}
		if signal.EventID != "" {
			if _, ok := events[signal.EventID]; !ok {
				return fmt.Errorf("signal %q references unknown event %q", signal.ID, signal.EventID)
			}
		}
		if _, exists := ids[signal.ID]; exists {
			return fmt.Errorf("duplicate output id %q", signal.ID)
		}
		ids[signal.ID] = struct{}{}
	}
	data, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("encode analyzer output: %w", err)
	}
	if len(data) > MaxOutputBytes {
		return fmt.Errorf("encoded output exceeds limit %d bytes", MaxOutputBytes)
	}
	return nil
}

func validSignalValue(value any) bool {
	switch v := value.(type) {
	case string, bool, json.Number:
		return true
	case float64:
		return !math.IsNaN(v) && !math.IsInf(v, 0)
	case float32:
		return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	default:
		return false
	}
}

func validDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != 71 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil && value == strings.ToLower(value)
}
