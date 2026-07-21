import { type CSSProperties, type KeyboardEvent, useMemo, useRef } from "react";
import { isContextEvent } from "./research";
import type { EventContext, TrajectoryEvent } from "./types";

const operationLabels = {
  compaction: "compaction",
  truncation: "truncation",
  injection: "injection",
  restore: "restore",
} as const;

function formattedInteger(value: number): string {
  return new Intl.NumberFormat("en-US", { maximumFractionDigits: 0 }).format(value);
}

function contextFact(context: EventContext | undefined): string {
  if (!context) return "legacy context marker; token count not reported; capacity not reported";
  const operation = context.operation ? operationLabels[context.operation] : "context observation";
  const before = context.input_tokens_before === undefined ? "" : `${formattedInteger(context.input_tokens_before)} input tokens before; `;
  const tokens = context.input_tokens === undefined ? "token count after not reported" : `${formattedInteger(context.input_tokens)} input tokens after`;
  const capacity = context.capacity === undefined ? "capacity not reported" : `${formattedInteger(context.capacity)} capacity`;
  const occupancy = context.input_tokens !== undefined && context.capacity !== undefined
    ? `; ${Math.round((context.input_tokens / context.capacity) * 100)}% occupancy`
    : "";
  const provenance = context.provenance === "adapter_derived" ? "adapter-derived" : "source-native";
  const derivation = context.provenance === "adapter_derived" && context.derivation ? `; derivation: ${context.derivation}` : "";
  return `${operation}; ${before}${tokens}; ${capacity}${occupancy}; ${provenance}${derivation}`;
}

function markerLabel(event: TrajectoryEvent): string {
  return `Event ${event.sequence}, ${contextFact(event.context)}`;
}

function stableContextEvents(events: TrajectoryEvent[]): Array<{ event: TrajectoryEvent; streamIndex: number }> {
  return events
    .map((event, streamIndex) => ({ event, streamIndex }))
    .sort((left, right) => left.event.sequence - right.event.sequence || left.streamIndex - right.streamIndex)
    .filter(({ event }) => isContextEvent(event));
}

export function ContextTrack({ events, eventTotal, selectedId, onSelect }: {
  events: TrajectoryEvent[];
  eventTotal: number;
  selectedId: string;
  onSelect: (id: string) => void;
}) {
  const markers = useMemo(() => stableContextEvents(events), [events]);
  const markerRefs = useRef<Array<HTMLButtonElement | null>>([]);
  if (!markers.length) return null;

  const ordered = events
    .map((event, streamIndex) => ({ event, streamIndex }))
    .sort((left, right) => left.event.sequence - right.event.sequence || left.streamIndex - right.streamIndex);
  const positions = new Map(ordered.map(({ event }, index) => [event.id, index]));
  const selectedMarker = markers.find(({ event }) => event.id === selectedId)?.event;
  const tabStop = Math.max(0, markers.findIndex(({ event }) => event.id === selectedId));
  const partial = events.length < eventTotal;

  const moveFocus = (index: number, target: number) => {
    const bounded = Math.max(0, Math.min(markers.length - 1, target));
    markerRefs.current[bounded]?.focus();
    if (bounded !== index) onSelect(markers[bounded].event.id);
  };
  const onMarkerKeyDown = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    let target: number | undefined;
    if (event.key === "ArrowLeft") target = index - 1;
    else if (event.key === "ArrowRight") target = index + 1;
    else if (event.key === "Home") target = 0;
    else if (event.key === "End") target = markers.length - 1;
    if (target === undefined) return;
    event.preventDefault();
    moveFocus(index, target);
  };

  return <nav className="context-track" data-state={partial ? "partial" : undefined} aria-label="Context events">
    <div className="context-track-heading">
      <span>Context</span>
      <span className="context-track-fact">{selectedMarker ? contextFact(selectedMarker.context) : `${markers.length} ${markers.length === 1 ? "marker" : "markers"}`}</span>
      <span>{partial ? `${events.length}/${eventTotal} events loaded · partial` : `${events.length} events`} · gaps unobserved</span>
    </div>
    <div className="context-track-rail">
      {markers.map(({ event }, index) => {
        const orderedIndex = positions.get(event.id) ?? 0;
        const position = ordered.length <= 1 ? 50 : 1.5 + (orderedIndex / (ordered.length - 1)) * 97;
        const operation = event.context?.operation ?? "observation";
        const label = markerLabel(event);
        return <button
          key={event.id}
          ref={(node) => { markerRefs.current[index] = node; }}
          type="button"
          className={`context-marker context-marker-${operation} ${event.context ? "" : "legacy"}`}
          style={{ "--context-position": `${position}%` } as CSSProperties}
          aria-label={label}
          aria-current={event.id === selectedId ? "true" : undefined}
          tabIndex={index === tabStop ? 0 : -1}
          title={label}
          onClick={() => onSelect(event.id)}
          onKeyDown={(keyEvent) => onMarkerKeyDown(keyEvent, index)}
        ><span aria-hidden="true"></span></button>;
      })}
    </div>
  </nav>;
}
