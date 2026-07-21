import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, PointerEvent as ReactPointerEvent, RefObject } from "react";
import { daemonProvider, ViewerProviderContext } from "./provider";
import type { ViewerProvider } from "./provider";
import { commandIds, commands, useCommands, useKeymapRevision } from "./commands";
import { attentionScore, axisX, episodeIndexForEvent, episodesFor, episodeWindow, firstAnomaly, glyphForKind, panWindowToInclude, verdictGlyph, zoomWindow } from "./instrument";
import type { Episode } from "./instrument";
import type { AnalysisResponse, BrowseResponse, BrowseTrajectory, Trajectory, TrajectoryEvent } from "./types";
import { preview, title } from "./format";
import { sampleTrajectory } from "./sample";
import { applyPresentationTheme } from "./presentation";
import type { PresentationConfig } from "./types";
import { defaultSeams, effectiveDepth, emptyWorkspace, laneId, legacyWorkspace, normalizeWorkspace, snapshotLabel, workspaceFromSearch, workspaceStorageKey, workspaceURL } from "./workspace";
import type { SeamRatios, WorkspaceLane, WorkspaceState } from "./workspace";
import { VirtualList } from "./VirtualList";

const fidelityNames = ["hairline", "marks", "texture", "glyphs", "previews", "full"];
const seamStorageKey = `${workspaceStorageKey}.seams`;
type SeamName = keyof SeamRatios;
type LaneData = { trajectory: Trajectory; analysis: AnalysisResponse | null; presentation?: PresentationConfig };

function metric(row: BrowseTrajectory, name: string): unknown {
  const metrics = row.metrics.metrics ?? row.metrics.normalized_metrics ?? row.metrics;
  return metrics[name] ?? row.metrics[name];
}

function rowKey(row: BrowseTrajectory): string { return laneId(row.source_id, row.trajectory.id); }
function eventDetail(event: TrajectoryEvent): unknown { return event.output ?? event.input ?? event.content ?? event.data ?? event.raw ?? event; }
function eventText(event: TrajectoryEvent): string { return title(event) || `${event.kind} event`; }
function eventReward(event: TrajectoryEvent): number | undefined {
  if (typeof event.reward === "number") return event.reward;
  if (event.kind === "reward" && event.data && typeof event.data === "object" && "total" in event.data && typeof event.data.total === "number") return event.data.total;
  return undefined;
}

function fakeBrowse(trajectory: Trajectory): BrowseResponse {
  return { sources: [{ id: "sample" }], count: 1, trajectories: [{
    source_id: "sample", source_name: "sample", case_name: trajectory.name, group_name: trajectory.group_id,
    trajectory: { ...trajectory, events: undefined } as Omit<Trajectory, "events">,
    metrics: { trajectory: { ...trajectory, events: undefined }, event_count: trajectory.events.length, error_count: trajectory.events.filter((event) => event.kind === "error").length, reward: trajectory.total_reward },
  }] };
}

function savedSeams(): SeamRatios {
  try {
    const parsed = JSON.parse(localStorage.getItem(seamStorageKey) ?? "null") as Partial<SeamRatios> | null;
    return normalizeWorkspace({ ...emptyWorkspace(), seams: parsed ?? defaultSeams })?.seams ?? { ...defaultSeams };
  } catch { return { ...defaultSeams }; }
}

function initialWorkspace(): WorkspaceState {
  return workspaceFromSearch(window.location.search) ?? legacyWorkspace(window.location.search) ?? { ...emptyWorkspace(), seams: savedSeams() };
}

function HelpOverlay({ onClose }: { onClose: () => void }) {
  useCommands("overlay", { [commandIds.trajectory.dismiss]: onClose, [commandIds.trajectory.toggleHelp]: onClose });
  const active = commands.filter((command) => (command.scope === "workspace" || command.scope === "trajectory" || command.scope === "all") && command.defaultBindings.length);
  return <div className="instrument-overlay" role="dialog" aria-label="Active keyboard shortcuts">
    <div className="help-card"><header><h2>workspace keys</h2><button onClick={onClose}>close Esc</button></header>
      <dl>{active.map((command) => <div key={command.id}><dt>{command.defaultBindings.join(" / ")}</dt><dd>{command.label}</dd></div>)}</dl>
    </div>
  </div>;
}

function Caterpillar({ row, fidelity }: { row: BrowseTrajectory; fidelity: number }) {
  const count = Math.max(1, Number(metric(row, "event_count") ?? 1));
  const errors = Number(metric(row, "error_count") ?? 0);
  const width = Math.min(100, 18 + Math.log2(count + 1) * 13);
  if (fidelity === 0) return <span className="cat-line" style={{ width: `${width}%` }}>{errors > 0 && <i className="abnormal" />}</span>;
  if (fidelity === 1) return <span className="cat-marks" style={{ width: `${width}%` }}>{Array.from({ length: Math.min(24, count) }, (_, index) => <i className={errors && index >= Math.min(23, count - 1) ? "abnormal" : ""} key={index} />)}</span>;
  if (fidelity === 2) return <span className="cat-texture" style={{ width: `${width}%` }}>{Array.from({ length: Math.min(32, count) }, (_, index) => <i className={`${index % 3 === 0 ? "strong" : ""} ${errors && index >= Math.min(31, count - 1) ? "abnormal" : ""}`} key={index} />)}</span>;
  const glyphs = `${"▸‒·▮".repeat(Math.max(1, Math.ceil(Math.min(count, 40) / 4))).slice(0, Math.min(count, 40))}${errors ? "✕" : ""}`;
  return <span className="cat-glyphs" style={{ width: `${width}%` }}>{glyphs}{fidelity >= 4 && <small>{count} events{errors ? ` · ${errors} errors` : ""}</small>}</span>;
}

function Rail({ root, rows, workspace, fidelity, tags, onActivate, onSelect, onOpen, onAdd, onQuery, onTag }: {
  root: RefObject<HTMLElement | null>; rows: BrowseTrajectory[]; workspace: WorkspaceState; fidelity: number; tags: Map<string, number>;
  onActivate: () => void; onSelect: (index: number) => void; onOpen: () => void; onAdd: () => void; onQuery: (query: string) => void; onTag: (tag: number) => void;
}) {
  const selected = Math.min(workspace.railSelected, Math.max(0, rows.length - 1));
  return <main ref={root} tabIndex={0} className={`workspace-rail ${workspace.active === "rail" ? "active-zone" : ""}`} aria-label="Browse trajectories" data-filter={workspace.railQuery} data-fidelity={fidelityNames[fidelity]} onFocus={onActivate}>
    <header><div><span className="eyebrow">Rail</span><h1>Known trajectories</h1><p>{rows.filter((row) => !tags.has(row.trajectory.id)).length} unresolved</p></div></header>
    <div className="rail-controls"><label>Filter <input id="browse-filter" value={workspace.railQuery} onChange={(event) => onQuery(event.target.value)} onKeyDown={(event) => { if (event.key === "Escape") { event.preventDefault(); root.current?.focus(); } }} /></label></div>
    <div className="fidelity-readout">fidelity <b>{fidelityNames[fidelity]}</b> · [ ]</div>
    <section className={`browse-list rail-fidelity-${fidelity}`} role="listbox" aria-label="Trajectory collection" data-fidelity-level={`L${fidelity}`}>
      {rows.map((row, index) => <button key={rowKey(row)} role="option" aria-selected={index === selected} data-fidelity-level={`L${fidelity}`} data-columns={fidelity >= 4 ? "true" : "false"} className={`browse-row ${index === selected ? "selected" : ""}`} onClick={() => onSelect(index)} onDoubleClick={onOpen}>
        {fidelity >= 1 && <span className="verdict">{verdictGlyph(row)}</span>}<span className="identity"><b>{row.trajectory.id}</b>{fidelity >= 4 && <small>{row.case_name ?? row.group_name ?? row.source_name}</small>}</span><Caterpillar row={row} fidelity={fidelity} />
        {fidelity >= 4 && <><span className="numeric events-column">{String(metric(row, "event_count") ?? "—")} ev</span><span className="numeric reward-column">{metric(row, "reward") === undefined ? "" : `r ${String(metric(row, "reward"))}`}</span></>}
        {fidelity >= 5 && <span className="row-state">{tags.has(row.trajectory.id) ? `tag ${tags.get(row.trajectory.id)}` : `${row.source_name}${row.group_name ? ` · ${row.group_name}` : ""}`}</span>}
      </button>)}
      {!rows.length && <p className="empty-state">No trajectories match this filter.</p>}
    </section>
    <footer className="zone-keys"><span><kbd>Enter</kbd> open</span><span><kbd>A</kbd> add</span><span><kbd>Tab</kbd> cycle</span></footer>
    <span className="rail-actions"><button onClick={onAdd}>add lane</button>{[1, 2, 3, 4].map((tag) => <button key={tag} onClick={() => onTag(tag)}>tag {tag}</button>)}</span>
  </main>;
}

