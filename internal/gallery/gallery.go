// Package gallery generates deterministic synthetic sources used by the RLViz demo.
package gallery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/TheSnakeFang/rlviz/internal/model"
)

const Seed int64 = 24052026

type stream struct{ records []any }

func (s *stream) add(values ...any) { s.records = append(s.records, values...) }

func (s *stream) bytes() ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	for _, record := range s.records {
		if err := encoder.Encode(record); err != nil {
			return nil, err
		}
	}
	if err := encoder.Encode(&model.Complete{RecordType: model.RecordComplete, Records: int64(len(s.records)), Warnings: 0}); err != nil {
		return nil, err
	}
	if err := model.Decode(bytes.NewReader(output.Bytes()), func(*model.Record) error { return nil }); err != nil {
		return nil, fmt.Errorf("validate generated gallery: %w", err)
	}
	return output.Bytes(), nil
}

// Generate writes all gallery files beneath directory.
func Generate(directory string) error {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	files := []struct {
		name string
		make func() ([]byte, error)
	}{
		{"coding-agent-bugfix.ndjson", codingAgentBugfix},
		{"web-research-agent.ndjson", webResearchAgent},
		{"checkout-cohort.ndjson", checkoutCohort},
	}
	for _, file := range files {
		content, err := file.make()
		if err != nil {
			return fmt.Errorf("generate %s: %w", file.name, err)
		}
		if err := writeFile(filepath.Join(directory, file.name), content); err != nil {
			return err
		}
	}
	return nil
}

func writeFile(path string, content []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".gallery-*.ndjson")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func base(runID, runName, caseID, caseName, groupID, groupName string) *stream {
	s := &stream{}
	s.add(
		&model.Run{RecordType: model.RecordRun, ID: runID, Name: runName, StartedAt: "2026-05-24T12:00:00Z", Metadata: model.Metadata{"synthetic": true, "gallery": true, "generator_seed": Seed}},
		&model.Case{RecordType: model.RecordCase, ID: caseID, RunID: runID, Name: caseName, Metadata: model.Metadata{"synthetic": true}},
		&model.Group{RecordType: model.RecordGroup, ID: groupID, CaseID: caseID, Name: groupName, Metadata: model.Metadata{"synthetic": true}},
	)
	return s
}

