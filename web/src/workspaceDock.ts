import type { RefObject } from "react";
import type { DockviewApi } from "dockview-react";
import type { WorkspaceLane, WorkspaceState } from "./workspace";

export const lanePanelId = (id: string) => `lane:${id}`;
export const laneIdFromPanel = (id: string) => id.startsWith("lane:") ? id.slice(5) : undefined;
export const pinnedDetailTarget = (id: string) => `detail:${id}`;
export const pinnedDetailLaneId = (target: string) => target.startsWith("detail:") ? target.slice(7) : undefined;
export const panelIdForTarget = (target: string) => target === "rail" ? "collection" : target === "guide" || target === "settings" || target === "detail" ? target : target.startsWith("detail:") ? target : lanePanelId(target);
export const targetFromPanelId = (id: string) => id === "collection" ? "rail" : id === "guide" || id === "settings" || id === "detail" ? id : id.startsWith("detail:") ? id : laneIdFromPanel(id);

export function defaultWorkspaceModuleSizes(width: number, height: number) {
  return {
    collectionWidth: Math.max(220, Math.round(width * 0.18)),
    guideWidth: Math.max(320, Math.round(width * 0.31)),
    settingsHeight: Math.max(150, Math.round(height * 0.2)),
  };
}

export function focusElementForTarget(target: string, railRef: RefObject<HTMLElement | null>): HTMLElement | null {
  const detailLane = pinnedDetailLaneId(target);
  if (target === "rail") return railRef.current;
  if (target === "guide") return document.querySelector<HTMLElement>(".workspace-guide");
  if (target === "settings") return document.querySelector<HTMLElement>(".workspace-settings");
  if (target === "detail") return document.querySelector<HTMLElement>(".workspace-console[data-shared-detail='true']");
  if (detailLane) return document.querySelector<HTMLElement>(`.workspace-console[data-pinned='true'][data-detail-lane-id="${CSS.escape(detailLane)}"]`);
  return document.querySelector<HTMLElement>(`[data-lane-id="${CSS.escape(target)}"]`);
}

export function addDefaultPanel(
  api: DockviewApi,
  workspace: WorkspaceState,
  id: string,
  kind: "collection" | "guide" | "settings" | "detail" | "lane",
  lane: WorkspaceLane | undefined,
  onDetailPosition: (position: "right" | "bottom") => void,
): void {
  const panels = api.panels;
  const defaultSizes = defaultWorkspaceModuleSizes(api.width, api.height);
  const firstLane = workspace.lanes.map((item) => api.getPanel(lanePanelId(item.id))).find(Boolean);
  const center = firstLane ?? panels.find((item) => item.id !== "collection" && item.id !== "detail") ?? panels[0];
  if (kind === "collection") {
    api.addPanel({ id, component: "workspace", tabComponent: "minimal", renderer: "always", title: "Collection", params: { kind, label: "collection" }, initialWidth: defaultSizes.collectionWidth, ...(center ? { position: { referencePanel: center, direction: "left" as const } } : {}) });
    return;
  }
  if (kind === "guide") {
    const anchor = api.getPanel("collection") ?? center;
    api.addPanel({ id, component: "workspace", tabComponent: "minimal", renderer: "always", title: "Guide", params: { kind, label: "guide" }, initialWidth: defaultSizes.guideWidth, ...(anchor ? { position: { referencePanel: anchor, direction: "right" as const } } : {}) });
    return;
  }
  if (kind === "settings") {
    const detail = api.getPanel("detail");
    const anchor = detail ?? api.getPanel("guide") ?? api.getPanel("collection") ?? center;
    api.addPanel({ id, component: "workspace", tabComponent: "minimal", renderer: "always", title: "Settings", params: { kind, label: "settings" }, initialWidth: Math.max(280, api.width * 0.28), initialHeight: defaultSizes.settingsHeight, ...(anchor ? { position: { referencePanel: anchor, direction: detail ? "below" as const : "right" as const } } : {}) });
    return;
  }
  if (kind === "detail") {
    const pinnedLane = lane ? api.getPanel(lanePanelId(lane.id)) : undefined;
    const bottom = workspace.direction === "columns";
    const anchor = pinnedLane ?? center;
    api.addPanel({ id, component: "workspace", tabComponent: "minimal", renderer: "always", title: lane ? `Detail · ${lane.trajectoryId}` : "Detail", params: { kind, laneId: lane?.id, label: lane ? `detail · ${lane.trajectoryId}` : "detail" }, initialWidth: Math.max(260, api.width * 0.28), initialHeight: Math.max(150, api.height * 0.28), ...(anchor ? { position: { referencePanel: anchor, direction: lane ? "right" as const : bottom ? "below" as const : "right" as const } } : {}) });
    onDetailPosition(bottom ? "bottom" : "right");
    return;
  }
  const focusPanels = workspace.lanes.filter((item) => item.band === "focus" && item.id !== lane?.id).map((item) => api.getPanel(lanePanelId(item.id))).filter(Boolean);
  const contextPanel = workspace.lanes.filter((item) => item.band === "context" && item.id !== lane?.id).map((item) => api.getPanel(lanePanelId(item.id))).find(Boolean);
  const reference = lane?.band === "context" ? contextPanel ?? focusPanels[0] ?? center : focusPanels.at(-1) ?? center;
  const direction = lane?.band === "context" ? "below" : workspace.direction === "rows" && focusPanels.length ? "below" : "right";
  api.addPanel({ id, component: "workspace", tabComponent: "minimal", renderer: "always", title: lane?.trajectoryId ?? "Lane", params: { kind, laneId: lane?.id, label: lane?.trajectoryId ?? "lane" }, initialHeight: lane?.band === "context" ? 92 : undefined, ...(reference ? { position: { referencePanel: reference, direction } } : {}) });
}

