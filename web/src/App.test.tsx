import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StrictMode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import { sampleTrajectory } from "./sample";
import { emptyWorkspace, laneId, serializeWorkspace } from "./workspace";

describe("instrument viewer", () => {
  afterEach(() => { vi.unstubAllGlobals(); window.history.replaceState({}, "", "/"); window.localStorage.clear(); document.documentElement.removeAttribute("data-theme"); });

  async function openRead() {
    render(<App initialTrajectory={sampleTrajectory} />);
    fireEvent.keyDown(window, { key: "Enter" });
    await screen.findByRole("main", { name: "Read trajectory" });
  }

  it("opens Browse into Read at the first anomaly and returns without losing queue selection", async () => {
    render(<App initialTrajectory={sampleTrajectory} />);
    expect(screen.getByRole("main", { name: "Browse trajectories" })).toBeInTheDocument();
    const chosen = screen.getByRole("option", { selected: true });
    fireEvent.keyDown(window, { key: "Enter" });
    expect(await screen.findByRole("main", { name: "Read trajectory" })).toBeInTheDocument();
    expect(screen.getByText("Stale confirmation token", { selector: ".moment.selected b" })).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "Escape" });
    expect(screen.getByRole("main", { name: "Browse trajectories" })).toBeInTheDocument();
    expect(screen.getByRole("option", { selected: true })).toHaveTextContent(chosen.textContent ?? "");
  });

  it("renders every global fidelity ladder family", () => {
    render(<App initialTrajectory={sampleTrajectory} />);
    expect(screen.getByText("glyphs", { selector: ".fidelity-readout b" })).toBeInTheDocument();
		expect(document.querySelector(".cat-glyphs")).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "]" });
		expect(screen.getByText("previews", { selector: ".fidelity-readout b" })).toBeInTheDocument();
		expect(document.querySelector(".cat-glyphs small")).toHaveTextContent(/events/);
		fireEvent.keyDown(window, { key: "]" });
		expect(screen.getByText("full", { selector: ".fidelity-readout b" })).toBeInTheDocument();
		fireEvent.keyDown(window, { key: "[" });
		fireEvent.keyDown(window, { key: "[" });
		fireEvent.keyDown(window, { key: "[" });
		expect(screen.getByText("texture", { selector: ".fidelity-readout b" })).toBeInTheDocument();
		expect(document.querySelector(".cat-texture")).toBeInTheDocument();
		fireEvent.keyDown(window, { key: "[" });
		expect(screen.getByText("marks", { selector: ".fidelity-readout b" })).toBeInTheDocument();
		fireEvent.keyDown(window, { key: "[" });
		expect(screen.getByText("hairline", { selector: ".fidelity-readout b" })).toBeInTheDocument();
		expect(document.querySelector(".cat-line")).toBeInTheDocument();
		expect(screen.queryByRole("button", { name: "table" })).not.toBeInTheDocument();
		expect(screen.queryByRole("button", { name: "caterpillars" })).not.toBeInTheDocument();
  });

  it("keeps pointer and keyboard layer transitions anchored to the selected moment", async () => {
    await openRead();
    const surfaceAnchor = screen.getByRole("region", { name: "Trajectory shape" }).getAttribute("data-selected-x");
    fireEvent.keyDown(window, { key: "Enter" });
    const episodes = screen.getByRole("region", { name: "Trajectory shape" });
    expect(episodes).toHaveAttribute("data-selected-x", surfaceAnchor!);
    fireEvent.click(document.querySelector(".episode-button.selected")!);
    expect(screen.getByRole("main", { name: "Read trajectory" })).toHaveAttribute("data-depth", "3");
    const compressed = screen.getByRole("region", { name: "Compressed trajectory shape" });
    expect(compressed).toHaveAttribute("data-selected-x", surfaceAnchor!);
    fireEvent.click(compressed.querySelector("svg")!);
    expect(screen.getByRole("main", { name: "Read trajectory" })).toHaveAttribute("data-depth", "2");
    expect(screen.getByRole("region", { name: "Trajectory shape" })).toHaveAttribute("data-selected-x", surfaceAnchor!);
  });

  it("traverses episodes and expands event scope for cross-episode landmarks", async () => {
    await openRead();
    fireEvent.keyDown(window, { key: "Enter" });
    const initialEpisode = screen.getByRole("main", { name: "Read trajectory" }).getAttribute("data-episode");
    fireEvent.keyDown(window, { key: "j" });
    expect(screen.getByRole("main", { name: "Read trajectory" })).not.toHaveAttribute("data-episode", initialEpisode!);
    fireEvent.keyDown(window, { key: "k" });
    expect(screen.getByRole("main", { name: "Read trajectory" })).toHaveAttribute("data-episode", initialEpisode!);
    fireEvent.keyDown(window, { key: "Enter" });
    const beforePan = screen.getByRole("main", { name: "Read trajectory" }).getAttribute("data-axis-start");
    fireEvent.keyDown(window, { key: "r" });
    expect(screen.getByRole("main", { name: "Read trajectory" })).not.toHaveAttribute("data-episode", initialEpisode!);
    expect(screen.getByRole("main", { name: "Read trajectory" })).not.toHaveAttribute("data-axis-start", beforePan!);
  });

  it("renders context lanes at Surface even when a deeper layer is stored", () => {
    const state = emptyWorkspace();
    const id = laneId("sample", sampleTrajectory.id);
    state.lanes = [{ id, sourceId: "sample", trajectoryId: sampleTrajectory.id, band: "context", selected: 8, depth: 4, fidelity: 3, axis: { start: 1, end: 10 } }];
    state.active = id;
    window.history.replaceState({ rlvizWorkspace: state }, "", `/?workspace=${encodeURIComponent(serializeWorkspace(state))}`);
    render(<App initialTrajectory={sampleTrajectory} />);
    const lane = screen.getByRole("main", { name: `Context lane ${sampleTrajectory.id}` });
    expect(lane).toHaveAttribute("data-depth", "1");
    expect(lane).toHaveAttribute("data-stored-depth", "4");
  });

  it("switches light and dark mode through the data-theme attribute", () => {
    document.documentElement.setAttribute("data-theme", "light");
    render(<App initialTrajectory={sampleTrajectory} />);
    const toggle = screen.getByRole("button", { name: "Switch to dark theme" });
    fireEvent.click(toggle);
    expect(document.documentElement).toHaveAttribute("data-theme", "dark");
    expect(toggle).toHaveAccessibleName("Switch to light theme");
  });

  it("focuses the reading surface on first load and restores it after navigation", async () => {
    render(<App initialTrajectory={sampleTrajectory} />);
    const browse = screen.getByRole("main", { name: "Browse trajectories" });
    expect(browse).toHaveFocus();
    fireEvent.keyDown(browse, { key: "Enter" });
    const read = await screen.findByRole("main", { name: "Read trajectory" });
    expect(read).toHaveFocus();
    fireEvent.keyDown(read, { key: "j" });
    expect(screen.getByText(/^#/i, { selector: ".selection-address" })).toHaveTextContent("#10");
  });

  it("keeps the selected event screen x stable across zoom in, out, and fit", async () => {
    await openRead();
    const strip = screen.getByRole("region", { name: "Trajectory shape" });
    const initial = strip.getAttribute("data-selected-x");
	const initialVisible = Number(strip.getAttribute("data-visible-events"));
    fireEvent.keyDown(window, { key: "+" });
    expect(strip).toHaveAttribute("data-selected-x", initial!);
	expect(Number(strip.getAttribute("data-visible-events"))).toBeLessThan(initialVisible);
    fireEvent.keyDown(window, { key: "-" });
    expect(strip).toHaveAttribute("data-selected-x", initial!);
    fireEvent.keyDown(window, { key: "0" });
    expect(strip).toHaveAttribute("data-selected-x", initial!);
  });

  it("ascends one read depth at a time before returning to Browse", async () => {
	await openRead();
	fireEvent.keyDown(window, { key: "Enter" });
	fireEvent.keyDown(window, { key: "Enter" });
	fireEvent.keyDown(window, { key: "Enter" });
	expect(screen.getByText(/depth 4\/4/)).toBeInTheDocument();
	expect(screen.getByRole("region", { name: "Event source" })).toBeInTheDocument();
	fireEvent.keyDown(window, { key: "Escape" });
	expect(screen.getByText(/depth 3\/4/)).toBeInTheDocument();
	fireEvent.keyDown(window, { key: "Escape" });
	expect(screen.getByText(/depth 2\/4/)).toBeInTheDocument();
	fireEvent.keyDown(window, { key: "Escape" });
	expect(screen.getByText(/depth 1\/4/)).toBeInTheDocument();
	fireEvent.keyDown(window, { key: "Escape" });
	expect(screen.getByRole("main", { name: "Browse trajectories" })).toBeInTheDocument();
  });

  it("serializes keyboard seam changes and restores the arrangement from its deep link", async () => {
    const first = render(<App initialTrajectory={sampleTrajectory} />);
    fireEvent.keyDown(window, { key: "Enter" });
    await screen.findByRole("main", { name: "Read trajectory" });
    fireEvent.keyDown(window, { key: "w", ctrlKey: true });
    expect(screen.getByRole("region", { name: "Workspace console" })).toHaveAttribute("data-resize-mode", "true");
    fireEvent.keyDown(window, { key: "ArrowUp" });
    fireEvent.keyDown(window, { key: "Escape" });
    const serialized = new URLSearchParams(window.location.search).get("workspace");
    expect(serialized).toBeTruthy();
    expect(JSON.parse(serialized!).seams.console).toBeCloseTo(0.3);
    first.unmount();
    render(<App initialTrajectory={sampleTrajectory} />);
    expect(await screen.findByRole("main", { name: "Read trajectory" })).toHaveAttribute("data-trajectory", sampleTrajectory.id);
    expect(parseFloat((document.querySelector(".instrument-shell") as HTMLElement).style.getPropertyValue("--console-height"))).toBeCloseTo(30);
  });

  it("walks arrangement depth backward and forward through the jumplist", async () => {
    await openRead();
    fireEvent.keyDown(window, { key: "Enter" });
    expect(screen.getByRole("main", { name: "Read trajectory" })).toHaveAttribute("data-depth", "2");
    fireEvent.keyDown(window, { key: "o", ctrlKey: true });
    expect(screen.getByRole("main", { name: "Read trajectory" })).toHaveAttribute("data-depth", "1");
    fireEvent.keyDown(window, { key: "i", ctrlKey: true });
    expect(screen.getByRole("main", { name: "Read trajectory" })).toHaveAttribute("data-depth", "2");
    expect(document.querySelector(".workspace-breadcrumb")).toHaveTextContent("1 lane");
  });

  it("pans a zoomed axis when a landmark jump leaves the window", async () => {
	await openRead();
	fireEvent.keyDown(window, { key: "+" });
	fireEvent.keyDown(window, { key: "r" });
	const strip = screen.getByRole("region", { name: "Trajectory shape" });
	await waitFor(() => {
	  const x = Number(strip.getAttribute("data-selected-x"));
	  expect(x).toBeGreaterThanOrEqual(20);
	  expect(x).toBeLessThanOrEqual(980);
	});
  });

  it("jumps to reward and context landmarks", async () => {
    const trajectory = { ...sampleTrajectory, events: [...sampleTrajectory.events.slice(0, 4), { id: "context", sequence: 4.5, kind: "state", title: "Context compacted", alignment_key: "context:compaction" }, ...sampleTrajectory.events.slice(4)] };
    render(<App initialTrajectory={trajectory} />);
    fireEvent.keyDown(window, { key: "Enter" });
    await screen.findByRole("main", { name: "Read trajectory" });
    fireEvent.keyDown(window, { key: "c" });
    expect(screen.getByText("Context compacted", { selector: ".moment.selected b" })).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "r" });
    expect(screen.getByText("Address update reward", { selector: ".moment.selected b" })).toBeInTheDocument();
  });

  it("uses hover as a skimmer without moving selection", async () => {
    await openRead();
    const before = screen.getByText(/^#/i, { selector: ".selection-address" }).textContent;
    const svg = screen.getByRole("region", { name: "Trajectory shape" }).querySelector("svg")!;
    Object.defineProperty(svg, "getBoundingClientRect", { value: () => ({ left: 0, top: 0, width: 1000, height: 200, right: 1000, bottom: 200, x: 0, y: 0, toJSON() {} }) });
    fireEvent.mouseMove(svg, { clientX: 22, clientY: 50 });
    expect(await screen.findByRole("status")).toBeInTheDocument();
    expect(screen.getByText(/^#/i, { selector: ".selection-address" })).toHaveTextContent(before ?? "");
    fireEvent.mouseLeave(svg);
    await waitFor(() => expect(screen.queryByRole("status")).not.toBeInTheDocument());
  });

  it("suppresses letter commands while typing and lists active bindings", () => {
    render(<App initialTrajectory={sampleTrajectory} />);
    const filter = screen.getByLabelText("Filter");
    filter.focus(); window.dispatchEvent(new Event("focus"));
    expect(filter).toHaveFocus();
    fireEvent.keyDown(filter, { key: "j" }); fireEvent.change(filter, { target: { value: "j" } });
    expect(filter).toHaveValue("j");
    fireEvent.change(filter, { target: { value: "" } });
    fireEvent.keyDown(window, { key: "?" });
    expect(screen.getByRole("dialog", { name: "Active keyboard shortcuts" })).toHaveTextContent("Increase fidelity");
  });

  it("surfaces a non-blocking palette fallback notice from presentation metadata", async () => {
    vi.stubGlobal("fetch", vi.fn(async (request: RequestInfo | URL) => {
      const url = String(request);
      if (url.includes("/api/v1/indexed/browse")) return new Response(JSON.stringify({ sources: [], trajectories: [] }));
      return new Response(JSON.stringify({
        trajectory: { id: "presented" },
        events: [{ id: "event", sequence: 0, kind: "message" }],
        presentation: { api_version: "rlviz.dev/v1alpha1", notices: ["Palette ignored because it contains an invalid hex color; built-in defaults are active."] },
      }));
    }));
    render(<App />);
    expect(await screen.findByRole("status")).toHaveTextContent("Palette ignored");
    expect(screen.getByRole("main", { name: "Browse trajectories" })).toBeInTheDocument();
  });

  it("does not let a slow boot replace an already-open Read session", async () => {
	let resolveTrajectory!: (value: Response) => void;
	let resolveBrowse!: (value: Response) => void;
	vi.stubGlobal("fetch", vi.fn((request: RequestInfo | URL) => new Promise<Response>((resolve) => {
	  if (String(request).includes("/indexed/browse")) resolveBrowse = resolve;
	  else resolveTrajectory = resolve;
	})));
	render(<App />);
	fireEvent.keyDown(window, { key: "Enter" });
	expect(await screen.findByRole("main", { name: "Read trajectory" })).toHaveTextContent(sampleTrajectory.id);
	resolveTrajectory(new Response(JSON.stringify({ trajectory: { id: "late" }, events: [{ id: "late-event", sequence: 0, kind: "message" }] })));
	resolveBrowse(new Response(JSON.stringify({ sources: [], trajectories: [], count: 0 })));
	await waitFor(() => expect(screen.getByRole("main", { name: "Read trajectory" })).toHaveTextContent(sampleTrajectory.id));
	expect(screen.queryByText("late")).not.toBeInTheDocument();
  });

  it("ignores AbortError from a superseded StrictMode boot", async () => {
	let calls = 0;
	vi.stubGlobal("fetch", vi.fn((request: RequestInfo | URL, init?: RequestInit) => {
	  calls++;
	  if (calls <= 2) return new Promise<Response>((_resolve, reject) => {
		init?.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")), { once: true });
	  });
	  if (String(request).includes("/indexed/browse")) return Promise.resolve(new Response(JSON.stringify({ sources: [], trajectories: [], count: 0 })));
	  return Promise.resolve(new Response(JSON.stringify({ trajectory: { id: "fresh" }, events: [{ id: "event", sequence: 0, kind: "message" }] })));
	}));
	render(<StrictMode><App /></StrictMode>);
	await waitFor(() => expect(calls).toBeGreaterThanOrEqual(4));
	expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