func codingAgentBugfix() ([]byte, error) {
	const trajectoryID = "coding-bugfix-rollout-01"
	s := base("run-coding-bugfix", "Synthetic coding-agent bugfix", "case-coding-bugfix", "Fix flaky cache invalidation", "group-coding-bugfix", "Bugfix attempts")
	s.add(&model.Trajectory{RecordType: model.RecordTrajectory, ID: trajectoryID, GroupID: "group-coding-bugfix", Status: "completed", Termination: "success", Metadata: model.Metadata{"synthetic": true, "model": "synthetic-coding-agent", "event_count": 300}})
	events := make([]*model.Event, 0, 300)
	parent := ""
	stages := []struct {
		start int
		name  string
	}{{0, "setup"}, {45, "reproduce"}, {105, "diagnose"}, {190, "patch"}, {245, "verify"}}
	stageName := func(index int) string {
		name := stages[0].name
		for _, stage := range stages {
			if index >= stage.start {
				name = stage.name
			}
		}
		return name
	}
	for index := 0; index < 300; index++ {
		stage := stageName(index)
		kind := []string{"generation", "tool", "observation", "generation", "log"}[index%5]
		alignment := fmt.Sprintf("stage:%s", stage)
		input, output, data := any(nil), any(nil), any(nil)
		metadata := model.Metadata{"title": fmt.Sprintf("%s step %03d", stage, index), "synthetic": true}
		switch kind {
		case "generation":
			output = map[string]any{"role": "assistant", "content": fmt.Sprintf("Inspecting the cache invalidation path during %s; hypothesis %d remains testable.", stage, index%7)}
		case "tool":
			path := []string{"internal/cache/store.go", "internal/cache/store_test.go", "cmd/server/main.go"}[index%3]
			input = map[string]any{"name": "read_file", "arguments": map[string]any{"path": path, "line_start": 1 + index%80}}
			output = map[string]any{"ok": true, "path": path, "excerpt": "generation := entry.Generation\nif generation != current { delete(entries, key) }"}
		case "observation":
			data = map[string]any{"summary": "The generation guard is checked after the stale value can escape.", "path": "internal/cache/store.go"}
		case "log":
			data = map[string]any{"level": "debug", "message": fmt.Sprintf("cache generation=%d key=user:%d", 40+index%4, index%11)}
		}
		if index == 0 {
			kind, alignment = "message", "episode:setup"
			input = map[string]any{"role": "system", "content": "Work locally, preserve public APIs, and prove the cache race with focused tests."}
			output, data = nil, nil
			metadata["title"] = "Repository constraints"
		}
		for _, marker := range stages {
			if index == marker.start {
				alignment = "episode:" + marker.name
				metadata["title"] = "Episode: " + marker.name
			}
		}
		// A repeated failure comb is intentionally dense and obvious in both renderers.
		if index >= 132 && index <= 166 {
			attempt := (index-132)/2 + 1
			switch (index - 132) % 2 {
			case 0:
				kind, alignment = "tool", "stage:diagnose:test:cache-race"
				input = map[string]any{"name": "run_tests", "arguments": map[string]any{"command": "go test ./internal/cache -run TestConcurrentInvalidation -count=50"}}
				output = map[string]any{"ok": false, "exit_code": 1, "attempt": attempt, "stderr": "expected generation 43, got stale generation 42"}
				metadata["title"] = fmt.Sprintf("Failing cache race test · attempt %d", attempt)
			case 1:
				kind, alignment = "error", "stage:diagnose:test-failure"
				data = map[string]any{"class": "test_failure", "message": "TestConcurrentInvalidation returned a stale cached value", "attempt": attempt, "recoverable": true}
				metadata["title"] = fmt.Sprintf("Retry %d failed", attempt)
			}
		}
		var context *model.Context
		if index == 178 {
			before, after, capacity := int64(11920), int64(3620), int64(12800)
			kind, alignment = "state", "context:compaction"
			context = &model.Context{Operation: "compaction", InputTokensBefore: &before, InputTokens: &after, Capacity: &capacity, Provenance: "source_native", Summary: "Retained reproduction, retry evidence, generation-race hypothesis, and required verification."}
			data = map[string]any{"before_tokens": before, "after_tokens": after, "reason": "context pressure"}
			metadata["title"] = "Context compacted after diagnosis"
		}
		if index == 210 {
			kind, alignment = "tool", "stage:patch:apply"
			input = map[string]any{"name": "apply_patch", "arguments": map[string]any{"path": "internal/cache/store.go", "diff": "@@ -88,6 +88,9 @@\n+ current := generations[key]\n+ if entry.Generation != current {\n+   return Value{}, false\n+ }"}}
			output = map[string]any{"ok": true, "files_changed": []string{"internal/cache/store.go", "internal/cache/store_test.go"}}
			metadata["title"] = "Apply generation guard before returning cached value"
		}
		if index == 260 {
			kind, alignment = "tool", "stage:verify:focused-tests"
			input = map[string]any{"name": "run_tests", "arguments": map[string]any{"command": "go test -race ./internal/cache -count=100"}}
			output = map[string]any{"ok": true, "passed": 100, "failed": 0, "duration_ms": 8420}
			metadata["title"] = "Focused race suite passes 100 runs"
		}
		if index == 290 {
			kind, alignment = "grader", "stage:verify:grader"
			input = map[string]any{"rubric": "Reproduces the race, fixes stale returns, preserves API behavior, and passes race tests."}
			output = map[string]any{"verdict": "pass", "score": 0.98, "evidence": []string{"coding-event-0132", "coding-event-0210", "coding-event-0260"}}
			metadata["grader"] = "synthetic-bugfix-grader"
			metadata["title"] = "Grader accepts bugfix"
		}
		if index == 295 {
			kind, alignment = "grader", "stage:verify:verifier"
			output = map[string]any{"verdict": "pass", "evidence": []string{"coding-event-0260"}, "checks": map[string]any{"race_suite": true, "api_compatibility": true}}
			metadata["grader"] = "synthetic-verifier"
			metadata["title"] = "Independent verifier passes"
		}
		if index == 298 {
			kind, alignment = "generation", "stage:verify:final"
			output = map[string]any{"role": "assistant", "content": "Fixed the stale cache race by checking the generation before returning a loaded entry. Added a deterministic concurrent invalidation regression. The focused race suite passed 100 consecutive runs and the full package suite is green."}
			metadata["title"] = "Final implementation summary"
		}
		if index == 299 {
			kind, alignment = "reward", "stage:verify:reward"
			data = map[string]any{"total": 0.96, "components": map[string]any{"reproduction": 0.2, "diagnosis": 0.25, "patch_correctness": 0.31, "verification": 0.2}}
			metadata["title"] = "Composite reward 0.96"
		}
		id := fmt.Sprintf("coding-event-%04d", index)
		event := &model.Event{RecordType: model.RecordEvent, ID: id, TrajectoryID: trajectoryID, Sequence: int64(index * 10), Kind: kind, ParentID: parent, AlignmentKey: alignment, Input: input, Output: output, Data: data, Context: context, Source: &model.SourceLocation{Path: "synthetic/coding-agent/session.ndjson"}, Metadata: metadata}
		events = append(events, event)
		parent = id
	}
	for _, event := range events {
		s.add(event)
	}
	s.add(
		&model.Artifact{RecordType: model.RecordArtifact, ID: "coding-artifact-patch", TrajectoryID: trajectoryID, EventID: "coding-event-0210", Name: "cache-race.patch", MediaType: "text/x-diff", Text: "--- a/internal/cache/store.go\n+++ b/internal/cache/store.go\n@@ -88,6 +88,9 @@\n+ current := generations[key]\n+ if entry.Generation != current { return Value{}, false }\n"},
		&model.Artifact{RecordType: model.RecordArtifact, ID: "coding-artifact-test-log", TrajectoryID: trajectoryID, EventID: "coding-event-0260", Name: "race-test.log", MediaType: "text/plain", Text: "PASS: TestConcurrentInvalidation (100 runs under -race)\n"},
		&model.Artifact{RecordType: model.RecordArtifact, ID: "coding-artifact-verifier", TrajectoryID: trajectoryID, EventID: "coding-event-0295", Name: "verifier-evidence.json", MediaType: "application/json", JSON: map[string]any{"synthetic": true, "passed": true, "checks": []string{"race_suite", "api_compatibility"}}},
		&model.Signal{RecordType: model.RecordSignal, ID: "coding-signal-pass", TrajectoryID: trajectoryID, EventID: "coding-event-0295", Name: "pass", Value: true},
		&model.Signal{RecordType: model.RecordSignal, ID: "coding-signal-reward", TrajectoryID: trajectoryID, EventID: "coding-event-0299", Name: "reward", Value: 0.96},
		&model.Signal{RecordType: model.RecordSignal, ID: "coding-signal-retries", TrajectoryID: trajectoryID, Name: "retry_count", Value: 7},
		&model.Signal{RecordType: model.RecordSignal, ID: "coding-signal-grader", TrajectoryID: trajectoryID, EventID: "coding-event-0290", Name: "reward.grader", Value: 0.98},
		&model.Signal{RecordType: model.RecordSignal, ID: "coding-signal-verifier", TrajectoryID: trajectoryID, EventID: "coding-event-0295", Name: "reward.verifier", Value: 1.0},
	)
	return s.bytes()
}

