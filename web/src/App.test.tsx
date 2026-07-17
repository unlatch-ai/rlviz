import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import { commandIds, setCommandBindings } from "./commands";
import { sampleTrajectory } from "./sample";
import type { Trajectory } from "./types";

const largeTrajectory: Trajectory = {
  ...sampleTrajectory,
  id: "large-trajectory",
  events: Array.from({ length: 10_000 }, (_, index) => ({
    id: `large-${index}`,
    sequence: index + 1,
    kind: index === 9_999 ? "error" : "generation",
    title: `Event ${index + 1}`,
    content: `Payload ${index + 1}`,
  })),
};

describe("trajectory navigation", () => {
  afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); window.history.replaceState({}, "", "/"); });
  it("navigates between events with j and k", () => {
    render(<App initialTrajectory={sampleTrajectory} />);
    expect(screen.getByText("Task prompt", { selector: ".selected-heading h3" })).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "j" });
    expect(screen.getByText("Assistant reasoning", { selector: ".selected-heading h3" })).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "k" });
    expect(screen.getByText("Task prompt", { selector: ".selected-heading h3" })).toBeInTheDocument();
  });

  it("applies keymap changes immediately to behavior and rendered shortcut labels", () => {
    const values = new Map<string, string>();
    vi.stubGlobal("localStorage", {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => { values.set(key, value); },
      removeItem: (key: string) => { values.delete(key); },
    });
    render(<App initialTrajectory={sampleTrajectory} />);
    fireEvent.click(screen.getByRole("tab", { name: /Events/ }));
    const expand = screen.getAllByRole("button", { name: /Expand event/ })[0];
    expect(expand).toHaveTextContent("Enter / Space");

    act(() => {
      setCommandBindings(commandIds.trajectory.toggleExpanded, ["z"]);
      setCommandBindings(commandIds.trajectory.next, ["n"]);
    });
    expect(expand).toHaveTextContent("z");
    expect(screen.getByText("n", { selector: ".keybar kbd" })).toBeInTheDocument();

    fireEvent.keyDown(window, { key: "j" });
    expect(screen.getByText("Task prompt", { selector: ".selected-heading h3" })).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "n" });
    expect(screen.getByText("Assistant reasoning", { selector: ".selected-heading h3" })).toBeInTheDocument();
  });

  it("restores and updates an event deep link without losing the auth fragment", () => {
    window.history.replaceState({}, "", "/?event=evt-003#token=secret");
    render(<App initialTrajectory={sampleTrajectory} />);
    expect(screen.getByText("get_order", { selector: ".selected-heading h3" })).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "j" });
    expect(new URLSearchParams(window.location.search).get("event")).toBe("evt-004");
    expect(window.location.hash).toBe("#token=secret");
  });

  it("selects a loaded filtered event from the overview and updates the deep link", () => {
    render(<App initialTrajectory={sampleTrajectory} />);
    fireEvent.change(screen.getByLabelText("Search events"), { target: { value: "submit_order" } });
    fireEvent.change(screen.getByRole("slider", { name: "Trajectory overview position" }), { target: { value: "7" } });
    const selected = sampleTrajectory.events.find((event) => event.title === "submit_order")!;
    expect(screen.getByText("submit_order", { selector: ".selected-heading h3" })).toBeInTheDocument();
    expect(new URLSearchParams(window.location.search).get("event")).toBe(selected.id);
  });

  it("labels the explicit bundled demo without treating normal fixtures as demo data", () => {
    const normal = render(<App initialTrajectory={sampleTrajectory} />);
    expect(screen.queryByText("Synthetic demo")).not.toBeInTheDocument();
    normal.unmount();

    window.history.replaceState({}, "", "/?demo=1");
    render(<App initialTrajectory={sampleTrajectory} />);
    expect(screen.getByText("Synthetic demo")).toBeInTheDocument();
  });

  it("jumps to errors and toggles raw JSON", () => {
    render(<App initialTrajectory={sampleTrajectory} />);
    fireEvent.keyDown(window, { key: "e" });
    expect(screen.getByText("Stale confirmation token", { selector: ".selected-heading h3" })).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "x" });
    expect(screen.getByText("Raw normalized record")).toBeInTheDocument();
    expect(screen.getByText(/STALE_CONFIRMATION/, { selector: ".raw-json" })).toBeInTheDocument();
  });

  it("jumps to source-native context changes with c", () => {
    const withCompaction: Trajectory = { ...sampleTrajectory, events: [...sampleTrajectory.events, { id: "context", sequence: 11, kind: "state", title: "Context compacted", alignment_key: "context:compaction", data: { before_tokens: 8000, after_tokens: 2000 } }] };
    const { container } = render(<App initialTrajectory={withCompaction} />);
    fireEvent.keyDown(window, { key: "c" });
    expect(screen.getByText("Context compacted", { selector: ".selected-heading h3" })).toBeInTheDocument();
    expect(container.querySelector(".keybar")).toHaveTextContent("context");
  });

  it("opens search with slash and filters events", () => {
    render(<App initialTrajectory={sampleTrajectory} />);
    fireEvent.keyDown(window, { key: "/" });
    const search = screen.getByLabelText("Search events");
    expect(search).toHaveFocus();
    fireEvent.change(search, { target: { value: "submit_order" } });
    expect(screen.getAllByText("submit_order").length).toBeGreaterThan(0);
    expect(screen.queryByText("Task prompt", { selector: ".event-card h2" })).not.toBeInTheDocument();
  });

  it("opens transcript by default and switches research surfaces with 1, 2, and 3", () => {
    const { container } = render(<App initialTrajectory={sampleTrajectory} />);
    expect(screen.getByLabelText("Trajectory overview")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /Transcript/ })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByLabelText("Trajectory transcript")).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "2" });
    expect(screen.getByLabelText("Trajectory overview")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /Events/ })).toHaveAttribute("aria-selected", "true");
    expect(document.querySelector(".event-card")).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "3" });
    expect(screen.getByLabelText("Trajectory overview")).toBeInTheDocument();
    expect(container.querySelector(".overview-window")).not.toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /Outcome/ })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByLabelText("Trajectory outcome")).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "1" });
    expect(new URLSearchParams(window.location.search).has("surface")).toBe(false);
  });

  it("bounds rendered nodes for 10,000 events and mounts keyboard-selected events", () => {
    const { container } = render(<App initialTrajectory={largeTrajectory} />);
    expect(container.querySelectorAll(".transcript-entry").length).toBeLessThan(30);
    expect(container.querySelectorAll(".outline-virtual button").length).toBeLessThan(30);
    expect(container.querySelectorAll(".overview-lane i")).toHaveLength(192);

    fireEvent.keyDown(window, { key: "e" });
    expect(screen.getByText("Event 10000", { selector: ".selected-heading h3" })).toBeInTheDocument();
    expect(container.querySelector("#event-large-9999")).toHaveClass("selected");
    expect(container.querySelector('.outline-virtual button[aria-current="true"]')).toHaveTextContent("Event 10000");
    expect(container.querySelectorAll(".transcript-entry").length).toBeLessThan(30);
    expect(container.querySelectorAll(".outline-virtual button").length).toBeLessThan(30);
  });

  it("keeps filtered results selectable without rendering the full trajectory", () => {
    const { container } = render(<App initialTrajectory={largeTrajectory} />);
    const search = screen.getByLabelText("Search events");
    fireEvent.change(search, { target: { value: "Event 5432" } });
    const result = screen.getByText("Event 5432", { selector: ".outline-text b" });
    fireEvent.click(result);
    expect(screen.getByText("Event 5432", { selector: ".selected-heading h3" })).toBeInTheDocument();
    expect(container.querySelectorAll(".transcript-entry")).toHaveLength(1);
    expect(container.querySelectorAll(".outline-virtual button")).toHaveLength(1);
  });

  it("cycles artifacts with o and jumps to their attached events", () => {
    const withArtifacts: Trajectory = {
      ...sampleTrajectory,
      artifacts: [
        { id: "first-artifact", trajectory_id: sampleTrajectory.id, event_id: sampleTrajectory.events[0].id, name: "first.log", media_type: "text/x-log", text: "first output" },
        { id: "second-artifact", trajectory_id: sampleTrajectory.id, event_id: sampleTrajectory.events[1].id, name: "second.json", media_type: "application/json", json: { result: "second" } },
      ],
    };
    render(<App initialTrajectory={withArtifacts} />);
    expect(screen.getByText("first output")).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "o" });
    expect(screen.getByText(/"result": "second"/)).toBeInTheDocument();
    expect(screen.getByText("Assistant reasoning", { selector: ".selected-heading h3" })).toBeInTheDocument();
  });

  it("does not fetch a path-backed artifact when o selects it", async () => {
    window.history.replaceState({}, "", "/?trajectory=source&indexed=1#token=secret");
    const fetch = vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).includes("/artifact/content?")) return new Response("trusted output", { status: 200 });
      return new Response(JSON.stringify({ cached: true, analysis: { api_version: "rlviz.dev/analyzer/v1alpha1", provenance: { name: "builtin.loop-retry", version: "0.1.0", digest: "sha256:a", input_digest: "sha256:b" }, findings: [], signals: [] } }), { status: 200 });
    });
    vi.stubGlobal("fetch", fetch);
    const withPath: Trajectory = {
      ...sampleTrajectory,
      artifacts: [
        { id: "inline", trajectory_id: sampleTrajectory.id, name: "inline.log", media_type: "text/plain", text: "inline output" },
        { id: "path", trajectory_id: sampleTrajectory.id, event_id: sampleTrajectory.events[1].id, name: "run.log", media_type: "text/x-log", path: "outputs/run.log" },
      ],
    };
    render(<App initialTrajectory={withPath} />);
    fireEvent.click(screen.getByRole("button", { name: "run.log text/x-log" }));
    expect(screen.getByRole("button", { name: "Load preview" })).toBeInTheDocument();
    expect(fetch.mock.calls.some(([url]) => String(url).includes("/artifact/content?"))).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "inline.log text/plain" }));
    fireEvent.keyDown(window, { key: "o" });
    expect(screen.getByRole("button", { name: "Load preview" })).toBeInTheDocument();
    expect(screen.getByText("outputs/run.log")).toBeInTheDocument();
    expect(fetch.mock.calls.some(([url]) => String(url).includes("/artifact/content?"))).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "Load preview" }));
    expect(await screen.findByText("trusted output")).toBeInTheDocument();
    expect(fetch.mock.calls.filter(([url]) => String(url).includes("/artifact/content?"))).toHaveLength(1);
  });

  it("loads analyzer findings and jumps through referenced events with a", async () => {
    window.history.replaceState({}, "", "/?trajectory=source&indexed=1#token=secret");
    const fetch = vi.fn(async () => ({ ok: true, json: async () => ({
      cached: true, analyzed_at: "2026-07-16T12:30:00Z",
      analysis: {
        api_version: "rlviz.dev/analyzer/v1alpha1",
        provenance: { name: "builtin.loop-retry", version: "0.1.0", digest: "sha256:a", input_digest: "sha256:b" },
        findings: [{ id: "retry", trajectory_id: sampleTrajectory.id, event_ids: ["evt-003", "evt-005"], kind: "retry", severity: "warning", title: "Repeated identical action" }],
        signals: [],
      },
    }) } as Response));
    vi.stubGlobal("fetch", fetch);
    render(<App initialTrajectory={sampleTrajectory} />);
    expect(await screen.findByText("Repeated identical action")).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "a" });
    await waitFor(() => expect(screen.getByText("get_order", { selector: ".selected-heading h3" })).toBeInTheDocument());
    fireEvent.keyDown(window, { key: "a" });
    expect(screen.getByText("update_shipping_address", { selector: ".selected-heading h3" })).toBeInTheDocument();
    expect(fetch).toHaveBeenCalledWith(expect.stringContaining("/api/v1/indexed/analysis?trajectory=source"), expect.objectContaining({ headers: expect.objectContaining({ Authorization: "Bearer secret" }) }));
  });

  it("does not poll completed indexed sources", async () => {
    window.history.replaceState({}, "", "/?trajectory=source&indexed=1#token=secret");
    const fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes("/analysis?")) return new Response(JSON.stringify({ cached: false, analysis: { api_version: "rlviz.dev/analyzer/v1alpha1", provenance: { name: "builtin.loop-retry", version: "0.1.0", digest: "sha256:a", input_digest: "sha256:b" }, findings: [], signals: [] } }), { status: 200 });
      return new Response(JSON.stringify({ trajectory: { ...sampleTrajectory, events: undefined }, events: sampleTrajectory.events, source: { id: "source", index_state: "complete" }, page: { count: sampleTrajectory.events.length, total: sampleTrajectory.events.length, limit: 200, has_more: false } }), { status: 200 });
    });
    vi.stubGlobal("fetch", fetch);
    render(<App />);
    expect(await screen.findByText("Task prompt", { selector: ".selected-heading h3" })).toBeInTheDocument();
    await waitFor(() => expect(fetch).toHaveBeenCalledWith(expect.stringContaining("/analysis?"), expect.anything()));
    expect(fetch.mock.calls.some(([url]) => String(url).includes("/indexed/events?"))).toBe(false);
  });

  it("recovers truncated signals and artifacts with stable child offsets", async () => {
    window.history.replaceState({}, "", "/?trajectory=source&indexed=1#token=secret");
    const firstArtifact = { id: "artifact-1", trajectory_id: sampleTrajectory.id, name: "first.log", media_type: "text/plain", text: "first" };
    const secondArtifact = { id: "artifact-2", trajectory_id: sampleTrajectory.id, name: "second.log", media_type: "text/plain", text: "second" };
    const fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes("/indexed/signals?")) return new Response(JSON.stringify({ signals: [{ id: "signal-2", trajectory_id: sampleTrajectory.id, name: "ReWaRd", value: 0.9 }], page: { count: 1, total: 2, limit: 200, offset: 1, has_more: false } }));
      if (url.includes("/indexed/artifacts?")) return new Response(JSON.stringify({ artifacts: [secondArtifact], page: { count: 1, total: 2, limit: 200, offset: 1, has_more: false } }));
      if (url.includes("/analysis?")) return new Response(JSON.stringify({ cached: false, analysis: { api_version: "rlviz.dev/analyzer/v1alpha1", provenance: { name: "builtin.loop-retry", version: "0.1.0", digest: "sha256:a", input_digest: "sha256:b" }, findings: [], signals: [] } }));
      return new Response(JSON.stringify({
        trajectory: { ...sampleTrajectory, events: undefined }, events: [sampleTrajectory.events[0]], artifacts: [firstArtifact],
        signals: [{ id: "signal-1", trajectory_id: sampleTrajectory.id, name: "score", value: 1 }],
        source: { id: "source", index_state: "complete" }, page: { count: 1, total: 1, limit: 200, has_more: false },
        signal_page: { count: 1, total: 2, limit: 100, has_more: true, next_offset: 1 },
        artifact_page: { count: 1, total: 2, limit: 100, has_more: true, next_offset: 1 },
      }));
    });
    vi.stubGlobal("fetch", fetch);
    render(<App />);
    expect(await screen.findByRole("button", { name: "second.log text/plain" })).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText("0.9", { selector: ".contextbar strong" })).toBeInTheDocument());
    expect(fetch).toHaveBeenCalledWith(expect.stringContaining(`/indexed/signals?trajectory=source&trajectory_id=${sampleTrajectory.id}&offset=1&limit=200`), expect.anything());
    expect(fetch).toHaveBeenCalledWith(expect.stringContaining(`/indexed/artifacts?trajectory=source&trajectory_id=${sampleTrajectory.id}&offset=1&limit=200`), expect.anything());
  });

  it("polls a growing source, appends events, then analyzes once complete", async () => {
    vi.useFakeTimers();
    window.history.replaceState({}, "", "/?trajectory=source&indexed=1&event=evt-002#token=secret");
    let analysisCalls = 0;
    const firstEvent = sampleTrajectory.events[0];
    const nextEvent = sampleTrajectory.events[1];
    const fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes("/indexed/events?")) return new Response(JSON.stringify({ events: [nextEvent], source: { id: "source", index_state: "complete" }, page: { count: 1, total: 2, limit: 200, has_more: false } }), { status: 200 });
      if (url.includes("/analysis?")) {
        analysisCalls += 1;
        return new Response(JSON.stringify({ cached: false, analysis: { api_version: "rlviz.dev/analyzer/v1alpha1", provenance: { name: "builtin.loop-retry", version: "0.1.0", digest: "sha256:a", input_digest: "sha256:b" }, findings: [], signals: [] } }), { status: 200 });
      }
      return new Response(JSON.stringify({ trajectory: { ...sampleTrajectory, events: undefined }, events: [firstEvent], source: { id: "source", index_state: "indexing" }, page: { count: 1, total: 1, limit: 200, has_more: false } }), { status: 200 });
    });
    vi.stubGlobal("fetch", fetch);
    render(<App />);
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(screen.getByText("Indexing")).toBeInTheDocument();
    expect(analysisCalls).toBe(0);
    await act(async () => { await vi.advanceTimersByTimeAsync(1000); });
    expect(screen.getByText("Assistant reasoning", { selector: ".outline-text b" })).toBeInTheDocument();
    expect(screen.getByText("Assistant reasoning", { selector: ".selected-heading h3" })).toBeInTheDocument();
    expect(window.location.hash).toBe("#token=secret");
    expect(screen.queryByText("Indexing")).not.toBeInTheDocument();
    expect(analysisCalls).toBe(1);
    expect(fetch.mock.calls.filter(([url]) => String(url).includes("/indexed/events?"))).toHaveLength(1);
  });
});
