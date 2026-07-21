import { completeEventPage, normalizeTrajectoryResponse } from "../../web/src/api";
import type { LoadResult } from "../../web/src/api";
import type { ViewerProvider } from "../../web/src/provider";
import type { AnalysisResponse, BrowseResponse, ComparisonResponse, TrajectoryResponse } from "../../web/src/types";
import { analyze, compare } from "./wasm";

export interface BrowserCollection {
  source: { id: string; name: string; format: string; size: number; index_state: string };
  browse: BrowseResponse;
  trajectories: Record<string, TrajectoryResponse & { page: { count: number; total: number; limit: number; has_more: boolean } }>;
}

export function createInMemoryProvider(collection: BrowserCollection, collectionId: string): ViewerProvider {
  const first = collection.browse.trajectories[0];
  if (!first) throw new Error("Trace contains no trajectories");
  const load = (id: string): LoadResult => {
    const value = collection.trajectories[id];
    if (!value) throw new Error(`Unknown trajectory ${id}`);
    if (!value.trajectory) throw new Error(`Trajectory ${id} has no metadata`);
    return {
      trajectory: normalizeTrajectoryResponse(value),
      isSample: false,
      page: completeEventPage(value.events?.length ?? value.page.count),
      source: value.source,
    };
  };
  return {
    async loadInitial() { return load(first.trajectory.id); },
    async loadBrowse() { return collection.browse; },
    async loadTrajectory(_sourceId, trajectoryId) { return load(trajectoryId); },
    async loadAnalysis(_sourceId, trajectoryId) { return analyze(collectionId, trajectoryId) as Promise<AnalysisResponse>; },
    async loadComparison(_sourceId, left, right) { return compare(collectionId, left, right) as Promise<ComparisonResponse>; },
    async loadArtifactContent(_sourceId, trajectoryId, artifactId) {
      const artifact = collection.trajectories[trajectoryId]?.artifacts?.find((item) => item.id === artifactId);
      if (!artifact) throw new Error(`Unknown artifact ${artifactId}`);
      if (artifact.text !== undefined) return new TextEncoder().encode(artifact.text);
      if (artifact.json !== undefined) return new TextEncoder().encode(JSON.stringify(artifact.json));
      throw new Error("Artifact preview requires the CLI");
    },
  };
}
