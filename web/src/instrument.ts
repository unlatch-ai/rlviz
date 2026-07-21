import type { AnalysisResponse, BrowseTrajectory, ComparisonSide, Trajectory, TrajectoryEvent } from "./types";

export type AxisWindow = { start: number; end: number };

export type Episode = {
  key: string;
  label: string;
  startIndex: number;
  endIndex: number;
  start: number;
  end: number;
  inferred: boolean;
};

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

function explicitEpisodePrefix(events: TrajectoryEvent[]): "episode:" | "stage:" | undefined {
  if (events.some((event) => event.alignment_key?.startsWith("episode:"))) return "episode:";
  if (events.some((event) => event.alignment_key?.startsWith("stage:"))) return "stage:";
  return undefined;
}

function fallbackBoundary(events: TrajectoryEvent[], index: number): boolean {
  if (index === 0) return true;
  const event = events[index], previous = events[index - 1];
  const context = (item: TrajectoryEvent) => !!item.context || !!item.alignment_key?.startsWith("context:");
  if (context(event)) return true;
  if (event.kind === "error" && previous.kind !== "error") return true;
  if (previous.kind === "error" && event.kind !== "error") return true;
  if ((event.kind === "tool" || event.kind === "environment_action") && previous.kind !== "tool" && previous.kind !== "environment_action") return true;
  return false;
}

/** Deterministic, source-registered reading units for lane depth 2/3. */
export function episodesFor(events: TrajectoryEvent[]): Episode[] {
  if (!events.length) return [];
  const prefix = explicitEpisodePrefix(events);
  const starts: Array<{ index: number; key: string; label: string }> = [];
  if (prefix) {
    let current = "";
    events.forEach((event, index) => {
      const explicit = event.alignment_key?.startsWith(prefix) ? event.alignment_key : undefined;
      if (!starts.length && !explicit) {
        current = "opening";
        starts.push({ index, key: current, label: "opening" });
      } else if (explicit && explicit !== current) {
        current = explicit;
        starts.push({ index, key: explicit, label: explicit.slice(prefix.length) || explicit });
      }
    });
  } else {
    events.forEach((event, index) => {
      if (!fallbackBoundary(events, index)) return;
      const context = !!event.context || !!event.alignment_key?.startsWith("context:");
      const label = context ? "context" : event.kind === "error" ? "errors" : event.kind === "tool" || event.kind === "environment_action" ? "tool run" : index === 0 ? "opening" : event.kind;
      starts.push({ index, key: `inferred:${index}`, label });
    });
  }
  const occurrences = new Map<string, number>();
  return starts.map((start, index) => {
    const endIndex = (starts[index + 1]?.index ?? events.length) - 1;
    const occurrence = (occurrences.get(start.key) ?? 0) + 1;
    occurrences.set(start.key, occurrence);
    return {
      key: `${start.key}#${occurrence}`,
      label: start.label,
      startIndex: start.index,
      endIndex,
      start: events[start.index].sequence,
      end: events[endIndex].sequence,
      inferred: !prefix,
    };
  });
}

export function episodeIndexForEvent(episodes: Episode[], eventIndex: number): number {
  const found = episodes.findIndex((episode) => eventIndex >= episode.startIndex && eventIndex <= episode.endIndex);
  return found >= 0 ? found : 0;
}

/** Zoom around an episode while preserving the selected event's screen x. */
export function episodeWindow(window: AxisWindow, episode: Episode, selectedSequence: number): AxisWindow {
  const oldSpan = Math.max(1, window.end - window.start);
  const fraction = Math.max(0.001, Math.min(0.999, (selectedSequence - window.start) / oldSpan));
  const left = Math.max(0, selectedSequence - episode.start) / fraction;
  const right = Math.max(0, episode.end - selectedSequence) / (1 - fraction);
  const span = Math.max(1, left, right);
  const start = selectedSequence - fraction * span;
  return { start, end: start + span };
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
