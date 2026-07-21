import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import type { RefObject } from "react";
import { daemonProvider } from "./provider";
import type { ViewerProvider } from "./provider";
import { bindingLabel, commandIds, commands, useCommands, useKeymapRevision } from "./commands";
import { attentionScore, axisX, firstAnomaly, glyphForKind, panWindowToInclude, stagesFor, stageChanged, verdictGlyph, zoomWindow } from "./instrument";
import type { Stage } from "./instrument";
import type { AnalysisResponse, BrowseResponse, BrowseTrajectory, ComparisonResponse, Trajectory, TrajectoryEvent } from "./types";
import { preview, title } from "./format";
import { sampleTrajectory } from "./sample";
import { applyPresentationTheme } from "./presentation";
import type { PresentationConfig } from "./types";

type Mode = "browse" | "read" | "compare";
const fidelityNames = ["hairline", "marks", "texture", "glyphs", "previews", "full"];

function isTextEntry(element: Element | null): boolean {
  return element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement || element instanceof HTMLSelectElement || (element instanceof HTMLElement && element.isContentEditable);
}

/** Keep the active reading surface keyboard-ready without taking focus from text entry. */
function useReadingSurfaceFocus(root: RefObject<HTMLElement | null>, restoreKey: string): void {
  useLayoutEffect(() => {
    const focusSurface = () => {
      if (!isTextEntry(document.activeElement)) root.current?.focus({ preventScroll: true });
    };
    focusSurface();
    const frame = window.requestAnimationFrame(focusSurface);
    window.addEventListener("focus", focusSurface);
    window.addEventListener("pageshow", focusSurface);
    return () => {
      window.cancelAnimationFrame(frame);
      window.removeEventListener("focus", focusSurface);
      window.removeEventListener("pageshow", focusSurface);
    };
  }, [root, restoreKey]);
}

function metric(row: BrowseTrajectory, name: string): unknown {
  const metrics = row.metrics.metrics ?? row.metrics.normalized_metrics ?? row.metrics;
  return metrics[name] ?? row.metrics[name];
}

function rowKey(row: BrowseTrajectory): string {
	return `${row.source_id}:${row.trajectory.id}`;
}

function isInfrastructureFailure(row: BrowseTrajectory): boolean {
  const failureClass = metric(row, "failure_class");
  return String(failureClass ?? row.trajectory.termination ?? "").toLowerCase().includes("infrastructure");
}

function eventDetail(event: TrajectoryEvent): unknown {
  return event.output ?? event.input ?? event.content ?? event.data ?? event.raw ?? event;
}

function eventText(event: TrajectoryEvent): string {
  return title(event) || `${event.kind} event`;
}

function fakeBrowse(trajectory: Trajectory): BrowseResponse {
  return {
    sources: [{ id: "sample" }], count: 1,
    trajectories: [{
      source_id: "sample", source_name: "sample", case_name: trajectory.name, group_name: trajectory.group_id,
      trajectory: { ...trajectory, events: undefined } as Omit<Trajectory, "events">,
      metrics: { trajectory: { ...trajectory, events: undefined }, event_count: trajectory.events.length, error_count: trajectory.events.filter((event) => event.kind === "error").length, reward: trajectory.total_reward },
    }],
  };
}

function HelpOverlay({ scope, onClose }: { scope: "group" | "trajectory" | "comparison"; onClose: () => void }) {
  useCommands("overlay", { [commandIds.trajectory.dismiss]: onClose, [commandIds.trajectory.toggleHelp]: onClose });
  const active = commands.filter((command) => (command.scope === scope || command.scope === "all") && command.defaultBindings.length);
  return <div className="instrument-overlay" role="dialog" aria-label="Active keyboard shortcuts">
    <div className="help-card"><header><h2>{scope} keys</h2><button onClick={onClose}>close Esc</button></header>
      <dl>{active.map((command) => <div key={command.id}><dt>{command.defaultBindings.join(" / ")}</dt><dd>{command.label}</dd></div>)}</dl>
    </div>
  </div>;
}

function Caterpillar({ row, fidelity }: { row: BrowseTrajectory; fidelity: number }) {
  const count = Math.max(1, Number(metric(row, "event_count") ?? 1));
  const errors = Number(metric(row, "error_count") ?? 0);
  const width = Math.min(100, 18 + Math.log2(count + 1) * 13);
  if (fidelity === 0) return <span className="cat-line" style={{ width: `${width}%` }} />;
  if (fidelity < 3) return <span className="cat-marks" style={{ width: `${width}%` }}>{Array.from({ length: Math.min(24, count) }, (_, index) => <i className={errors && index >= Math.min(23, count - 1) ? "abnormal" : ""} key={index} />)}</span>;
  const glyphs = `${"▸‒·▮".repeat(Math.max(1, Math.ceil(Math.min(count, 40) / 4))).slice(0, Math.min(count, 40))}${errors ? "✕" : ""}`;
  return <span className="cat-glyphs" style={{ width: `${width}%` }}>{glyphs}{fidelity >= 4 && <small>{count} events{errors ? ` · ${errors} errors` : ""}</small>}</span>;
}

