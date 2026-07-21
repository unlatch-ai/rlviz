export type LaneBand = "focus" | "context";
export type WorkspaceDirection = "rows" | "columns";
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

export interface SeamRatios {
  rail: number;
  focusContext: number;
  focusLane: number;
  console: number;
}

export interface WorkspaceState {
  version: 2;
  railExpanded: boolean;
  railQuery: string;
  railSelected: number;
  lanes: WorkspaceLane[];
  direction: WorkspaceDirection;
  reference?: string;
  active: WorkspaceTarget;
  seams: SeamRatios;
}

export const workspaceStorageKey = "rlviz.workspace.v2";
export const defaultSeams: SeamRatios = { rail: 0.24, focusContext: 0.78, focusLane: 0.5, console: 0.28 };

export function emptyWorkspace(): WorkspaceState {
  return {
    version: 2,
    railExpanded: true,
    railQuery: "",
    railSelected: 0,
    lanes: [],
    direction: "rows",
    active: "rail",
    seams: { ...defaultSeams },
  };
}

export function laneId(sourceId: string, trajectoryId: string): string {
  return `${encodeURIComponent(sourceId)}:${encodeURIComponent(trajectoryId)}`;
}

const finite = (value: unknown): value is number => typeof value === "number" && Number.isFinite(value);
const clamp = (value: number, minimum: number, maximum: number) => Math.max(minimum, Math.min(maximum, value));

export function normalizeWorkspace(value: unknown): WorkspaceState | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const raw = value as Partial<WorkspaceState>;
  if (raw.version !== 2 || !Array.isArray(raw.lanes)) return undefined;
  const seen = new Set<string>();
  const lanes = raw.lanes.flatMap((candidate) => {
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
      fidelity: clamp(finite(lane.fidelity) ? Math.round(lane.fidelity) : 3, 0, 5), axis, descentStack,
    } satisfies WorkspaceLane];
  });
  // A restored malformed link cannot overfill the focus band.
  let focusCount = 0;
  lanes.forEach((lane) => {
    if (lane.band === "focus" && focusCount < 2) focusCount++;
    else if (lane.band === "focus") lane.band = "context";
  });
  const ids = new Set(lanes.map((lane) => lane.id));
  const seams = raw.seams ?? defaultSeams;
  const railExpanded = raw.railExpanded !== false || lanes.length === 0;
  const requestedActive = raw.active === "rail" || (typeof raw.active === "string" && ids.has(raw.active)) ? raw.active : "rail";
  return {
    version: 2,
    railExpanded,
    railQuery: typeof raw.railQuery === "string" ? raw.railQuery.slice(0, 500) : "",
    railSelected: clamp(finite(raw.railSelected) ? Math.round(raw.railSelected) : 0, 0, Number.MAX_SAFE_INTEGER),
    lanes,
    direction: raw.direction === "columns" ? "columns" : "rows",
    reference: typeof raw.reference === "string" && ids.has(raw.reference) ? raw.reference : undefined,
    active: requestedActive === "rail" && !railExpanded ? lanes[0].id : requestedActive,
    seams: {
      rail: clamp(finite(seams.rail) ? seams.rail : defaultSeams.rail, 0.14, 0.42),
      focusContext: clamp(finite(seams.focusContext) ? seams.focusContext : defaultSeams.focusContext, 0.42, 0.9),
      focusLane: clamp(finite(seams.focusLane) ? seams.focusLane : defaultSeams.focusLane, 0.2, 0.8),
      console: clamp(finite(seams.console) ? seams.console : defaultSeams.console, 0.18, 0.52),
    },
  };
}

export function serializeWorkspace(workspace: WorkspaceState): string {
  return JSON.stringify(workspace);
}

export function workspaceURL(workspace: WorkspaceState, location: Pick<Location, "pathname" | "search" | "hash"> = window.location): string {
  const params = new URLSearchParams(location.search);
  ["trajectory", "trajectory_id", "indexed", "mode", "view", "left", "right"].forEach((key) => params.delete(key));
  params.set("workspace", serializeWorkspace(workspace));
  return `${location.pathname}?${params}${location.hash}`;
}

export function workspaceFromSearch(search: string): WorkspaceState | undefined {
  const encoded = new URLSearchParams(search).get("workspace");
  if (!encoded) return undefined;
  try { return normalizeWorkspace(JSON.parse(encoded)); }
  catch { return undefined; }
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
  workspace.lanes = ids.map((id) => ({ id: laneId(sourceId, id), sourceId, trajectoryId: id, band: "focus", selected: 0, depth: 1, fidelity: 3, axis: { start: 0, end: 1 }, descentStack: [] }));
  workspace.active = workspace.lanes[0].id;
  if (ids.length === 2) workspace.reference = workspace.lanes[0].id;
  return workspace;
}

export function snapshotLabel(workspace: WorkspaceState): string {
  if (!workspace.lanes.length) return "Browse";
  const focus = workspace.lanes.filter((lane) => lane.band === "focus").length;
  const context = workspace.lanes.length - focus;
  const active = workspace.active === "rail" ? "rail" : workspace.lanes.find((lane) => lane.id === workspace.active)?.trajectoryId ?? "lane";
  return `${workspace.lanes.length} lane${workspace.lanes.length === 1 ? "" : "s"} · ${focus} focus${context ? ` + ${context} context` : ""} · ${active}`;
}
