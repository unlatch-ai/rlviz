import { useCallback, useEffect, useRef, useState } from "react";
import type { MutableRefObject, RefObject } from "react";
import type { DockviewApi, DockviewReadyEvent, GroupNavigationDirection, Position } from "dockview-react";
import type { WorkspaceState } from "./workspace";
import {
  focusElementForTarget,
  lanePanelId,
  panelIdForTarget,
  pinnedDetailTarget,
  reconcileDockPanels,
  targetFromPanelId,
} from "./workspaceDock";

type WorkspaceChange = (update: (current: WorkspaceState) => WorkspaceState, snapshot?: boolean) => void;

export function useWorkspaceDock({
  workspace,
  workspaceRef,
  railRef,
  change,
  collectionTitle,
  trajectoryTitles,
}: {
  workspace: WorkspaceState;
  workspaceRef: MutableRefObject<WorkspaceState>;
  railRef: RefObject<HTMLElement | null>;
  change: WorkspaceChange;
  collectionTitle?: string;
  trajectoryTitles: Record<string, string | undefined>;
}) {
  const apiRef = useRef<DockviewApi | null>(null);
  const syncing = useRef(false);
  const settling = useRef(false);
  const tabPointerAt = useRef(0);
  const openingPinned = useRef<string | undefined>(undefined);
  const focusFrame = useRef<number | undefined>(undefined);
  const geometryFrame = useRef<number | undefined>(undefined);
  const persistenceTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const persistedLayout = useRef<string | undefined>(workspace.layout ? JSON.stringify(workspace.layout) : undefined);
  const cleanup = useRef<(() => void)[]>([]);
  const [ready, setReady] = useState(false);
  const [detailPosition, setDetailPosition] = useState<"right" | "bottom">(workspace.direction === "columns" ? "bottom" : "right");

  const scheduleFocus = useCallback((target = workspaceRef.current.active, onlyWhenLost = false) => {
    if (onlyWhenLost && focusFrame.current !== undefined) return;
    if (focusFrame.current !== undefined) cancelAnimationFrame(focusFrame.current);
    focusFrame.current = requestAnimationFrame(() => {
      focusFrame.current = undefined;
      if (onlyWhenLost && document.activeElement && document.activeElement !== document.body) return;
      if (document.activeElement instanceof HTMLInputElement || document.activeElement instanceof HTMLTextAreaElement) return;
      const node = focusElementForTarget(target, railRef);
      if (node) {
        node.focus({ preventScroll: true });
        if (openingPinned.current === target) openingPinned.current = undefined;
      }
      settling.current = false;
    });
  }, [railRef, workspaceRef]);

  const annotateGeometry = useCallback((api = apiRef.current) => {
    if (!api) return;
    const modules = {
      rail: document.querySelector<HTMLElement>(".workspace-rail"),
      console: document.querySelector<HTMLElement>(".workspace-console"),
      focusLane: document.querySelectorAll<HTMLElement>(".lane-track.focus-lane")[1],
      focusContext: document.querySelector<HTMLElement>(".lane-track.context-lane"),
    };
    const sashes = [...document.querySelectorAll<HTMLElement>(".rlviz-dockview .dv-sash:not(.dv-disabled)")];
    sashes.forEach((sash) => sash.removeAttribute("data-seam"));
    const used = new Set<HTMLElement>();
    for (const [name, module] of Object.entries(modules)) {
      if (!module) continue;
      const box = module.getBoundingClientRect();
      let best: HTMLElement | undefined;
      let distance = Infinity;
      for (const sash of sashes) {
        if (used.has(sash)) continue;
        const candidate = sash.getBoundingClientRect();
        const vertical = candidate.height > candidate.width;
        const next = vertical
          ? Math.min(Math.abs(candidate.left - box.left), Math.abs(candidate.left - box.right))
          : Math.min(Math.abs(candidate.top - box.top), Math.abs(candidate.top - box.bottom));
        if (next < distance) { best = sash; distance = next; }
      }
      if (best) { best.dataset.seam = name; used.add(best); }
    }
    const detail = api.getPanel("detail")?.api.group.api.boundingBox;
    const center = workspaceRef.current.lanes.map((lane) => api.getPanel(lanePanelId(lane.id))?.api.group.api.boundingBox).find(Boolean);
    if (detail && center) setDetailPosition(detail.top >= center.top + center.height * 0.5 ? "bottom" : "right");
  }, [workspaceRef]);

  const persistLayout = useCallback((api = apiRef.current) => {
    if (!api || syncing.current || !api.totalPanels) return;
    const layout = api.toJSON();
    const serialized = JSON.stringify(layout);
    if (serialized === persistedLayout.current) return;
    persistedLayout.current = serialized;
    change((current) => ({ ...current, layout }), false);
  }, [change]);

  const scheduleGeometry = useCallback((api: DockviewApi) => {
    if (geometryFrame.current !== undefined) return;
    geometryFrame.current = requestAnimationFrame(() => {
      geometryFrame.current = undefined;
      annotateGeometry(api);
    });
  }, [annotateGeometry]);

  const schedulePersistence = useCallback((api: DockviewApi, delay = 140) => {
    if (persistenceTimer.current !== undefined) clearTimeout(persistenceTimer.current);
    persistenceTimer.current = setTimeout(() => {
      persistenceTimer.current = undefined;
      persistLayout(api);
    }, delay);
  }, [persistLayout]);

  const flushPersistence = useCallback((api: DockviewApi) => {
    if (persistenceTimer.current !== undefined) clearTimeout(persistenceTimer.current);
    persistenceTimer.current = undefined;
    persistLayout(api);
  }, [persistLayout]);

  const reconcile = useCallback((api: DockviewApi) => {
    settling.current = true;
    syncing.current = true;
    try {
      reconcileDockPanels(api, workspaceRef.current, setDetailPosition);
    } finally {
      syncing.current = false;
    }
    requestAnimationFrame(() => {
      annotateGeometry(api);
      persistLayout(api);
      const target = workspaceRef.current.active;
      const panel = api.getPanel(panelIdForTarget(target));
      if (panel && api.activePanel?.id !== panel.id) panel.api.setActive();
      scheduleFocus(target);
    });
  }, [annotateGeometry, persistLayout, scheduleFocus, workspaceRef]);

  const disposeLifecycle = useCallback(() => {
    cleanup.current.splice(0).forEach((dispose) => dispose());
    if (focusFrame.current !== undefined) cancelAnimationFrame(focusFrame.current);
    if (geometryFrame.current !== undefined) cancelAnimationFrame(geometryFrame.current);
    if (persistenceTimer.current !== undefined) clearTimeout(persistenceTimer.current);
    focusFrame.current = undefined;
    geometryFrame.current = undefined;
    persistenceTimer.current = undefined;
  }, []);

  const onReady = useCallback((event: DockviewReadyEvent) => {
    disposeLifecycle();
    const api = event.api;
    apiRef.current = api;
    syncing.current = true;
    if (workspaceRef.current.layout) {
      try { api.fromJSON(workspaceRef.current.layout); }
      catch { api.clear(); }
    }
    syncing.current = false;
    persistedLayout.current = workspaceRef.current.layout ? JSON.stringify(workspaceRef.current.layout) : undefined;

    const activeDisposable = api.onDidActivePanelChange(({ panel }) => {
      if (syncing.current || !panel || Date.now() - tabPointerAt.current > 400) return;
      const active = targetFromPanelId(panel.id);
      if (active) change((current) => ({ ...current, active }), false);
    });
    const onTabPointer = (event: PointerEvent) => {
      if (event.target instanceof Element && event.target.closest(".dv-tabs-and-actions-container, .dv-tab")) tabPointerAt.current = Date.now();
    };
    document.addEventListener("pointerdown", onTabPointer, true);
    const layoutDisposable = api.onDidLayoutChange(() => {
      scheduleGeometry(api);
      schedulePersistence(api);
      scheduleFocus(workspaceRef.current.active, true);
    });
    cleanup.current.push(
      () => activeDisposable.dispose(),
      () => layoutDisposable.dispose(),
      () => document.removeEventListener("pointerdown", onTabPointer, true),
    );
    reconcile(api);
    document.querySelectorAll(".dv-live-region, .dv-live-region-assertive").forEach((node) => node.removeAttribute("role"));
    setReady(true);
  }, [change, disposeLifecycle, reconcile, scheduleFocus, scheduleGeometry, schedulePersistence, workspaceRef]);

  useEffect(() => () => {
    disposeLifecycle();
    apiRef.current = null;
  }, [disposeLifecycle]);
  useEffect(() => {
    const api = apiRef.current;
    if (api) reconcile(api);
  }, [reconcile, workspace.details, workspace.guideOpen, workspace.settingsOpen, workspace.lanes, workspace.railExpanded]);
  useEffect(() => {
    const api = apiRef.current;
    if (!api) return;
    const panelId = panelIdForTarget(workspace.active);
    const panel = api.getPanel(panelId);
    if (panel && api.activePanel?.id !== panelId) panel.api.setActive();
    scheduleFocus(workspace.active);
  }, [ready, scheduleFocus, workspace.active]);
  useEffect(() => {
    const api = apiRef.current;
    if (!api) return;
    api.getPanel("collection")?.api.setTitle(collectionTitle ?? "Collection");
    api.getPanel("detail")?.api.setTitle("Detail");
    workspace.lanes.forEach((lane) => {
      const title = trajectoryTitles[lane.id] ?? lane.trajectoryId;
      api.getPanel(lanePanelId(lane.id))?.api.setTitle(title);
      api.getPanel(pinnedDetailTarget(lane.id))?.api.setTitle(`Detail · ${title}`);
    });
  }, [collectionTitle, ready, trajectoryTitles, workspace.lanes]);

  const dockDetail = useCallback((position: "right" | "bottom") => {
    const api = apiRef.current;
    const detail = api?.getPanel("detail");
    const center = workspaceRef.current.lanes.map((lane) => api?.getPanel(lanePanelId(lane.id))).find(Boolean) ?? api?.getPanel("collection");
    if (!api || !detail || !center || detail.id === center.id) return;
    detail.api.moveTo({ group: center.api.group, position });
    setDetailPosition(position);
    requestAnimationFrame(() => flushPersistence(api));
  }, [flushPersistence, workspaceRef]);

  const arrangeLanes = useCallback((direction: "rows" | "columns") => {
    change((current) => ({ ...current, direction }));
    requestAnimationFrame(() => {
      const api = apiRef.current;
      if (!api) return;
      const panels = workspaceRef.current.lanes.map((lane) => api.getPanel(lanePanelId(lane.id))).filter((panel): panel is NonNullable<typeof panel> => !!panel);
      for (let index = 1; index < panels.length; index++) panels[index].api.moveTo({ group: panels[index - 1].api.group, position: direction === "rows" ? "bottom" : "right" });
      dockDetail(direction === "rows" ? "right" : "bottom");
      requestAnimationFrame(() => flushPersistence(api));
    });
  }, [change, dockDetail, flushPersistence, workspaceRef]);

  const arrangeWelcomeLayout = useCallback(() => {
    let attempts = 0;
    const arrange = () => {
      const api = apiRef.current;
      const lanes = workspaceRef.current.lanes.slice(0, 2).map((lane) => api?.getPanel(lanePanelId(lane.id)));
      const collection = api?.getPanel("collection"), guide = api?.getPanel("guide"), detail = api?.getPanel("detail"), settings = api?.getPanel("settings");
      if (!api || !collection || !guide || !detail || !settings || lanes.some((lane) => !lane)) {
        if (++attempts < 8) requestAnimationFrame(arrange);
        return;
      }
      guide.api.moveTo({ group: collection.api.group, position: "right" });
      lanes[0]!.api.moveTo({ group: guide.api.group, position: "right" });
      settings.api.moveTo({ group: lanes[0]!.api.group, position: "bottom" });
      detail.api.moveTo({ group: lanes[0]!.api.group, position: "right" });
      lanes[1]!.api.moveTo({ group: lanes[0]!.api.group, position: "bottom" });
      collection.api.group.api.setSize({ width: Math.max(240, Math.round(api.width * 0.22)) });
      guide.api.group.api.setSize({ width: Math.max(360, Math.round(api.width * 0.34)) });
      settings.api.group.api.setSize({ height: Math.max(180, Math.round(api.height * 0.34)) });
      setDetailPosition("right");
      requestAnimationFrame(() => flushPersistence(api));
    };
    requestAnimationFrame(arrange);
  }, [flushPersistence, workspaceRef]);

  const moveActiveModule = useCallback((key: string) => {
    const api = apiRef.current;
    const panel = api?.activePanel;
    if (!api || !panel) return;
    const center = workspaceRef.current.lanes.map((lane) => api.getPanel(lanePanelId(lane.id))).find((candidate) => candidate && candidate.id !== panel.id)
      ?? api.panels.find((candidate) => candidate.id !== panel.id && candidate.id !== "collection" && candidate.id !== "detail")
      ?? api.panels.find((candidate) => candidate.id !== panel.id);
    if (!center) return;
    const position: Position = key === "ArrowLeft" ? "left" : key === "ArrowRight" ? "right" : key === "ArrowDown" ? "bottom" : "top";
    panel.api.moveTo({ group: center.api.group, position });
    if (panel.id === "detail") setDetailPosition(position === "bottom" ? "bottom" : "right");
    requestAnimationFrame(() => flushPersistence(api));
  }, [flushPersistence, workspaceRef]);

  const resizeNearest = useCallback((key: string) => {
    const api = apiRef.current;
    const singleLane = workspaceRef.current.lanes.length === 1 && workspaceRef.current.lanes[0].id === workspaceRef.current.active;
    const panel = singleLane ? api?.getPanel("detail") : api?.activePanel;
    const box = panel?.api.group.api.boundingBox;
    if (!panel || !box) return;
    const delta = key === "ArrowRight" || key === "ArrowDown" ? 24 : -24;
    if (singleLane || key === "ArrowLeft" || key === "ArrowRight") panel.api.group.api.setSize({ width: Math.max(120, box.width + delta) });
    else panel.api.group.api.setSize({ height: Math.max(72, box.height + delta) });
    if (api) requestAnimationFrame(() => flushPersistence(api));
  }, [flushPersistence, workspaceRef]);

  const activateTarget = useCallback((target: string) => {
    apiRef.current?.getPanel(panelIdForTarget(target))?.api.setActive();
    scheduleFocus(target);
  }, [scheduleFocus]);

  const activateAdjacent = useCallback((key: string) => {
    const direction = key.slice(5).toLowerCase() as GroupNavigationDirection;
    const api = apiRef.current;
    const current = api?.getPanel(panelIdForTarget(workspaceRef.current.active));
    const group = current?.api.group;
    const panel = api && group ? api.adjacentGroupInDirection(group, direction)?.activePanel : undefined;
    const target = panel ? targetFromPanelId(panel.id) : undefined;
    if (panel && target) { panel.api.setActive(); scheduleFocus(target); }
    return target;
  }, [scheduleFocus, workspaceRef]);

  const beginPinnedOpen = useCallback((laneId: string) => {
    const target = pinnedDetailTarget(laneId);
    openingPinned.current = target;
    change((current) => ({ ...current, details: current.details.includes(laneId) ? current.details : [...current.details, laneId], active: target }));
  }, [change]);

  const canActivateContent = useCallback((target: string) => {
    return !(syncing.current || settling.current || (target === "detail" && openingPinned.current));
  }, []);

  return {
    apiRef,
    detailPosition,
    onReady,
    persistLayout,
    arrangeLanes,
    arrangeWelcomeLayout,
    moveActiveModule,
    resizeNearest,
    activateTarget,
    activateAdjacent,
    beginPinnedOpen,
    canActivateContent,
  };
}