function ShapeStrip({ trajectory, selected, hover, axis, compact = false, label, onSelect, onHover, onAscend }: {
  trajectory: Trajectory; selected: number; hover?: number; axis: { start: number; end: number }; compact?: boolean; label?: string; onSelect: (index: number) => void; onHover: (index?: number) => void; onAscend?: () => void;
}) {
  const events = trajectory.events;
  const min = events[0]?.sequence ?? 0, max = events.at(-1)?.sequence ?? min + 1;
  const x = (sequence: number) => axisX(sequence, axis);
  const visible = events.map((event, index) => ({ event, index })).filter(({ event }) => event.sequence >= axis.start && event.sequence <= axis.end);
  const bands = episodesFor(events);
  const rewards: Array<{ x: number; value: number }> = []; let reward = 0;
  visible.forEach(({ event }) => { reward = eventReward(event) ?? reward; rewards.push({ x: x(event.sequence), value: reward }); });
  const rewardMin = Math.min(0, ...rewards.map((point) => point.value)), rewardMax = Math.max(1, ...rewards.map((point) => point.value));
  const path = rewards.map((point, index) => `${index ? "L" : "M"}${point.x},${182 - ((point.value - rewardMin) / Math.max(1, rewardMax - rewardMin)) * 26}`).join(" ");
  const selectedX = x(events[selected]?.sequence ?? min);
  return <section className={`shape-strip ${compact ? "compact" : ""}`} aria-label={label ?? "Trajectory shape"} data-selected-x={selectedX.toFixed(4)} data-visible-events={visible.length}>
    <svg viewBox="0 0 1000 200" preserveAspectRatio="none" onMouseLeave={() => onHover(undefined)} onMouseMove={(pointer) => {
      const rect = pointer.currentTarget.getBoundingClientRect(); const px = ((pointer.clientX - rect.left) / Math.max(1, rect.width)) * 1000;
      const nearest = visible.reduce((best, item) => Math.abs(x(item.event.sequence) - px) < Math.abs(x(events[best]?.sequence ?? min) - px) ? item.index : best, visible[0]?.index ?? 0); onHover(nearest);
    }} onClick={() => onAscend ? onAscend() : hover !== undefined && onSelect(hover)}>
      {!compact && <><text className="lane-label" x="20" y="13">episodes</text>{bands.filter((band) => band.end >= axis.start && band.start <= axis.end).map((band) => <g key={band.key} data-episode-key={band.key} data-episode-start={band.start} data-episode-end={band.end}><rect className="episode-band" x={x(Math.max(axis.start, band.start))} y="18" width={Math.max(1, x(Math.min(axis.end, band.end)) - x(Math.max(axis.start, band.start)))} height="23" /><text className="episode-label" x={x(Math.max(axis.start, band.start)) + 4} y="34">{band.label}</text></g>)}</>}
      <text className="lane-label" x="20" y={compact ? 34 : 61}>events</text>
      {visible.map(({ event, index }) => { const px = x(event.sequence), offset = compact ? -45 : 0;
        if (event.kind === "error") return <path data-event-index={index} key={event.id} className="event-shape error" d={`M${px - 5},${105 + offset} L${px},${88 + offset} L${px + 5},${105 + offset} Z`} />;
        if (event.kind === "tool" || event.kind === "environment_action") return <rect data-event-index={index} key={event.id} className="event-shape tool" x={px - 2} y={75 + offset} width="4" height="30" />;
        if (event.kind === "reward" || event.kind === "grader") return <circle data-event-index={index} key={event.id} className="event-shape evidence" cx={px} cy={84 + offset} r="5" />;
        if (event.context || event.alignment_key?.startsWith("context:")) return <path data-event-index={index} key={event.id} className="event-shape context" d={`M${px},${68 + offset} l6,8 -6,8 -6,-8 Z`} />;
        return <line data-event-index={index} key={event.id} className="event-shape nominal" x1={px} x2={px} y1={94 + offset} y2={105 + offset} />;
      })}
      {!compact && <><text className="lane-label" x="20" y="123">context</text>{visible.map(({ event }) => event.context?.input_tokens !== undefined && event.context.capacity ? <rect key={`ctx:${event.id}`} className="context-pressure" x={x(event.sequence) - 2} y={153 - 25 * event.context.input_tokens / event.context.capacity} width="4" height={25 * event.context.input_tokens / event.context.capacity} /> : null)}<text className="lane-label" x="20" y="166">reward</text><path className="reward-curve" d={path} /></>}
      {hover !== undefined && <line className="skimmer-line" x1={x(events[hover].sequence)} x2={x(events[hover].sequence)} y1="5" y2="195" />}<line data-testid="playhead" className="playhead" x1={selectedX} x2={selectedX} y1="5" y2="195" />
    </svg>
  </section>;
}

function EpisodeStrip({ trajectory, lane, episodes, selectedEpisode, onDescend }: { trajectory: Trajectory; lane: WorkspaceLane; episodes: Episode[]; selectedEpisode: number; onDescend: (episode: Episode) => void }) {
  const x = (sequence: number) => axisX(sequence, lane.axis);
  const selectedSequence = trajectory.events[lane.selected]?.sequence ?? trajectory.events[0]?.sequence ?? 0;
  return <section className="episode-strip" aria-label="Trajectory shape" data-selected-x={x(selectedSequence).toFixed(4)} data-selected-episode={episodes[selectedEpisode]?.key ?? ""}>
    <div className="episode-axis">
      {episodes.filter((episode) => episode.end >= lane.axis.start && episode.start <= lane.axis.end).map((episode, index) => {
        const actualIndex = episodes.indexOf(episode);
        const nextStart = episodes[actualIndex + 1]?.start ?? Math.max(episode.end + 1, lane.axis.end);
        const left = x(Math.max(lane.axis.start, episode.start)) / 10;
        const right = x(Math.min(lane.axis.end, nextStart)) / 10;
        return <button key={episode.key} className={`episode-button ${actualIndex === selectedEpisode ? "selected" : ""}`} data-episode-index={actualIndex} data-episode-key={episode.key} data-episode-start={episode.start} data-episode-end={episode.end} style={{ left: `${left}%`, width: `${Math.max(0.8, right - left)}%` }} onClick={(event) => { event.stopPropagation(); onDescend(episode); }}>
          <b>{episode.label}</b><small>{episode.inferred ? "inferred" : "adapter"} · {episode.endIndex - episode.startIndex + 1} events</small>
        </button>;
      })}
      <i className="episode-playhead" style={{ left: `${x(selectedSequence) / 10}%` }} />
    </div>
    {episodes[selectedEpisode] && <EpisodeSummary trajectory={trajectory} episode={episodes[selectedEpisode]} />}
  </section>;
}