function Browse({ rows, selected, fidelity, projection, marks, tags, query, onSelected, onFidelity, onProjection, onOpen, onToggleMark, onCompare, onTag, onQuery, help, setHelp }: {
  rows: BrowseTrajectory[]; selected: number; fidelity: number; projection: "table" | "caterpillar"; marks: Set<string>; tags: Map<string, number>; query: string;
  onSelected: (value: number) => void; onFidelity: (delta: number) => void; onProjection: (value: "table" | "caterpillar") => void;
  onOpen: () => void; onToggleMark: () => void; onCompare: () => void; onTag: (tag: number) => void; onQuery: (value: string) => void;
  help: boolean; setHelp: (value: boolean) => void;
}) {
  const root = useRef<HTMLElement>(null);
  useReadingSurfaceFocus(root, rows.map((row) => `${row.source_id}:${row.trajectory.id}`).join("\0"));
  const move = (delta: number) => { if (rows.length) onSelected((selected + delta + rows.length) % rows.length); };
  useCommands("group", {
    [commandIds.group.back]: () => root.current?.focus(),
    [commandIds.group.next]: () => move(1), [commandIds.group.previous]: () => move(-1),
    [commandIds.group.open]: onOpen, [commandIds.group.toggleCompare]: onToggleMark, [commandIds.group.compare]: () => marks.size === 2 ? onCompare() : false,
    [commandIds.group.search]: () => document.getElementById("browse-filter")?.focus(),
    [commandIds.group.tagVerdict1]: () => onTag(1), [commandIds.group.tagVerdict2]: () => onTag(2),
    [commandIds.group.tagVerdict3]: () => onTag(3), [commandIds.group.tagVerdict4]: () => onTag(4),
    [commandIds.view.fidelityUp]: () => onFidelity(1), [commandIds.view.fidelityDown]: () => onFidelity(-1),
    [commandIds.view.toggleHelp]: () => setHelp(!help),
  }, !help);
  return <main ref={root} tabIndex={0} className="instrument browse-mode" aria-label="Browse trajectories" data-selected-trajectory={rows[selected]?.trajectory.id ?? ""} data-fidelity={fidelityNames[fidelity]} data-filter={query}>
    <header className="instrument-head"><div><span className="eyebrow">Browse</span><h1>Known trajectories</h1><p>{rows.filter((row) => !tags.has(row.trajectory.id)).length} unresolved · attention ordered</p></div>
      <div className="browse-controls"><label>Filter <input id="browse-filter" value={query} onChange={(event) => onQuery(event.target.value)} /></label><button className={projection === "table" ? "active" : ""} onClick={() => onProjection("table")}>table</button><button className={projection === "caterpillar" ? "active" : ""} onClick={() => onProjection("caterpillar")}>caterpillars</button><button onClick={() => setHelp(true)}>?</button></div>
    </header>
    <div className="fidelity-readout">fidelity <b>{fidelityNames[fidelity]}</b> · [ ]</div>
    <section className={`browse-list projection-${projection}`} role="listbox" aria-label="Trajectory collection">
      {rows.map((row, index) => <button key={`${row.source_id}:${row.trajectory.id}`} role="option" aria-selected={index === selected} className={`browse-row ${index === selected ? "selected" : ""} ${marks.has(row.trajectory.id) ? "marked" : ""} ${isInfrastructureFailure(row) ? "failure-infra" : ""}`} onClick={() => onSelected(index)} onDoubleClick={onOpen}>
        <span className="verdict" aria-label={verdictGlyph(row) ? "attention required" : "nominal"}>{verdictGlyph(row)}</span>
        <span className="identity"><b>{row.trajectory.id}</b><small>{[row.source_name, row.run_name, row.case_name ?? row.group_name].filter(Boolean).join(" · ") || "uncategorized"}</small></span>
        <Caterpillar row={row} fidelity={fidelity} />
        {projection === "table" && <><span className="numeric">{String(metric(row, "event_count") ?? "—")} ev</span><span className="numeric">{metric(row, "reward") === undefined ? "" : `r ${String(metric(row, "reward"))}`}</span></>}
        <span className="row-state">{tags.has(row.trajectory.id) ? `tag ${tags.get(row.trajectory.id)}` : marks.has(row.trajectory.id) ? "reference set" : ""}</span>
      </button>)}
      {!rows.length && <p className="empty-state">No trajectories match this filter.</p>}
    </section>
    <footer className="instrument-keys"><span><kbd>j</kbd><kbd>k</kbd> select</span><span><kbd>Enter</kbd> read</span><span><kbd>Space</kbd> mark</span><span><kbd>v</kbd> compare</span><span><kbd>1–4</kbd> tag</span><span><kbd>?</kbd> keys</span></footer>
    {help && <HelpOverlay scope="group" onClose={() => setHelp(false)} />}
  </main>;
}

