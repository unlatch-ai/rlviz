import { diffToolArguments, diffToolResults } from "./structuredDiff";
import type { StructuredDiffResult, StructuredDiffRow } from "./structuredDiff";
import type { TrajectoryEvent } from "./types";

function record(value: unknown): Record<string, unknown> | undefined {
  return value !== null && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : undefined;
}

function toolArguments(event: TrajectoryEvent): unknown {
  const input = record(event.input);
  return input && "arguments" in input ? input.arguments : event.input;
}

function toolName(event: TrajectoryEvent): string {
  const input = record(event.input);
  return typeof input?.name === "string" && input.name.trim() ? input.name : event.title || event.kind;
}

function changedRows(result: StructuredDiffResult): StructuredDiffRow[] {
  return result.rows.filter((row) => row.status !== "equal");
}

function DiffRows({ label, result }: { label: string; result: StructuredDiffResult }) {
  const rows = changedRows(result);
  return <section className="structured-diff-section">
    <header><strong>{label}</strong><span>{rows.length} changed · {result.rows.length - rows.length} equal</span></header>
    {rows.length ? <div className="structured-diff-table" role="table" aria-label={`${label} field differences`}>
      <div className="structured-diff-heading" role="row"><span role="columnheader">Field</span><span role="columnheader">Left</span><span role="columnheader">Right</span></div>
      {rows.map((row) => <div className={`structured-diff-row ${row.status}`} role="row" key={`${row.path}:${row.status}`}>
        <code role="cell" title={row.path}>{row.path}</code>
        <span role="cell" title={row.left.preview}>{row.left.preview}</span>
        <span role="cell" title={row.right.preview}>{row.right.preview}</span>
      </div>)}
    </div> : <p>No field changes</p>}
    {result.truncated && <small data-state="truncated">Bounded view · {result.truncationReasons.join(", ")}</small>}
  </section>;
}

/** Selected-event-first, display-only differences for paired tool payloads. */
export function StructuredToolDiff({ left, right }: { left?: TrajectoryEvent; right?: TrajectoryEvent }) {
  if (!left || !right || (left.kind !== "tool" && right.kind !== "tool")) return null;
  if ([left.input, left.output, right.input, right.output].every((value) => value === undefined)) return null;

  const options = { maxDepth: 5, maxRows: 80, maxArrayItems: 30, maxObjectKeys: 60, maxStringLength: 160 };
  const argumentsDiff = diffToolArguments(toolArguments(left) ?? {}, toolArguments(right) ?? {}, options);
  const resultsDiff = diffToolResults(left.output ?? {}, right.output ?? {}, options);
  return <section className="structured-tool-diff" aria-label="Structured tool payload differences">
    <header><div><span>Structured differences</span><strong>{toolName(left)} <i>vs</i> {toolName(right)}</strong></div><small>selected canonical events</small></header>
    <DiffRows label="Arguments" result={argumentsDiff} />
    <DiffRows label="Results" result={resultsDiff} />
  </section>;
}
