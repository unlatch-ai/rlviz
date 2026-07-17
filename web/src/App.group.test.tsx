import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import { sampleTrajectory } from "./sample";
import type { ComparisonResponse, GroupResponse } from "./types";

const mixedGroup: GroupResponse = {
  group_id: "group-eval-7",
  trajectories: [
    { trajectory: { value: { id: "attempt-good", group_id: "group-eval-7", status: "completed" } }, normalized_metrics: { reward: 1, pass: true } },
    { trajectory: { value: { id: "attempt-bad", group_id: "group-eval-7", status: "failed" } }, normalized_metrics: { reward: -0.2, pass: false } },
  ],
};

const directComparison: ComparisonResponse = {
  left: { trajectory: { id: "attempt-good" }, events: [{ id: "left-0", sequence: 0, kind: "tool", title: "Left zero" }, { id: "left-1", sequence: 1, kind: "tool", title: "Left one" }] },
  right: { trajectory: { id: "attempt-bad" }, events: [{ id: "right-0", sequence: 0, kind: "tool", title: "Right zero" }, { id: "right-1", sequence: 1, kind: "error", title: "Right one" }] },
  alignment: { common_behavioral_prefix: 1, first_meaningful_divergence: 1, steps: [{ operation: "match", left_index: 0, right_index: 0, meaningful: false }, { operation: "replace", left_index: 1, right_index: 1, meaningful: true }] },
  differences: { reward: { changed: false }, status: { changed: true }, termination: { changed: true }, event_count: { left: 2, right: 2, delta: 0 } },
};

