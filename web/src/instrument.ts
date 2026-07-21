import type { AnalysisResponse, BrowseTrajectory, ComparisonSide, Trajectory, TrajectoryEvent } from "./types";

export type AxisWindow = { start: number; end: number };

export const glyphForKind = (kind: string): string => ({
  generation: "▸", message: "‒", observation: "·", error: "✕", tool: "▮",
  environment_action: "▮", reward: "◆", grader: "◆", state: "~", log: "·", artifact: "◆",
}[kind] ?? "·");

export function zoomWindow(window: AxisWindow, anchor: number, factor: number, min: number, max: number): AxisWindow {
  const fullSpan = Math.max(1, max - min);
  const oldSpan = Math.max(1, window.end - window.start);
  const span = Math.max(Math.min(7, fullSpan), Math.min(fullSpan, oldSpan / factor));
  const fraction = oldSpan === 0 ? 0.5 : (anchor - window.start) / oldSpan;
  let start = anchor - fraction * span;
  let end = start + span;
  if (start < min) { start = min; end = min + span; }
  if (end > max) { end = max; start = max - span; }
  return { start, end };
}

export function axisX(sequence: number, window: AxisWindow, width = 1000, inset = 20): number {
  const span = Math.max(1, window.end - window.start);
  return inset + ((sequence - window.start) / span) * (width - inset * 2);
}

export function panWindowToInclude(window: AxisWindow, sequence: number, min: number, max: number, marginFraction = 0.15): AxisWindow {
  const span = Math.max(1, window.end - window.start);
  const margin = span * marginFraction;
  if (sequence >= window.start + margin && sequence <= window.end - margin) return window;
  let start = sequence < window.start + margin ? sequence - margin : sequence + margin - span;
  let end = start + span;
  if (start < min) { start = min; end = Math.min(max, min + span); }
  if (end > max) { end = max; start = Math.max(min, max - span); }
  return { start, end };
}

export function firstAnomaly(trajectory: Trajectory, analysis?: AnalysisResponse | null): number {
  const events = trajectory.events;
  const error = events.findIndex((event) => event.kind === "error");
  if (error >= 0) return error;
  const divergence = events.findIndex((event) => event.alignment_key?.startsWith("divergence:"));
  if (divergence >= 0) return divergence;
  const findings = new Set((analysis?.analysis.findings ?? []).flatMap((finding) => finding.event_ids ?? []));
  const finding = events.findIndex((event) => findings.has(event.id));
  return finding >= 0 ? finding : 0;
}

function summaryMetrics(row: BrowseTrajectory) {
  const summary = row.metrics;
  return summary.metrics ?? summary.normalized_metrics ?? summary;
}

export function attentionScore(row: BrowseTrajectory): number {
  const metrics = summaryMetrics(row);
  const errors = Number(metrics.error_count ?? row.metrics.error_count ?? 0);
  const pass = metrics.pass ?? metrics.success ?? row.metrics.pass ?? row.metrics.success;
  const reward = Number(metrics.reward ?? row.metrics.reward ?? 0);
  return errors * 100 + (pass === false ? 60 : 0) + (reward > 0 && reward < 1 ? 30 : reward < 0 ? 40 : 0);
}

export function verdictGlyph(row: BrowseTrajectory): string {
  const metrics = summaryMetrics(row);
  const errors = Number(metrics.error_count ?? row.metrics.error_count ?? 0);
  const pass = metrics.pass ?? metrics.success ?? row.metrics.pass ?? row.metrics.success;
  if (errors > 0 || pass === false) return "✕";
  return "";
}

export type Stage = { key: string; label: string; events: TrajectoryEvent[] };
export type AlignmentTier = "adapter episodes" | "annotated stages" | "outcome only";

function explicitStageKey(event: TrajectoryEvent, prefix: "episode:" | "stage:"): string | undefined {
  const key = event.alignment_key;
  return key?.startsWith(prefix) ? key : undefined;
}

export function stagesFor(side: ComparisonSide): { tier: AlignmentTier; stages: Stage[] } {
  const prefix = side.events.some((event) => explicitStageKey(event, "episode:")) ? "episode:"
    : side.events.some((event) => explicitStageKey(event, "stage:")) ? "stage:" : undefined;
  if (!prefix) return { tier: "outcome only", stages: [{ key: "outcome", label: "outcome", events: side.events }] };
  const stages: Stage[] = [];
  let current: Stage = { key: "opening", label: "opening", events: [] };
  for (const event of side.events) {
    const key = explicitStageKey(event, prefix);
    if (key && key !== current.key) {
      if (current.events.length) stages.push(current);
      current = { key, label: key.split(":").slice(1).join(":") || key, events: [] };
    }
    current.events.push(event);
  }
  if (current.events.length) stages.push(current);
  return { tier: prefix === "episode:" ? "adapter episodes" : "annotated stages", stages };
}

export function stageChanged(left?: Stage, right?: Stage): boolean {
  if (!left || !right) return true;
  const outcome = (stage: Stage) => stage.events.filter((event) => ["tool", "environment_action", "error", "reward", "grader"].includes(event.kind))
    .map((event) => `${event.kind}:${event.alignment_key ?? ""}:${JSON.stringify(event.output ?? event.data ?? null)}`)
    .sort();
  return JSON.stringify(outcome(left)) !== JSON.stringify(outcome(right));
}