function eventReward(event: TrajectoryEvent): number | undefined {
  if (typeof event.reward === "number") return event.reward;
  if (event.kind === "reward" && event.data && typeof event.data === "object" && "total" in event.data && typeof event.data.total === "number") return event.data.total;
  return undefined;
}

function ShapeStrip({ trajectory, selected, hover, axis, onSelect, onHover }: {
  trajectory: Trajectory; selected: number; hover?: number; axis: { start: number; end: number }; onSelect: (index: number) => void; onHover: (index?: number) => void;
}) {
  const events = trajectory.events;
  const min = events[0]?.sequence ?? 0, max = events.at(-1)?.sequence ?? min + 1;
  const x = (sequence: number) => axisX(sequence, axis);
  const visible = events.map((event, index) => ({ event, index })).filter(({ event }) => event.sequence >= axis.start && event.sequence <= axis.end);
  const explicit = events.map((event, index) => ({ event, index })).filter(({ event }) => event.alignment_key?.startsWith("episode:") || event.alignment_key?.startsWith("stage:"));
  const bands = explicit.length ? explicit.map(({ event, index }, n) => ({ label: event.alignment_key!.split(":").slice(1).join(":"), start: event.sequence, end: explicit[n + 1]?.event.sequence ?? max, index })) : [{ label: "outcome", start: min, end: max, index: 0 }];
  const rewards: Array<{ x: number; value: number }> = [];
  let reward = 0;
  visible.forEach(({ event }) => { reward = eventReward(event) ?? reward; rewards.push({ x: x(event.sequence), value: reward }); });
  const rewardMin = Math.min(0, ...rewards.map((point) => point.value)), rewardMax = Math.max(1, ...rewards.map((point) => point.value));
  const path = rewards.map((point, index) => `${index ? "L" : "M"}${point.x},${182 - ((point.value - rewardMin) / Math.max(1, rewardMax - rewardMin)) * 26}`).join(" ");
  const selectedX = x(events[selected]?.sequence ?? min);
  return <section className="shape-strip" aria-label="Trajectory shape" data-selected-x={selectedX.toFixed(4)} data-visible-events={visible.length}>
    <svg viewBox="0 0 1000 200" preserveAspectRatio="none" onMouseLeave={() => onHover(undefined)} onMouseMove={(pointer) => {
      const rect = pointer.currentTarget.getBoundingClientRect();
      const px = ((pointer.clientX - rect.left) / rect.width) * 1000;
      const nearest = visible.reduce((best, item) => Math.abs(x(item.event.sequence) - px) < Math.abs(x(events[best]?.sequence ?? min) - px) ? item.index : best, visible[0]?.index ?? 0);
      onHover(nearest);
    }} onClick={() => hover !== undefined && onSelect(hover)}>
      <text className="lane-label" x="20" y="13">episodes</text>
      {bands.filter((band) => band.end >= axis.start && band.start <= axis.end).map((band) => <g key={`${band.label}:${band.start}`}><rect className="episode-band" x={x(Math.max(axis.start, band.start))} y="18" width={Math.max(1, x(Math.min(axis.end, band.end)) - x(Math.max(axis.start, band.start)))} height="23" /><text className="episode-label" x={x(Math.max(axis.start, band.start)) + 4} y="34">{band.label}</text></g>)}
      <text className="lane-label" x="20" y="61">events</text>
      {visible.map(({ event, index }) => {
        const px = x(event.sequence);
        if (event.kind === "error") return <path data-event-index={index} data-event-x={px.toFixed(4)} key={event.id} className="event-shape error" d={`M${px - 5},105 L${px},88 L${px + 5},105 Z`} />;
        if (event.kind === "tool" || event.kind === "environment_action") return <rect data-event-index={index} data-event-x={px.toFixed(4)} key={event.id} className="event-shape tool" x={px - 2} y="75" width="4" height="30" />;
        if (event.kind === "reward" || event.kind === "grader") return <circle data-event-index={index} data-event-x={px.toFixed(4)} key={event.id} className="event-shape evidence" cx={px} cy="84" r="5" />;
        if (event.context || event.alignment_key?.startsWith("context:")) return <path data-event-index={index} data-event-x={px.toFixed(4)} key={event.id} className="event-shape context" d={`M${px},68 l6,8 -6,8 -6,-8 Z`} />;
        return <line data-event-index={index} data-event-x={px.toFixed(4)} key={event.id} className="event-shape nominal" x1={px} x2={px} y1="94" y2="105" />;
      })}
      <text className="lane-label" x="20" y="123">context</text>
      {visible.map(({ event }) => event.context?.input_tokens !== undefined && event.context.capacity ? <rect key={`ctx:${event.id}`} className="context-pressure" x={x(event.sequence) - 2} y={153 - 25 * event.context.input_tokens / event.context.capacity} width="4" height={25 * event.context.input_tokens / event.context.capacity} /> : null)}
      <text className="lane-label" x="20" y="166">reward</text><path className="reward-curve" d={path} />
      {hover !== undefined && <line className="skimmer-line" x1={x(events[hover].sequence)} x2={x(events[hover].sequence)} y1="6" y2="194" />}
      <line data-testid="playhead" className="playhead" x1={selectedX} x2={selectedX} y1="5" y2="195" />
    </svg>
  </section>;
}

