import { act, renderHook, waitFor } from "@testing-library/react";
import { createRef, useRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { emptyWorkspace, type WorkspaceState } from "./workspace";

const dockMocks = vi.hoisted(() => ({
  focusElementForTarget: vi.fn(),
  reconcileDockPanels: vi.fn(),
}));

vi.mock("./workspaceDock", async (importOriginal) => ({
  ...await importOriginal<typeof import("./workspaceDock")>(),
  focusElementForTarget: dockMocks.focusElementForTarget,
  reconcileDockPanels: dockMocks.reconcileDockPanels,
}));

import { useWorkspaceDock } from "./useWorkspaceDock";

function fakeApi() {
  let activeListener: ((event: { panel?: { id: string } }) => void) | undefined;
  let layoutListener: (() => void) | undefined;
  const activeDispose = vi.fn();
  const layoutDispose = vi.fn();
  const api = {
    activePanel: undefined,
    panels: [],
    totalPanels: 1,
    clear: vi.fn(),
    fromJSON: vi.fn(),
    getPanel: vi.fn(() => undefined),
    adjacentGroupInDirection: vi.fn(() => undefined),
    onDidActivePanelChange: vi.fn((listener) => { activeListener = listener; return { dispose: activeDispose }; }),
    onDidLayoutChange: vi.fn((listener) => { layoutListener = listener; return { dispose: layoutDispose }; }),
    toJSON: vi.fn(() => ({ grid: { root: { type: "leaf", data: { views: [] as string[] } } }, panels: {}, activeGroup: undefined })),
  };
  return { api, activeDispose, layoutDispose, emitActive: (id: string) => activeListener?.({ panel: { id } }), emitLayout: () => layoutListener?.() };
}

function renderDock(workspace: WorkspaceState, change = vi.fn()) {
  const railRef = createRef<HTMLElement>();
  return {
    change,
    ...renderHook(({ state }) => {
      const workspaceRef = useRef(state);
      workspaceRef.current = state;
      return useWorkspaceDock({ workspace: state, workspaceRef, railRef, change, trajectoryTitles: {} });
    }, { initialProps: { state: workspace } }),
  };
}

describe("useWorkspaceDock lifecycle", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    dockMocks.focusElementForTarget.mockReset();
    dockMocks.reconcileDockPanels.mockReset();
    document.body.replaceChildren();
  });

  it("replaces and disposes Dockview subscriptions and the capture listener", () => {
    const added = vi.spyOn(document, "addEventListener");
    const removed = vi.spyOn(document, "removeEventListener");
    const view = renderDock(emptyWorkspace());
    const first = fakeApi(), second = fakeApi();

    act(() => view.result.current.onReady({ api: first.api } as never));
    const firstPointer = added.mock.calls.find(([type, , options]) => type === "pointerdown" && options === true)?.[1];
    act(() => view.result.current.onReady({ api: second.api } as never));

    expect(first.activeDispose).toHaveBeenCalledOnce();
    expect(first.layoutDispose).toHaveBeenCalledOnce();
    expect(removed).toHaveBeenCalledWith("pointerdown", firstPointer, true);
    view.unmount();
    expect(second.activeDispose).toHaveBeenCalledOnce();
    expect(second.layoutDispose).toHaveBeenCalledOnce();
  });

  it("recovers malformed saved geometry, reconciles, persists, and focuses the logical target", async () => {
    const workspace = { ...emptyWorkspace(), layout: { invalid: true } as never };
    const focusTarget = document.createElement("main");
    focusTarget.tabIndex = 0;
    document.body.append(focusTarget);
    dockMocks.focusElementForTarget.mockReturnValue(focusTarget);
    const view = renderDock(workspace);
    const dock = fakeApi();
    dock.api.fromJSON.mockImplementation(() => { throw new Error("bad layout"); });

    act(() => view.result.current.persistLayout(dock.api as never));
    expect(view.change).toHaveBeenCalledWith(expect.any(Function), false);
    const persist = view.change.mock.calls.at(-1)?.[0];
    expect(persist?.(workspace).layout).toEqual(dock.api.toJSON.mock.results.at(-1)?.value);
    expect(dock.api.toJSON).toHaveBeenCalled();
    act(() => view.result.current.onReady({ api: dock.api } as never));

    expect(dock.api.clear).toHaveBeenCalledOnce();
    expect(dockMocks.reconcileDockPanels).toHaveBeenCalledWith(dock.api, workspace, expect.any(Function));
    await waitFor(() => expect(focusTarget).toHaveFocus());
  });

  it("accepts active-panel changes only after a user points at a Dockview tab", () => {
    const view = renderDock(emptyWorkspace());
    const dock = fakeApi();
    act(() => view.result.current.onReady({ api: dock.api } as never));

    const changesAfterReady = view.change.mock.calls.length;
    act(() => dock.emitActive("detail"));
    expect(view.change).toHaveBeenCalledTimes(changesAfterReady);

    const tab = document.createElement("button");
    tab.className = "dv-tab";
    document.body.append(tab);
    act(() => {
      tab.dispatchEvent(new Event("pointerdown", { bubbles: true }));
      dock.emitActive("detail");
    });
    const update = view.change.mock.calls.at(-1)?.[0];
    expect(update?.(emptyWorkspace()).active).toBe("detail");
  });

  it("activates the spatially adjacent module for an arrow direction", () => {
    const view = renderDock(emptyWorkspace());
    const dock = fakeApi();
    const group = { api: { boundingBox: { top: 0, left: 0, width: 100, height: 100 } } };
    const current = { id: "collection", api: { group, setActive: vi.fn(), setTitle: vi.fn() } };
    const setActive = vi.fn();
    const detail = { id: "detail", api: { setActive } };
    dock.api.getPanel.mockReturnValue(current as never);
    dock.api.adjacentGroupInDirection.mockReturnValue({ activePanel: detail } as never);
    act(() => view.result.current.onReady({ api: dock.api } as never));

    expect(view.result.current.activateAdjacent("ArrowRight")).toBe("detail");
    expect(dock.api.adjacentGroupInDirection).toHaveBeenCalledWith(group, "right");
    expect(setActive).toHaveBeenCalledOnce();
  });

  it("coalesces rapid layout geometry and persists only after the layout settles", () => {
    vi.useFakeTimers();
    const frames: FrameRequestCallback[] = [];
    vi.stubGlobal("requestAnimationFrame", vi.fn((callback: FrameRequestCallback) => { frames.push(callback); return frames.length; }));
    vi.stubGlobal("cancelAnimationFrame", vi.fn());
    const view = renderDock(emptyWorkspace());
    const dock = fakeApi();
    act(() => view.result.current.onReady({ api: dock.api } as never));
    while (frames.length) act(() => frames.shift()!(0));
    view.change.mockClear();
    dock.api.toJSON.mockClear();
    dock.api.toJSON.mockReturnValue({ grid: { root: { type: "leaf", data: { views: ["detail"] } } }, panels: {}, activeGroup: undefined });

    act(() => { dock.emitLayout(); dock.emitLayout(); dock.emitLayout(); });
    expect(frames).toHaveLength(2);
    expect(view.change).not.toHaveBeenCalled();
    act(() => vi.advanceTimersByTime(139));
    expect(view.change).not.toHaveBeenCalled();
    act(() => vi.advanceTimersByTime(1));
    expect(view.change).toHaveBeenCalledTimes(1);
    const persist = view.change.mock.calls[0][0];
    persist(emptyWorkspace());
    expect(dock.api.toJSON).toHaveBeenCalledTimes(1);
  });
});
