import { afterEach, describe, expect, it, vi } from "vitest";
import { analysisEndpoint, artifactContentEndpoint, comparisonEndpoint, daemonToken, groupEndpoint, groupPathsEndpoint, loadAnalysis, loadArtifactContent, loadBrowse, loadComparison, loadGroupPaths, loadIndexedTrajectory, loadTrajectory, normalizeTrajectoryResponse, trajectoryEndpoint } from "./api";

afterEach(() => { vi.unstubAllGlobals(); window.history.replaceState({}, "", "/"); window.localStorage.clear(); });

describe("trajectory API normalization", () => {
	it("uses the stable trajectory ID from the viewer URL", () => {
		expect(trajectoryEndpoint("?trajectory=abc/123")).toBe("/api/v1/trajectory?trajectory=abc%2F123");
		expect(trajectoryEndpoint("?trajectory=abc&demo=1&indexed=1&event=evt-2&surface=outcome&view=compare&left=a&right=b&step=3&cohort_filter=pass%3Afalse")).toBe("/api/v1/indexed/trajectory?trajectory=abc&limit=200");
		expect(trajectoryEndpoint("?workspace=%7B%22version%22%3A2%7D&mode=read")).toBe("/api/v1/trajectory");
		expect(trajectoryEndpoint("")).toBe("/api/v1/trajectory");
	});

	it("persists a hash token and uses it for hash-less navigation", () => {
		expect(daemonToken("#token=abc%2F123")).toBe("abc/123");
		expect(window.localStorage.getItem("rlviz.daemon-token")).toBe("abc/123");
		expect(daemonToken("")).toBe("abc/123");
	});

	it("clears a stale stored token and retries Browse exactly once", async () => {
		window.localStorage.setItem("rlviz.daemon-token", "stale");
		const fetch = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
			if (fetch.mock.calls.length === 1) return new Response(null, { status: 401 });
			return new Response(JSON.stringify({ sources: [], trajectories: [], count: 0 }), { status: 200 });
		});
		vi.stubGlobal("fetch", fetch);
		await expect(loadBrowse()).resolves.toEqual({ sources: [], trajectories: [], count: 0 });
		expect(fetch).toHaveBeenCalledTimes(2);
		expect(fetch.mock.calls[0][1]?.headers).toMatchObject({ Authorization: "Bearer stale" });
		expect(fetch.mock.calls[1][1]?.headers).not.toHaveProperty("Authorization");
		expect(window.localStorage.getItem("rlviz.daemon-token")).toBeNull();
	});

	it("clears stored tokens on auth-shaped trajectory failures", async () => {
		localStorage.setItem("rlviz.daemon-token", "stale");
		vi.stubGlobal("fetch", vi.fn(async () => new Response("forbidden", { status: 403 })));
		await expect(loadTrajectory()).resolves.toMatchObject({ isSample: true });
		expect(localStorage.getItem("rlviz.daemon-token")).toBeNull();

		localStorage.setItem("rlviz.daemon-token", "stale-again");
		await expect(loadIndexedTrajectory("source", "trajectory")).rejects.toThrow("403");
		expect(localStorage.getItem("rlviz.daemon-token")).toBeNull();

		localStorage.setItem("rlviz.daemon-token", "stale-browse");
		await expect(loadBrowse()).rejects.toThrow("403");
		expect(localStorage.getItem("rlviz.daemon-token")).toBeNull();
	});

	it("surfaces an actionable error after the retry is also unauthorized", async () => {
		window.localStorage.setItem("rlviz.daemon-token", "stale");
		const fetch = vi.fn(async () => new Response(null, { status: 401 }));
		vi.stubGlobal("fetch", fetch);
		await expect(loadBrowse()).rejects.toThrow(/rlviz open/i);
		expect(fetch).toHaveBeenCalledTimes(2);
	});

  it("merges sibling events and canonical event data", () => {
    const trajectory = normalizeTrajectoryResponse({
      trajectory: { id: "traj-1", group_id: "group-1" },
      run: { id: "run-1", name: "checkout evaluation", started_at: "2026-01-01T00:00:00Z", metadata: { checkpoint: "checkpoint-42" } },
      case: { id: "case-1", run_id: "run-1", name: "example case" },
      group: { id: "group-1", case_id: "case-1", name: "deliberate" },
      signals: [{ trajectory_id: "traj-1", name: "reward", value: 0.75 }],
      events: [{
        id: "evt-1", sequence: 0, kind: "reward", data: { reward: 0.75 },
        source: { path: "trace.jsonl", byte_offset: 100, byte_length: 42 }, metadata: { token_count: 44 },
      }],
    });
    expect(trajectory.events).toHaveLength(1);
    expect(trajectory.events[0].reward).toBe(0.75);
    expect(trajectory.events[0].source?.byte_offset).toBe(100);
    expect(trajectory.run_id).toBe("run-1");
    expect(trajectory.run_name).toBe("checkout evaluation");
    expect(trajectory.case_id).toBe("case-1");
    expect(trajectory.case_name).toBe("example case");
    expect(trajectory.group_name).toBe("deliberate");
    expect(trajectory.events[0].token_count).toBe(44);
    expect(trajectory.model).toBe("checkpoint-42");
    expect(trajectory.total_reward).toBe(0.75);
  });

  it("retains validated top-level presentation metadata", async () => {
    const presentation = { api_version: "rlviz.dev/v1alpha1", fields: { reward: { label: "Return" } }, inspector: { sections: ["analysis", "properties"] }, theme: { focus: "#8be6d0" }, palette: { name: "high-contrast", light: { ctx: "#005fcc" }, dark: { ctx: "#66aaff" } }, notices: ["Palette fallback active."] };
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      trajectory: { id: "presented" },
      events: [{ id: "evt", sequence: 0, kind: "message" }],
      presentation,
    }))));
    const result = await loadTrajectory();
    expect(result.isSample).toBe(false);
    expect(result.presentation).toEqual(presentation);
  });

  it("loads every indexed event page and reports a completed page", async () => {
    const first = Array.from({ length: 200 }, (_, sequence) => ({ id: `event-${sequence}`, sequence, kind: "message" }));
    const rest = Array.from({ length: 50 }, (_, offset) => ({ id: `event-${offset + 200}`, sequence: offset + 200, kind: "message" }));
    const fetch = vi.fn(async (input: RequestInfo | URL) => String(input).includes("/indexed/events?")
      ? new Response(JSON.stringify({ events: rest, page: { count: 50, total: 250, limit: 200, after_sequence: 199, has_more: false } }))
      : new Response(JSON.stringify({ trajectory: { id: "long" }, events: first, page: { count: 200, total: 250, limit: 200, next_sequence: 199, has_more: true } })));
    vi.stubGlobal("fetch", fetch);
    const loaded = await loadIndexedTrajectory("source", "long");
    expect(loaded.trajectory.events).toHaveLength(250);
    expect(loaded.trajectory.events.at(-1)?.sequence).toBe(249);
    expect(loaded.page).toEqual({ count: 250, total: 250, limit: 250, has_more: false });
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(String(fetch.mock.calls[1][0])).toContain("after_sequence=199");
  });

  it("builds a stable encoded group endpoint", () => {
    expect(groupEndpoint("source/one", "group two")).toBe("/api/v1/indexed/group?trajectory=source%2Fone&group_id=group+two");
    expect(groupPathsEndpoint("source/one", "group two")).toBe("/api/v1/indexed/paths?trajectory=source%2Fone&group_id=group+two");
  });

  it("requests authenticated compact group paths", async () => {
    window.history.replaceState({}, "", "/#token=path-secret");
    const fetch = vi.fn(async () => ({ ok: true, json: async () => ({
      group_id: "group", source_native_branches: false, source_native_branch_count: 0, count: 1, total_events: 1,
      tree: { trajectory_count: 1, terminal_count: 0, narrative_only_count: 0, behavioral_event_count: 1, narrative_event_count: 0, root_narrative_event_count: 0, children: [] },
    }) } as Response));
    vi.stubGlobal("fetch", fetch);
    await loadGroupPaths("source", "group");
    expect(fetch).toHaveBeenCalledWith(expect.stringContaining("/api/v1/indexed/paths?"), expect.objectContaining({ headers: expect.objectContaining({ Authorization: "Bearer path-secret" }) }));
  });

  it("requests authenticated built-in analysis", async () => {
    window.history.replaceState({}, "", "/#token=analysis-secret");
    const fetch = vi.fn(async () => ({ ok: true, json: async () => ({
      cached: true, analyzed_at: "2026-07-16T00:00:00Z",
      analysis: { api_version: "rlviz.dev/analyzer/v1alpha1", provenance: { name: "builtin.loop-retry", version: "0.1.0", digest: "sha256:a", input_digest: "sha256:b" }, findings: [], signals: [] },
    }) } as Response));
    vi.stubGlobal("fetch", fetch);
    await loadAnalysis("source/one", "attempt two");
    expect(analysisEndpoint("source/one", "attempt two")).toBe("/api/v1/indexed/analysis?trajectory=source%2Fone&trajectory_id=attempt+two&analyzer=loop-retry");
    expect(fetch).toHaveBeenCalledWith(expect.stringContaining("/api/v1/indexed/analysis?"), expect.objectContaining({ headers: expect.objectContaining({ Authorization: "Bearer analysis-secret" }) }));
  });

  it("fetches path-backed artifacts with authentication", async () => {
    window.history.replaceState({}, "", "/#token=artifact-secret");
    const fetch = vi.fn(async () => new Response("safe text", { status: 200, headers: { "Content-Type": "text/plain" } }));
    vi.stubGlobal("fetch", fetch);
    const content = await loadArtifactContent("source/one", "trajectory two", "artifact/three");
    expect(new TextDecoder().decode(content)).toBe("safe text");
    expect(artifactContentEndpoint("source/one", "trajectory two", "artifact/three")).toBe("/api/v1/indexed/artifact/content?trajectory=source%2Fone&trajectory_id=trajectory+two&artifact_id=artifact%2Fthree");
    expect(fetch).toHaveBeenCalledWith(expect.stringContaining("/api/v1/indexed/artifact/content?"), expect.objectContaining({ headers: expect.objectContaining({ Authorization: "Bearer artifact-secret" }) }));
  });

  it("requests an authenticated, encoded pair comparison", async () => {
    window.history.replaceState({}, "", "/#token=compare-secret");
    const fetch = vi.fn(async () => ({ ok: true, json: async () => ({
      left: { trajectory: { id: "left/one" }, events: [] }, right: { trajectory: { id: "right two" }, events: [] },
      alignment: { steps: [], common_behavioral_prefix: 0 },
      differences: { event_count: { left: 0, right: 0, delta: 0 }, status: { changed: false }, termination: { changed: false }, reward: { changed: false } },
    }) } as Response));
    vi.stubGlobal("fetch", fetch);
    await loadComparison("source/one", "left/one", "right two");
    expect(comparisonEndpoint("source/one", "left/one", "right two")).toBe("/api/v1/indexed/compare?trajectory=source%2Fone&left=left%2Fone&right=right+two");
    expect(fetch).toHaveBeenCalledWith(expect.stringContaining("/api/v1/indexed/compare?"), expect.objectContaining({ headers: expect.objectContaining({ Authorization: "Bearer compare-secret" }) }));
  });
});
