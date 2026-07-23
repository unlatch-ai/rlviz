import { sampleTrajectory } from "./sample";
import type { AnalysisResponse, BrowseResponse, ComparisonResponse, EventPageResponse, GroupPathsResponse, GroupResponse, IndexedSource, PageMetadata, PresentationConfig, Trajectory, TrajectoryArtifact, TrajectoryEvent, TrajectoryResponse, TrajectorySignal } from "./types";

export interface LoadResult { trajectory: Trajectory; isSample: boolean; page?: PageMetadata; signalPage?: PageMetadata; artifactPage?: PageMetadata; source?: IndexedSource; presentation?: PresentationConfig; error?: string }

export function completeEventPage(count: number): PageMetadata {
  return { count, total: count, limit: count, has_more: false };
}

export function trajectoryEndpoint(search = globalThis.location?.search ?? ""): string {
  const params = new URLSearchParams(search);
  const indexed = params.get("indexed") === "1";
  params.delete("indexed");
  // Viewer state belongs in shareable URLs, but is not part of the daemon API.
  for (const key of ["demo", "event", "surface", "view", "mode", "left", "right", "step", "cohort_filter", "workspace", "workspace_id"]) params.delete(key);
  if (indexed && !params.has("limit")) params.set("limit", "200");
  const query = params.toString();
  return `${indexed ? "/api/v1/indexed/trajectory" : "/api/v1/trajectory"}${query ? `?${query}` : ""}`;
}

const TOKEN_STORAGE_KEY = "rlviz.daemon-token";