func webResearchAgent() ([]byte, error) {
	const trajectoryID = "web-research-rollout-01"
	s := base("run-web-research", "Synthetic web research agent", "case-web-research", "Compare browser isolation designs", "group-web-research", "Research run")
	s.add(&model.Trajectory{RecordType: model.RecordTrajectory, ID: trajectoryID, GroupID: "group-web-research", Status: "completed", Termination: "success", Metadata: model.Metadata{"synthetic": true, "model": "synthetic-research-agent", "event_count": 120}})
	parent := ""
	for index := 0; index < 120; index++ {
		kind := "generation"
		phase := index % 7
		input, output, data := any(nil), any(nil), any(nil)
		alignment := fmt.Sprintf("stage:research-round-%02d", index/7+1)
		title := fmt.Sprintf("Research round %02d · think", index/7+1)
		if index == 119 {
			alignment, title = "episode:synthesis", "Final research synthesis"
			output = map[string]any{"role": "assistant", "content": stringsRepeat("The evidence favors process isolation with explicit capability grants, a loopback-only control plane, immutable request logs, and per-tool timeouts. ", 45) + "The recommendation distinguishes documented guarantees from implementation inference and lists unresolved portability tests."}
		} else if phase >= 1 && phase <= 3 {
			kind = "tool"
			engine := []string{"search", "open_page", "find_in_page"}[phase-1]
			input = map[string]any{"name": engine, "arguments": map[string]any{"query": fmt.Sprintf("browser sandbox capability isolation source %d", index/7+1), "source": fmt.Sprintf("https://docs.example.test/isolation/%d", index%9)}}
			output = map[string]any{"ok": true, "request_id": fmt.Sprintf("research-request-%03d", index)}
			title = fmt.Sprintf("Parallel tool burst · %s", engine)
		} else if phase >= 4 {
			kind = "observation"
			data = map[string]any{"source": fmt.Sprintf("docs.example.test/reference/%d", index%11), "finding": fmt.Sprintf("Capability boundary evidence %d confirms explicit grants and bounded IPC.", index), "confidence": 0.82 + float64(index%6)/100}
			title = "Observation burst · source evidence"
		} else {
			output = map[string]any{"role": "assistant", "content": "I will fan out independent primary-source checks, then reconcile guarantees, tradeoffs, and gaps."}
		}
		id := fmt.Sprintf("research-event-%04d", index)
		s.add(&model.Event{RecordType: model.RecordEvent, ID: id, TrajectoryID: trajectoryID, Sequence: int64(index * 10), Kind: kind, ParentID: parent, AlignmentKey: alignment, Input: input, Output: output, Data: data, Source: &model.SourceLocation{Path: "synthetic/research-agent/session.ndjson"}, Metadata: model.Metadata{"title": title, "synthetic": true}})
		parent = id
	}
	s.add(
		&model.Signal{RecordType: model.RecordSignal, ID: "research-signal-pass", TrajectoryID: trajectoryID, EventID: "research-event-0119", Name: "pass", Value: true},
		&model.Signal{RecordType: model.RecordSignal, ID: "research-signal-reward", TrajectoryID: trajectoryID, EventID: "research-event-0119", Name: "reward", Value: 0.91},
	)
	return s.bytes()
}

