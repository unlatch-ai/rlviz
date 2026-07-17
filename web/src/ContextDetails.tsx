import { isContextEvent } from "./research";
import type { TrajectoryEvent } from "./types";

function formattedInteger(value: number): string {
  return new Intl.NumberFormat("en-US", { maximumFractionDigits: 0 }).format(value);
}

function ContextReferences({ label, ids, onJump }: { label: string; ids: string[] | undefined; onJump: (id: string) => void }) {
  if (!ids?.length) return null;
  return <div className="context-references"><span>{label}</span><div>{ids.map((id) => <button type="button" key={id} onClick={() => onJump(id)}>{id}</button>)}</div></div>;
}

export function ContextDetails({ event, onJump }: { event: TrajectoryEvent; onJump: (id: string) => void }) {
  if (!isContextEvent(event)) return null;
  const context = event.context;
  if (!context) return <section className="context-details"><h4>Context</h4><dl><div><dt>Evidence</dt><dd>Legacy marker</dd></div><div><dt>Alignment</dt><dd>{event.alignment_key}</dd></div></dl></section>;

  const occupancy = context.input_tokens !== undefined && context.capacity !== undefined
    ? `${Math.round((context.input_tokens / context.capacity) * 100)}%`
    : undefined;
  const rows: Array<[string, string | number | undefined]> = [
    ["Operation", context.operation],
    ["Before", context.input_tokens_before === undefined ? undefined : formattedInteger(context.input_tokens_before)],
    ["Input tokens", context.input_tokens === undefined ? undefined : formattedInteger(context.input_tokens)],
    ["Capacity", context.capacity === undefined ? undefined : formattedInteger(context.capacity)],
    ["Occupancy", occupancy],
    ["Provenance", context.provenance === "adapter_derived" ? "Adapter-derived" : "Source-native"],
  ];
  return <section className="context-details">
    <h4>Context</h4>
    <dl>{rows.flatMap(([key, value]) => value === undefined ? [] : [<div key={key}><dt>{key}</dt><dd>{value}</dd></div>])}</dl>
    {context.derivation && <div className="context-note"><span>Derivation</span><p>{context.derivation}</p></div>}
    {context.summary && <div className="context-note"><span>Summary</span><p>{context.summary}</p></div>}
    <ContextReferences label="Retained" ids={context.retained_event_ids} onJump={onJump} />
    <ContextReferences label="Dropped" ids={context.dropped_event_ids} onJump={onJump} />
    <ContextReferences label="Summarized" ids={context.summarized_event_ids} onJump={onJump} />
  </section>;
}
