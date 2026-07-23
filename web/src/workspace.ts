export type LaneBand = "focus" | "context";
export type WorkspaceDirection = "rows" | "columns";
export type CollectionView = "rollouts" | "trials";
export type WorkspaceTarget = "rail" | string;

export interface AxisWindow { start: number; end: number }
export interface LaneDepthSnapshot { depth: number; axis: AxisWindow }

export interface WorkspaceLane {
  id: string;
  sourceId: string;
  trajectoryId: string;
  band: LaneBand;
  selected: number;
  depth: number;
  fidelity: number;
  axis: AxisWindow;
  descentStack: LaneDepthSnapshot[];
}

/** Context lanes render Surface without discarding the focus lane's stored depth. */
export function effectiveDepth(lane: WorkspaceLane): number {
  return lane.band === "context" ? 1 : lane.depth;
}

export interface WorkspaceState {
  version: 3;
  railExpanded: boolean;
  railQuery: string;
  railSelected: number;
  collectionView: CollectionView;
  guideOpen: boolean;
  settingsOpen: boolean;
  lanes: WorkspaceLane[];
  /** Rollout-pinned detail modules. Values are lane IDs. */
  details: string[];
  direction: WorkspaceDirection;
  reference?: string;
  active: WorkspaceTarget;
  /** Dockview's canonical layout. Undefined means build the default layout. */
  layout?: SerializedDockview;
}

// The onboarding topology changed materially: do not restore older geometry
// over the full-width Settings row in the welcome workspace.
export const workspaceStorageKey = "rlviz.workspace.v6";
const maximumDockLayoutBytes = 256 * 1024;
const maximumDockPanels = 64;
const maximumDockNodes = 256;
const maximumDockDepth = 32;

export function emptyWorkspace(): WorkspaceState {
  return {
    version: 3,
    railExpanded: true,
    railQuery: "",
    railSelected: 0,
    collectionView: "rollouts",
    guideOpen: true,
    settingsOpen: true,
    lanes: [],
    details: [],
    direction: "rows",
    active: "rail",
  };
}

export function laneId(sourceId: string, trajectoryId: string): string {
  return `${encodeURIComponent(sourceId)}:${encodeURIComponent(trajectoryId)}`;
}

const finite = (value: unknown): value is number => typeof value === "number" && Number.isFinite(value);
const clamp = (value: number, minimum: number, maximum: number) => Math.max(minimum, Math.min(maximum, value));

export function normalizeWorkspace(value: unknown): WorkspaceState | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const input = value as Record<string, unknown> & { version?: number; lanes?: unknown[]; layout?: unknown };
  if ((input.version !== 2 && input.version !== 3) || !Array.isArray(input.lanes)) return undefined;
  const raw = input as unknown as Partial<WorkspaceState>;
  const seen = new Set<string>();
  const lanes = input.lanes.flatMap((candidate) => {
    if (!candidate || typeof candidate !== "object") return [];
    const lane = candidate as Partial<WorkspaceLane>;
    if (typeof lane.sourceId !== "string" || typeof lane.trajectoryId !== "string" || (lane.band !== "focus" && lane.band !== "context")) return [];
    const axis = lane.axis && finite(lane.axis.start) && finite(lane.axis.end) && lane.axis.end >= lane.axis.start ? lane.axis : { start: 0, end: 1 };
    const descentStack = Array.isArray(lane.descentStack) ? lane.descentStack.flatMap((candidate) => {
      if (!candidate || typeof candidate !== "object") return [];
      const snapshot = candidate as Partial<LaneDepthSnapshot>;
      if (!finite(snapshot.depth) || !snapshot.axis || !finite(snapshot.axis.start) || !finite(snapshot.axis.end) || snapshot.axis.end < snapshot.axis.start) return [];
      return [{ depth: clamp(Math.round(snapshot.depth), 1, 3), axis: snapshot.axis }];
    }).slice(-3) : [];
    const id = laneId(lane.sourceId, lane.trajectoryId);
    if (seen.has(id)) return [];
    seen.add(id);
    return [{
      id, sourceId: lane.sourceId, trajectoryId: lane.trajectoryId, band: lane.band,
      selected: clamp(finite(lane.selected) ? Math.round(lane.selected) : 0, 0, Number.MAX_SAFE_INTEGER),
      depth: clamp(finite(lane.depth) ? Math.round(lane.depth) : 1, 1, 4),
      fidelity: clamp(finite(lane.fidelity) ? Math.round(lane.fidelity) : 1, 0, 2), axis, descentStack,
    } satisfies WorkspaceLane];
  });
  // A restored malformed link cannot overfill the focus band.
  let focusCount = 0;
  lanes.forEach((lane) => {
    if (lane.band === "focus" && focusCount < 2) focusCount++;
    else if (lane.band === "focus") lane.band = "context";
  });
  const ids = new Set(lanes.map((lane) => lane.id));
  const details = Array.isArray(raw.details) ? [...new Set(raw.details.filter((id): id is string => typeof id === "string" && ids.has(id)))] : [];
  const detailTargets = new Set(details.map((id) => `detail:${id}`));
  const railExpanded = raw.railExpanded !== false || lanes.length === 0;
  // Older saved/read links with lanes keep their existing topology. A truly
  // empty legacy workspace gets the new first-run Guide.
  const guideOpen = typeof raw.guideOpen === "boolean" ? raw.guideOpen : lanes.length === 0;
  const settingsOpen = typeof raw.settingsOpen === "boolean" ? raw.settingsOpen : lanes.length === 0;
  const requestedActive = raw.active === "rail" || (raw.active === "detail" && lanes.length > 0) || (raw.active === "guide" && guideOpen) || (raw.active === "settings" && settingsOpen) || (typeof raw.active === "string" && (ids.has(raw.active) || detailTargets.has(raw.active))) ? raw.active : "rail";
  const layout = input.version === 3 ? normalizeDockLayout(raw.layout) : undefined;
  return {
    version: 3,
    railExpanded,
    railQuery: typeof raw.railQuery === "string" ? raw.railQuery.slice(0, 500) : "",
    railSelected: clamp(finite(raw.railSelected) ? Math.round(raw.railSelected) : 0, 0, Number.MAX_SAFE_INTEGER),
    collectionView: raw.collectionView === "trials" ? "trials" : "rollouts",
    guideOpen,
    settingsOpen,
    lanes,
    details,
    direction: raw.direction === "columns" ? "columns" : "rows",
    ...(typeof raw.reference === "string" && ids.has(raw.reference) ? { reference: raw.reference } : {}),
    active: requestedActive === "rail" && !railExpanded ? lanes[0].id : requestedActive,
    ...(layout ? { layout } : {}),
  };
}

