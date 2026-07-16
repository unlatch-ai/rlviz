import { sampleTrajectory } from "./sample";
import type { Trajectory, TrajectoryResponse } from "./types";

export interface LoadResult { trajectory: Trajectory; isSample: boolean; error?: string }

export function trajectoryEndpoint(search = globalThis.location?.search ?? ""): string {
  const id = new URLSearchParams(search).get("trajectory");
  return id ? `/api/v1/trajectory?trajectory=${encodeURIComponent(id)}` : "/api/v1/trajectory";
}

export function daemonToken(hash = globalThis.location?.hash ?? ""): string | null {
  return new URLSearchParams(hash.replace(/^#/, "")).get("token");
}

export function normalizeTrajectoryResponse(payload: TrajectoryResponse): Trajectory {
  if (payload.trajectory) {
    const events = (payload.events ?? payload.trajectory.events ?? []).map((event) => {
      const metadata = event.metadata ?? {};
      const dataReward = typeof event.data === "object" && event.data && "reward" in event.data && typeof event.data.reward === "number" ? event.data.reward : undefined;
      return {
        ...event,
        title: event.title ?? (typeof metadata.title === "string" ? metadata.title : undefined),
        summary: event.summary ?? (typeof metadata.summary === "string" ? metadata.summary : undefined),
        duration_ms: event.duration_ms ?? (typeof metadata.duration_ms === "number" ? metadata.duration_ms : undefined),
        reward: event.reward ?? (typeof metadata.reward === "number" ? metadata.reward : dataReward),
      };
    });
    const trajectorySignals = (payload.signals ?? []).filter((signal) => signal.trajectory_id === payload.trajectory?.id);
    const rewardSignal = trajectorySignals.find((signal) => signal.name === "reward" && typeof signal.value === "number");
    const runModel = payload.run?.metadata && typeof payload.run.metadata.model === "string" ? payload.run.metadata.model : undefined;
    return {
      ...payload.trajectory,
      name: payload.trajectory.name ?? payload.case?.name,
      run_id: payload.trajectory.run_id ?? payload.run?.id ?? payload.case?.run_id,
      case_id: payload.trajectory.case_id ?? payload.case?.id ?? payload.group?.case_id,
      group_id: payload.trajectory.group_id ?? payload.group?.id,
      model: payload.trajectory.model ?? runModel,
      started_at: payload.trajectory.started_at ?? payload.run?.started_at,
      total_reward: payload.trajectory.total_reward ?? (typeof rewardSignal?.value === "number" ? rewardSignal.value : undefined),
      events,
    };
  }
  return {
    id: typeof payload.id === "string" ? payload.id : "trajectory",
    name: typeof payload.name === "string" ? payload.name : undefined,
    events: Array.isArray(payload.events) ? payload.events : [],
  };
}

export async function loadTrajectory(signal?: AbortSignal): Promise<LoadResult> {
  try {
    const token = daemonToken();
    const headers: Record<string, string> = { Accept: "application/json" };
    if (token) headers.Authorization = `Bearer ${token}`;
    const response = await fetch(trajectoryEndpoint(), { signal, headers });
    if (!response.ok) throw new Error(`API returned ${response.status}`);
    const trajectory = normalizeTrajectoryResponse((await response.json()) as TrajectoryResponse);
    if (!trajectory.events.length) throw new Error("trajectory contains no events");
    return { trajectory, isSample: false };
  } catch (error) {
    if (signal?.aborted) throw error;
    return { trajectory: sampleTrajectory, isSample: true, error: error instanceof Error ? error.message : "API unavailable" };
  }
}
