package analyzers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/TheSnakeFang/rlviz/internal/model"
)

const (
	LoopRetryName    = "builtin.loop-retry"
	LoopRetryVersion = "0.1.0"
	LoopRetryDigest  = "sha256:00d8c907e39f7864de37655517dc34db5549b7f01b746427000e402df2c51c0b"
)

type LoopRetry struct {
	MinRepeats int
	MaxPeriod  int
}

type action struct {
	event       model.Event
	fingerprint string
}

func (a LoopRetry) Analyze(ctx context.Context, input Input) (Output, error) {
	input = NormalizeInput(input)
	if err := ValidateInput(input); err != nil {
		return Output{}, err
	}
	inputDigest, err := InputDigest(input)
	if err != nil {
		return Output{}, err
	}
	minRepeats := a.MinRepeats
	if minRepeats == 0 {
		minRepeats = 3
	}
	maxPeriod := a.MaxPeriod
	if maxPeriod == 0 {
		maxPeriod = 4
	}
	if minRepeats < 2 || maxPeriod < 1 {
		return Output{}, fmt.Errorf("min repeats must be >= 2 and max period >= 1")
	}

	actions := make([]action, 0, len(input.Events))
	for _, event := range input.Events {
		if err := ctx.Err(); err != nil {
			return Output{}, err
		}
		if event.Kind != "tool" && event.Kind != "environment_action" {
			continue
		}
		fingerprint, err := actionFingerprint(event)
		if err != nil {
			return Output{}, fmt.Errorf("fingerprint event %q: %w", event.ID, err)
		}
		actions = append(actions, action{event: event, fingerprint: fingerprint})
	}

	output := Output{APIVersion: APIVersion, Provenance: Provenance{Name: LoopRetryName, Version: LoopRetryVersion, Digest: LoopRetryDigest, InputDigest: inputDigest}}
	consumedUntil := 0
	for end := minRepeats; end <= len(actions); end++ {
		if err := ctx.Err(); err != nil {
			return Output{}, err
		}
		for period := 1; period <= maxPeriod && period*minRepeats <= end; period++ {
			start := end - period*minRepeats
			if start < consumedUntil || !repeatedPattern(actions[start:end], period, minRepeats) {
				continue
			}
			eventIDs := make([]string, 0, end-start)
			for _, item := range actions[start:end] {
				eventIDs = append(eventIDs, item.event.ID)
			}
			kind := "loop"
			title := fmt.Sprintf("Repeated %d-step action loop", period)
			if period == 1 {
				kind, title = "retry", "Repeated identical action"
			}
			fingerprint := patternFingerprint(actions[start : start+period])
			id := findingID(fingerprint, eventIDs)
			last := actions[end-1].event
			finding := Finding{ID: id, TrajectoryID: input.TrajectoryID, EventIDs: eventIDs, Kind: kind, Severity: "warning", Title: title, Summary: fmt.Sprintf("The same action pattern repeated %d times across %d behavioral actions.", minRepeats, len(eventIDs)), Fingerprint: fingerprint, Metadata: model.Metadata{"period": float64(period), "repeats": float64(minRepeats)}}
			output.Findings = append(output.Findings, finding)
			output.Signals = append(output.Signals, model.Signal{RecordType: model.RecordSignal, ID: "signal-" + id, TrajectoryID: input.TrajectoryID, EventID: last.ID, Name: "analyzer.loop_retry.detected", Value: true, Metadata: model.Metadata{"finding_id": id, "kind": kind}})
			consumedUntil = end
			break
		}
		if len(output.Findings) >= MaxFindings || len(output.Signals) >= MaxOutputSignals {
			break
		}
	}
	if err := ValidateOutput(output, input, output.Provenance); err != nil {
		return Output{}, err
	}
	return output, nil
}

func actionFingerprint(event model.Event) (string, error) {
	// Output, timestamps, event IDs, and source offsets are intentionally absent:
	// retries are defined by the attempted behavior, not its result or storage.
	payload := struct {
		Kind  string `json:"kind"`
		Input any    `json:"input,omitempty"`
		Data  any    `json:"data,omitempty"`
	}{event.Kind, event.Input, event.Data}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func repeatedPattern(actions []action, period, repeats int) bool {
	if len(actions) != period*repeats {
		return false
	}
	for i := period; i < len(actions); i++ {
		if actions[i].fingerprint != actions[i%period].fingerprint {
			return false
		}
	}
	return true
}

func patternFingerprint(actions []action) string {
	hash := sha256.New()
	for _, item := range actions {
		_, _ = hash.Write([]byte(item.fingerprint))
		_, _ = hash.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func findingID(fingerprint string, eventIDs []string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(fingerprint))
	for _, id := range eventIDs {
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(id))
	}
	return "loop-retry-" + hex.EncodeToString(hash.Sum(nil))[:16]
}
