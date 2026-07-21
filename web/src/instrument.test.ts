import { describe, expect, it } from "vitest";
import { attentionScore, axisX, episodesFor, episodeWindow, stageChanged, stagesFor } from "./instrument";
import type { BrowseTrajectory, ComparisonSide } from "./types";

const row = (id: string, values: { errors?: number; pass?: boolean; reward?: number }): BrowseTrajectory => ({
  source_id: "source",
  source_name: "gallery.ndjson",
  trajectory: { id },
  metrics: {
    trajectory: { id },
    event_count: 10,
    error_count: values.errors ?? 0,
    pass: values.pass,
    reward: values.reward,
  },
});

describe("instrument projections", () => {
  it("orders Browse attention above marginal and nominal rollouts", () => {
    const ordered = [
      row("nominal", { pass: true, reward: 1 }),
      row("marginal", { pass: true, reward: 0.4 }),
      row("policy-failure", { pass: false, reward: -1 }),
      row("tool-errors", { errors: 2, pass: false, reward: -1 }),
    ].sort((left, right) => attentionScore(right) - attentionScore(left));
    expect(ordered.map(({ trajectory }) => trajectory.id)).toEqual(["tool-errors", "policy-failure", "marginal", "nominal"]);
  });

  it("labels sidecar stage anchors as annotated stages", () => {
    const anchored: ComparisonSide = {
      trajectory: { id: "anchored" },
      events: [
        { id: "start", sequence: 0, kind: "message" },
        { id: "work", sequence: 1, kind: "tool", alignment_key: "stage:work" },
        { id: "outcome", sequence: 2, kind: "grader", alignment_key: "stage:outcome" },
      ],
    };
    expect(stagesFor(anchored)).toMatchObject({
      tier: "annotated stages",
      stages: [{ key: "opening" }, { key: "stage:work" }, { key: "stage:outcome" }],
    });
    expect(stagesFor({ trajectory: { id: "plain" }, events: anchored.events.map(({ alignment_key: _alignment, ...event }) => event) })).toMatchObject({
      tier: "outcome only",
      stages: [{ key: "outcome" }],
    });
  });

  it("labels adapter-declared episode boundaries separately", () => {
	const side: ComparisonSide = { trajectory: { id: "episodes" }, events: [
	  { id: "start", sequence: 0, kind: "message", alignment_key: "episode:setup" },
	  { id: "work", sequence: 1, kind: "tool", alignment_key: "episode:work" },
	] };
	expect(stagesFor(side).tier).toBe("adapter episodes");
  });

  it("derives registered episode spans and deterministic fallback clusters", () => {
    const explicit = episodesFor([
      { id: "a", sequence: 0, kind: "message", alignment_key: "episode:setup" },
      { id: "b", sequence: 3, kind: "tool", alignment_key: "episode:work" },
      { id: "c", sequence: 7, kind: "error", alignment_key: "episode:work" },
    ]);
    expect(explicit.map((episode) => [episode.key, episode.startIndex, episode.endIndex, episode.inferred])).toEqual([
      ["episode:setup#1", 0, 0, false], ["episode:work#1", 1, 2, false],
    ]);
    const fallback = episodesFor([
      { id: "a", sequence: 0, kind: "message" }, { id: "b", sequence: 1, kind: "tool" },
      { id: "c", sequence: 2, kind: "observation" }, { id: "d", sequence: 3, kind: "error" },
      { id: "e", sequence: 4, kind: "error" }, { id: "f", sequence: 5, kind: "state", context: { operation: "compaction", provenance: "source_native" } },
    ]);
    expect(fallback.map((episode) => episode.label)).toEqual(["opening", "tool run", "errors", "context"]);
  });

  it("identifies recurring registered episodes by key and occurrence", () => {
    const episodes = episodesFor([
      { id: "diagnose-1", sequence: 0, kind: "message", alignment_key: "stage:diagnose" },
      { id: "diagnose-2", sequence: 1, kind: "tool", alignment_key: "stage:diagnose" },
      { id: "test", sequence: 2, kind: "tool", alignment_key: "stage:test" },
      { id: "diagnose-again", sequence: 3, kind: "observation", alignment_key: "stage:diagnose" },
    ]);
    expect(episodes.map(({ key, label, start, end }) => ({ key, label, start, end }))).toEqual([
      { key: "stage:diagnose#1", label: "diagnose", start: 0, end: 1 },
      { key: "stage:test#1", label: "test", start: 2, end: 2 },
      { key: "stage:diagnose#2", label: "diagnose", start: 3, end: 3 },
    ]);
  });

  it("zooms to an episode without moving the selected event anchor", () => {
    const before = { start: 0, end: 50 }, selected = 30;
    const after = episodeWindow(before, { key: "verify", label: "verify", startIndex: 3, endIndex: 3, start: 30, end: 30, inferred: false }, selected);
    expect(axisX(selected, after)).toBeCloseTo(axisX(selected, before));
  });

  it("fits every endpoint of a multi-event episode", () => {
    const episode = { key: "stage:verify#1", label: "verify", startIndex: 0, endIndex: 4, start: 29, end: 34, inferred: false };
    const after = episodeWindow({ start: 29.24, end: 36.24 }, episode, 34);
    expect(after.start).toBeLessThanOrEqual(episode.start);
    expect(after.end).toBeGreaterThanOrEqual(episode.end);
  });

  it("compares within-stage behavioral outcomes as order-insensitive multisets", () => {
	const left = { key: "stage:work", label: "work", events: [
	  { id: "a", sequence: 1, kind: "tool", alignment_key: "tool:a", output: { ok: true } },
	  { id: "b", sequence: 2, kind: "tool", alignment_key: "tool:b", output: { ok: false } },
	] };
	const permuted = { ...left, events: [left.events[1], left.events[0]] };
	const changed = { ...left, events: [left.events[1], { ...left.events[0], output: { ok: false } }] };
	expect(stageChanged(left, permuted)).toBe(false);
	expect(stageChanged(left, changed)).toBe(true);
  });
});
