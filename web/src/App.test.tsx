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

  it("renders the three-level fidelity ladder with truthful strips", () => {
    render(<App initialTrajectory={sampleTrajectory} />);
    // default: glyphs — the sample provider carries no shape summary, so the
    // strip must degrade to length + counts, never synthetic texture.
    expect(screen.getByText("glyphs", { selector: ".fidelity-readout b" })).toBeInTheDocument();
    expect(document.querySelector(".cat-texture")).not.toBeInTheDocument();
    fireEvent.keyDown(window, { key: "]" });
    expect(screen.getByText("detail", { selector: ".fidelity-readout b" })).toBeInTheDocument();
    expect(document.querySelector("[data-columns='true']")).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "]" });
    expect(screen.getByText("detail", { selector: ".fidelity-readout b" })).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "[" });
    fireEvent.keyDown(window, { key: "[" });
    expect(screen.getByText("hairline", { selector: ".fidelity-readout b" })).toBeInTheDocument();
    expect(document.querySelector(".cat-line")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "table" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "caterpillars" })).not.toBeInTheDocument();
  });

  it("reveals keyboard collection selection and applies wheel deltas to the collection scroller", () => {
    const bounds = vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
      if (this.getAttribute("role") === "listbox") return { top: 100, bottom: 300 } as DOMRect;
      if (this.getAttribute("role") === "option") return { top: 320, bottom: 360 } as DOMRect;
      return { top: 0, bottom: 0 } as DOMRect;
    });
    render(<App initialTrajectory={sampleTrajectory} />);
    const list = screen.getByRole("listbox", { name: "Trajectory collection" });
    expect(list.scrollTop).toBe(60);
    Object.defineProperty(list, "scrollHeight", { configurable: true, value: 900 });
    Object.defineProperty(list, "clientHeight", { configurable: true, value: 200 });
    fireEvent.keyDown(window, { key: "j" });
    fireEvent.wheel(list, { deltaY: 80 });
    expect(list.scrollTop).toBe(140);
    bounds.mockRestore();
  });

  it("switches the collection between flat rollouts and trial groups", () => {
    render(<App initialTrajectory={sampleTrajectory} />);
    const trials = screen.getByRole("button", { name: "trials" });
    fireEvent.click(trials);
    expect(screen.getByRole("main", { name: "Browse trajectories" })).toHaveAttribute("data-collection-view", "trials");
    expect(trials).toHaveAttribute("aria-pressed", "true");
    expect(document.querySelector(".rail-trial-group")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "rollouts" }));
    expect(document.querySelector(".rail-trial-group")).not.toBeInTheDocument();
  });

  it("edits and restores local collection and trajectory metadata", async () => {
    const first = render(<App initialTrajectory={sampleTrajectory} />);
    fireEvent.click(screen.getByRole("button", { name: "Edit collection title and description" }));
    fireEvent.change(screen.getByRole("textbox", { name: "collection title" }), { target: { value: "Checkout evaluation" } });
    fireEvent.change(screen.getByRole("textbox", { name: "collection description" }), { target: { value: "Saved-card checkout rollouts" } });
    fireEvent.click(screen.getByRole("button", { name: "save" }));
    expect(document.querySelector(".workspace-rail h1")).toHaveTextContent("Checkout evaluation");
    expect(document.querySelector(".workspace-rail .editable-metadata p")).toHaveTextContent("Saved-card checkout rollouts");

    fireEvent.keyDown(window, { key: "Enter" });
    await screen.findByRole("main", { name: "Read trajectory" });
    fireEvent.click(screen.getByRole("button", { name: "Edit trajectory title and description" }));
    fireEvent.change(screen.getByRole("textbox", { name: "trajectory title" }), { target: { value: "Failed confirmation" } });
    fireEvent.change(screen.getByRole("textbox", { name: "trajectory description" }), { target: { value: "Token expires before confirmation" } });
    fireEvent.click(screen.getByRole("button", { name: "save" }));
    expect(document.querySelector(".lane-track header b")).toHaveTextContent("Failed confirmation");
    expect(document.querySelector(".workspace-console h2")).toHaveTextContent("Failed confirmation");
    expect(document.querySelector(".workspace-console .editable-metadata p")).toHaveTextContent("Token expires before confirmation");

    await waitFor(() => expect(JSON.parse(window.localStorage.getItem("rlviz.viewer-metadata.v1")!).trajectories).toBeTruthy());
    first.unmount();
    render(<App initialTrajectory={sampleTrajectory} />);
    expect(await screen.findByText("Checkout evaluation", { selector: ".workspace-rail h1" })).toBeInTheDocument();
    expect(await screen.findByText("Failed confirmation", { selector: ".lane-track header b" })).toBeInTheDocument();
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

  it("registers identical episode boundaries at Surface and Episodes depth", async () => {
    const trajectory = { id: "recurring", events: [
      { id: "opening", sequence: 0, kind: "message" },
      { id: "diagnose-1", sequence: 2, kind: "tool", alignment_key: "stage:diagnose" },
      { id: "diagnose-2", sequence: 4, kind: "observation", alignment_key: "stage:diagnose" },
      { id: "test", sequence: 6, kind: "tool", alignment_key: "stage:test" },
      { id: "diagnose-3", sequence: 9, kind: "error", alignment_key: "stage:diagnose" },
    ] };
    render(<App initialTrajectory={trajectory} />);
    fireEvent.keyDown(window, { key: "Enter" });
    await screen.findByRole("main", { name: "Read trajectory" });
    const boundaries = (selector: string) => [...document.querySelectorAll<HTMLElement>(selector)].map((element) => [element.dataset.episodeKey, element.dataset.episodeStart, element.dataset.episodeEnd]);
    const surface = boundaries(".shape-strip:not(.compact) [data-episode-key]");
    fireEvent.keyDown(window, { key: "Enter" });
    expect(boundaries(".episode-button")).toEqual(surface);
  });

  it("restores the exact pre-descend axis on keyboard and strip ascent", async () => {
    await openRead();
    fireEvent.keyDown(window, { key: "+" });
    fireEvent.keyDown(window, { key: "Enter" });
    const lane = screen.getByRole("main", { name: "Read trajectory" });
    const before = [lane.getAttribute("data-axis-start"), lane.getAttribute("data-axis-end")];
    fireEvent.keyDown(window, { key: "Enter" });
    fireEvent.keyDown(window, { key: "Escape" });
    expect([lane.getAttribute("data-axis-start"), lane.getAttribute("data-axis-end")]).toEqual(before);
    fireEvent.keyDown(window, { key: "Enter" });
    fireEvent.click(screen.getByRole("region", { name: "Compressed trajectory shape" }).querySelector("svg")!);
    expect([lane.getAttribute("data-axis-start"), lane.getAttribute("data-axis-end")]).toEqual(before);
  });

  it("gives pointer and keyboard descent the same containing episode window", async () => {
    const trajectory = { id: "layered", events: [
      { id: "start", sequence: 0, kind: "message", alignment_key: "stage:setup" },
      { id: "act", sequence: 20, kind: "tool", alignment_key: "stage:act" },
      ...[29, 30, 31, 32].map((sequence) => ({ id: `verify-${sequence}`, sequence, kind: "observation", alignment_key: "stage:verify" })),
      { id: "error", sequence: 34, kind: "error", alignment_key: "stage:verify" },
      { id: "outcome", sequence: 50, kind: "reward", alignment_key: "stage:outcome" },
    ] };
    const descend = async (pointer: boolean) => {
      const view = render(<App initialTrajectory={trajectory} />);
      fireEvent.keyDown(window, { key: "Enter" });
      const lane = await screen.findByRole("main", { name: "Read trajectory" });
      fireEvent.keyDown(window, { key: "+" }); fireEvent.keyDown(window, { key: "+" }); fireEvent.keyDown(window, { key: "+" });
      expect(Number(lane.getAttribute("data-axis-start"))).toBeGreaterThan(29);
      fireEvent.keyDown(window, { key: "Enter" });
      if (pointer) fireEvent.click(document.querySelector(".episode-button.selected")!);
      else fireEvent.keyDown(window, { key: "Enter" });
      const axis = [Number(lane.getAttribute("data-axis-start")), Number(lane.getAttribute("data-axis-end"))];
      view.unmount(); window.history.replaceState({}, "", "/"); window.localStorage.clear();
      return axis;
    };
    const pointer = await descend(true);
    const keyboard = await descend(false);
    expect(pointer).toEqual(keyboard);
    expect(pointer[0]).toBeLessThanOrEqual(29);
    expect(pointer[1]).toBeGreaterThanOrEqual(34);
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
    state.lanes = [{ id, sourceId: "sample", trajectoryId: sampleTrajectory.id, band: "context", selected: 8, depth: 4, fidelity: 3, axis: { start: 1, end: 10 }, descentStack: [] }];
    state.active = id;
    window.history.replaceState({ rlvizWorkspace: state }, "", `/?workspace=${encodeURIComponent(serializeWorkspace(state))}`);
    render(<App initialTrajectory={sampleTrajectory} />);
    const lane = screen.getByRole("main", { name: `Context lane ${sampleTrajectory.id}` });
    expect(lane).toHaveAttribute("data-depth", "1");
    expect(lane).toHaveAttribute("data-stored-depth", "4");
  });

  it("restores a context lane's stored depth when it is promoted to focus", async () => {
    const state = emptyWorkspace();
    const focusId = laneId("sample", sampleTrajectory.id);
    const contextId = laneId("sample", "context-copy");
    state.lanes = [
      { id: focusId, sourceId: "sample", trajectoryId: sampleTrajectory.id, band: "focus", selected: 0, depth: 1, fidelity: 3, axis: { start: 0, end: 10 }, descentStack: [] },
      { id: contextId, sourceId: "sample", trajectoryId: "context-copy", band: "context", selected: 8, depth: 3, fidelity: 3, axis: { start: 1, end: 10 }, descentStack: [{ depth: 2, axis: { start: 0, end: 10 } }] },
    ];
    state.active = contextId;
    window.history.replaceState({ rlvizWorkspace: state }, "", `/?workspace=${encodeURIComponent(serializeWorkspace(state))}`);
    render(<App initialTrajectory={sampleTrajectory} />);
    const context = await screen.findByRole("main", { name: "Context lane context-copy" });
    expect(context).toHaveAttribute("data-depth", "1");
    fireEvent.keyDown(window, { key: "Enter", shiftKey: true });
    expect(screen.getByRole("main", { name: "Read trajectory" })).toHaveAttribute("data-trajectory", "context-copy");
    expect(screen.getByRole("main", { name: "Read trajectory" })).toHaveAttribute("data-depth", "3");
  });

  it("switches light and dark mode through the data-theme attribute", () => {
    document.documentElement.setAttribute("data-theme", "light");
    render(<App initialTrajectory={sampleTrajectory} />);
    const dark = screen.getByRole("button", { name: "Dark" });
    const light = screen.getByRole("button", { name: "Light" });
    fireEvent.click(dark);
    expect(document.documentElement).toHaveAttribute("data-theme", "dark");
    expect(dark).toHaveAttribute("aria-pressed", "true");
    fireEvent.click(light);
    expect(document.documentElement).toHaveAttribute("data-theme", "light");
  });

  it("opens browser visitors on the overview with installation commands and data actions", () => {
    render(<App initialTrajectory={sampleTrajectory} setup={{ mode: "browser" }} />);
    expect(screen.getByRole("button", { name: "Overview" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("article", { name: "RLViz guide" })).toHaveTextContent("Welcome to RLViz");
    expect(screen.getByRole("article", { name: "RLViz guide" })).toHaveTextContent("Please read");
    expect(screen.getByRole("article", { name: "RLViz guide" })).toHaveTextContent("Inspect AI EvalLog JSON");
    expect(screen.getByRole("article", { name: "RLViz guide" })).toHaveTextContent("brew install TheSnakeFang/tap/rlviz");
    expect(screen.getByRole("button", { name: "Open trace directory" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open-source repository" })).toHaveAttribute("href", "https://github.com/TheSnakeFang/rlviz");
    expect(screen.getByRole("link", { name: "Updates on X" })).toHaveAttribute("href", "https://x.com/sofangtastic");
  });

  it("restores the active rollout after closing Guide or Settings", async () => {
    render(<App initialTrajectory={sampleTrajectory} />);
    fireEvent.click(screen.getByRole("article", { name: "RLViz guide" }).querySelector("button")!);
    fireEvent.click(screen.getByRole("region", { name: "RLViz settings" }).querySelector("button")!);
    fireEvent.keyDown(window, { key: "Enter" });
    const lane = await screen.findByRole("main", { name: "Read trajectory" });
    const laneID = lane.getAttribute("data-lane-id");
    fireEvent.keyDown(window, { key: "?" });
    fireEvent.keyDown(window, { key: "?" });
    expect(document.querySelector(".instrument-shell")).toHaveAttribute("data-active-zone", laneID);
    fireEvent.keyDown(window, { key: "S", shiftKey: true });
    fireEvent.keyDown(window, { key: "S", shiftKey: true });
    expect(document.querySelector(".instrument-shell")).toHaveAttribute("data-active-zone", laneID);
  });

  it("focuses the reading surface on first load and restores it after navigation", async () => {
    render(<App initialTrajectory={sampleTrajectory} />);
    const browse = screen.getByRole("main", { name: "Browse trajectories" });
    expect(browse).toHaveFocus();
    fireEvent.keyDown(browse, { key: "Enter" });
    const read = await screen.findByRole("main", { name: "Read trajectory" });
    await waitFor(() => expect(read).toHaveFocus());
    fireEvent.keyDown(read, { key: "j" });
    expect(document.querySelector(".workspace-console .moment.selected .address")).toHaveTextContent("10");
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
	expect(screen.getByText("raw")).toBeInTheDocument();
	expect(screen.getByRole("region", { name: "Event source" })).toBeInTheDocument();
	fireEvent.keyDown(window, { key: "Escape" });
	expect(screen.getByText("events")).toBeInTheDocument();
	fireEvent.keyDown(window, { key: "Escape" });
	expect(screen.getByText("episodes")).toBeInTheDocument();
	fireEvent.keyDown(window, { key: "Escape" });
	expect(screen.getByText(/overview · glyphs/)).toBeInTheDocument();
	fireEvent.keyDown(window, { key: "Escape" });
	expect(screen.getByRole("main", { name: "Browse trajectories" })).toBeInTheDocument();
  });

  it("keeps dockview geometry local while the shareable workspace URL stays compact", async () => {
    const first = render(<App initialTrajectory={sampleTrajectory} />);
    fireEvent.keyDown(window, { key: "Enter" });
    await screen.findByRole("main", { name: "Read trajectory" });
    fireEvent.keyDown(window, { key: "w", ctrlKey: true });
    expect(screen.getByRole("region", { name: "Workspace console" })).toHaveAttribute("data-resize-mode", "true");
    fireEvent.keyDown(window, { key: "ArrowUp" });
    fireEvent.keyDown(window, { key: "Escape" });
    const serialized = new URLSearchParams(window.location.search).get("workspace");
    expect(serialized).toBeTruthy();
    expect(JSON.parse(serialized!).layout).toBeUndefined();
    expect(JSON.parse(window.localStorage.getItem("rlviz.workspace.v6")!).layout).toBeTruthy();
    first.unmount();
    render(<App initialTrajectory={sampleTrajectory} />);
    expect(await screen.findByRole("main", { name: "Read trajectory" })).toHaveAttribute("data-trajectory", sampleTrajectory.id);
    expect(document.querySelector(".rlviz-dockview")).toBeInTheDocument();
  });

  it("restores a state-less history entry from its workspace URL", async () => {
    render(<App initialTrajectory={sampleTrajectory} />);
    const id = laneId("sample", sampleTrajectory.id);
    const linked = emptyWorkspace();
    linked.railExpanded = false;
    linked.active = id;
    linked.lanes = [{ id, sourceId: "sample", trajectoryId: sampleTrajectory.id, band: "focus", selected: 0, depth: 1, fidelity: 1, axis: { start: 0, end: 10 }, descentStack: [] }];
    window.history.replaceState(null, "", `/?workspace=${encodeURIComponent(serializeWorkspace(linked))}`);
    fireEvent(window, new PopStateEvent("popstate", { state: null }));
    await waitFor(() => expect(document.querySelector(".instrument-shell")).toHaveAttribute("data-active-zone", id));
  });

  it("disposes its capture listener when the dock lifecycle unmounts", async () => {
    const added = vi.spyOn(document, "addEventListener");
    const removed = vi.spyOn(document, "removeEventListener");
    const view = render(<App initialTrajectory={sampleTrajectory} />);
    await screen.findByRole("main", { name: "Browse trajectories" });
    const registrations = added.mock.calls.filter(([type, , options]) => type === "pointerdown" && options === true);
    expect(registrations.length).toBeGreaterThan(0);
    view.unmount();
    expect(registrations.some(([, listener]) => removed.mock.calls.some(([type, removedListener, options]) => type === "pointerdown" && removedListener === listener && options === true))).toBe(true);
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

  it("keeps the descent axis stack in jumplist snapshots", async () => {
    await openRead();
    fireEvent.keyDown(window, { key: "+" });
    fireEvent.keyDown(window, { key: "Enter" });
    const lane = screen.getByRole("main", { name: "Read trajectory" });
    const before = [lane.getAttribute("data-axis-start"), lane.getAttribute("data-axis-end")];
    fireEvent.keyDown(window, { key: "Enter" });
    const descended = [lane.getAttribute("data-axis-start"), lane.getAttribute("data-axis-end")];
    fireEvent.keyDown(window, { key: "o", ctrlKey: true });
    expect([lane.getAttribute("data-axis-start"), lane.getAttribute("data-axis-end")]).toEqual(before);
    fireEvent.keyDown(window, { key: "i", ctrlKey: true });
    expect([lane.getAttribute("data-axis-start"), lane.getAttribute("data-axis-end")]).toEqual(descended);
    fireEvent.keyDown(window, { key: "Escape" });
    expect([lane.getAttribute("data-axis-start"), lane.getAttribute("data-axis-end")]).toEqual(before);
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
    const before = document.querySelector(".workspace-console .moment.selected .address")?.textContent;
    const svg = screen.getByRole("region", { name: "Trajectory shape" }).querySelector("svg")!;
    Object.defineProperty(svg, "getBoundingClientRect", { value: () => ({ left: 0, top: 0, width: 1000, height: 200, right: 1000, bottom: 200, x: 0, y: 0, toJSON() {} }) });
    fireEvent.mouseMove(svg, { clientX: 22, clientY: 50 });
    expect(await screen.findByRole("status")).toBeInTheDocument();
    expect(document.querySelector(".workspace-console .moment.selected .address")).toHaveTextContent(before ?? "");
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
    expect(screen.getByRole("region", { name: "Active module shortcuts" })).toHaveTextContent("Increase fidelity");
  });

  it("moves detail from right to bottom with the keyboard and persists its dockview layout", async () => {
    const first = render(<App initialTrajectory={sampleTrajectory} />);
    fireEvent.keyDown(window, { key: "Enter" });
    await screen.findByRole("main", { name: "Read trajectory" });
    fireEvent.keyDown(window, { key: "Tab" });
    expect(document.querySelector(".instrument-shell")).toHaveAttribute("data-active-zone", "detail");
    fireEvent.keyDown(window, { key: "m", ctrlKey: true });
    expect(document.querySelector(".instrument-shell")).toHaveAttribute("data-move-mode", "true");
    expect(screen.getByRole("region", { name: "Active module shortcuts" })).toHaveTextContent("Exit move mode");
    expect(screen.getByRole("region", { name: "Active module shortcuts" })).not.toHaveTextContent("Next event");
    fireEvent.keyDown(window, { key: "m", ctrlKey: true });
    expect(document.querySelector(".instrument-shell")).toHaveAttribute("data-move-mode", "false");
    fireEvent.keyDown(window, { key: "m", ctrlKey: true });
    fireEvent.keyDown(window, { key: "ArrowDown" });
    expect(screen.getByRole("region", { name: "Workspace console" })).toHaveAttribute("data-dock-position", "bottom");
    fireEvent.keyDown(window, { key: "Escape" });
    expect(document.querySelector(".instrument-shell")).toHaveAttribute("data-move-mode", "false");
    fireEvent.keyDown(window, { key: "w", ctrlKey: true });
    expect(document.querySelector(".instrument-shell")).toHaveAttribute("data-resize-mode", "true");
    expect(screen.getByRole("region", { name: "Active module shortcuts" })).toHaveTextContent("Exit resize mode");
    fireEvent.keyDown(window, { key: "w", ctrlKey: true });
    expect(document.querySelector(".instrument-shell")).toHaveAttribute("data-resize-mode", "false");
    const serialized = new URLSearchParams(window.location.search).get("workspace");
    expect(JSON.parse(serialized!).layout).toBeUndefined();
    expect(JSON.parse(window.localStorage.getItem("rlviz.workspace.v6")!).layout).toBeTruthy();
    first.unmount();
    render(<App initialTrajectory={sampleTrajectory} />);
    expect(await screen.findByRole("region", { name: "Workspace console" })).toHaveAttribute("data-dock-position", "bottom");
  });

  it("changes overview fidelity in the active lane and names every visible step at detail fidelity", async () => {
    await openRead();
    const lane = screen.getByRole("main", { name: "Read trajectory" });
    expect(lane).toHaveAttribute("data-fidelity", "glyphs");
    fireEvent.keyDown(window, { key: "]" });
    expect(lane).toHaveAttribute("data-fidelity", "detail");
    expect(screen.getByRole("region", { name: "Rollout steps" })).toHaveTextContent("submit_order");
    expect(screen.getByRole("region", { name: "Rollout steps" }).querySelectorAll("button")).toHaveLength(sampleTrajectory.events.length);
    fireEvent.keyDown(window, { key: "[" });
    fireEvent.keyDown(window, { key: "[" });
    expect(lane).toHaveAttribute("data-fidelity", "hairline");
  });

  it("exposes an adjustable full-rollout timeline viewport", async () => {
    await openRead();
    const lane = screen.getByRole("main", { name: "Read trajectory" });
    const timeline = screen.getByRole("region", { name: "Rollout timeline viewport" });
    expect(timeline).toBeInTheDocument();
    fireEvent.change(screen.getByRole("slider", { name: "Viewport start" }), { target: { value: "3" } });
    expect(lane).toHaveAttribute("data-axis-start", "3.0000");
    expect(timeline.querySelector(".axis-window")).toBeInTheDocument();
  });

  it("recenters, pans, and resizes the timeline window with pointer gestures", async () => {
    vi.stubGlobal("PointerEvent", MouseEvent);
    await openRead();
    const lane = screen.getByRole("main", { name: "Read trajectory" });
    const map = screen.getByLabelText("Timeline overview");
    vi.spyOn(map, "getBoundingClientRect").mockReturnValue({ left: 100, right: 300, top: 0, bottom: 20, width: 200, height: 20, x: 100, y: 0, toJSON: () => ({}) } as DOMRect);
    fireEvent.change(screen.getByRole("slider", { name: "Viewport start" }), { target: { value: "3" } });
    fireEvent.change(screen.getByRole("slider", { name: "Viewport end" }), { target: { value: "7" } });

    fireEvent.pointerDown(map, { button: 0, pointerId: 1, clientX: 290 });
    fireEvent.pointerUp(map, { pointerId: 1, clientX: 290 });
    expect(Number(lane.getAttribute("data-axis-start"))).toBeCloseTo(6);
    expect(Number(lane.getAttribute("data-axis-end"))).toBeCloseTo(10);

    fireEvent.pointerDown(map, { button: 0, pointerId: 2, clientX: 256 });
    fireEvent.pointerMove(map, { pointerId: 2, clientX: 216 });
    fireEvent.pointerUp(map, { pointerId: 2, clientX: 216 });
    expect(Number(lane.getAttribute("data-axis-start"))).toBeCloseTo(4.2);
    expect(Number(lane.getAttribute("data-axis-end"))).toBeCloseTo(8.2);

    fireEvent.pointerDown(map, { button: 0, pointerId: 3, clientX: 171 });
    fireEvent.pointerMove(map, { pointerId: 3, clientX: 191 });
    fireEvent.pointerUp(map, { pointerId: 3, clientX: 191 });
    expect(Number(lane.getAttribute("data-axis-start"))).toBeCloseTo(5.1, 1);
    expect(Number(lane.getAttribute("data-axis-end"))).toBeCloseTo(8.2);
  });

  it("bins dense timeline marks while preserving exact tools, errors, and selection", async () => {
    const trajectory = {
      id: "dense-timeline",
      events: Array.from({ length: 1000 }, (_, index) => ({
        id: `event-${index}`,
        sequence: index,
        kind: index === 555 ? "error" : index % 100 === 0 ? "tool" : "message",
      })),
    };
    render(<App initialTrajectory={trajectory} />);
    fireEvent.keyDown(window, { key: "Enter" });
    await screen.findByRole("main", { name: "Read trajectory" });
    const timeline = screen.getByLabelText("Timeline overview");
    expect(timeline).toHaveAttribute("data-axis-mode", "binned");
    expect(Number(timeline.getAttribute("data-axis-nodes"))).toBeLessThan(500);
    expect(timeline.querySelectorAll(".axis-mark.tool").length).toBeGreaterThan(0);
    expect(timeline.querySelector(".axis-mark.error.selected")).toBeInTheDocument();
  });

  it("opens a rollout-pinned detail module whose navigation acts on that rollout", async () => {
    await openRead();
    fireEvent.keyDown(window, { key: "d" });
    const detail = await screen.findByRole("region", { name: `Detail for ${sampleTrajectory.id}` });
    expect(detail).toHaveAttribute("data-pinned", "true");
    await waitFor(() => expect(document.querySelector(".instrument-shell")).toHaveAttribute("data-active-zone", `detail:${laneId("sample", sampleTrajectory.id)}`));
    await waitFor(() => expect(detail).toHaveFocus());
    expect(detail.querySelector(".moment.selected")).toHaveTextContent("Stale confirmation token");
    fireEvent.keyDown(window, { key: "j" });
    expect(detail.querySelector(".moment.selected")).toHaveTextContent("Task completion grader");
    expect(screen.getByRole("region", { name: "Active module shortcuts" })).toHaveTextContent("Previous event");
  });

  it("removes the empty lane group when the final lane closes", async () => {
    render(<App initialTrajectory={sampleTrajectory} />);
    fireEvent.keyDown(window, { key: "Enter" });
    await screen.findByRole("main", { name: "Read trajectory" });
    fireEvent.keyDown(window, { key: "x" });
    expect(screen.getByRole("article", { name: "RLViz guide" })).toBeInTheDocument();
    expect(document.querySelector(".detail-empty")).not.toBeInTheDocument();
    expect(document.querySelector(".empty-stage")).not.toBeInTheDocument();
    expect(document.querySelectorAll(".dv-groupview:has(.lane-track)")).toHaveLength(0);
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