function EpisodeSummary({ trajectory, episode }: { trajectory: Trajectory; episode: Episode }) {
  const events = trajectory.events.slice(episode.startIndex, episode.endIndex + 1);
  const counts = new Map<string, number>(); events.forEach((event) => counts.set(event.kind, (counts.get(event.kind) ?? 0) + 1));
  const errors = events.filter((event) => event.kind === "error").length;
  return <div className="episode-summary" aria-label="Selected episode summary"><span><b>{episode.label}</b><small>{episode.inferred ? "deterministic fallback" : "alignment key"}</small></span><span><small>kinds</small><b>{[...counts].map(([kind, count]) => `${kind} ${count}`).join(" · ")}</b></span><span><small>errors</small><b>{errors}</b></span><span><small>span</small><b>#{episode.start}–#{episode.end}</b></span></div>;
}

function ScopedEvents({ trajectory, episode, selected, onSelect }: { trajectory: Trajectory; episode: Episode; selected: number; onSelect: (index: number) => void }) {
  return <section className="lane-events" aria-label="Episode events" data-episode-key={episode.key}>
    {trajectory.events.slice(episode.startIndex, episode.endIndex + 1).map((event, offset) => { const index = episode.startIndex + offset; return <button key={event.id} className={`lane-event ${index === selected ? "selected" : ""}`} onClick={(pointer) => { pointer.stopPropagation(); onSelect(index); }}><span className="address">#{event.sequence}</span><span className="kind-glyph">{glyphForKind(event.kind)}</span><span><small>{event.kind}</small><b>{eventText(event)}</b></span></button>; })}
  </section>;
}

function SourceRecord({ event }: { event: TrajectoryEvent }) {
  return <section className="lane-source" aria-label="Event source"><div className="source-provenance"><h3>Provenance</h3><dl><dt>event</dt><dd>{event.id}</dd><dt>address</dt><dd>#{event.sequence}</dd><dt>source</dt><dd>{event.source?.path ?? "canonical record"}{event.source?.line ? `:${event.source.line}` : ""}</dd>{event.source?.byte_start !== undefined && <><dt>bytes</dt><dd>{event.source.byte_start}–{event.source.byte_end ?? "?"}</dd></>}</dl></div><div><h3>Raw normalized record</h3><pre>{JSON.stringify(event.raw ?? event, null, 2)}</pre></div></section>;
}

function LaneTrack({ lane, data, active, reference, hover, onActivate, onSelect, onHover, onDescend, onAscend }: {
  lane: WorkspaceLane; data?: LaneData; active: boolean; reference: boolean; hover?: number; onActivate: () => void; onSelect: (index: number) => void; onHover: (index?: number) => void; onDescend: (episode?: Episode) => void; onAscend: () => void;
}) {
  const trajectory = data?.trajectory;
  const depth = effectiveDepth(lane);
  const selected = trajectory ? Math.min(lane.selected, trajectory.events.length - 1) : 0;
  const episodes = trajectory ? episodesFor(trajectory.events) : [];
  const selectedEpisode = episodeIndexForEvent(episodes, selected);
  const episode = episodes[selectedEpisode];
  return <main tabIndex={0} aria-label={lane.band === "focus" ? "Read trajectory" : `Context lane ${lane.trajectoryId}`} className={`lane-track depth-${depth} ${lane.band}-lane ${active ? "active-zone" : ""} ${reference ? "reference-lane" : ""}`} data-lane-id={lane.id} data-trajectory={lane.trajectoryId} data-depth={depth} data-stored-depth={lane.depth} data-episode={episode?.key ?? ""} data-fidelity={fidelityNames[lane.fidelity]} data-axis-start={lane.axis.start.toFixed(4)} data-axis-end={lane.axis.end.toFixed(4)} onFocus={onActivate} onClick={onActivate}>
    <header><span><b>{lane.trajectoryId}</b><small>{lane.band}{reference ? " · reference" : ""}</small></span><span className="lane-state">{["", "surface", "episodes", "events", "source"][depth]} · depth {depth}/4 · {fidelityNames[lane.fidelity]}</span></header>
    {!trajectory ? <div className="lane-loading">loading trajectory…</div> : depth === 1 ? <ShapeStrip trajectory={trajectory} selected={selected} hover={hover} axis={lane.axis} compact={lane.band === "context"} label={lane.band === "focus" ? "Trajectory shape" : `Trajectory shape ${lane.trajectoryId}`} onSelect={onSelect} onHover={onHover} /> : depth === 2 ? <EpisodeStrip trajectory={trajectory} lane={lane} episodes={episodes} selectedEpisode={selectedEpisode} onDescend={(target) => onDescend(target)} /> : <><ShapeStrip trajectory={trajectory} selected={selected} hover={hover} axis={lane.axis} compact label="Compressed trajectory shape" onSelect={onSelect} onHover={onHover} onAscend={onAscend} />{depth === 3 && episode && <ScopedEvents trajectory={trajectory} episode={episode} selected={selected} onSelect={onSelect} />}{depth === 4 && trajectory.events[selected] && <SourceRecord event={trajectory.events[selected]} />}</>}
    {trajectory && hover !== undefined && depth === 1 && lane.band === "focus" && <aside className="skim-preview" role="status"><b>#{trajectory.events[hover].sequence} · {trajectory.events[hover].kind}</b><span>{eventText(trajectory.events[hover])}</span></aside>}
  </main>;
}

function ContextBand({ lanes, workspace, laneData, hover, activate, select, setLaneHover }: {
  lanes: WorkspaceLane[]; workspace: WorkspaceState; laneData: Map<string, LaneData>; hover: Record<string, number | undefined>;
  activate: (id: string) => void; select: (id: string, index: number) => void; setLaneHover: (id: string, value?: number) => void;
}) {
  const scrollRef = useRef<HTMLDivElement>(null);
  if (!lanes.length) return <div ref={scrollRef} className="context-band" aria-label="Context band"><span className="context-empty">context lanes</span></div>;
  return <div ref={scrollRef} className="context-band" aria-label="Context band"><VirtualList items={lanes} estimateSize={71} overscan={2} selectedIndex={lanes.findIndex((lane) => lane.id === workspace.active)} scrollRef={scrollRef} className="context-lane-list" itemKey={(lane) => lane.id} renderItem={(lane) => <LaneTrack lane={lane} data={laneData.get(lane.id)} active={workspace.active === lane.id} reference={workspace.reference === lane.id} hover={hover[lane.id]} onActivate={() => activate(lane.id)} onSelect={(value) => select(lane.id, value)} onHover={(value) => setLaneHover(lane.id, value)} onDescend={() => undefined} onAscend={() => undefined} />} /></div>;
}

function judgesFor(trajectory: Trajectory): Array<{ label: string; value: string; eventId?: string }> {
  const judges: Array<{ label: string; value: string; eventId?: string }> = [];
  for (const event of trajectory.events.filter((item) => item.kind === "grader")) {
    const output = event.output && typeof event.output === "object" ? event.output as Record<string, unknown> : {};
    judges.push({ label: String(event.metadata?.grader ?? "grader"), value: String(output.verdict ?? output.score ?? "recorded"), eventId: event.id });
  }
  const reward = trajectory.signals?.find((signal) => signal.name === "reward"); if (reward) judges.push({ label: "reward", value: String(reward.value), eventId: reward.event_id });
  const pass = trajectory.signals?.find((signal) => signal.name === "pass"); if (pass) judges.push({ label: "verifier", value: String(pass.value), eventId: pass.event_id });
  return judges;
}

function Console({ workspace, lane, data, breadcrumb, resizeMode, onSelect, onHelp }: {
  workspace: WorkspaceState; lane?: WorkspaceLane; data?: LaneData; breadcrumb: string; resizeMode: boolean; onSelect: (index: number) => void; onHelp: () => void;
}) {
  const trajectory = data?.trajectory; const current = trajectory?.events[Math.min(lane?.selected ?? 0, Math.max(0, trajectory.events.length - 1))];
  const around = lane ? Math.max(1, lane.fidelity + 1) : 1;
  const detailRows = trajectory && current ? trajectory.events.slice(Math.max(0, trajectory.events.indexOf(current) - around), Math.min(trajectory.events.length, trajectory.events.indexOf(current) + around + 1)) : [];
  return <section className="workspace-console" aria-label="Workspace console" data-resize-mode={resizeMode ? "true" : "false"}>
    <header className="console-header"><div><span className="eyebrow">Console</span><h2>{trajectory?.name ?? trajectory?.id ?? "No lane selected"}</h2><p className="workspace-breadcrumb">{breadcrumb}</p></div>
      {trajectory && <div className="judge-list">{judgesFor(trajectory).map((judge, index) => <button key={`${judge.label}:${index}`} className={/false|fail/i.test(judge.value) ? "failure" : judge.label === "verifier" && /true|pass/i.test(judge.value) ? "verifier-pass" : ""} onClick={() => { const found = trajectory.events.findIndex((event) => event.id === judge.eventId); if (found >= 0) onSelect(found); }}><small>{judge.label}</small><b>{judge.value}</b></button>)}</div>}
      <div className="console-meta"><span>reference: <b data-testid="reference-name">{workspace.reference ? workspace.lanes.find((item) => item.id === workspace.reference)?.trajectoryId ?? "none" : "none"}</b></span>{resizeMode && <strong>resize mode · arrows · Esc</strong>}<button onClick={onHelp}>?</button></div>
    </header>
    <section className="detail-region" aria-label="Selected moment">{detailRows.map((event) => <button key={event.id} className={`moment ${event.id === current?.id ? "selected" : ""}`} onClick={() => onSelect(trajectory!.events.indexOf(event))}><span className="address">{event.sequence}</span><span className="kind-glyph">{glyphForKind(event.kind)}</span><span className="moment-copy"><small>{event.kind}</small><b>{eventText(event)}</b>{event.id === current?.id && lane && lane.fidelity >= 2 && <pre>{preview(eventDetail(event), lane.fidelity >= 5 ? 1600 : 500)}</pre>}{event.id === current?.id && <em>source · {event.source?.path ?? "canonical record"}{event.source?.line ? `:${event.source.line}` : ""}</em>}</span></button>)}</section>
    <footer className="instrument-keys"><span><kbd>Tab</kbd> zones</span><span><kbd>n</kbd><kbd>p</kbd> sweep</span><span><kbd>Shift+Enter</kbd> promote</span><span><kbd>Ctrl+w</kbd> resize</span>{current && <span className="selection-address">#{current.sequence}</span>}</footer>
  </section>;
}

function Sash({ name, orientation, onPointerDown, onReset }: { name: SeamName; orientation: "horizontal" | "vertical"; onPointerDown: (event: ReactPointerEvent<HTMLDivElement>, name: SeamName) => void; onReset: (name: SeamName) => void }) {
  return <div role="separator" aria-label={`${name} seam`} aria-orientation={orientation} className={`workspace-sash ${orientation}`} data-seam={name} onPointerDown={(event) => onPointerDown(event, name)} onDoubleClick={() => onReset(name)} />;
}

export function App({ initialTrajectory, provider = daemonProvider }: { initialTrajectory?: Trajectory; provider?: ViewerProvider }) {
  useKeymapRevision();
  const [workspace, setWorkspace] = useState<WorkspaceState>(initialWorkspace);
  const workspaceRef = useRef(workspace); workspaceRef.current = workspace;
  const [browse, setBrowse] = useState<BrowseResponse>(() => fakeBrowse(initialTrajectory ?? sampleTrajectory));
  const [laneData, setLaneData] = useState<Map<string, LaneData>>(() => new Map(initialTrajectory ? [[laneId("sample", initialTrajectory.id), { trajectory: initialTrajectory, analysis: null }]] : []));
  const laneDataRef = useRef(laneData); laneDataRef.current = laneData;
  const [railFidelity, setRailFidelity] = useState(3);
  const [tags, setTags] = useState<Map<string, number>>(new Map());
  const [hover, setHover] = useState<Record<string, number | undefined>>({});
  const [help, setHelp] = useState(false); const [resizeMode, setResizeMode] = useState(false); const [error, setError] = useState("");
  const [presentation, setPresentation] = useState<PresentationConfig>();
  const [theme, setTheme] = useState<"light" | "dark">(() => document.documentElement.getAttribute("data-theme") === "dark" || (!document.documentElement.getAttribute("data-theme") && window.matchMedia?.("(prefers-color-scheme: dark)").matches) ? "dark" : "light");
  const [breadcrumb, setBreadcrumb] = useState(() => snapshotLabel(workspace));
  const railRef = useRef<HTMLElement>(null); const rackRef = useRef<HTMLDivElement>(null); const stageRef = useRef<HTMLDivElement>(null); const focusRef = useRef<HTMLDivElement>(null);
  const lastFocus = useRef<string | undefined>(undefined);
  const jumpList = useRef<WorkspaceState[]>([workspace]); const jumpIndex = useRef(0); const restoring = useRef(false); const openRevision = useRef(0);
  const laneDataLRU = useRef<string[]>([]);
  const pendingReplace = useRef<WorkspaceState | undefined>(undefined); const replaceFrame = useRef<number | undefined>(undefined);
  const legacyReadIntent = useRef((() => { const params = new URLSearchParams(window.location.search); return (params.get("mode") === "read" || params.get("view") === "read") && !params.get("trajectory_id"); })());

  const ordered = useMemo(() => [...browse.trajectories].sort((a, b) => attentionScore(b) - attentionScore(a)), [browse]);
  const filtered = useMemo(() => ordered.filter((row) => !workspace.railQuery || `${row.trajectory.id} ${row.source_name} ${row.case_name ?? ""} ${row.group_name ?? ""}`.toLowerCase().includes(workspace.railQuery.toLowerCase())), [ordered, workspace.railQuery]);
  const boundedRail = Math.min(workspace.railSelected, Math.max(0, filtered.length - 1)); const selectedRow = filtered[boundedRail];
  const activeLane = workspace.active === "rail" ? undefined : workspace.lanes.find((lane) => lane.id === workspace.active);

  const writeURL = useCallback((next: WorkspaceState, push: boolean) => {
    try { localStorage.setItem(seamStorageKey, JSON.stringify(next.seams)); } catch { /* storage is optional */ }
    const state = { rlvizWorkspace: next };
    if (push) {
      if (replaceFrame.current !== undefined) cancelAnimationFrame(replaceFrame.current);
      replaceFrame.current = undefined; pendingReplace.current = undefined;
      window.history.pushState(state, "", workspaceURL(next));
      return;
    }
    pendingReplace.current = next;
    if (replaceFrame.current !== undefined) return;
    replaceFrame.current = requestAnimationFrame(() => {
      replaceFrame.current = undefined;
      const latest = pendingReplace.current; pendingReplace.current = undefined;
      if (latest) window.history.replaceState({ rlvizWorkspace: latest }, "", workspaceURL(latest));
    });
  }, []);
  const applyWorkspace = useCallback((next: WorkspaceState, snapshot = true) => {
    const normalized = normalizeWorkspace(next); if (!normalized) return;
    if (JSON.stringify(normalized) === JSON.stringify(workspaceRef.current)) return;
    workspaceRef.current = normalized; setWorkspace(normalized); setBreadcrumb(snapshotLabel(normalized)); writeURL(normalized, snapshot && !restoring.current);
    if (snapshot && !restoring.current) {
      const serialized = JSON.stringify(normalized), current = JSON.stringify(jumpList.current[jumpIndex.current]);
      if (serialized !== current) { jumpList.current = [...jumpList.current.slice(0, jumpIndex.current + 1), normalized]; jumpIndex.current = jumpList.current.length - 1; }
    }
  }, [writeURL]);
  const change = useCallback((update: (current: WorkspaceState) => WorkspaceState, snapshot = true) => applyWorkspace(update(workspaceRef.current), snapshot), [applyWorkspace]);

  const rememberLaneData = useCallback((id: string) => {
    laneDataLRU.current = [...laneDataLRU.current.filter((item) => item !== id), id];
  }, []);
  const putLaneData = useCallback((id: string, data: LaneData) => {
    rememberLaneData(id);
    setLaneData((current) => { const next = new Map(current).set(id, data); laneDataRef.current = next; return next; });
  }, [rememberLaneData]);
  const deleteLaneData = useCallback((id: string) => {
    laneDataLRU.current = laneDataLRU.current.filter((item) => item !== id);
    setLaneData((current) => { if (!current.has(id)) return current; const next = new Map(current); next.delete(id); laneDataRef.current = next; return next; });
  }, []);
  const pruneOffLaneData = useCallback(() => {
    const active = new Set(workspaceRef.current.lanes.map((lane) => lane.id));
    const offLane = laneDataLRU.current.filter((id) => laneDataRef.current.has(id) && !active.has(id));
    const evict = new Set(offLane.slice(0, Math.max(0, offLane.length - 8)));
    if (!evict.size) return;
    laneDataLRU.current = laneDataLRU.current.filter((id) => !evict.has(id));
    setLaneData((current) => { const next = new Map(current); evict.forEach((id) => next.delete(id)); laneDataRef.current = next; return next; });
  }, []);

  const ensureLaneData = useCallback(async (lane: WorkspaceLane) => {
    if (laneDataRef.current.has(lane.id)) return;
    const revision = openRevision.current;
    try {
      const loaded = lane.sourceId === "sample" ? { trajectory: initialTrajectory ?? sampleTrajectory, presentation: undefined } : await provider.loadTrajectory(lane.sourceId, lane.trajectoryId);
      if (revision !== openRevision.current || !workspaceRef.current.lanes.some((item) => item.id === lane.id)) return;
      const data: LaneData = { trajectory: loaded.trajectory, analysis: null, presentation: loaded.presentation };
      if (lane.id === workspaceRef.current.active) setPresentation(loaded.presentation);
      putLaneData(lane.id, data);
      change((current) => ({ ...current, lanes: current.lanes.map((item) => item.id === lane.id && item.axis.end <= item.axis.start + 1 ? { ...item, selected: firstAnomaly(loaded.trajectory), axis: { start: loaded.trajectory.events[0]?.sequence ?? 0, end: loaded.trajectory.events.at(-1)?.sequence ?? 1 } } : item) }), false);
      if (lane.sourceId !== "sample") provider.loadAnalysis(lane.sourceId, lane.trajectoryId).then((analysis) => setLaneData((current) => { const existing = current.get(lane.id); if (!existing) return current; const next = new Map(current).set(lane.id, { ...existing, analysis }); laneDataRef.current = next; return next; })).catch(() => undefined);
    } catch (reason) { setError(reason instanceof Error ? reason.message : "Could not load trajectory"); }
  }, [change, initialTrajectory, provider, putLaneData]);

  useEffect(() => {
    const controller = new AbortController();
    if (initialTrajectory) { setBrowse(fakeBrowse(initialTrajectory)); workspace.lanes.forEach((lane) => void ensureLaneData(lane)); return () => controller.abort(); }
    if (workspaceRef.current.lanes.length) {
      provider.loadBrowse(controller.signal).then((collection) => { setBrowse(collection); workspaceRef.current.lanes.forEach((lane) => void ensureLaneData(lane)); }).catch((reason) => { if (!controller.signal.aborted && !(reason instanceof Error && reason.name === "AbortError")) setError(reason instanceof Error ? reason.message : "Could not load viewer"); });
      return () => controller.abort();
    }
    Promise.all([provider.loadInitial(controller.signal), provider.loadBrowse(controller.signal)]).then(([loaded, collection]) => {
      setBrowse(collection); setPresentation(loaded.presentation);
      const sourceId = collection.trajectories.find((row) => row.trajectory.id === loaded.trajectory.id)?.source_id;
      if (sourceId) putLaneData(laneId(sourceId, loaded.trajectory.id), { trajectory: loaded.trajectory, analysis: null, presentation: loaded.presentation });
      if (sourceId && legacyReadIntent.current && !workspaceRef.current.lanes.length) {
        const id = laneId(sourceId, loaded.trajectory.id);
        applyWorkspace({ ...workspaceRef.current, railExpanded: false, active: id, lanes: [{ id, sourceId, trajectoryId: loaded.trajectory.id, band: "focus", selected: firstAnomaly(loaded.trajectory), depth: 1, fidelity: 3, axis: { start: loaded.trajectory.events[0]?.sequence ?? 0, end: loaded.trajectory.events.at(-1)?.sequence ?? 1 }, descentStack: [] }] }, false);
      }
      workspaceRef.current.lanes.forEach((lane) => void ensureLaneData(lane));
    }).catch((reason) => { if (!controller.signal.aborted && !(reason instanceof Error && reason.name === "AbortError")) setError(reason instanceof Error ? reason.message : "Could not load viewer"); });
    return () => controller.abort();
  }, [applyWorkspace, ensureLaneData, initialTrajectory, provider, putLaneData]);

  useEffect(() => applyPresentationTheme(presentation), [presentation]);
  useEffect(() => { if (activeLane && laneData.has(activeLane.id)) setPresentation(laneData.get(activeLane.id)?.presentation); }, [activeLane, laneData]);
  useEffect(() => { document.documentElement.setAttribute("data-theme", theme); }, [theme]);
  useEffect(() => { writeURL(workspaceRef.current, false); }, [writeURL]);
  useEffect(() => {
    const onPop = (event: PopStateEvent) => {
      const next = normalizeWorkspace((event.state as { rlvizWorkspace?: unknown } | null)?.rlvizWorkspace) ?? workspaceFromSearch(location.search) ?? legacyWorkspace(location.search);
      if (!next) return;
      const serialized = JSON.stringify(next); let found = -1; for (let index = jumpList.current.length - 1; index >= 0; index--) { if (JSON.stringify(jumpList.current[index]) === serialized) { found = index; break; } } if (found >= 0) jumpIndex.current = found;
      restoring.current = true; workspaceRef.current = next; setWorkspace(next); setBreadcrumb(snapshotLabel(next)); openRevision.current++; next.lanes.forEach((lane) => void ensureLaneData(lane)); restoring.current = false;
    };
    window.addEventListener("popstate", onPop); return () => window.removeEventListener("popstate", onPop);
  }, [ensureLaneData]);
  useEffect(() => {
    const lane = workspace.lanes.find((item) => item.id === workspace.active); if (lane?.band === "focus") lastFocus.current = lane.id;
    const target = workspace.active === "rail" ? railRef.current : document.querySelector<HTMLElement>(`[data-lane-id="${CSS.escape(workspace.active)}"]`);
    if (target && document.activeElement !== target && !(document.activeElement instanceof HTMLInputElement)) target.focus({ preventScroll: true });
  }, [workspace.active, workspace.lanes.length]);

  const loadRowIntoLane = useCallback(async (row: BrowseTrajectory, add: boolean, preserve?: WorkspaceLane) => {
    const id = rowKey(row); const existing = workspaceRef.current.lanes.find((lane) => lane.id === id);
    if (existing) { change((current) => ({ ...current, active: existing.id })); return; }
    const loaded = row.source_id === "sample" ? { trajectory: initialTrajectory ?? sampleTrajectory, presentation: undefined } : await provider.loadTrajectory(row.source_id, row.trajectory.id);
    const focus = workspaceRef.current.lanes.filter((lane) => lane.band === "focus");
    const band = add && focus.length >= 2 ? "context" : "focus";
    const base: WorkspaceLane = { id, sourceId: row.source_id, trajectoryId: row.trajectory.id, band, selected: preserve?.selected ?? firstAnomaly(loaded.trajectory), depth: preserve?.depth ?? 1, fidelity: preserve?.fidelity ?? 3, axis: preserve?.axis ?? { start: loaded.trajectory.events[0]?.sequence ?? 0, end: loaded.trajectory.events.at(-1)?.sequence ?? 1 }, descentStack: preserve?.descentStack ?? [] };
    putLaneData(id, { trajectory: loaded.trajectory, analysis: null, presentation: loaded.presentation });
    change((current) => {
      if (add || !current.lanes.length) return { ...current, lanes: [...current.lanes, base], active: id, railExpanded: current.railExpanded };
      if (preserve) return { ...current, lanes: current.lanes.map((lane) => lane.id === preserve.id ? { ...base, band: preserve.band } : lane), active: id, reference: current.reference === preserve.id ? undefined : current.reference };
      const replaceId = current.lanes.find((lane) => lane.id === current.active && lane.band === "focus")?.id ?? current.lanes.find((lane) => lane.id === lastFocus.current && lane.band === "focus")?.id ?? current.lanes.find((lane) => lane.band === "focus")?.id;
      if (!replaceId) return { ...current, lanes: [...current.lanes, base], active: id };
      return { ...current, lanes: current.lanes.map((lane) => lane.id === replaceId ? { ...base, band: "focus" } : lane), active: id, reference: current.reference === replaceId ? undefined : current.reference };
    });
    pruneOffLaneData();
    if (row.source_id !== "sample") provider.loadAnalysis(row.source_id, row.trajectory.id).then((analysis) => setLaneData((current) => { const data = current.get(id); if (!data) return current; const next = new Map(current).set(id, { ...data, analysis }); laneDataRef.current = next; return next; })).catch(() => undefined);
  }, [change, initialTrajectory, provider, pruneOffLaneData, putLaneData]);

  const openSelected = (add: boolean) => { if (selectedRow) void loadRowIntoLane(selectedRow, add).catch((reason) => setError(reason instanceof Error ? reason.message : "Could not load trajectory")); };
  const updateLane = useCallback((id: string, update: (lane: WorkspaceLane, data?: LaneData) => WorkspaceLane, snapshot = true) => change((current) => ({ ...current, lanes: current.lanes.map((lane) => lane.id === id ? update(lane, laneDataRef.current.get(id)) : lane) }), snapshot), [change]);
  const selectEvent = useCallback((id: string, index: number) => updateLane(id, (lane, data) => {
    if (!data) return { ...lane, selected: index };
    const min = data.trajectory.events[0]?.sequence ?? 0, max = data.trajectory.events.at(-1)?.sequence ?? 1, sequence = data.trajectory.events[index]?.sequence ?? min;
    return { ...lane, selected: index, axis: panWindowToInclude(lane.axis, sequence, min, max) };
  }, false), [updateLane]);
  const descendLane = useCallback((id: string, target?: Episode) => updateLane(id, (lane, data) => {
    if (lane.band === "context" || !data) return lane;
    const depth = effectiveDepth(lane);
    if (depth >= 4) return lane;
    const events = data.trajectory.events, episodes = episodesFor(events);
    const current = target ?? episodes[episodeIndexForEvent(episodes, lane.selected)];
    const selected = current && (lane.selected < current.startIndex || lane.selected > current.endIndex) ? current.startIndex : lane.selected;
    const sequence = events[selected]?.sequence ?? events[0]?.sequence ?? 0;
    const min = events[0]?.sequence ?? 0, max = events.at(-1)?.sequence ?? 1;
    const panned = panWindowToInclude(lane.axis, sequence, min, max);
    const axis = depth === 2 && current ? episodeWindow(panned, current, sequence) : lane.axis;
    return { ...lane, selected, depth: depth + 1, axis, descentStack: [...lane.descentStack, { depth, axis: lane.axis }] };
  }), [updateLane]);
  const ascendLane = useCallback((id: string) => updateLane(id, (lane) => {
    const depth = effectiveDepth(lane);
    if (depth <= 1) return lane;
    const snapshot = lane.descentStack.at(-1);
    return { ...lane, depth: depth - 1, axis: snapshot?.axis ?? lane.axis, descentStack: lane.descentStack.slice(0, -1) };
  }), [updateLane]);
  const moveEvent = (delta: number) => {
    if (!activeLane) return;
    const data = laneData.get(activeLane.id); if (!data) return;
    const events = data.trajectory.events, episodes = episodesFor(events), episodeIndex = episodeIndexForEvent(episodes, activeLane.selected);
    const depth = effectiveDepth(activeLane);
    if (depth === 2) {
      const target = episodes[Math.max(0, Math.min(episodes.length - 1, episodeIndex + delta))];
      if (target) selectEvent(activeLane.id, target.startIndex);
      return;
    }
    if (depth >= 3) {
      const episode = episodes[episodeIndex]; if (!episode) return;
      selectEvent(activeLane.id, Math.max(episode.startIndex, Math.min(episode.endIndex, activeLane.selected + delta)));
      return;
    }
    selectEvent(activeLane.id, Math.max(0, Math.min(events.length - 1, activeLane.selected + delta)));
  };
  const jumpEvent = (predicate: (event: TrajectoryEvent) => boolean) => { if (!activeLane) return; const events = laneData.get(activeLane.id)?.trajectory.events; if (!events) return; const next = events.findIndex((event, index) => index > activeLane.selected && predicate(event)), wrapped = events.findIndex(predicate); if (next >= 0 || wrapped >= 0) selectEvent(activeLane.id, next >= 0 ? next : wrapped); };
  const cycleZone = (delta: number) => { const zones = [...(workspaceRef.current.railExpanded ? ["rail"] : []), ...workspaceRef.current.lanes.map((lane) => lane.id)]; if (!zones.length) return; const index = zones.indexOf(workspaceRef.current.active); change((current) => ({ ...current, active: zones[((index < 0 ? 0 : index) + delta + zones.length) % zones.length] })); };
  const sweep = (delta: number) => { if (!activeLane || !filtered.length) return; const occupied = new Set(workspaceRef.current.lanes.filter((lane) => lane.id !== activeLane.id).map((lane) => lane.id)); const candidates = filtered.filter((row) => !occupied.has(rowKey(row))); if (!candidates.length) return; const index = candidates.findIndex((row) => rowKey(row) === activeLane.id); const row = candidates[((index < 0 ? 0 : index) + delta + candidates.length) % candidates.length]; change((current) => ({ ...current, railSelected: filtered.indexOf(row) }), false); void loadRowIntoLane(row, false, activeLane); };
  const closeLane = () => { if (!activeLane) return; deleteLaneData(activeLane.id); change((current) => { const lanes = current.lanes.filter((lane) => lane.id !== activeLane.id); return { ...current, lanes, railExpanded: lanes.length ? current.railExpanded : true, active: lanes[0]?.id ?? "rail", reference: current.reference === activeLane.id ? undefined : current.reference }; }); };
  const promoteDemote = () => { if (!activeLane) return; change((current) => { const lane = current.lanes.find((item) => item.id === activeLane.id); if (!lane) return current; const counterpart = lane.band === "context" ? current.lanes.find((item) => item.id === lastFocus.current && item.band === "focus") ?? current.lanes.find((item) => item.band === "focus") : current.lanes.find((item) => item.band === "context"); if (!counterpart) return current; return { ...current, lanes: current.lanes.map((item) => item.id === lane.id ? { ...item, band: counterpart.band } : item.id === counterpart.id ? { ...item, band: lane.band } : item) }; }); };
  const jump = (delta: number) => { const nextIndex = jumpIndex.current + delta; if (nextIndex < 0 || nextIndex >= jumpList.current.length) return; jumpIndex.current = nextIndex; restoring.current = true; const next = jumpList.current[nextIndex]; applyWorkspace(next, false); next.lanes.forEach((lane) => void ensureLaneData(lane)); restoring.current = false; };
  const adjustFidelity = (delta: number, all: boolean) => { if (workspaceRef.current.active === "rail" && !all) { setRailFidelity((value) => Math.max(0, Math.min(5, value + delta))); return; } change((current) => ({ ...current, lanes: current.lanes.map((lane) => all || lane.id === current.active ? { ...lane, fidelity: Math.max(0, Math.min(5, lane.fidelity + delta)) } : lane) }), false); };
  const adjustZoom = (factor: number | "fit", all: boolean) => change((current) => ({ ...current, lanes: current.lanes.map((lane) => { if (!all && lane.id !== current.active) return lane; const data = laneDataRef.current.get(lane.id); if (!data) return lane; const min = data.trajectory.events[0]?.sequence ?? 0, max = data.trajectory.events.at(-1)?.sequence ?? 1, sequence = data.trajectory.events[lane.selected]?.sequence ?? min; return { ...lane, axis: factor === "fit" ? { start: min, end: max } : zoomWindow(lane.axis, sequence, factor, min, max) }; }) }), false);

  const resizeNearest = (key: string) => {
    const active = workspaceRef.current.active === "rail" ? "rail" : workspaceRef.current.lanes.find((lane) => lane.id === workspaceRef.current.active)?.band === "context" ? "focusContext" : workspaceRef.current.lanes.filter((lane) => lane.band === "focus").length > 1 ? "focusLane" : "console";
    const positive = key === "ArrowRight" || key === "ArrowDown"; const delta = positive ? 0.02 : -0.02;
    change((current) => ({ ...current, seams: normalizeWorkspace({ ...current, seams: { ...current.seams, [active]: current.seams[active] + (active === "console" ? -delta : delta) } })!.seams }));
  };

  useCommands("workspace", {
    [commandIds.workspace.toggleRail]: () => change((current) => { const railExpanded = !current.railExpanded; return { ...current, railExpanded, active: !railExpanded && current.active === "rail" && current.lanes.length ? current.lanes[0].id : current.active }; }),
    [commandIds.workspace.addLane]: () => workspaceRef.current.active === "rail" ? openSelected(true) : false,
    [commandIds.workspace.closeLane]: () => activeLane ? closeLane() : false,
    [commandIds.workspace.cycleNext]: () => cycleZone(1), [commandIds.workspace.cyclePrevious]: () => cycleZone(-1),
    [commandIds.workspace.nextRollout]: () => activeLane ? sweep(1) : false, [commandIds.workspace.previousRollout]: () => activeLane ? sweep(-1) : false,
    [commandIds.workspace.promoteDemote]: () => activeLane ? promoteDemote() : false,
    [commandIds.workspace.pinReference]: () => activeLane ? change((current) => ({ ...current, reference: current.reference === activeLane.id ? undefined : activeLane.id })) : false,
    [commandIds.workspace.directionRows]: () => change((current) => ({ ...current, direction: "rows" })), [commandIds.workspace.directionColumns]: () => change((current) => ({ ...current, direction: "columns" })),
    [commandIds.workspace.descend]: () => { if (!activeLane) { openSelected(false); return; } if (activeLane.band === "context") return false; descendLane(activeLane.id); },
    // Esc is structural (ascend, then close the lane, keeping current rail
    // state); history rewind is exclusively Ctrl+o, so backing out of a lane
    // never restores a stale rail selection.
    [commandIds.workspace.ascend]: () => { if (resizeMode) { setResizeMode(false); return; } if (!activeLane) return false; if (effectiveDepth(activeLane) > 1) ascendLane(activeLane.id); else closeLane(); },
    [commandIds.workspace.jumpBack]: () => jump(-1), [commandIds.workspace.jumpForward]: () => jump(1), [commandIds.workspace.resizeMode]: () => setResizeMode(true),
    [commandIds.view.fidelityUp]: () => adjustFidelity(1, false), [commandIds.view.fidelityDown]: () => adjustFidelity(-1, false),
    [commandIds.view.fidelityUpAll]: () => adjustFidelity(1, true), [commandIds.view.fidelityDownAll]: () => adjustFidelity(-1, true),
    [commandIds.view.zoomIn]: () => activeLane ? adjustZoom(2, false) : false, [commandIds.view.zoomOut]: () => activeLane ? adjustZoom(0.5, false) : false, [commandIds.view.zoomFit]: () => activeLane ? adjustZoom("fit", false) : false,
    [commandIds.view.zoomInAll]: () => activeLane ? adjustZoom(2, true) : false, [commandIds.view.zoomOutAll]: () => activeLane ? adjustZoom(0.5, true) : false, [commandIds.view.zoomFitAll]: () => activeLane ? adjustZoom("fit", true) : false,
    [commandIds.view.toggleHelp]: () => setHelp(true),
  }, !help);
  useCommands("trajectory", {
    [commandIds.trajectory.next]: () => activeLane ? moveEvent(1) : workspaceRef.current.active === "rail" ? change((current) => ({ ...current, railSelected: Math.min(filtered.length - 1, current.railSelected + 1) }), false) : false,
    [commandIds.trajectory.previous]: () => activeLane ? moveEvent(-1) : workspaceRef.current.active === "rail" ? change((current) => ({ ...current, railSelected: Math.max(0, current.railSelected - 1) }), false) : false,
    [commandIds.trajectory.nextError]: () => jumpEvent((event) => event.kind === "error"), [commandIds.trajectory.nextContext]: () => jumpEvent((event) => !!event.context || !!event.alignment_key?.startsWith("context:")),
    [commandIds.trajectory.nextReward]: () => jumpEvent((event) => event.kind === "reward" || event.kind === "grader"), [commandIds.trajectory.nextFinding]: () => { if (!activeLane) return false; const ids = new Set((laneData.get(activeLane.id)?.analysis?.analysis.findings ?? []).flatMap((finding) => finding.event_ids ?? [])); jumpEvent((event) => ids.has(event.id)); },
  }, !help);
  useCommands("group", {
    [commandIds.group.tagVerdict1]: () => { if (!selectedRow) return false; setTags((current) => new Map(current).set(selectedRow.trajectory.id, 1)); change((current) => ({ ...current, railSelected: Math.min(filtered.length - 1, current.railSelected + 1) })); },
    [commandIds.group.tagVerdict2]: () => { if (!selectedRow) return false; setTags((current) => new Map(current).set(selectedRow.trajectory.id, 2)); change((current) => ({ ...current, railSelected: Math.min(filtered.length - 1, current.railSelected + 1) })); },
    [commandIds.group.tagVerdict3]: () => { if (!selectedRow) return false; setTags((current) => new Map(current).set(selectedRow.trajectory.id, 3)); change((current) => ({ ...current, railSelected: Math.min(filtered.length - 1, current.railSelected + 1) })); },
    [commandIds.group.tagVerdict4]: () => { if (!selectedRow) return false; setTags((current) => new Map(current).set(selectedRow.trajectory.id, 4)); change((current) => ({ ...current, railSelected: Math.min(filtered.length - 1, current.railSelected + 1) })); },
  }, !help && workspace.active === "rail");

  useEffect(() => {
    if (!resizeMode) return;
    const onKey = (event: KeyboardEvent) => {
      if (help || event.target instanceof HTMLInputElement || event.target instanceof HTMLTextAreaElement) return;
      if (["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"].includes(event.key)) { event.preventDefault(); resizeNearest(event.key); }
    };
    window.addEventListener("keydown", onKey); return () => window.removeEventListener("keydown", onKey);
  }, [help, resizeMode]);

  const beginResize = (event: ReactPointerEvent<HTMLDivElement>, name: SeamName) => {
    event.preventDefault(); const rack = rackRef.current?.getBoundingClientRect(), stage = stageRef.current?.getBoundingClientRect(), focus = focusRef.current?.getBoundingClientRect(); if (!rack || !stage || !focus) return;
    const move = (pointer: PointerEvent) => {
      let value = workspaceRef.current.seams[name];
      if (name === "rail") value = (pointer.clientX - rack.left) / rack.width;
      if (name === "focusContext") value = (pointer.clientY - stage.top) / stage.height;
      if (name === "console") value = (rack.bottom - pointer.clientY) / rack.height;
      if (name === "focusLane") value = workspaceRef.current.direction === "rows" ? (pointer.clientY - focus.top) / focus.height : (pointer.clientX - focus.left) / focus.width;
      change((current) => ({ ...current, seams: normalizeWorkspace({ ...current, seams: { ...current.seams, [name]: value } })!.seams }), false);
    };
    const up = () => { window.removeEventListener("pointermove", move); window.removeEventListener("pointerup", up); applyWorkspace(workspaceRef.current, true); };
    window.addEventListener("pointermove", move); window.addEventListener("pointerup", up, { once: true });
  };
  const resetSeam = (name: SeamName) => change((current) => ({ ...current, seams: { ...current.seams, [name]: defaultSeams[name] } }));

  const focus = workspace.lanes.filter((lane) => lane.band === "focus"), context = workspace.lanes.filter((lane) => lane.band === "context");
  const rackStyle = { "--rail-width": `${(workspace.railExpanded ? workspace.seams.rail : 0) * 100}vw`, "--focus-height": `${workspace.seams.focusContext * 100}%`, "--console-height": `${workspace.seams.console * 100}vh` } as CSSProperties;
  return <ViewerProviderContext.Provider value={provider}><div ref={rackRef} className={`instrument-shell workspace-rack rail-${workspace.railExpanded ? "open" : "closed"}`} data-filter={workspace.railQuery} data-direction={workspace.direction} data-active-zone={workspace.active} style={rackStyle}>
    <button className="theme-toggle" aria-label={`Switch to ${theme === "light" ? "dark" : "light"} theme`} onClick={() => setTheme((current) => current === "light" ? "dark" : "light")}>{theme}</button>
    {error && <div className="instrument-error" role="alert">{error}</div>}{presentation?.notices?.map((notice) => <div className="presentation-notice" role="status" key={notice}>{notice}</div>)}
    <div className="rack-body">
      {workspace.railExpanded && <Rail root={railRef} rows={filtered} workspace={{ ...workspace, railSelected: boundedRail }} fidelity={railFidelity} tags={tags} onActivate={() => change((current) => ({ ...current, active: "rail" }))} onSelect={(index) => change((current) => ({ ...current, railSelected: index, active: "rail" }))} onOpen={() => openSelected(false)} onAdd={() => openSelected(true)} onQuery={(railQuery) => change((current) => { const next = ordered.filter((row) => !railQuery || `${row.trajectory.id} ${row.source_name} ${row.case_name ?? ""} ${row.group_name ?? ""}`.toLowerCase().includes(railQuery.toLowerCase())); const kept = selectedRow ? next.findIndex((row) => rowKey(row) === rowKey(selectedRow)) : -1; return { ...current, railQuery, railSelected: kept >= 0 ? kept : 0 }; })} onTag={(tag) => { if (!selectedRow) return; setTags((current) => new Map(current).set(selectedRow.trajectory.id, tag)); }} />}
      <Sash name="rail" orientation="vertical" onPointerDown={beginResize} onReset={resetSeam} />
      <section ref={stageRef} className="workspace-stage" aria-label="Trajectory stage">
        <div ref={focusRef} className={`focus-band direction-${workspace.direction}`} aria-label="Focus band">
          {focus.map((lane, index) => <div className="focus-slot" key={lane.id} style={{ flexBasis: focus.length > 1 ? `calc(${(index === 0 ? workspace.seams.focusLane : 1 - workspace.seams.focusLane) * 100}% - 2.5px)` : "100%" }}><LaneTrack lane={lane} data={laneData.get(lane.id)} active={workspace.active === lane.id} reference={workspace.reference === lane.id} hover={hover[lane.id]} onActivate={() => change((current) => ({ ...current, active: lane.id }))} onSelect={(value) => selectEvent(lane.id, value)} onHover={(value) => setHover((current) => ({ ...current, [lane.id]: value }))} onDescend={(episode) => descendLane(lane.id, episode)} onAscend={() => ascendLane(lane.id)} /></div>)}
          {focus.length > 1 && <Sash name="focusLane" orientation={workspace.direction === "rows" ? "horizontal" : "vertical"} onPointerDown={beginResize} onReset={resetSeam} />}
          {!focus.length && <div className="empty-stage"><span>Stage</span><b>Open a rollout from the rail.</b><small>Enter replaces · A adds · t toggles the rail</small></div>}
        </div>
        <Sash name="focusContext" orientation="horizontal" onPointerDown={beginResize} onReset={resetSeam} />
        <ContextBand lanes={context} workspace={workspace} laneData={laneData} hover={hover} activate={(id) => change((current) => ({ ...current, active: id }))} select={selectEvent} setLaneHover={(id, value) => setHover((current) => ({ ...current, [id]: value }))} />
      </section>
    </div>
    <Sash name="console" orientation="horizontal" onPointerDown={beginResize} onReset={resetSeam} />
    <Console workspace={workspace} lane={activeLane} data={activeLane ? laneData.get(activeLane.id) : undefined} breadcrumb={breadcrumb} resizeMode={resizeMode} onSelect={(index) => activeLane && selectEvent(activeLane.id, index)} onHelp={() => setHelp(true)} />
    {help && <HelpOverlay onClose={() => setHelp(false)} />}
  </div></ViewerProviderContext.Provider>;
}