export function reconcileDockPanels(
  api: DockviewApi,
  workspace: WorkspaceState,
  onDetailPosition: (position: "right" | "bottom") => void,
): boolean {
  let changed = false;
  const desired = new Set([
    ...(workspace.lanes.length && workspace.detailOpen ? ["detail"] : []),
    ...(workspace.guideOpen ? ["guide"] : []),
    ...(workspace.settingsOpen ? ["settings"] : []),
    ...(workspace.railExpanded ? ["collection"] : []),
    ...workspace.lanes.map((lane) => lanePanelId(lane.id)),
    ...workspace.details.map(pinnedDetailTarget),
  ]);
  api.panels.filter((panel) => !desired.has(panel.id)).forEach((panel) => { changed = true; api.removePanel(panel); });
  workspace.lanes.forEach((lane) => {
    if (!api.getPanel(lanePanelId(lane.id))) {
      changed = true;
      addDefaultPanel(api, workspace, lanePanelId(lane.id), "lane", lane, onDetailPosition);
    }
  });
  if (workspace.railExpanded && !api.getPanel("collection")) {
    changed = true;
    addDefaultPanel(api, workspace, "collection", "collection", undefined, onDetailPosition);
  }
  if (workspace.guideOpen && !api.getPanel("guide")) {
    changed = true;
    addDefaultPanel(api, workspace, "guide", "guide", undefined, onDetailPosition);
  }
  if (workspace.settingsOpen && !api.getPanel("settings")) {
    changed = true;
    addDefaultPanel(api, workspace, "settings", "settings", undefined, onDetailPosition);
  }
  if (workspace.lanes.length && workspace.detailOpen && !api.getPanel("detail")) {
    changed = true;
    addDefaultPanel(api, workspace, "detail", "detail", undefined, onDetailPosition);
  }
  workspace.details.forEach((id) => {
    const lane = workspace.lanes.find((item) => item.id === id);
    const panelId = pinnedDetailTarget(id);
    if (lane && !api.getPanel(panelId)) {
      changed = true;
      addDefaultPanel(api, workspace, panelId, "detail", lane, onDetailPosition);
    }
  });
  return changed;
}