func stringsRepeat(value string, count int) string {
	var output bytes.Buffer
	for index := 0; index < count; index++ {
		_, _ = io.WriteString(&output, value)
	}
	return output.String()
}

func checkoutCohort() ([]byte, error) {
	s := &stream{}
	s.add(
		&model.Run{RecordType: model.RecordRun, ID: "run-checkout-cohort", Name: "Synthetic checkout evaluation", StartedAt: "2026-05-24T12:00:00Z", Metadata: model.Metadata{"synthetic": true, "gallery": true, "generator_seed": Seed, "model": "checkout-policy-1200"}},
		&model.Case{RecordType: model.RecordCase, ID: "case-checkout", RunID: "run-checkout-cohort", Name: "Complete checkout with saved card", Metadata: model.Metadata{"synthetic": true}},
		&model.Group{RecordType: model.RecordGroup, ID: "group-checkout-deliberate", CaseID: "case-checkout", Name: "Deliberate · temperature 0.2", Metadata: model.Metadata{"synthetic": true, "variant": "deliberate", "temperature": 0.2}},
		&model.Group{RecordType: model.RecordGroup, ID: "group-checkout-direct", CaseID: "case-checkout", Name: "Direct · temperature 0.8", Metadata: model.Metadata{"synthetic": true, "variant": "direct", "temperature": 0.8}},
	)
	lengths := []int{28, 34, 42, 31, 55, 24, 47, 38, 62, 29, 51, 36, 44, 70, 33, 49}
	for index := range 16 {
		status, termination := "completed", "success"
		if index == 5 {
			status, termination = "failed", "policy_violation"
		} else if index == 9 {
			status, termination = "failed", "infrastructure_error"
		}
		groupID := "group-checkout-deliberate"
		if index >= 8 {
			groupID = "group-checkout-direct"
		}
		s.add(&model.Trajectory{RecordType: model.RecordTrajectory, ID: fmt.Sprintf("checkout-rollout-%02d", index+1), GroupID: groupID, Status: status, Termination: termination, Metadata: model.Metadata{"synthetic": true, "sample_index": index, "event_count": lengths[index], "recovery_after_retries": index == 13}})
	}
	for rollout := range 16 {
		trajectoryID := fmt.Sprintf("checkout-rollout-%02d", rollout+1)
		parent := ""
		passed := rollout != 5 && rollout != 9
		toolByStage := map[string]string{"setup": "inspect_checkout", "cart": "read_cart", "address": "update_shipping_address", "payment": "select_saved_payment", "submit": "submit_order", "verify": "read_order_confirmation"}
		for index := 0; index < lengths[rollout]; index++ {
			stage := []string{"setup", "cart", "address", "payment", "submit", "verify"}[min(5, index*6/lengths[rollout])]
			kind := []string{"generation", "tool", "observation"}[index%3]
			input, output, data := any(nil), any(nil), any(nil)
			title := fmt.Sprintf("%s · step %02d", stage, index)
			if kind == "tool" {
				input = map[string]any{"name": toolByStage[stage], "arguments": map[string]any{"checkout_id": fmt.Sprintf("synthetic-%02d", rollout+1), "stage": stage}}
				output = map[string]any{"ok": true, "stage": stage, "revision": index / 3}
			} else if kind == "generation" {
				output = map[string]any{"role": "assistant", "content": fmt.Sprintf("Proceed through %s using the saved-card flow and verify visible state.", stage)}
			} else {
				data = map[string]any{"visible": true, "stage": stage, "cart_total": "42.00 USD"}
			}
			if rollout == 5 && index == lengths[rollout]-3 {
				kind, title = "error", "Policy failure: attempted disallowed card export"
				data, input, output = map[string]any{"class": "policy_violation", "message": "Exporting full card details is prohibited", "recoverable": false}, nil, nil
			}
			if rollout == 9 && index == lengths[rollout]-3 {
				kind, title = "error", "Infrastructure failure: checkout sandbox unavailable"
				data, input, output = map[string]any{"class": "infrastructure_error", "message": "Checkout sandbox returned 503 before submission", "recoverable": true}, nil, nil
			}
			if rollout == 13 && index >= 45 && index <= 55 && index%3 == 0 {
				kind, title = "error", fmt.Sprintf("Recoverable submit timeout · retry %d", (index-45)/3+1)
				data, input, output = map[string]any{"class": "timeout", "message": "Submit response timed out; checkout state remains recoverable", "recoverable": true}, nil, nil
			}
			if rollout == 13 && index == 58 {
				kind, title = "observation", "Recovered after retries; confirmation visible"
				data = map[string]any{"order_id": "SYNTHETIC-1042", "confirmation_visible": true, "recovered": true}
			}
			if index == lengths[rollout]-1 {
				kind, title = "grader", "Checkout task grader"
				input, data = map[string]any{"rubric": "Create the order with the saved card, preserve the source cart, and expose a confirmation."}, nil
				output = map[string]any{"verdict": map[bool]string{true: "pass", false: "fail"}[passed], "score": map[bool]float64{true: 1, false: 0}[passed], "reason": map[bool]string{true: "Order state and confirmation satisfy every deterministic check.", false: "The rollout did not reach a verifiable completed checkout."}[passed], "checks": map[string]any{"order_created": passed, "source_unchanged": true, "confirmation_visible": passed}, "evidence": []string{fmt.Sprintf("checkout-%02d-event-%03d", rollout+1, max(0, index-2)), fmt.Sprintf("checkout-%02d-event-%03d", rollout+1, max(0, index-1))}}
			}
			id := fmt.Sprintf("checkout-%02d-event-%03d", rollout+1, index)
			metadata := model.Metadata{"title": title, "synthetic": true, "duration_ms": 80 + (index*37+rollout*19)%900}
			if kind == "generation" || kind == "grader" {
				metadata["token_count"] = 24 + (index*11+rollout*7)%180
			}
			if kind == "grader" {
				metadata["grader"] = "checkout-state-verifier"
				metadata["verifier_type"] = "deterministic state verifier"
				metadata["version"] = "2"
			}
			s.add(&model.Event{RecordType: model.RecordEvent, ID: id, TrajectoryID: trajectoryID, Sequence: int64(index * 10), Kind: kind, ParentID: parent, AlignmentKey: "stage:" + stage, Input: input, Output: output, Data: data, Source: &model.SourceLocation{Path: "synthetic/checkout/cohort.ndjson"}, Metadata: metadata})
			parent = id
		}
		reward := 0.84 + float64((rollout*17+int(Seed%13))%15)/100
		if !passed {
			reward = -0.4
		}
		lastEvent := fmt.Sprintf("checkout-%02d-event-%03d", rollout+1, lengths[rollout]-1)
		s.add(
			&model.Signal{RecordType: model.RecordSignal, ID: fmt.Sprintf("checkout-%02d-pass", rollout+1), TrajectoryID: trajectoryID, EventID: lastEvent, Name: "pass", Value: passed},
			&model.Signal{RecordType: model.RecordSignal, ID: fmt.Sprintf("checkout-%02d-reward", rollout+1), TrajectoryID: trajectoryID, EventID: lastEvent, Name: "reward", Value: reward},
			&model.Signal{RecordType: model.RecordSignal, ID: fmt.Sprintf("checkout-%02d-tokens", rollout+1), TrajectoryID: trajectoryID, Name: "token_count", Value: 760 + lengths[rollout]*31 + rollout*43, Unit: "tokens"},
			&model.Signal{RecordType: model.RecordSignal, ID: fmt.Sprintf("checkout-%02d-cost", rollout+1), TrajectoryID: trajectoryID, Name: "cost_usd", Value: float64(82+lengths[rollout]*3+rollout*2) / 10000, Unit: "USD"},
		)
		if rollout == 5 || rollout == 9 {
			class := "policy"
			if rollout == 9 {
				class = "infrastructure"
			}
			s.add(&model.Signal{RecordType: model.RecordSignal, ID: fmt.Sprintf("checkout-%02d-failure", rollout+1), TrajectoryID: trajectoryID, Name: "failure_class", Value: class})
		}
		if rollout == 13 {
			s.add(&model.Signal{RecordType: model.RecordSignal, ID: "checkout-14-recovery", TrajectoryID: trajectoryID, EventID: "checkout-14-event-058", Name: "recovery_after_retries", Value: true})
		}
	}
	return s.bytes()
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
