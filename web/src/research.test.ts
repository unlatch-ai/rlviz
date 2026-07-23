import { describe, expect, it } from "vitest";
import { sampleTrajectory } from "./sample";
import { artifactsForEvent, deriveLandmark, deriveLandmarkRail, deriveMessage, deriveOutcome, deriveTokenTotals, deriveTool, findingsForEvent, groupTranscript, isContextEvent, linkedEventIds } from "./research";
import type { AnalyzerFinding, Trajectory, TrajectoryArtifact, TrajectoryEvent, TrajectorySignal } from "./types";

describe("research view derivation", () => {
  it("reads source-native messages from generic input and output envelopes", () => {
    const user: TrajectoryEvent = { id: "user", sequence: 0, kind: "message", input: { role: "user", content: "Fix it" } };
    const assistant: TrajectoryEvent = { id: "assistant", sequence: 1, kind: "generation", output: { role: "assistant", content: [{ type: "text", text: "Done" }] } };
    expect(deriveMessage(user)).toEqual({ eventId: "user", role: { value: "user", provenance: "source-native", source: "input.role" }, content: "Fix it", contentSource: "input" });
    expect(deriveMessage(assistant)?.content).toEqual([{ type: "text", text: "Done" }]);
    expect(deriveMessage(assistant)?.role?.provenance).toBe("source-native");
  });

  it("labels conservative fallbacks as inferred and preserves structured content", () => {
    const generation: TrajectoryEvent = { id: "g", sequence: 0, kind: "generation", content: { reasoning: "inspect first" } };
    expect(deriveMessage(generation)).toMatchObject({ role: { value: "assistant", provenance: "inferred" }, content: { reasoning: "inspect first" } });
    expect(deriveMessage({ id: "m", sequence: 1, kind: "message", content: "unknown speaker" })?.role).toBeUndefined();
    expect(deriveMessage({ id: "log", sequence: 2, kind: "log", content: "not a message" })).toBeUndefined();
    expect(deriveMessage({ id: "tool", sequence: 3, kind: "tool", output: { content: "file bytes" } })).toBeUndefined();
  });

  it("derives tool call and result without discarding unknown payloads", () => {
    const tool = deriveTool(sampleTrajectory.events.find((event) => event.id === "evt-003")!);
    expect(tool).toMatchObject({ name: { value: "get_order", provenance: "inferred" }, call: { order_id: "1842" }, result: { status: "draft" } });
    const canonical = deriveTool({ id: "tool", sequence: 0, kind: "tool", input: { name: "read_file", arguments: { path: "a" } }, output: { ok: true } });
    expect(canonical).toEqual({ eventId: "tool", name: { value: "read_file", provenance: "source-native", source: "input.name" }, call: { path: "a" }, result: { ok: true } });
  });

  it("creates useful landmarks while exposing inferred labels", () => {
    expect(deriveLandmark(sampleTrajectory.events[8])).toMatchObject({ category: "error", label: "Stale confirmation token", provenance: "source-native" });
    expect(deriveLandmark({ id: "reward", sequence: 2, kind: "reward" })).toEqual({ eventId: "reward", sequence: 2, category: "reward", label: "reward", provenance: "inferred" });
    const compaction: TrajectoryEvent = { id: "compact", sequence: 3, kind: "state", title: "Context compacted", alignment_key: "context:compaction" };
    expect(isContextEvent(compaction)).toBe(true);
    expect(deriveLandmark(compaction)).toMatchObject({ category: "context", label: "Context compacted", provenance: "source-native" });
  });

  it("selects sparse landmark reasons with stable precedence and ordering", () => {
    const events: TrajectoryEvent[] = [
      { id: "last", sequence: 9, kind: "generation", output: { role: "assistant", content: "done" } },
      { id: "context", sequence: 4, kind: "state", context: { operation: "compaction", provenance: "source_native" } },
      { id: "tool", sequence: 3, kind: "tool", title: "read_file" },
      { id: "user", sequence: 2, kind: "message", input: { role: "user", content: "fix it" } },
      { id: "start", sequence: 1, kind: "generation", content: "planning" },
      { id: "error", sequence: 5, kind: "error" },
      { id: "reward", sequence: 6, kind: "reward" },
    ];
    const rail = deriveLandmarkRail(
      events,
      [{ id: "artifact", trajectory_id: "t", event_id: "context", media_type: "text/plain" }, { id: "global", trajectory_id: "t", media_type: "text/plain" }],
      [{ id: "finding", trajectory_id: "t", event_ids: ["context", "tool", "missing"], kind: "retry", severity: "warning", title: "Retry" }],
      [{ id: "signal", trajectory_id: "t", event_id: "tool", name: "score", value: 1 }],
      { selectedId: "last", complete: true },
    );
    expect(rail.map(({ eventId, reason }) => [eventId, reason])).toEqual([
      ["start", "start"],
      ["user", "turn"],
      ["tool", "finding"],
      ["context", "context"],
      ["error", "failure"],
      ["reward", "evaluation"],
      ["last", "end"],
    ]);
  });

  it("withholds a terminal landmark for incomplete streams and injects only the selection", () => {
    const events = Array.from({ length: 10_000 }, (_, index): TrajectoryEvent => ({ id: `event-${index}`, sequence: index, kind: "generation" }));
    const rail = deriveLandmarkRail(events, [], [], [], { selectedId: "event-5431", complete: false });
    expect(rail.map(({ eventId, reason }) => [eventId, reason])).toEqual([["event-0", "start"], ["event-5431", "selected"]]);
  });

  it("prefers structured context semantics while retaining legacy landmarks", () => {
    const sourceNative: TrajectoryEvent = {
      id: "structured",
      sequence: 4,
      kind: "state",
      context: { operation: "compaction", input_tokens_before: 8000, input_tokens: 2000, provenance: "source_native" },
    };
    expect(isContextEvent(sourceNative)).toBe(true);
    expect(deriveLandmark(sourceNative)).toEqual({
      eventId: "structured",
      sequence: 4,
      category: "context",
      label: "Context compacted",
      provenance: "source-native",
    });

    const adapterDerived: TrajectoryEvent = {
      id: "derived",
      sequence: 5,
      kind: "state",
      alignment_key: "context:compaction",
      context: { operation: "truncation", input_tokens: 4096, provenance: "adapter_derived", derivation: "tokenizer count" },
      title: "Prompt occupancy",
    };
    expect(deriveLandmark(adapterDerived)).toMatchObject({ label: "Prompt occupancy", provenance: "adapter-derived" });

    const observation: TrajectoryEvent = {
      id: "observation",
      sequence: 6,
      kind: "state",
      context: { input_tokens: 1024, provenance: "source_native" },
    };
    expect(deriveLandmark(observation)).toMatchObject({ category: "context", label: "Context observation", provenance: "source-native" });

    const legacy: TrajectoryEvent = { id: "legacy", sequence: 7, kind: "state", alignment_key: "context:truncation" };
    expect(isContextEvent(legacy)).toBe(true);
    expect(deriveLandmark(legacy)).toMatchObject({ category: "context", label: "context", provenance: "inferred" });
  });

  it("forms stable inferred turns from explicit user anchors", () => {
    const events: TrajectoryEvent[] = [
      { id: "answer", sequence: 30, kind: "generation", output: { role: "assistant", content: "a" } },
      { id: "system", sequence: 0, kind: "message", input: { role: "system", content: "s" } },
      { id: "user-1", sequence: 10, kind: "message", input: { role: "user", content: "u1" } },
      { id: "tool", sequence: 20, kind: "tool" },
      { id: "user-2", sequence: 40, kind: "message", input: { role: "user", content: "u2" } },
    ];
    expect(groupTranscript(events)).toEqual([
      { id: "preamble:system", eventIds: ["system"], kind: "preamble", anchorRole: "system", provenance: "inferred" },
      { id: "turn:user-1", eventIds: ["user-1", "tool", "answer"], kind: "turn", anchorRole: "user", provenance: "inferred" },
      { id: "turn:user-2", eventIds: ["user-2"], kind: "turn", anchorRole: "user", provenance: "inferred" },
    ]);
  });

  it("uses reported token totals but retains the independently available event sum", () => {
    const signals: TrajectorySignal[] = [{ id: "tokens", trajectory_id: sampleTrajectory.id, name: "token_count", value: 141 }];
    expect(deriveTokenTotals(sampleTrajectory.events, signals)).toEqual({ total: 141, provenance: "source-native", source: "signal", eventTotal: 112, countedEventIds: ["evt-001", "evt-002", "evt-007"] });
    expect(deriveTokenTotals(sampleTrajectory.events)).toMatchObject({ total: 112, provenance: "inferred", source: "events" });
  });

  it("summarizes final output, grader, reward, and errors without treating status as a verdict", () => {
    const signals: TrajectorySignal[] = [{ id: "pass", trajectory_id: sampleTrajectory.id, event_id: "evt-010", name: "pass", value: false }];
    const outcome = deriveOutcome(sampleTrajectory, signals);
    expect(outcome).toMatchObject({ status: "failed", pass: { value: false, provenance: "source-native" }, reward: { total: { value: 0.25, source: "trajectory.total_reward" }, eventIds: ["evt-006"] }, errorEventIds: ["evt-009"] });
    expect(outcome.graders[0]).toMatchObject({ eventId: "evt-010", verdict: "fail", score: 0.5, evidenceEventIds: ["evt-005", "evt-008", "evt-009"] });
    expect(outcome.finalOutput?.eventId).toBe("evt-007");
    expect(outcome.finalOutputSelection?.provenance).toBe("inferred");
  });

  it("supports canonical demo-style grader evidence and reward components", () => {
    const trajectory: Trajectory = { id: "demo", status: "completed", termination: "success", events: [
      { id: "final", sequence: 2, kind: "generation", alignment_key: "message:assistant-final", output: { role: "assistant", content: "All 18 tests pass." } },
      { id: "grader", sequence: 1, kind: "grader", output: { verdict: "pass", score: 1, evidence: ["test"] } },
      { id: "reward", sequence: 3, kind: "reward", data: { total: 1, components: { correctness: 0.7, tests: 0.3 } } },
    ] };
    const outcome = deriveOutcome(trajectory);
    expect(outcome.finalOutput?.content).toBe("All 18 tests pass.");
    expect(outcome.finalOutputSelection).toEqual({ value: "final", provenance: "source-native", source: "event.alignment_key=message:assistant-final" });
    expect(outcome.graders[0].evidenceEventIds).toEqual(["test"]);
    expect(outcome.pass).toEqual({ value: true, provenance: "inferred", source: "grader:grader.verdict" });
    expect(outcome.reward.components).toEqual({ correctness: 0.7, tests: 0.3 });
  });

  it("links artifacts and analyzer findings only through explicit event references", () => {
    const artifacts: TrajectoryArtifact[] = [
      { id: "a", trajectory_id: "t", event_id: "e1", media_type: "text/plain", text: "a" },
      { id: "global", trajectory_id: "t", media_type: "text/plain", text: "g" },
    ];
    const findings: AnalyzerFinding[] = [{ id: "f", trajectory_id: "t", event_ids: ["e1", "e2"], kind: "retry", severity: "warning", title: "Retry" }];
    expect(artifactsForEvent(artifacts, "e1").map(({ id }) => id)).toEqual(["a"]);
    expect(artifactsForEvent(artifacts).map(({ id }) => id)).toEqual(["global"]);
    expect(findingsForEvent(findings, "e2")).toEqual(findings);
    expect([...linkedEventIds(artifacts, findings)]).toEqual(["e1", "e2"]);
  });
});