function judgesFor(trajectory: Trajectory): Array<{ label: string; value: string; eventId?: string }> {
  const judges: Array<{ label: string; value: string; eventId?: string }> = [];
  for (const event of trajectory.events.filter((item) => item.kind === "grader")) {
    const output = event.output && typeof event.output === "object" ? event.output as Record<string, unknown> : {};
    judges.push({ label: String(event.metadata?.grader ?? "grader"), value: String(output.verdict ?? output.score ?? "recorded"), eventId: event.id });
  }
  const reward = trajectory.signals?.find((signal) => signal.name === "reward");
  if (reward) judges.push({ label: "reward", value: String(reward.value), eventId: reward.event_id });
  const pass = trajectory.signals?.find((signal) => signal.name === "pass");
  if (pass) judges.push({ label: "verifier", value: String(pass.value), eventId: pass.event_id });
  return judges;
}

function Read({ trajectory, analysis, queueIndex, queueTotal, selected, fidelity, axis, hover, help, onSelected, onFidelity, onAxis, onHover, onBrowse, onRollout, onCompare, setHelp }: {
  trajectory: Trajectory; analysis: AnalysisResponse | null; queueIndex: number; queueTotal: number; selected: number; fidelity: number; axis: { start: number; end: number }; hover?: number; help: boolean;
  onSelected: (value: number) => void; onFidelity: (delta: number) => void; onAxis: (value: { start: number; end: number }) => void; onHover: (value?: number) => void;
  onBrowse: () => void; onRollout: (delta: number) => void; onCompare: () => void; setHelp: (value: boolean) => void;
}) {
  const root = useRef<HTMLElement>(null);
  const [depth, setDepth] = useState(1);
  useReadingSurfaceFocus(root, trajectory.id);
  const events = trajectory.events;
  const current = events[selected];
  const min = events[0]?.sequence ?? 0, max = events.at(-1)?.sequence ?? min + 1;
	const previousSequence = useRef(current.sequence);
  const move = (delta: number) => onSelected(Math.max(0, Math.min(events.length - 1, selected + delta)));
  const jump = (predicate: (event: TrajectoryEvent) => boolean) => {
    const next = events.findIndex((event, index) => index > selected && predicate(event));
    const wrapped = events.findIndex(predicate);
    if (next >= 0 || wrapped >= 0) onSelected(next >= 0 ? next : wrapped);
  };
  useEffect(() => {
	if (previousSequence.current === current.sequence) return;
	previousSequence.current = current.sequence;
    const next = panWindowToInclude(axis, current.sequence, min, max);
    if (next.start !== axis.start || next.end !== axis.end) onAxis(next);
  }, [axis, current.sequence, max, min, onAxis]);
  const findingIds = new Set((analysis?.analysis.findings ?? []).flatMap((finding) => finding.event_ids ?? []));
  useCommands("trajectory", {
    [commandIds.trajectory.next]: () => move(1), [commandIds.trajectory.previous]: () => move(-1),
    [commandIds.trajectory.nextError]: () => jump((event) => event.kind === "error"),
    [commandIds.trajectory.nextContext]: () => jump((event) => !!event.context || !!event.alignment_key?.startsWith("context:")),
    [commandIds.trajectory.nextReward]: () => jump((event) => event.kind === "reward" || event.kind === "grader"),
    [commandIds.trajectory.nextFinding]: () => jump((event) => findingIds.has(event.id)),
    [commandIds.trajectory.nextRollout]: () => onRollout(1), [commandIds.trajectory.previousRollout]: () => onRollout(-1),
    [commandIds.trajectory.ascend]: () => depth > 1 ? setDepth((value) => value - 1) : onBrowse(),
    [commandIds.trajectory.toggleExpanded]: () => setDepth((value) => Math.min(3, value + 1)), [commandIds.trajectory.openGroup]: onCompare,
    [commandIds.view.fidelityUp]: () => onFidelity(1), [commandIds.view.fidelityDown]: () => onFidelity(-1),
    [commandIds.view.zoomIn]: () => onAxis(zoomWindow(axis, current.sequence, 2, min, max)),
    [commandIds.view.zoomOut]: () => onAxis(zoomWindow(axis, current.sequence, 0.5, min, max)),
    [commandIds.view.zoomFit]: () => onAxis({ start: min, end: max }), [commandIds.view.toggleHelp]: () => setHelp(!help),
  }, !help);
  const around = Math.max(1, fidelity + 1);
  const detailRows = events.slice(Math.max(0, selected - around), Math.min(events.length, selected + around + 1));
  const judges = judgesFor(trajectory);
  return <main ref={root} tabIndex={0} className="instrument read-mode" aria-label="Read trajectory" data-trajectory={trajectory.id} data-depth={depth} data-fidelity={fidelityNames[fidelity]} data-axis-start={axis.start.toFixed(4)} data-axis-end={axis.end.toFixed(4)}>
    <header className="verdict-header"><div><span className="eyebrow">Read · {queueIndex + 1}/{queueTotal}</span><h1>{trajectory.name ?? trajectory.id}</h1><p>{trajectory.id} · ended: {trajectory.termination ?? trajectory.status ?? "recorded"}</p></div>
      <div className="judge-list">{judges.map((judge, index) => <button key={`${judge.label}:${index}`} className={/false|fail/i.test(judge.value) ? "failure" : judge.label === "verifier" && /true|pass/i.test(judge.value) ? "verifier-pass" : ""} onClick={() => { const found = events.findIndex((event) => event.id === judge.eventId); if (found >= 0) onSelected(found); }}><small>{judge.label}</small><b>{judge.value}</b></button>)}{!judges.length && <span className="silent-verdict">no judge outcome recorded</span>}</div>
      <button onClick={() => setHelp(true)}>?</button>
    </header>
    <ShapeStrip trajectory={trajectory} selected={selected} hover={hover} axis={axis} onSelect={onSelected} onHover={onHover} />
    {hover !== undefined && <aside className="skim-preview" role="status"><b>#{events[hover].sequence} · {events[hover].kind}</b><span>{eventText(events[hover])}</span></aside>}
    <section className="detail-region" aria-label="Selected moment">
      {detailRows.map((event) => <button id={`event-${event.id}`} key={event.id} className={`moment ${event.id === current.id ? "selected" : ""}`} onClick={() => onSelected(events.indexOf(event))}>
        <span className="address">{event.sequence}</span><span className="kind-glyph">{glyphForKind(event.kind)}</span><span className="moment-copy"><small>{event.kind}</small><b>{eventText(event)}</b>{event.id === current.id && fidelity >= 2 && <pre>{preview(eventDetail(event), fidelity >= 5 ? 1600 : 500)}</pre>}{event.id === current.id && <em>source · {event.source?.path ?? "canonical record"}{event.source?.line ? `:${event.source.line}` : ""}</em>}</span>
      </button>)}
    </section>
    <footer className="instrument-keys"><span><kbd>j</kbd><kbd>k</kbd> events</span><span><kbd>e</kbd><kbd>c</kbd><kbd>r</kbd><kbd>a</kbd> landmarks</span><span><kbd>+</kbd><kbd>-</kbd><kbd>0</kbd> zoom</span><span><kbd>[</kbd><kbd>]</kbd> {fidelityNames[fidelity]}</span><span><kbd>Enter</kbd><kbd>Esc</kbd> depth {depth}/3</span><span><kbd>g</kbd> compare</span><span className="selection-address">#{current.sequence}</span></footer>
    {help && <HelpOverlay scope="trajectory" onClose={() => setHelp(false)} />}
  </main>;
}

function Compare({ comparison, help, selected, onSelected, onBack, onOpenLeft, setHelp }: { comparison: ComparisonResponse; help: boolean; selected: number; onSelected: (value: number) => void; onBack: () => void; onOpenLeft: () => void; setHelp: (value: boolean) => void }) {
  const root = useRef<HTMLElement>(null);
  useReadingSurfaceFocus(root, `${comparison.left.trajectory.id}:${comparison.right.trajectory.id}`);
  const left = stagesFor(comparison.left), right = stagesFor(comparison.right);
  const tier = left.tier === right.tier ? left.tier : "outcome only";
  const leftStages = tier === "outcome only" ? [{ key: "outcome", label: "outcome", events: comparison.left.events }] : left.stages;
  const rightStages = tier === "outcome only" ? [{ key: "outcome", label: "outcome", events: comparison.right.events }] : right.stages;
  const keys = [...new Set([...leftStages.map((stage) => stage.key), ...rightStages.map((stage) => stage.key)])];
  const stage = (items: Stage[], key: string) => items.find((item) => item.key === key);
  const divergent = keys.findIndex((key) => stageChanged(stage(leftStages, key), stage(rightStages, key)));
  const [curve, setCurve] = useState(true);
  const [fidelity, setFidelity] = useState(3);
  useCommands("comparison", {
    [commandIds.comparison.back]: onBack,
    [commandIds.comparison.next]: () => onSelected(Math.min(keys.length - 1, selected + 1)),
    [commandIds.comparison.previous]: () => onSelected(Math.max(0, selected - 1)),
    [commandIds.comparison.firstDivergence]: () => divergent >= 0 ? onSelected(divergent) : false,
    [commandIds.comparison.toggleDivergenceCurve]: () => setCurve((value) => !value),
    [commandIds.comparison.openLeft]: onOpenLeft,
    [commandIds.view.fidelityUp]: () => setFidelity((value) => Math.min(5, value + 1)),
    [commandIds.view.fidelityDown]: () => setFidelity((value) => Math.max(0, value - 1)),
    [commandIds.view.toggleHelp]: () => setHelp(!help),
  }, !help);
  return <main ref={root} tabIndex={0} className="instrument compare-mode" aria-label="Pair Compare" data-selected-stage={keys[selected] ?? ""}>
    <header className="instrument-head"><div><span className="eyebrow">Pair Compare</span><h1>{comparison.left.trajectory.id} vs {comparison.right.trajectory.id}</h1><p>reference: <b>{comparison.left.trajectory.id}</b> · aligned by {tier} · never step index</p></div><button onClick={() => setHelp(true)}>?</button></header>
    <section className="stage-grid" aria-label="Stage aligned comparison">
      <div className="stage-head reference">reference</div><div className="stage-head">stage</div><div className="stage-head">candidate</div><div className="stage-head">Δ events</div>
      {keys.map((key, index) => { const l = stage(leftStages, key), r = stage(rightStages, key); const changed = stageChanged(l, r); const delta = (r?.events.length ?? 0) - (l?.events.length ?? 0); return <button key={key} className={`stage-row ${selected === index ? "selected" : ""} ${changed ? "divergent" : ""}`} onClick={() => onSelected(index)}>
        <span>{l ? `${l.events.length} events` : "absent"}{fidelity >= 4 && l && <small>{l.events.map((event) => glyphForKind(event.kind)).join("")}</small>}</span><b>{l?.label ?? r?.label ?? key}{index === divergent ? " ◂ first divergence" : ""}</b><span>{r ? `${r.events.length} events` : "absent"}{fidelity >= 4 && r && <small>{r.events.map((event) => glyphForKind(event.kind)).join("")}</small>}</span><em className={delta > 0 ? "ahead" : ""}>{delta >= 0 ? "+" : ""}{delta}</em>
      </button>; })}
    </section>
    {curve && <section className="delta-curve" aria-label="Cumulative event delta">cumulative cost delta {keys.map((key, index) => { const upto = keys.slice(0, index + 1).reduce((sum, stageKey) => sum + (stage(rightStages, stageKey)?.events.length ?? 0) - (stage(leftStages, stageKey)?.events.length ?? 0), 0); return <i key={key} className={upto > 0 ? "ahead" : ""} style={{ height: `${Math.max(2, Math.abs(upto) * 5)}px` }} title={`${key}: ${upto >= 0 ? "+" : ""}${upto}`} />; })}</section>}
    <section className="compare-detail"><div><h2>{comparison.left.trajectory.id} · reference</h2><pre>{preview(stage(leftStages, keys[selected])?.events.map(eventDetail), 1400)}</pre></div><div><h2>{comparison.right.trajectory.id}</h2><pre>{preview(stage(rightStages, keys[selected])?.events.map(eventDetail), 1400)}</pre></div></section>
    <footer className="instrument-keys"><span><kbd>j</kbd><kbd>k</kbd> stage</span><span><kbd>d</kbd> first divergent</span><span><kbd>Enter</kbd> read reference</span><span><kbd>Shift+D</kbd> curve</span><span><kbd>[</kbd><kbd>]</kbd> fidelity</span><span><kbd>Esc</kbd> Browse</span></footer>
    {help && <HelpOverlay scope="comparison" onClose={() => setHelp(false)} />}
  </main>;
}

export function App({ initialTrajectory, provider = daemonProvider }: { initialTrajectory?: Trajectory; provider?: ViewerProvider }) {
  useKeymapRevision();
  const [mode, setMode] = useState<Mode>("browse");
  const [browse, setBrowse] = useState<BrowseResponse>(() => fakeBrowse(initialTrajectory ?? sampleTrajectory));
  const [trajectory, setTrajectory] = useState(initialTrajectory ?? sampleTrajectory);
  const [analysis, setAnalysis] = useState<AnalysisResponse | null>(null);
  const [comparison, setComparison] = useState<ComparisonResponse | null>(null);
  const [comparisonSelected, setComparisonSelected] = useState(0);
  const [readReturnMode, setReadReturnMode] = useState<"browse" | "compare">("browse");
  const [browseIndex, setBrowseIndex] = useState(0);
  const [selected, setSelected] = useState(0);
  const [fidelity, setFidelity] = useState(3);
  const [projection, setProjection] = useState<"table" | "caterpillar">("table");
  const [marks, setMarks] = useState<Set<string>>(new Set());
  const [tags, setTags] = useState<Map<string, number>>(new Map());
  const [query, setQuery] = useState("");
  const [hover, setHover] = useState<number>();
  const [axis, setAxis] = useState({ start: trajectory.events[0]?.sequence ?? 0, end: trajectory.events.at(-1)?.sequence ?? 1 });
  const [help, setHelp] = useState(false);
  const [error, setError] = useState("");
  const [presentation, setPresentation] = useState<PresentationConfig>();
  const [theme, setTheme] = useState<"light" | "dark">(() => {
    const explicit = document.documentElement.getAttribute("data-theme");
    if (explicit === "light" || explicit === "dark") return explicit;
    return window.matchMedia?.("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  });
  const loading = useRef(false);
  const activeRow = useRef("");
  const openRequest = useRef(0);
  const selectionRevision = useRef(0);

  const [bootAttempt, setBootAttempt] = useState(0);
  useEffect(() => {
    if (initialTrajectory) return;
    const controller = new AbortController();
    setError("");
    Promise.all([provider.loadInitial(controller.signal), provider.loadBrowse(controller.signal)]).then(([loaded, collection]) => {
	  setBrowse(collection);
	  if (activeRow.current) return;
	  setTrajectory(loaded.trajectory); setPresentation(loaded.presentation);
	  const attentionOrdered = [...collection.trajectories].sort((a, b) => attentionScore(b) - attentionScore(a));
	  const rowIndex = attentionOrdered.findIndex((row) => row.trajectory.id === loaded.trajectory.id);
      setBrowseIndex(Math.max(0, rowIndex));
    }).catch((reason) => {
	  if (controller.signal.aborted || (reason instanceof Error && reason.name === "AbortError")) return;
	  setError(reason instanceof Error ? reason.message : "Could not load viewer");
	});
    return () => controller.abort();
  }, [initialTrajectory, bootAttempt, provider]);

  // A hash-only navigation is same-document: no reload, so the boot effect
  // would never re-run. Pasting a fresh `#token=` URL into a dead tab must
  // recover without a manual reload.
  useEffect(() => {
    const onHashChange = () => setBootAttempt((attempt) => attempt + 1);
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  useEffect(() => applyPresentationTheme(presentation), [presentation]);
  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
  }, [theme]);

  const ordered = useMemo(() => [...browse.trajectories].sort((a, b) => attentionScore(b) - attentionScore(a)), [browse]);
  const filtered = useMemo(() => ordered.filter((row) => !query || `${row.trajectory.id} ${row.source_name} ${row.case_name ?? ""} ${row.group_name ?? ""}`.toLowerCase().includes(query.toLowerCase())), [ordered, query]);
  const boundedBrowseIndex = Math.min(browseIndex, Math.max(0, filtered.length - 1));
  const selectedRow = filtered[boundedBrowseIndex];

  const openRow = async (row = selectedRow, preserveAxis = false) => {
    if (!row || loading.current) return;
    const rowID = rowKey(row);
    const requestID = ++openRequest.current;
    activeRow.current = rowID;
    loading.current = true; setError("");
    try {
      const loaded = row.source_id === "sample" ? { trajectory, isSample: true, presentation: undefined } : await provider.loadTrajectory(row.source_id, row.trajectory.id);
	  if (requestID !== openRequest.current || activeRow.current !== rowID) return;
      setTrajectory(loaded.trajectory); setPresentation(loaded.presentation); setAnalysis(null);
      const anomaly = firstAnomaly(loaded.trajectory); setSelected(anomaly); setHover(undefined);
	  const initialSelectionRevision = selectionRevision.current;
      if (!preserveAxis) setAxis({ start: loaded.trajectory.events[0].sequence, end: loaded.trajectory.events.at(-1)!.sequence });
      setMode("read");
      if (row.source_id !== "sample") provider.loadAnalysis(row.source_id, row.trajectory.id).then((result) => {
		if (activeRow.current !== rowID || requestID !== openRequest.current) return;
		setAnalysis(result);
		if (selectionRevision.current === initialSelectionRevision) setSelected(firstAnomaly(loaded.trajectory, result));
      }).catch(() => undefined);
	} catch (reason) {
	  if (activeRow.current === rowID && requestID === openRequest.current) activeRow.current = "";
	  setError(reason instanceof Error ? reason.message : "Could not load trajectory");
	}
    finally { loading.current = false; }
  };

  const toggleMark = () => {
    if (!selectedRow) return;
    setMarks((current) => { const next = new Set(current); next.has(selectedRow.trajectory.id) ? next.delete(selectedRow.trajectory.id) : next.size < 2 && next.add(selectedRow.trajectory.id); return next; });
  };
  const compareRows = async (ids = [...marks]) => {
    let pair = ids;
    if (pair.length !== 2 && mode === "read") {
      const reference = ordered.find((row) => row.trajectory.id !== trajectory.id && metric(row, "pass") === true) ?? ordered.find((row) => row.trajectory.id !== trajectory.id);
      if (reference) pair = [reference.trajectory.id, trajectory.id];
    }
    if (pair.length !== 2) return;
    const left = ordered.find((row) => row.trajectory.id === pair[0]), right = ordered.find((row) => row.trajectory.id === pair[1]);
    if (!left || !right || left.source_id !== right.source_id) { setError("Pair Compare requires two trajectories from one indexed source"); return; }
    try {
      const loaded = await provider.loadComparison(left.source_id, left.trajectory.id, right.trajectory.id);
      const leftStages = stagesFor(loaded.left), rightStages = stagesFor(loaded.right);
      const keys = [...new Set([...leftStages.stages.map((stage) => stage.key), ...rightStages.stages.map((stage) => stage.key)])];
      setComparisonSelected(Math.max(0, keys.findIndex((key) => stageChanged(leftStages.stages.find((stage) => stage.key === key), rightStages.stages.find((stage) => stage.key === key)))));
      setComparison(loaded); setMode("compare");
    }
    catch (reason) { setError(reason instanceof Error ? reason.message : "Could not compare trajectories"); }
  };
  const nextRollout = (delta: number) => {
	const index = filtered.findIndex((row) => rowKey(row) === activeRow.current);
	if (index < 0 || !filtered.length) return;
	const nextIndex = (index + delta + filtered.length) % filtered.length;
	const next = filtered[nextIndex];
	setBrowseIndex(nextIndex);
	void openRow(next, true);
  };

  const adjustFidelity = (delta: number) => setFidelity((value) => Math.max(0, Math.min(5, value + delta)));
  const selectEvent = (value: number) => { selectionRevision.current++; setSelected(value); };
  return <div className="instrument-shell" data-filter={query}>
    <button className="theme-toggle" aria-label={`Switch to ${theme === "light" ? "dark" : "light"} theme`} onClick={() => setTheme((current) => current === "light" ? "dark" : "light")}>{theme}</button>
    {error && <div className="instrument-error" role="alert">{error}</div>}
    {presentation?.notices?.map((notice) => <div className="presentation-notice" role="status" key={notice}>{notice}</div>)}
    {mode === "browse" && <Browse rows={filtered} selected={boundedBrowseIndex} fidelity={fidelity} projection={projection} marks={marks} tags={tags} query={query} onSelected={setBrowseIndex} onFidelity={adjustFidelity} onProjection={setProjection} onOpen={() => { setReadReturnMode("browse"); void openRow(); }} onToggleMark={toggleMark} onCompare={() => void compareRows()} onTag={(tag) => { if (!selectedRow) return; setTags((current) => new Map(current).set(selectedRow.trajectory.id, tag)); if (boundedBrowseIndex < filtered.length - 1) setBrowseIndex(boundedBrowseIndex + 1); }} onQuery={setQuery} help={help} setHelp={setHelp} />}
	{mode === "read" && <Read trajectory={trajectory} analysis={analysis} queueIndex={filtered.findIndex((row) => rowKey(row) === activeRow.current)} queueTotal={filtered.length} selected={selected} fidelity={fidelity} axis={axis} hover={hover} help={help} onSelected={selectEvent} onFidelity={adjustFidelity} onAxis={setAxis} onHover={setHover} onBrowse={() => { setHelp(false); setMode(readReturnMode); }} onRollout={nextRollout} onCompare={() => void compareRows()} setHelp={setHelp} />}
    {mode === "compare" && comparison && <Compare comparison={comparison} help={help} selected={comparisonSelected} onSelected={setComparisonSelected} onBack={() => { setHelp(false); setMode("browse"); }} onOpenLeft={() => { const left = ordered.find((row) => row.trajectory.id === comparison.left.trajectory.id); if (left) { setReadReturnMode("compare"); void openRow(left); } }} setHelp={setHelp} />}
  </div>;
}
