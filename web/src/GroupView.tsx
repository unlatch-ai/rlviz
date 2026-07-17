import { useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties } from "react";
import { matchesCohortQuery, parseCohortQuery } from "./cohortQuery";
import type { CohortQueryRow, CohortScalar } from "./cohortQuery";
import { loadGroupColumnLayout, optionalBuiltinColumns, resetGroupColumnLayout, saveGroupColumnLayout } from "./columnLayout";
import type { GroupColumnLayout, OptionalBuiltinColumn } from "./columnLayout";
import { bindingLabel, commandIds, useCommands, useKeymapRevision } from "./commands";
import type { GroupPathNode, GroupPathsResponse, GroupResponse, GroupTrajectorySummary, Trajectory } from "./types";

type Scalar = CohortScalar;
type KnownKey = "id" | "reward" | "pass" | "status" | "termination" | "events" | "errors" | "tokens" | "latency";
type Column = { key: string; label: string; signal?: string };
type Row = CohortQueryRow;
type FlatPath = { node: GroupPathNode; branch: boolean };
const MaxSignalColumns = 8;
const MaxDiscoveredSignals = 64;
const knownSignalNames = new Set(["reward", "pass", "success", "outcome", "event_count", "error_count", "token_count", "latency_ms", "duration_ms"]);
const builtinColumnLabels: Record<OptionalBuiltinColumn, string> = { reward: "Reward", pass: "Pass", status: "Status", termination: "Termination", events: "Events", errors: "Errors", tokens: "Tokens", latency: "Latency" };

function flattenPaths(nodes: GroupPathNode[], branch = false): FlatPath[] {
  return nodes.flatMap((node) => [{ node, branch }, ...flattenPaths(node.children, node.children.length > 1)]);
}

function pathLabel(node: GroupPathNode): string {
  const fingerprint = node.fingerprint;
  if (fingerprint.alignment_key) return fingerprint.alignment_key;
  if (fingerprint.state_hash) return `state ${fingerprint.state_hash.slice(0, 10)}`;
  return fingerprint.kind || fingerprint.class;
}

