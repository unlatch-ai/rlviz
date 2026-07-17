import { type CSSProperties, type ChangeEvent, useMemo } from "react";
import type { TrajectoryEvent } from "./types";
import type { VisibleRange } from "./VirtualList";

export type OverviewLane = "model" | "interaction" | "evaluation";

export interface OverviewBin {
  model: number;
  interaction: number;
  evaluation: number;
}

const laneKinds: Record<OverviewLane, ReadonlySet<string>> = {
  model: new Set(["message", "generation"]),
  interaction: new Set(["tool", "environment_action", "observation", "state", "artifact", "log"]),
  evaluation: new Set(["reward", "grader", "error"]),
};

function laneFor(kind: string): OverviewLane {
  if (laneKinds.model.has(kind)) return "model";
  if (laneKinds.evaluation.has(kind)) return "evaluation";
  return "interaction";
}

export function stableOverviewEvents(events: TrajectoryEvent[]): TrajectoryEvent[] {
  return events
    .map((event, sourceIndex) => ({ event, sourceIndex }))
    .sort((left, right) => left.event.sequence - right.event.sequence || left.sourceIndex - right.sourceIndex)
    .map(({ event }) => event);
}

export function deriveOverviewBins(events: TrajectoryEvent[], eventTotal: number, requestedBins = 64): OverviewBin[] {
  const ordered = stableOverviewEvents(events);
  const total = Math.max(eventTotal, ordered.length, 1);
  const binCount = Math.max(1, Math.min(requestedBins, total));
  const bins = Array.from({ length: binCount }, () => ({ model: 0, interaction: 0, evaluation: 0 }));
  ordered.forEach((event, rank) => {
    const bin = Math.min(binCount - 1, Math.floor((rank / total) * binCount));
    bins[bin][laneFor(event.kind)] += 1;
  });
  return bins;
}

function percent(rank: number, total: number): number {
  if (total <= 1) return 0;
  return Math.max(0, Math.min(100, (rank / (total - 1)) * 100));
}

function nearestEvent(events: TrajectoryEvent[], ranks: Map<string, number>, target: number): TrajectoryEvent | undefined {
  return events.reduce<TrajectoryEvent | undefined>((closest, event) => {
    if (!closest) return event;
    return Math.abs((ranks.get(event.id) ?? 0) - target) < Math.abs((ranks.get(closest.id) ?? 0) - target) ? event : closest;
  }, undefined);
}

export function TrajectoryOverview({ events, eventTotal, visibleEvents, selectedId, visibleRange, onSelect }: {
  events: TrajectoryEvent[];
  eventTotal: number;
  visibleEvents: TrajectoryEvent[];
  selectedId: string;
  visibleRange?: VisibleRange;
  onSelect: (id: string) => void;
}) {
  const ordered = useMemo(() => stableOverviewEvents(events), [events]);
  const bins = useMemo(() => deriveOverviewBins(events, eventTotal), [eventTotal, events]);
  const ranks = useMemo(() => new Map(ordered.map((event, rank) => [event.id, rank])), [ordered]);
  const total = Math.max(eventTotal, ordered.length, 1);
  const loaded = ordered.length;
  const loadedPercent = Math.min(100, (loaded / total) * 100);
  const selectedRank = ranks.get(selectedId);
  const selectedPercent = selectedRank === undefined ? undefined : percent(selectedRank, total);
  const firstVisible = visibleRange && visibleEvents[visibleRange.start];
  const lastVisible = visibleRange && visibleEvents[Math.max(visibleRange.start, visibleRange.end - 1)];
  const viewportStart = firstVisible ? percent(ranks.get(firstVisible.id) ?? 0, total) : undefined;
  const viewportEnd = lastVisible ? percent(ranks.get(lastVisible.id) ?? 0, total) : undefined;
  const laneMax = {
    model: Math.max(1, ...bins.map((bin) => bin.model)),
    interaction: Math.max(1, ...bins.map((bin) => bin.interaction)),
    evaluation: Math.max(1, ...bins.map((bin) => bin.evaluation)),
  };
  const selectable = visibleEvents.filter((event) => ranks.has(event.id));
  const sliderMax = Math.max(1, loaded - 1);
  const sliderValue = Math.min(sliderMax, selectedRank ?? 0);
  const choose = (event: ChangeEvent<HTMLInputElement>) => {
    const nearest = nearestEvent(selectable, ranks, Number(event.target.value));
    if (nearest) onSelect(nearest.id);
  };
  const style = {
    "--overview-bins": bins.length,
    "--overview-loaded": `${loadedPercent}%`,
    "--overview-selected": selectedPercent === undefined ? "-1%" : `${selectedPercent}%`,
    "--overview-window-start": viewportStart === undefined ? "-1%" : `${viewportStart}%`,
    "--overview-window-width": viewportStart === undefined || viewportEnd === undefined ? "0%" : `${Math.max(0.5, viewportEnd - viewportStart)}%`,
  } as CSSProperties;

  return <section className="trajectory-overview" aria-label="Trajectory overview" style={style}>
    <div className="overview-summary">
      <strong>Overview</strong>
      <span>{loaded < total ? `${loaded}/${total} loaded` : `${total} events`}</span>
    </div>
    <div className="overview-chart">
      <div className="overview-lanes" aria-hidden="true">
        {(["model", "interaction", "evaluation"] as const).map((lane) => <div className={`overview-lane overview-${lane}`} key={lane}>
          {bins.map((bin, index) => <i key={index} style={{ opacity: bin[lane] ? 0.25 + (bin[lane] / laneMax[lane]) * 0.75 : 0.06 }} />)}
        </div>)}
      </div>
      {viewportStart !== undefined && <span className="overview-window" aria-hidden="true" />}
      {selectedPercent !== undefined && <span className="overview-selection" aria-hidden="true" />}
      {loaded < total && <span className="overview-unavailable" aria-hidden="true" />}
      <input
        type="range"
        min={0}
        max={sliderMax}
        value={sliderValue}
        disabled={!selectable.length}
        aria-label="Trajectory overview position"
        aria-valuetext={selectedRank === undefined ? "No selected loaded event" : `Event ${selectedRank + 1} of ${total}`}
        onChange={choose}
      />
    </div>
    <div className="overview-legend" aria-label="Overview lanes">
      <span><i className="model" />Model</span>
      <span><i className="interaction" />Interaction</span>
      <span><i className="evaluation" />Evaluation</span>
    </div>
  </section>;
}