export function daemonToken(hash = globalThis.location?.hash ?? ""): string | null {
  const fromHash = new URLSearchParams(hash.replace(/^#/, "")).get("token");
  // Persist per-origin so reloads and hash-less navigation keep working; the
  // origin includes the daemon port. Ports can be recycled, so failed auth
  // clears a token that may belong to an earlier daemon on the same origin.
  if (fromHash) {
    try { globalThis.localStorage?.setItem(TOKEN_STORAGE_KEY, fromHash); } catch { /* storage unavailable */ }
    return fromHash;
  }
  try { return globalThis.localStorage?.getItem(TOKEN_STORAGE_KEY) ?? null; } catch { return null; }
}

export function clearStoredDaemonToken(): void {
  try { globalThis.localStorage?.removeItem(TOKEN_STORAGE_KEY); } catch { /* storage unavailable */ }
}

function requestHeaders(): Record<string, string> {
  const headers: Record<string, string> = { Accept: "application/json" };
  const token = daemonToken();
  if (token) headers.Authorization = `Bearer ${token}`;
  return headers;
}

function clearTokenOnAuthFailure(response: Response): void {
  if (response.status === 401 || response.status === 403) clearStoredDaemonToken();
}

function normalizeEvent(event: TrajectoryEvent): TrajectoryEvent {
  const metadata = event.metadata ?? {};
  const dataReward = typeof event.data === "object" && event.data && "reward" in event.data && typeof event.data.reward === "number" ? event.data.reward : undefined;
  return {
    ...event,
    title: event.title ?? (typeof metadata.title === "string" ? metadata.title : undefined),
    summary: event.summary ?? (typeof metadata.summary === "string" ? metadata.summary : undefined),
    duration_ms: event.duration_ms ?? (typeof metadata.duration_ms === "number" ? metadata.duration_ms : undefined),
    token_count: event.token_count ?? (typeof metadata.token_count === "number" ? metadata.token_count : undefined),
    reward: event.reward ?? (typeof metadata.reward === "number" ? metadata.reward : dataReward),
  };
}

export function normalizeTrajectoryResponse(payload: TrajectoryResponse): Trajectory {
  if (payload.trajectory) {
    const events = (payload.events ?? payload.trajectory.events ?? []).map(normalizeEvent);
    const trajectorySignals = (payload.signals ?? []).filter((signal) => signal.trajectory_id === payload.trajectory?.id);
    const rewardSignal = trajectorySignals.find((signal) => signal.name.toLowerCase() === "reward" && typeof signal.value === "number");
    const trajectoryModel = typeof payload.trajectory.metadata?.model === "string" ? payload.trajectory.metadata.model
      : typeof payload.trajectory.metadata?.checkpoint === "string" ? payload.trajectory.metadata.checkpoint : undefined;
    const runModel = payload.run?.metadata && typeof payload.run.metadata.model === "string" ? payload.run.metadata.model
      : payload.run?.metadata && typeof payload.run.metadata.checkpoint === "string" ? payload.run.metadata.checkpoint : undefined;
    return {
      ...payload.trajectory,
      name: payload.trajectory.name ?? payload.case?.name,
      run_id: payload.trajectory.run_id ?? payload.run?.id ?? payload.case?.run_id,
      run_name: payload.run?.name,
      case_id: payload.trajectory.case_id ?? payload.case?.id ?? payload.group?.case_id,
      case_name: payload.case?.name,
      group_id: payload.trajectory.group_id ?? payload.group?.id,
      group_name: payload.group?.name,
      model: payload.trajectory.model ?? trajectoryModel ?? runModel,
      started_at: payload.trajectory.started_at ?? payload.run?.started_at,
      total_reward: payload.trajectory.total_reward ?? (typeof rewardSignal?.value === "number" ? rewardSignal.value : undefined),
      events,
      artifacts: payload.artifacts ?? [],
      signals: trajectorySignals,
    };
  }
  return {
    id: typeof payload.id === "string" ? payload.id : "trajectory",
    name: typeof payload.name === "string" ? payload.name : undefined,
    events: Array.isArray(payload.events) ? payload.events : [],
    artifacts: payload.artifacts ?? [],
  };
}

export async function loadTrajectory(signal?: AbortSignal): Promise<LoadResult> {
  try {
    const response = await fetch(trajectoryEndpoint(), { signal, headers: requestHeaders() });
    clearTokenOnAuthFailure(response);
    if (!response.ok) throw new Error(`API returned ${response.status}`);
    const payload = (await response.json()) as TrajectoryResponse & { page?: PageMetadata };
    const trajectory = normalizeTrajectoryResponse(payload);
    if (!trajectory.events.length) throw new Error("trajectory contains no events");
    return { trajectory, isSample: false, page: payload.page, signalPage: payload.signal_page, artifactPage: payload.artifact_page, source: payload.source, presentation: payload.presentation };
  } catch (error) {
    if (signal?.aborted) throw error;
    return { trajectory: sampleTrajectory, isSample: true, error: error instanceof Error ? error.message : "API unavailable" };
  }
}

export async function loadBrowse(signal?: AbortSignal): Promise<BrowseResponse> {
  let response = await fetch("/api/v1/indexed/browse", { signal, headers: requestHeaders() });
  clearTokenOnAuthFailure(response);
  if (response.status === 401) {
    // A stored token can go stale when the daemon restarts; drop it and retry
    // once with whatever the URL hash carries before reporting.
    clearStoredDaemonToken();
    response = await fetch("/api/v1/indexed/browse", { signal, headers: requestHeaders() });
    clearTokenOnAuthFailure(response);
  }
  if (response.status === 401) throw new Error("Not authorized: this tab has no valid daemon token. Re-open the viewer with `rlviz open` or `rlviz demo` — the printed URL carries the token.");
  if (!response.ok) throw new Error(`Browse API returned ${response.status}`);
  const payload = (await response.json()) as BrowseResponse;
  if (!Array.isArray(payload.sources) || !Array.isArray(payload.trajectories)) throw new Error("Browse API returned an invalid response");
  return payload;
}

export async function loadIndexedTrajectory(sourceId: string, trajectoryId: string, signal?: AbortSignal): Promise<LoadResult> {
  const params = new URLSearchParams({ trajectory: sourceId, trajectory_id: trajectoryId, limit: "200" });
  const response = await fetch(`/api/v1/indexed/trajectory?${params}`, { signal, headers: requestHeaders() });
  clearTokenOnAuthFailure(response);
  if (!response.ok) throw new Error(`Trajectory API returned ${response.status}`);
  const payload = (await response.json()) as TrajectoryResponse & { page?: PageMetadata };
  const trajectory = normalizeTrajectoryResponse(payload);
  if (!trajectory.events.length) throw new Error("trajectory contains no events");
  let page = payload.page;
  while (page?.has_more) {
    if (page.next_sequence === undefined) throw new Error("Event API returned a truncated page without a continuation sequence");
    const previous = page.next_sequence;
    const next = await loadEventPage(sourceId, trajectoryId, previous, signal);
    if (!next.events.length || (next.page.next_sequence !== undefined && next.page.next_sequence <= previous)) {
      throw new Error("Event API pagination did not advance");
    }
    trajectory.events.push(...next.events);
    page = next.page;
  }
  return { trajectory, isSample: false, page: completeEventPage(trajectory.events.length), signalPage: payload.signal_page, artifactPage: payload.artifact_page, source: payload.source, presentation: payload.presentation };
}

export async function loadChildPage(kind: "signals", sourceId: string, trajectoryId: string, offset: number, signal?: AbortSignal): Promise<{ items: TrajectorySignal[]; page: PageMetadata }>;
export async function loadChildPage(kind: "artifacts", sourceId: string, trajectoryId: string, offset: number, signal?: AbortSignal): Promise<{ items: TrajectoryArtifact[]; page: PageMetadata }>;
export async function loadChildPage(kind: "signals" | "artifacts", sourceId: string, trajectoryId: string, offset: number, signal?: AbortSignal): Promise<{ items: Array<TrajectorySignal | TrajectoryArtifact>; page: PageMetadata }> {
  const params = new URLSearchParams({ trajectory: sourceId, trajectory_id: trajectoryId, offset: String(offset), limit: "200" });
  const response = await fetch(`/api/v1/indexed/${kind}?${params}`, { signal, headers: requestHeaders() });
  if (!response.ok) throw new Error(`${kind === "signals" ? "Signal" : "Artifact"} API returned ${response.status}`);
  const payload = (await response.json()) as { signals?: TrajectorySignal[]; artifacts?: TrajectoryArtifact[]; page: PageMetadata };
  const items = kind === "signals" ? payload.signals : payload.artifacts;
  if (!Array.isArray(items) || !payload.page) throw new Error(`${kind} API returned an invalid response`);
  return { items, page: payload.page };
}

export async function loadEventPage(sourceId: string, trajectoryId: string, afterSequence: number, signal?: AbortSignal): Promise<EventPageResponse> {
  const params = new URLSearchParams({ trajectory: sourceId, trajectory_id: trajectoryId, after_sequence: String(afterSequence), limit: "200" });
  const response = await fetch(`/api/v1/indexed/events?${params}`, { signal, headers: requestHeaders() });
  if (!response.ok) throw new Error(`Event API returned ${response.status}`);
  const payload = (await response.json()) as EventPageResponse;
  return { ...payload, events: payload.events.map(normalizeEvent) };
}

export function groupEndpoint(sourceId: string, groupId: string): string {
  const params = new URLSearchParams({ trajectory: sourceId, group_id: groupId });
  return `/api/v1/indexed/group?${params}`;
}

export async function loadGroup(sourceId: string, groupId: string, signal?: AbortSignal): Promise<GroupResponse> {
  const response = await fetch(groupEndpoint(sourceId, groupId), { signal, headers: requestHeaders() });
  if (!response.ok) throw new Error(`Group API returned ${response.status}`);
  const payload = (await response.json()) as GroupResponse;
  if (!Array.isArray(payload.trajectories)) throw new Error("Group API returned an invalid response");
  return payload;
}

export function groupPathsEndpoint(sourceId: string, groupId: string): string {
  const params = new URLSearchParams({ trajectory: sourceId, group_id: groupId });
  return `/api/v1/indexed/paths?${params}`;
}

export async function loadGroupPaths(sourceId: string, groupId: string, signal?: AbortSignal): Promise<GroupPathsResponse> {
  const response = await fetch(groupPathsEndpoint(sourceId, groupId), { signal, headers: requestHeaders() });
  if (!response.ok) throw new Error(`Group paths API returned ${response.status}`);
  const payload = (await response.json()) as GroupPathsResponse;
  if (!payload.tree || !Array.isArray(payload.tree.children)) throw new Error("Group paths API returned an invalid response");
  return payload;
}

export function comparisonEndpoint(sourceId: string, left: string, right: string): string {
  const params = new URLSearchParams({ trajectory: sourceId, left, right });
  return `/api/v1/indexed/compare?${params}`;
}

export async function loadComparison(sourceId: string, left: string, right: string, signal?: AbortSignal): Promise<ComparisonResponse> {
  const response = await fetch(comparisonEndpoint(sourceId, left, right), { signal, headers: requestHeaders() });
  if (!response.ok) throw new Error(`Comparison API returned ${response.status}`);
  const payload = (await response.json()) as ComparisonResponse;
  if (!Array.isArray(payload.alignment?.steps) || !Array.isArray(payload.left?.events) || !Array.isArray(payload.right?.events)) {
    throw new Error("Comparison API returned an invalid response");
  }
  return {
    ...payload,
    left: { ...payload.left, events: payload.left.events.map(normalizeEvent) },
    right: { ...payload.right, events: payload.right.events.map(normalizeEvent) },
  };
}

export function analysisEndpoint(sourceId: string, trajectoryId: string): string {
  const params = new URLSearchParams({ trajectory: sourceId, trajectory_id: trajectoryId, analyzer: "loop-retry" });
  return `/api/v1/indexed/analysis?${params}`;
}

export async function loadAnalysis(sourceId: string, trajectoryId: string, signal?: AbortSignal): Promise<AnalysisResponse> {
  const response = await fetch(analysisEndpoint(sourceId, trajectoryId), { signal, headers: requestHeaders() });
  if (!response.ok) throw new Error(`Analysis API returned ${response.status}`);
  const payload = (await response.json()) as AnalysisResponse;
  if (!payload.analysis?.provenance || !Array.isArray(payload.analysis.findings ?? []) || !Array.isArray(payload.analysis.signals ?? [])) {
    throw new Error("Analysis API returned an invalid response");
  }
  return payload;
}

export function artifactContentEndpoint(sourceId: string, trajectoryId: string, artifactId: string): string {
  const params = new URLSearchParams({ trajectory: sourceId, trajectory_id: trajectoryId, artifact_id: artifactId });
  return `/api/v1/indexed/artifact/content?${params}`;
}

export async function loadArtifactContent(sourceId: string, trajectoryId: string, artifactId: string, signal?: AbortSignal): Promise<ArrayBuffer> {
  const response = await fetch(artifactContentEndpoint(sourceId, trajectoryId, artifactId), { signal, headers: requestHeaders() });
  if (!response.ok) throw new Error(`Artifact API returned ${response.status}`);
  return response.arrayBuffer();
}