function number(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function boolean(value: unknown): boolean | undefined {
  return typeof value === "boolean" ? value : undefined;
}

function scalar(value: unknown): Scalar | undefined {
  return typeof value === "string" || typeof value === "boolean" || (typeof value === "number" && Number.isFinite(value)) ? value : undefined;
}

function signalLabel(name: string): string {
  const words = name.replace(/[._-]+/g, " ").trim();
  return words ? words.replace(/\b\w/g, (letter) => letter.toUpperCase()) : name;
}

function rowFromSummary(summary: GroupTrajectorySummary): Row {
  const wrapped = summary.trajectory as { value?: Omit<Trajectory, "events"> };
  const trajectory = (wrapped?.value ?? summary.trajectory) as Omit<Trajectory, "events">;
  const metrics = summary.normalized_metrics ?? summary.metrics ?? {};
  const metadata = trajectory.metadata ?? {};
  const signals = Object.fromEntries(Object.entries(summary.signals ?? {}).flatMap(([name, value]) => {
    const parsed = scalar(value);
    return parsed === undefined ? [] : [[name, parsed]];
  }));
  const canonicalSignals = Object.fromEntries(Object.entries(signals).map(([name, value]) => [name.toLowerCase(), value]));
  return {
    id: trajectory.id,
    reward: number(metrics.reward) ?? number(summary.reward) ?? number(metadata.reward) ?? number(canonicalSignals.reward),
    pass: boolean(metrics.pass) ?? boolean(metrics.success) ?? boolean(summary.pass) ?? boolean(summary.success) ?? boolean(metadata.pass) ?? boolean(metadata.success) ?? boolean(canonicalSignals.pass) ?? boolean(canonicalSignals.success),
    outcome: typeof metrics.outcome === "string" ? metrics.outcome : typeof summary.outcome === "string" ? summary.outcome : typeof metadata.outcome === "string" ? metadata.outcome : undefined,
    status: trajectory.status ?? summary.status,
    termination: trajectory.termination ?? summary.termination,
    events: number(metrics.event_count) ?? number(summary.event_count),
    errors: number(metrics.error_count) ?? number(summary.error_count) ?? number(canonicalSignals.error_count),
    tokens: number(metrics.token_count) ?? number(summary.token_count) ?? number(canonicalSignals.token_count) ?? number(canonicalSignals.total_tokens) ?? number(canonicalSignals.tokens),
    latency: number(metrics.latency_ms) ?? number(metrics.duration_ms) ?? number(summary.latency_ms) ?? number(summary.duration_ms) ?? number(canonicalSignals.latency_ms) ?? number(canonicalSignals.duration_ms),
    signals,
  };
}

function rowValue(row: Row, key: string): Scalar | undefined {
  if (key.startsWith("signal:")) return row.signals[key.slice(7)];
  return row[key as KnownKey];
}

function displayScalar(value: Scalar | undefined): string {
  if (value === undefined) return "—";
  if (typeof value === "number") return displayNumber(value);
  if (typeof value === "boolean") return value ? "TRUE" : "FALSE";
  return value;
}

function displayNumber(value?: number): string {
  return value === undefined ? "—" : new Intl.NumberFormat("en-US", { maximumFractionDigits: 3 }).format(value);
}

function displayLatency(value?: number): string {
  if (value === undefined) return "—";
  return value >= 1000 ? `${(value / 1000).toFixed(value >= 10000 ? 1 : 2)}s` : `${Math.round(value)}ms`;
}

function rewardRankIndex(rows: Row[], rank: "best" | "median" | "worst" | "outlier"): number | undefined {
  const candidates = rows.map((row, index) => ({ row, index })).filter(({ row }) => row.reward !== undefined)
    .sort((left, right) => left.row.reward! - right.row.reward! || left.row.id.localeCompare(right.row.id));
  if (!candidates.length) return undefined;
  if (rank === "worst") return candidates[0].index;
  if (rank === "best") return candidates[candidates.length - 1].index;
  const median = candidates[Math.floor((candidates.length - 1) / 2)].row.reward!;
  if (rank === "median") return candidates[Math.floor((candidates.length - 1) / 2)].index;
  return [...candidates].sort((left, right) => Math.abs(right.row.reward! - median) - Math.abs(left.row.reward! - median) || left.row.id.localeCompare(right.row.id))[0].index;
}

function isInfrastructureFailure(row: Row): boolean {
  const failureClass = Object.entries(row.signals).find(([name]) => name.toLowerCase() === "failure_class")?.[1];
  return (typeof failureClass === "string" && failureClass.toLowerCase() === "infrastructure")
    || row.outcome?.toLowerCase() === "infrastructure_error"
    || row.termination?.toLowerCase() === "infrastructure_error";
}

export function GroupView({ group, paths, pathsError, initialQuery = "", onQueryChange, onClose, onOpen, onCompare }: { group: GroupResponse; paths?: GroupPathsResponse | null; pathsError?: string; initialQuery?: string; onQueryChange?: (query: string) => void; onClose: () => void; onOpen: (id: string) => void; onCompare?: (left: string, right: string) => void }) {
  useKeymapRevision();
  const rows = useMemo(() => group.trajectories.map(rowFromSummary).filter((row) => row.id), [group]);
  const [query, setQuery] = useState(initialQuery);
  const [sort, setSort] = useState<string>("reward");
  const [descending, setDescending] = useState(true);
  const [selected, setSelected] = useState(0);
  const [compareIds, setCompareIds] = useState<string[]>([]);
  const [mode, setMode] = useState<"trajectories" | "paths">("trajectories");
  const [selectedPath, setSelectedPath] = useState(0);
  const [columnLayout, setColumnLayout] = useState<GroupColumnLayout>(() => loadGroupColumnLayout());
  const [columnsOpen, setColumnsOpen] = useState(false);
  const searchRef = useRef<HTMLInputElement>(null);
  const columnsButtonRef = useRef<HTMLButtonElement>(null);
  const columnsCloseRef = useRef<HTMLButtonElement>(null);
  const columnsWereOpen = useRef(false);
  const flatPaths = useMemo(() => flattenPaths(paths?.tree.children ?? []), [paths]);

  const builtinColumns = useMemo<Column[]>(() => optionalBuiltinColumns
    .filter((key) => rows.some((row) => rowValue(row, key) !== undefined))
    .map((key) => ({ key, label: builtinColumnLabels[key] })), [rows]);
  const allSignalColumns = useMemo<Column[]>(() => {
    const coverage = new Map<string, number>();
    rows.forEach((row) => Object.keys(row.signals).forEach((name) => {
      if (!knownSignalNames.has(name.toLowerCase())) coverage.set(name, (coverage.get(name) ?? 0) + 1);
    }));
    return [...coverage].sort((left, right) => right[1] - left[1] || left[0].localeCompare(right[0])).slice(0, MaxDiscoveredSignals)
      .map(([name]) => ({ key: `signal:${name}`, label: signalLabel(name), signal: name }));
  }, [rows]);
  const defaultSignalNames = useMemo(() => allSignalColumns.slice(0, MaxSignalColumns).map((column) => column.signal!), [allSignalColumns]);
  const selectedSignalNames = columnLayout.signalNames ?? defaultSignalNames;
  const selectedSignalSet = useMemo(() => new Set(selectedSignalNames), [selectedSignalNames]);
  const columns = useMemo<Column[]>(() => [
    ...builtinColumns.filter((column) => !columnLayout.hiddenBuiltins.includes(column.key as OptionalBuiltinColumn)),
    ...allSignalColumns.filter((column) => selectedSignalSet.has(column.signal!)).slice(0, MaxSignalColumns),
  ], [allSignalColumns, builtinColumns, columnLayout.hiddenBuiltins, selectedSignalSet]);
  const signalColumnCount = columns.filter((column) => column.signal).length;
  const availableSignalCount = useMemo(() => new Set(rows.flatMap((row) => Object.keys(row.signals).filter((name) => !knownSignalNames.has(name.toLowerCase())))).size, [rows]);
  const parsedQuery = useMemo(() => parseCohortQuery(query), [query]);
  const queryDiagnostic = parsedQuery.diagnostics[0];

  const visible = useMemo(() => {
    const filtered = query.trim() ? rows.filter((row) => matchesCohortQuery(row, parsedQuery)) : rows;
    return [...filtered].sort((a, b) => {
      const left = rowValue(a, sort); const right = rowValue(b, sort);
      if (left === undefined) return 1;
      if (right === undefined) return -1;
      const result = typeof left === "number" && typeof right === "number" ? left - right : String(left).localeCompare(String(right), undefined, { numeric: true });
      return descending ? -result : result;
    });
  }, [rows, query, parsedQuery, sort, descending]);

  const chooseSort = (key: string) => { if (sort === key) setDescending((value) => !value); else { setSort(key); setDescending(key !== "id"); } };
  const toggleCompare = (id: string) => setCompareIds((current) => current.includes(id) ? current.filter((value) => value !== id) : current.length < 2 ? [...current, id] : [current[1], id]);
  const updateQuery = (value: string) => { setQuery(value); onQueryChange?.(value); };
  const closeColumns = () => setColumnsOpen(false);
  const updateColumnLayout = (next: GroupColumnLayout) => { setColumnLayout(next); saveGroupColumnLayout(next); };
  const toggleBuiltinColumn = (key: OptionalBuiltinColumn) => updateColumnLayout({
    ...columnLayout,
    hiddenBuiltins: columnLayout.hiddenBuiltins.includes(key) ? columnLayout.hiddenBuiltins.filter((item) => item !== key) : [...columnLayout.hiddenBuiltins, key],
  });
  const toggleSignalColumn = (name: string) => {
    const current = columnLayout.signalNames ?? defaultSignalNames;
    updateColumnLayout({ ...columnLayout, signalNames: current.includes(name) ? current.filter((item) => item !== name) : [...current, name] });
  };
  const selectRewardRank = (rank: "best" | "median" | "worst" | "outlier") => { const index = rewardRankIndex(visible, rank); if (index === undefined) return false; setSelected(index); };
  const selectNext = (predicate: (row: Row) => boolean) => {
    if (!visible.some(predicate)) return false;
    for (let offset = 1; offset <= visible.length; offset += 1) {
      const index = (selected + offset) % visible.length;
      if (predicate(visible[index])) { setSelected(index); return; }
    }
  };

  useEffect(() => setSelected((index) => Math.min(index, Math.max(visible.length - 1, 0))), [visible.length]);
  useEffect(() => setSelectedPath((index) => Math.min(index, Math.max(flatPaths.length - 1, 0))), [flatPaths.length]);
  useEffect(() => {
    if (columnsOpen) columnsCloseRef.current?.focus();
    else if (columnsWereOpen.current) columnsButtonRef.current?.focus();
    columnsWereOpen.current = columnsOpen;
  }, [columnsOpen]);
  useCommands("group", {
    [commandIds.group.back]: (event) => {
      const typing = event.target instanceof HTMLInputElement || event.target instanceof HTMLTextAreaElement;
      if (typing) { searchRef.current?.blur(); updateQuery(""); } else onClose();
    },
    [commandIds.group.togglePaths]: () => (paths || pathsError) ? void setMode("paths") : false,
    [commandIds.group.search]: () => searchRef.current?.focus(),
    [commandIds.group.next]: () => setSelected((value) => visible.length ? (value + 1) % visible.length : 0),
    [commandIds.group.previous]: () => setSelected((value) => visible.length ? (value - 1 + visible.length) % visible.length : 0),
    [commandIds.group.open]: () => visible[selected] ? void onOpen(visible[selected].id) : false,
    [commandIds.group.toggleCompare]: () => visible[selected] ? void toggleCompare(visible[selected].id) : false,
    [commandIds.group.compare]: () => compareIds.length === 2 && onCompare ? void onCompare(compareIds[0], compareIds[1]) : false,
    [commandIds.group.best]: () => selectRewardRank("best"),
    [commandIds.group.median]: () => selectRewardRank("median"),
    [commandIds.group.worst]: () => selectRewardRank("worst"),
    [commandIds.group.rewardOutlier]: () => selectRewardRank("outlier"),
    [commandIds.group.nextFailure]: () => selectNext((row) => row.pass === false),
    [commandIds.group.nextInfraFailure]: () => selectNext(isInfrastructureFailure),
    [commandIds.group.toggleColumns]: () => setColumnsOpen((value) => !value),
  }, mode === "trajectories" && !columnsOpen);
  useCommands("group", {
    [commandIds.group.back]: closeColumns,
    [commandIds.group.toggleColumns]: closeColumns,
  }, mode === "trajectories" && columnsOpen);
  useCommands("paths", {
    [commandIds.paths.back]: onClose,
    [commandIds.paths.togglePaths]: () => setMode("trajectories"),
    [commandIds.paths.next]: () => setSelectedPath((value) => flatPaths.length ? (value + 1) % flatPaths.length : 0),
    [commandIds.paths.previous]: () => setSelectedPath((value) => flatPaths.length ? (value - 1 + flatPaths.length) % flatPaths.length : 0),
    [commandIds.paths.open]: () => flatPaths[selectedPath]?.node.trajectory_ids[0] ? void onOpen(flatPaths[selectedPath].node.trajectory_ids[0]) : false,
  }, mode === "paths");

  const rewards = rows.map((row) => row.reward).filter((value): value is number => value !== undefined);
  const passed = rows.filter((row) => row.pass === true).length;
  const failed = rows.filter((row) => row.pass === false).length;
  const outcomes = Object.entries(rows.reduce<Record<string, number>>((counts, row) => {
    const outcome = row.outcome ?? row.termination ?? row.status;
    if (outcome) counts[outcome] = (counts[outcome] ?? 0) + 1;
    return counts;
  }, {})).sort((a, b) => b[1] - a[1]);

  return <main className="group-view" aria-label="Trajectory group">
    <header className="group-heading">
      <div><span className="eyebrow">Rollout group</span><h1>{group.group_id}</h1><p>{rows.length} trajectories · compare outcomes and open any run</p></div>
      <div className="group-heading-actions">{(paths || pathsError) && <button className={mode === "paths" ? "active" : ""} onClick={() => setMode((value) => value === "paths" ? "trajectories" : "paths")}>{mode === "paths" ? "Trajectories" : "Behavioral paths"} <kbd>{bindingLabel(mode === "paths" ? commandIds.paths.togglePaths : commandIds.group.togglePaths)}</kbd></button>}<button onClick={onClose}>Back to trajectory <kbd>{bindingLabel(mode === "paths" ? commandIds.paths.back : commandIds.group.back)}</kbd></button></div>
    </header>
    <section className="group-summary" aria-label="Group distribution">
      <div><span>REWARD MEAN</span><strong>{rewards.length ? displayNumber(rewards.reduce((sum, value) => sum + value, 0) / rewards.length) : "—"}</strong><small>{rewards.length ? `${displayNumber(Math.min(...rewards))}–${displayNumber(Math.max(...rewards))}` : "not reported"}</small></div>
      <div><span>PASS RATE</span><strong>{passed + failed ? `${Math.round(passed / (passed + failed) * 100)}%` : "—"}</strong><small>{passed + failed ? `${passed} pass · ${failed} fail` : "not reported"}</small></div>
      <div className="outcome-distribution"><span>OUTCOMES</span><div>{outcomes.length ? outcomes.map(([outcome, count]) => <b key={outcome}>{outcome}<i>{count}</i></b>) : <small>not reported</small>}</div></div>
    </section>
    {mode === "paths" ? <section className="group-path-panel" aria-label="Behavioral paths">
      <header><div><span className="eyebrow">Derived behavioral prefixes</span><h2>{paths ? `${flatPaths.length} compact nodes` : "Paths unavailable"}</h2></div>{paths && <div className="path-totals"><span>{paths.tree.behavioral_event_count} behavioral</span><span>{paths.tree.narrative_event_count} narrative compressed</span>{paths.source_native_branches && <span title="Source-native parent and branch annotations are reported separately from this derived tree">{paths.source_native_branch_count} native links</span>}</div>}</header>
      {paths ? <div className="path-list" role="tree" aria-label="Compact behavioral path tree">
        {paths.tree.root_narrative_event_count > 0 && <div className="path-compressed">⋯ {paths.tree.root_narrative_event_count} narrative events before the first behavior</div>}
        {flatPaths.map(({ node, branch }, index) => <button key={`${node.depth}-${node.fingerprint.alignment_key ?? node.fingerprint.state_hash ?? node.fingerprint.digest ?? node.fingerprint.kind}-${index}`} role="treeitem" aria-level={node.depth + 1} aria-selected={index === selectedPath} className={`path-row ${index === selectedPath ? "selected" : ""} ${branch ? "branch" : ""}`} style={{ "--path-indent": `${node.depth * 25}px` } as CSSProperties} onClick={() => setSelectedPath(index)} onDoubleClick={() => node.trajectory_ids[0] && onOpen(node.trajectory_ids[0])}>
          <span className="path-rail">{branch ? "├" : node.depth ? "│" : "●"}</span><span className={`kind kind-${node.fingerprint.kind}`}>{node.fingerprint.kind.slice(0, 2).toUpperCase()}</span><span className="path-copy"><strong>{pathLabel(node)}</strong><small>{node.fingerprint.class}{node.narrative_event_count ? ` · ${node.narrative_event_count} narrative compressed` : ""}</small></span>
          <span className="path-flow">{node.count > 1 && <b>shared ×{node.count}</b>}{node.children.length > 1 && <i>splits {node.children.length}</i>}{node.terminal_count > 0 && <em>{node.terminal_count} end</em>}</span><span className="path-sample">{node.trajectory_ids.slice(0, 2).join(", ")}{node.trajectory_ids_truncated || node.trajectory_ids.length > 2 ? "…" : ""}</span>
        </button>)}
        {!flatPaths.length && <div className="group-empty">No behavioral events · {paths.tree.narrative_only_count} narrative-only trajectories</div>}
      </div> : <div className="group-empty">{pathsError || "Compact paths are still loading"}</div>}
    </section> : <section className="group-table-panel">
      <div className="group-tools"><label><span>⌕</span><input ref={searchRef} aria-label="Filter trajectories" aria-invalid={queryDiagnostic ? true : undefined} aria-errormessage={queryDiagnostic ? "group-filter-error" : undefined} value={query} onChange={(event) => updateQuery(event.target.value)} placeholder="Filter · pass:false reward<0" title="Plain text or structured clauses, for example pass:false reward<0 signal.policy_reward<0" /><kbd>{bindingLabel(commandIds.group.search)}</kbd></label>{queryDiagnostic && <small id="group-filter-error" className="group-filter-error" role="status">{queryDiagnostic.message}</small>}<div className="compare-tools"><span>{compareIds.length}/2 selected</span><button disabled={compareIds.length !== 2 || !onCompare} onClick={() => onCompare?.(compareIds[0], compareIds[1])}>Compare <kbd>{bindingLabel(commandIds.group.compare)}</kbd></button></div><button ref={columnsButtonRef} className="column-picker-button" aria-haspopup="dialog" aria-expanded={columnsOpen} onClick={() => setColumnsOpen(true)}>Columns <kbd>{bindingLabel(commandIds.group.toggleColumns)}</kbd></button>{availableSignalCount > 0 && <span title={`${availableSignalCount} scalar canonical signals available`}>{signalColumnCount}/{availableSignalCount} signals</span>}<span>{visible.length}/{rows.length}</span></div>
      <div className="group-table-scroll">
        <table className="group-table"><thead><tr><th className="compare-check-heading">Compare</th><th className="trajectory-column"><button onClick={() => chooseSort("id")}>Trajectory {sort === "id" ? (descending ? "↓" : "↑") : ""}</button></th>{columns.map(({ key, label, signal }) => <th key={key} title={signal ? `Canonical signal: ${signal}` : undefined}><button onClick={() => chooseSort(key)}>{label} {sort === key ? (descending ? "↓" : "↑") : ""}</button></th>)}<th></th></tr></thead>
          <tbody>{visible.map((row, index) => <tr key={row.id} className={`${index === selected ? "selected" : ""} ${compareIds.includes(row.id) ? "compare-selected" : ""}`} aria-selected={index === selected} onClick={() => setSelected(index)} onDoubleClick={() => onOpen(row.id)}>
            <td className="compare-check"><input type="checkbox" aria-label={`Select ${row.id} for comparison`} checked={compareIds.includes(row.id)} onClick={(event) => event.stopPropagation()} onChange={() => toggleCompare(row.id)} /></td><td className="trajectory-column"><strong>{row.id}</strong>{row.outcome && <small>{row.outcome}</small>}</td>
            {columns.map(({ key }) => <td key={key} className={key === "reward" ? (row.reward ?? 0) < 0 ? "negative" : "positive" : ""}>{key === "pass" ? row.pass === undefined ? "—" : row.pass ? "PASS" : "FAIL" : key === "latency" ? displayLatency(row.latency) : displayScalar(rowValue(row, key))}</td>)}
            <td><button aria-label={`Open trajectory ${row.id}`} onClick={(event) => { event.stopPropagation(); onOpen(row.id); }}>Open</button></td>
          </tr>)}</tbody></table>
        {!visible.length && <div className="group-empty">{queryDiagnostic ? `Invalid filter · ${queryDiagnostic.message}` : "No matching trajectories"}</div>}
      </div>
    </section>}
    {columnsOpen && <div className="column-picker-backdrop" onMouseDown={closeColumns}>
      <section className="column-picker" role="dialog" aria-modal="true" aria-label="Table columns" onMouseDown={(event) => event.stopPropagation()}>
        <header><div><span className="eyebrow">Saved locally</span><h2>Table columns</h2><p>Trajectory identity, comparison, and Open always remain visible.</p></div><button ref={columnsCloseRef} aria-label="Close table columns" onClick={closeColumns}>×</button></header>
        <div className="column-picker-list">
          <fieldset><legend>Metrics</legend>{builtinColumns.map((column) => <label key={column.key}><input type="checkbox" checked={!columnLayout.hiddenBuiltins.includes(column.key as OptionalBuiltinColumn)} onChange={() => toggleBuiltinColumn(column.key as OptionalBuiltinColumn)} /><span>{column.label}</span></label>)}</fieldset>
          <fieldset><legend>Canonical signals <small>{signalColumnCount}/{MaxSignalColumns} shown</small></legend>{allSignalColumns.map((column) => {
            const checked = selectedSignalSet.has(column.signal!);
            return <label key={column.key} title={column.signal}><input type="checkbox" checked={checked} disabled={!checked && signalColumnCount >= MaxSignalColumns} onChange={() => toggleSignalColumn(column.signal!)} /><span>{column.label}</span><code>{column.signal}</code></label>;
          })}{!allSignalColumns.length && <p>No additional scalar signals in this group.</p>}{availableSignalCount > MaxDiscoveredSignals && <p>Showing the {MaxDiscoveredSignals} highest-coverage signals.</p>}</fieldset>
        </div>
        <footer><button onClick={() => { setColumnLayout(resetGroupColumnLayout()); }}>Reset default</button><button onClick={closeColumns}>Done</button></footer>
      </section>
    </div>}
    <footer className="group-keybar"><span><kbd>{bindingLabel(mode === "paths" ? commandIds.paths.togglePaths : commandIds.group.togglePaths)}</kbd> {mode === "paths" ? "trajectories" : "paths"}</span><span><kbd>{bindingLabel(mode === "paths" ? commandIds.paths.next : commandIds.group.next)}</kbd><kbd>{bindingLabel(mode === "paths" ? commandIds.paths.previous : commandIds.group.previous)}</kbd> select</span>{mode === "paths" ? <><span><kbd>{bindingLabel(commandIds.paths.open)}</kbd> open sample</span></> : <><span><kbd>{bindingLabel(commandIds.group.toggleCompare)}</kbd> mark</span><span><kbd>{bindingLabel(commandIds.group.compare)}</kbd> compare</span><span><kbd>{bindingLabel(commandIds.group.best)}</kbd><kbd>{bindingLabel(commandIds.group.median)}</kbd><kbd>{bindingLabel(commandIds.group.worst)}</kbd> reward rank</span><span><kbd>{bindingLabel(commandIds.group.rewardOutlier)}</kbd> outlier</span><span><kbd>{bindingLabel(commandIds.group.nextFailure)}</kbd><kbd>{bindingLabel(commandIds.group.nextInfraFailure)}</kbd> failures</span><span><kbd>{bindingLabel(commandIds.group.open)}</kbd> open</span><span><kbd>{bindingLabel(commandIds.group.search)}</kbd> filter</span></>}</footer>
  </main>;
}