function normalizeDockLayout(value: unknown): SerializedDockview | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const layout = value as Partial<SerializedDockview>;
  if (!layout.grid || typeof layout.grid !== "object" || !layout.grid.root || typeof layout.grid.root !== "object" ||
      !layout.panels || typeof layout.panels !== "object" || Array.isArray(layout.panels) ||
      !finite(layout.grid.width) || !finite(layout.grid.height)) return undefined;
  const panelCount = Object.keys(layout.panels).length;
  if (panelCount > maximumDockPanels) return undefined;
  let encoded: string;
  try { encoded = JSON.stringify(layout); }
  catch { return undefined; }
  if (encoded.length > maximumDockLayoutBytes) return undefined;
  const clone = JSON.parse(encoded) as SerializedDockview;
  let nodes = 0;
  const visit = (node: unknown, depth: number): boolean => {
    if (!node || typeof node !== "object" || depth > maximumDockDepth || ++nodes > maximumDockNodes) return false;
    const record = node as Record<string, unknown>;
    if (Array.isArray(record.data)) return record.data.every((child) => visit(child, depth + 1));
    return true;
  };
  if (!visit(clone.grid.root, 0)) return undefined;
  // Floating and popout state is never accepted, even from local storage or
  // crafted history state.
  delete clone.floatingGroups;
  delete clone.popoutGroups;
  return clone;
}

export function serializeWorkspace(workspace: WorkspaceState): string {
  return JSON.stringify(workspace);
}

export function workspaceTopologyKey(workspace: WorkspaceState): string {
  return JSON.stringify({
    rail: workspace.railExpanded,
    guide: workspace.guideOpen,
    settings: workspace.settingsOpen,
    lanes: workspace.lanes.map((lane) => lane.id).sort(),
    details: [...workspace.details].sort(),
  });
}

export function workspaceWithoutLayout(workspace: WorkspaceState): WorkspaceState {
  const { layout: _layout, ...shareable } = workspace;
  return shareable;
}

export function workspaceURL(workspace: WorkspaceState, location: Pick<Location, "pathname" | "search" | "hash"> = window.location): string {
  const params = new URLSearchParams(location.search);
  ["trajectory", "trajectory_id", "indexed", "mode", "view", "left", "right"].forEach((key) => params.delete(key));
  // Exact Dockview geometry is device-local. Links carry only logical viewer
  // state, which keeps them bounded and stable across viewport sizes.
  params.set("workspace", serializeWorkspace(workspaceWithoutLayout(workspace)));
  return `${location.pathname}?${params}${location.hash}`;
}

export function workspaceFromSearch(search: string): WorkspaceState | undefined {
  const encoded = new URLSearchParams(search).get("workspace");
  if (!encoded) return undefined;
  try { return normalizeWorkspace(JSON.parse(encoded)); }
  catch { return emptyWorkspace(); }
}

export function legacyWorkspace(search: string): WorkspaceState | undefined {
  const params = new URLSearchParams(search);
  const sourceId = params.get("trajectory");
  const trajectoryId = params.get("trajectory_id");
  const left = params.get("left"), right = params.get("right");
  if (!sourceId || (!trajectoryId && !(left && right))) return undefined;
  const workspace = emptyWorkspace();
  const ids = left && right ? [left, right] : [trajectoryId!];
  workspace.railExpanded = false;
  workspace.lanes = ids.map((id) => ({ id: laneId(sourceId, id), sourceId, trajectoryId: id, band: "focus", selected: 0, depth: 1, fidelity: 1, axis: { start: 0, end: 1 }, descentStack: [] }));
  workspace.active = workspace.lanes[0].id;
  if (ids.length === 2) workspace.reference = workspace.lanes[0].id;
  return workspace;
}

export function snapshotLabel(workspace: WorkspaceState): string {
  if (!workspace.lanes.length) return "Browse";
  const focus = workspace.lanes.filter((lane) => lane.band === "focus").length;
  const context = workspace.lanes.length - focus;
  const detailLane = workspace.active.startsWith("detail:") ? workspace.active.slice(7) : undefined;
  const active = workspace.active === "rail" ? "rail" : workspace.active === "guide" ? "guide" : workspace.active === "detail" ? "detail" : detailLane ? `detail ${workspace.lanes.find((lane) => lane.id === detailLane)?.trajectoryId ?? "rollout"}` : workspace.lanes.find((lane) => lane.id === workspace.active)?.trajectoryId ?? "lane";
  return `${workspace.lanes.length} lane${workspace.lanes.length === 1 ? "" : "s"} · ${focus} focus${context ? ` + ${context} context` : ""} · ${active}`;
}
import type { SerializedDockview } from "dockview";