describe("group workflow", () => {
  afterEach(() => { vi.unstubAllGlobals(); window.history.replaceState({}, "", "/"); });

  it("restores group view directly from the URL", async () => {
    window.history.replaceState({}, "", "/?trajectory=source-1&indexed=1&view=group&cohort_filter=pass%3Afalse#token=secret");
    const fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes("/indexed/group")) return new Response(JSON.stringify(mixedGroup));
      if (url.includes("/indexed/paths")) return new Response(JSON.stringify({ group_id: "group-eval-7", source_native_branches: false, source_native_branch_count: 0, count: 0, total_events: 0, tree: { trajectory_count: 0, terminal_count: 0, narrative_only_count: 0, behavioral_event_count: 0, narrative_event_count: 0, root_narrative_event_count: 0, children: [] } }));
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetch);
    render(<App initialTrajectory={{ ...sampleTrajectory, group_id: "group-eval-7" }} />);
    expect(await screen.findByRole("main", { name: "Trajectory group" })).toBeInTheDocument();
    const filter = screen.getByLabelText("Filter trajectories");
    expect(filter).toHaveValue("pass:false");
    expect(screen.queryByText("attempt-good")).not.toBeInTheDocument();
    fireEvent.change(filter, { target: { value: "reward>=1" } });
    expect(new URLSearchParams(window.location.search).get("cohort_filter")).toBe("reward>=1");
    expect(window.location.hash).toBe("#token=secret");
  });

  it("restores a bounded pair comparison and selected alignment step", async () => {
    window.history.replaceState({}, "", "/?trajectory=source-1&indexed=1&view=compare&left=attempt-good&right=attempt-bad&step=0#token=secret");
    const fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes("/indexed/compare")) return new Response(JSON.stringify(directComparison));
      if (url.includes("/indexed/group")) return new Response(JSON.stringify(mixedGroup));
      if (url.includes("/indexed/paths")) return new Response(JSON.stringify({ group_id: "group-eval-7", source_native_branches: false, source_native_branch_count: 0, count: 0, total_events: 0, tree: { trajectory_count: 0, terminal_count: 0, narrative_only_count: 0, behavioral_event_count: 0, narrative_event_count: 0, root_narrative_event_count: 0, children: [] } }));
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetch);
    render(<App initialTrajectory={{ ...sampleTrajectory, group_id: "group-eval-7" }} />);
    expect(await screen.findByRole("main", { name: "Trajectory comparison" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Alignment step 1: match" })).toHaveClass("selected");
    fireEvent.keyDown(window, { key: "j" });
    await waitFor(() => expect(new URLSearchParams(window.location.search).get("step")).toBe("1"));
    expect(window.location.hash).toBe("#token=secret");
  });

  it("opens a group and loads the keyboard-selected trajectory without a page reload", async () => {
    window.history.replaceState({}, "", "/?trajectory=source-1&indexed=1#token=secret");
    const fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes("/indexed/group")) return { ok: true, json: async () => mixedGroup } as Response;
      if (url.includes("trajectory_id=attempt-bad")) return { ok: true, json: async () => ({ trajectory: { id: "attempt-bad", group_id: "group-eval-7", status: "failed" }, events: [{ id: "bad-event", sequence: 0, kind: "error", title: "Selected failure" }], page: { count: 1, total: 1, limit: 200, has_more: false } }) } as Response;
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetch);
    render(<App initialTrajectory={{ ...sampleTrajectory, group_id: "group-eval-7" }} />);

    fireEvent.keyDown(window, { key: "g" });
    expect(await screen.findByRole("main", { name: "Trajectory group" })).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "w" });
    fireEvent.keyDown(window, { key: "Enter" });

    await waitFor(() => expect(screen.getByText("Selected failure", { selector: ".selected-heading h3" })).toBeInTheDocument());
    expect(window.location.search).toContain("trajectory_id=attempt-bad");
    expect(window.location.hash).toBe("#token=secret");
    expect(fetch.mock.calls.some(([url]) => String(url).includes("/api/v1/indexed/group?trajectory=source-1&group_id=group-eval-7"))).toBe(true);
  });

  it("does not append a pending event page from the previously open trajectory", async () => {
    window.history.replaceState({}, "", "/?trajectory=source-1&indexed=1#token=secret");
    let resolveOldPage: ((value: Response) => void) | undefined;
    const oldPage = new Promise<Response>((resolve) => { resolveOldPage = resolve; });
    const fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes("/indexed/group")) return { ok: true, json: async () => mixedGroup } as Response;
      if (url.includes("/indexed/events")) return oldPage;
      if (url.includes("trajectory_id=attempt-bad")) return { ok: true, json: async () => ({ trajectory: { id: "attempt-bad", group_id: "group-eval-7", status: "failed" }, events: [{ id: "new-event", sequence: 0, kind: "error", title: "New trajectory event" }], page: { count: 1, total: 1, limit: 200, has_more: false } }) } as Response;
      if (url.includes("/indexed/trajectory")) return { ok: true, json: async () => ({ trajectory: { id: "attempt-old", group_id: "group-eval-7", status: "completed" }, events: [{ id: "old-event", sequence: 0, kind: "generation", title: "Old trajectory event" }], page: { count: 1, total: 2, limit: 200, next_sequence: 0, has_more: true } }) } as Response;
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetch);
    render(<App />);
    expect(await screen.findByText("Old trajectory event", { selector: ".selected-heading h3" })).toBeInTheDocument();
    await waitFor(() => expect(fetch.mock.calls.some(([url]) => String(url).includes("/indexed/events"))).toBe(true));

    fireEvent.keyDown(window, { key: "g" });
    expect(await screen.findByRole("main", { name: "Trajectory group" })).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "w" });
    fireEvent.keyDown(window, { key: "Enter" });
    expect(await screen.findByText("New trajectory event", { selector: ".selected-heading h3" })).toBeInTheDocument();

    await act(async () => resolveOldPage?.({ ok: true, json: async () => ({ events: [{ id: "stale-event", sequence: 1, kind: "error", title: "Stale old page" }], page: { count: 1, total: 2, limit: 200, has_more: false } }) } as Response));
    expect(screen.queryByText("Stale old page")).not.toBeInTheDocument();
  });
});
