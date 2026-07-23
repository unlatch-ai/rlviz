import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, ReactNode, RefObject } from "react";
import { DockviewReact } from "dockview-react";
import type { IDockviewPanelHeaderProps, IDockviewPanelProps } from "dockview-react";
import { daemonProvider, ViewerProviderContext } from "./provider";
import type { ViewerProvider } from "./provider";
import { commandDefinition, commandIds, dispatchCommand, firstBindingLabel, useCommands, useKeymapRevision } from "./commands";
import type { CommandId } from "./commands";
import { axisX, episodeIndexForEvent, episodesFor, episodeWindow, firstAnomaly, glyphForKind, layoutStrip, panWindowToInclude, stripX, verdictGlyph, zoomWindow } from "./instrument";
import type { Episode, StripMark } from "./instrument";
import type { BrowseResponse, BrowseTrajectory, Trajectory, TrajectoryEvent } from "./types";
import { preview, title } from "./format";
import { sampleTrajectory } from "./sample";
import { applyPresentationTheme } from "./presentation";
import type { PresentationConfig } from "./types";
import { effectiveDepth, laneId, workspaceStorageKey } from "./workspace";
import type { WorkspaceLane, WorkspaceState } from "./workspace";
import { useWorkspaceController } from "./workspaceController";
import { useLaneDataLoader } from "./laneLoader";
import type { LaneData } from "./laneLoader";
import { lanePanelId, pinnedDetailLaneId, pinnedDetailTarget } from "./workspaceDock";
import { collectionMetadataKey, useViewerMetadata } from "./viewerMetadata";
import type { EditableMetadata } from "./viewerMetadata";
import { useWorkspaceDock } from "./useWorkspaceDock";
import { Guide } from "./Guide";
import { Settings } from "./Settings";
import type { ViewerSetup } from "./Settings";

const fidelityNames = ["hairline", "glyphs", "detail"];

function metric(row: BrowseTrajectory, name: string): unknown {
  const metrics = row.metrics.metrics ?? row.metrics.normalized_metrics ?? row.metrics;
  return metrics[name] ?? row.metrics[name];
}

function rowKey(row: BrowseTrajectory): string { return laneId(row.source_id, row.trajectory.id); }
function rowSearchText(row: BrowseTrajectory, metadata?: EditableMetadata): string {
  return `${metadata?.title ?? ""} ${metadata?.description ?? ""} ${row.trajectory.id} ${row.source_name} ${row.case_name ?? ""} ${row.group_name ?? ""}`.toLowerCase();
}
function eventDetail(event: TrajectoryEvent): unknown { return event.output ?? event.input ?? event.content ?? event.data ?? event.raw ?? event; }
function eventText(event: TrajectoryEvent): string { return title(event) || `${event.kind} event`; }
function eventToolNames(event: TrajectoryEvent): string[] {
  const names = new Set<string>();
  const inspect = (value: unknown) => {
    if (!value || typeof value !== "object") return;
    const record = value as Record<string, unknown>;
    if (typeof record.tool_name === "string") names.add(record.tool_name);
    if (event.kind === "tool" && typeof record.name === "string") names.add(record.name);
    if (record.function && typeof record.function === "object" && typeof (record.function as Record<string, unknown>).name === "string") names.add((record.function as Record<string, unknown>).name as string);
    if (Array.isArray(record.tool_calls)) record.tool_calls.forEach(inspect);
  };
  [event.input, event.output, event.content, event.data].forEach(inspect);
  return [...names];
}
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

function EditableHeader({ kind, fallbackTitle, metadata, context, onSave }: {
  kind: "collection" | "trajectory";
  fallbackTitle: string;
  metadata?: EditableMetadata;
  context?: string;
  onSave: (value: EditableMetadata) => void;
}) {
  const [editing, setEditing] = useState(false);
  const [titleDraft, setTitleDraft] = useState("");
  const [descriptionDraft, setDescriptionDraft] = useState("");
  const Heading = kind === "collection" ? "h1" : "h2";
  const begin = () => {
    setTitleDraft(metadata?.title ?? "");
    setDescriptionDraft(metadata?.description ?? "");
    setEditing(true);
  };
  if (editing) return <form className={`metadata-editor ${kind}`} onClick={(event) => event.stopPropagation()} onSubmit={(event) => {
    event.preventDefault();
    onSave({ title: titleDraft, description: descriptionDraft });
    setEditing(false);
  }} onKeyDown={(event) => { if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); setEditing(false); } }}>
    <label>Title<input aria-label={`${kind} title`} autoFocus maxLength={120} value={titleDraft} placeholder={fallbackTitle} onChange={(event) => setTitleDraft(event.target.value)} /></label>
    <label>Description<textarea aria-label={`${kind} description`} maxLength={500} value={descriptionDraft} placeholder="Add a concise description" onChange={(event) => setDescriptionDraft(event.target.value)} /></label>
    <span><button type="submit">save</button><button type="button" onClick={() => setEditing(false)}>cancel</button></span>
  </form>;
  return <div className={`editable-metadata ${kind}`}>
    <div><Heading>{metadata?.title ?? fallbackTitle}</Heading>{metadata?.description && <p>{metadata.description}</p>}{context && <small className={kind === "trajectory" ? "workspace-breadcrumb" : undefined}>{context}</small>}</div>
    <button type="button" className="metadata-edit" aria-label={`Edit ${kind} title and description`} onClick={(event) => { event.stopPropagation(); begin(); }}>edit</button>
  </div>;
}

function CollectionStrip({ row, fidelity }: { row: BrowseTrajectory; fidelity: number }) {
  const count = Math.max(1, Number(metric(row, "event_count") ?? 1));
  const errors = Number(metric(row, "error_count") ?? 0);
  const width = Math.min(100, 18 + Math.log2(count + 1) * 13);
  const shape = row.shape;
  if (!shape) {
    // No shape summary from this provider yet: render only what is true —
    // length and counts. Never synthesize texture or positions.
    return <span className="cat-line" style={{ width: `${width}%` }}>{errors > 0 && <em className="strip-badge" title={`${errors} errors`}>{errors}✕</em>}</span>;
  }
  if (fidelity === 0) {
    return <span className="cat-line" style={{ width: `${width}%` }}>
      {shape.slots.map((slot, index) => slot.landmark ? <i key={index} className={`strip-landmark ${slot.landmark}`} style={{ left: `${((index + 0.5) / shape.slots.length) * 100}%` }} /> : null)}
    </span>;
  }
  const glyphSlots = shape.slots.map((slot, index) => {
    if (slot.landmark === "error") return <b key={index} className="g-error">✕</b>;
    if (slot.landmark === "context") return <b key={index} className="g-context">◇</b>;
    if (slot.landmark === "evidence") return <b key={index} className="g-evidence">◆</b>;
    if (!slot.count) return <span key={index} className="g-gap"> </span>;
    return <span key={index}>{slot.tools * 2 > slot.count ? "▮" : "·"}</span>;
  });
  return <span className="cat-glyphs" style={{ width: `${width}%` }}>{glyphSlots}{fidelity >= 2 && <small>{shape.events} events{errors ? ` · ${errors} errors` : ""}</small>}</span>;
}

function Rail({ root, rows, workspace, fidelity, metadata, trajectoryMetadata, onMetadata, onActivate, onSelect, onOpen, onAdd, onQuery, onCollectionView }: {
  root: RefObject<HTMLElement | null>; rows: BrowseTrajectory[]; workspace: WorkspaceState; fidelity: number;
  metadata?: EditableMetadata; trajectoryMetadata: Record<string, EditableMetadata>; onMetadata: (value: EditableMetadata) => void;
  onActivate: () => void; onSelect: (index: number) => void; onOpen: () => void; onAdd: () => void; onQuery: (query: string) => void; onCollectionView: (view: WorkspaceState["collectionView"]) => void;
}) {
  const selected = Math.min(workspace.railSelected, Math.max(0, rows.length - 1));
  const listRef = useRef<HTMLElement>(null);
  useEffect(() => {
    const list = listRef.current;
    const option = list?.querySelector<HTMLElement>(`[role="option"][data-index="${selected}"]`);
    if (!list || !option) return;
    // Dockview's clipped panel boundary is not consistently treated as a
    // native scroll ancestor, so adjust this list directly.
    const listBounds = list.getBoundingClientRect(), optionBounds = option.getBoundingClientRect();
    if (optionBounds.top < listBounds.top) list.scrollTop -= listBounds.top - optionBounds.top;
    else if (optionBounds.bottom > listBounds.bottom) list.scrollTop += optionBounds.bottom - listBounds.bottom;
  }, [selected]);
  const indexedRows = rows.map((row, index) => ({ row, index }));
  const trials = [...indexedRows.reduce((groups, item) => {
    const label = item.row.case_name ?? item.row.group_name ?? item.row.source_name ?? "Ungrouped";
    groups.set(label, [...(groups.get(label) ?? []), item]);
    return groups;
  }, new Map<string, typeof indexedRows>())];
  const renderRow = ({ row, index }: typeof indexedRows[number]) => <button key={rowKey(row)} role="option" data-index={index} aria-selected={index === selected} data-fidelity-level={`L${fidelity}`} data-columns={fidelity >= 2 ? "true" : "false"} className={`browse-row ${index === selected ? "selected" : ""}`} onClick={() => onSelect(index)} onDoubleClick={onOpen}>
    {fidelity >= 1 && <span className="verdict">{verdictGlyph(row)}</span>}<span className="identity"><b>{trajectoryMetadata[rowKey(row)]?.title ?? row.trajectory.id}</b>{fidelity >= 2 && <small>{trajectoryMetadata[rowKey(row)]?.title ? `${row.trajectory.id}${trajectoryMetadata[rowKey(row)]?.description ? ` · ${trajectoryMetadata[rowKey(row)]?.description}` : ""}` : row.case_name ?? row.group_name ?? row.source_name}</small>}</span><CollectionStrip row={row} fidelity={fidelity} />
    {fidelity >= 2 && <><span className="numeric events-column">{String(metric(row, "event_count") ?? "—")} ev</span><span className="numeric reward-column">{metric(row, "reward") === undefined ? "" : `r ${String(metric(row, "reward"))}`}</span></>}
    {fidelity >= 2 && <span className="row-state">{row.source_name}{row.group_name ? ` · ${row.group_name}` : ""}</span>}
  </button>;
  return <main ref={root} tabIndex={0} className={`workspace-rail ${workspace.active === "rail" ? "active-zone" : ""}`} aria-label="Browse trajectories" data-filter={workspace.railQuery} data-fidelity={fidelityNames[fidelity]} data-collection-view={workspace.collectionView} onFocus={onActivate}>
    <header><EditableHeader kind="collection" fallbackTitle="Trajectories" metadata={metadata} context={rows.length === 1 ? "1 trajectory" : `${rows.length} trajectories`} onSave={onMetadata} /></header>
    <div className="rail-controls"><label>Filter <input id="browse-filter" value={workspace.railQuery} onChange={(event) => onQuery(event.target.value)} onKeyDown={(event) => { if (event.key === "Escape") { event.preventDefault(); root.current?.focus(); } }} /></label><div className="rail-view-toggle" role="group" aria-label="Collection view"><span>view</span><button className={workspace.collectionView === "rollouts" ? "active" : ""} aria-pressed={workspace.collectionView === "rollouts"} onClick={() => onCollectionView("rollouts")}>rollouts</button><button className={workspace.collectionView === "trials" ? "active" : ""} aria-pressed={workspace.collectionView === "trials"} onClick={() => onCollectionView("trials")}>trials</button></div></div>
    <div className="fidelity-readout">fidelity <b>{fidelityNames[fidelity]}</b> · [ ]</div>
    <section ref={listRef} className={`browse-list rail-fidelity-${fidelity}`} role="listbox" aria-label="Trajectory collection" data-fidelity-level={`L${fidelity}`} onWheel={(event) => {
      const node = event.currentTarget;
      if (node.scrollHeight <= node.clientHeight || !event.deltaY) return;
      event.preventDefault();
      node.scrollTop += event.deltaY;
    }}>
      {workspace.collectionView === "rollouts" ? indexedRows.map(renderRow) : trials.map(([label, items]) => <div key={label} className="rail-trial-group" role="group" aria-label={label}><header><b>{label}</b><small>{items.length} rollout{items.length === 1 ? "" : "s"}</small></header>{items.map(renderRow)}</div>)}
      {!rows.length && <p className="empty-state">No trajectories match this filter.</p>}
    </section>
    <span className="rail-actions"><button onClick={onAdd}>add lane</button></span>
  </main>;
}

function useMeasuredWidth(ref: RefObject<HTMLElement | null>, fallback = 800): number {
  const [width, setWidth] = useState(fallback);
  useEffect(() => {
    const node = ref.current;
    if (!node) return;
    const measure = () => setWidth((current) => {
      const next = Math.round(node.getBoundingClientRect().width);
      return next >= 120 ? next : current;
    });
    measure();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(measure);
    observer.observe(node);
    return () => observer.disconnect();
  }, [ref]);
  return width;
}

function ShapeStrip({ trajectory, selected, hover, axis, compact = false, fidelity = 1, label, onSelect, onHover, onAscend }: {
  trajectory: Trajectory; selected: number; hover?: number; axis: { start: number; end: number }; compact?: boolean; fidelity?: number; label?: string; onSelect: (index: number) => void; onHover: (index?: number) => void; onAscend?: () => void;
}) {
  // Pixel-space rendering (workspace-spec v3 §0.1): marks have fixed sizes at
  // true positions; the SVG viewBox tracks the measured width so nothing
  // stretches. Density is handled by layoutStrip's binning, never by scaling.
  const containerRef = useRef<HTMLElement>(null);
  const width = useMeasuredWidth(containerRef);
  const events = trajectory.events;
  const min = events[0]?.sequence ?? 0;
  const height = compact || fidelity === 0 ? 56 : 200;
  const x = (sequence: number) => stripX(sequence, axis, width);
  const layout = layoutStrip(events, axis, width);
  const visibleCount = layout.mode === "marks" ? layout.marks.length : layout.bins.reduce((total, bin) => total + bin.count, 0) + layout.landmarks.length;
  const bands = compact || fidelity === 0 ? [] : episodesFor(events);
  const baseline = compact || fidelity === 0 ? 44 : 105;
  const markFor = (mark: StripMark) => {
    if (mark.kind === "error") return <path data-event-index={mark.index} key={`m${mark.index}`} className="event-shape error" d={`M${mark.x - 5},${baseline} L${mark.x},${baseline - 15} L${mark.x + 5},${baseline} Z`} />;
    if (mark.kind === "context") return <path data-event-index={mark.index} key={`m${mark.index}`} className="event-shape context" d={`M${mark.x},${baseline - 18} l6,8 -6,8 -6,-8 Z`} />;
    if (mark.kind === "evidence") return <circle data-event-index={mark.index} key={`m${mark.index}`} className="event-shape evidence" cx={mark.x} cy={baseline - 12} r="5" />;
    if (mark.kind === "tool") return <rect data-event-index={mark.index} key={`m${mark.index}`} className="event-shape tool" x={mark.x - 2} y={baseline - 26} width="4" height="26" />;
    return <line data-event-index={mark.index} key={`m${mark.index}`} className="event-shape nominal" x1={mark.x} x2={mark.x} y1={baseline - 10} y2={baseline} />;
  };
  const rewards: Array<{ x: number; value: number }> = [];
  if (!compact && fidelity > 0) {
    let reward = 0;
    events.forEach((event) => {
      if (event.sequence < axis.start || event.sequence > axis.end) return;
      reward = eventReward(event) ?? reward;
      rewards.push({ x: x(event.sequence), value: reward });
    });
  }
  const rewardMin = Math.min(0, ...rewards.map((point) => point.value)), rewardMax = Math.max(1, ...rewards.map((point) => point.value));
  const rewardPath = rewards.map((point, index) => `${index ? "L" : "M"}${point.x},${182 - ((point.value - rewardMin) / Math.max(1, rewardMax - rewardMin)) * 26}`).join(" ");
  const selectedX = x(events[selected]?.sequence ?? min);
  return <section ref={containerRef} className={`shape-strip ${compact || fidelity === 0 ? "compact" : ""} fidelity-${fidelity}`} aria-label={label ?? "Trajectory shape"} data-selected-x={selectedX.toFixed(4)} data-visible-events={visibleCount} data-strip-mode={layout.mode} data-fidelity-level={`L${fidelity}`}>
    <svg width="100%" height={height} viewBox={`0 0 ${width} ${height}`} onMouseLeave={() => onHover(undefined)} onMouseMove={(pointer) => {
      const rect = pointer.currentTarget.getBoundingClientRect();
      const px = pointer.clientX - rect.left;
      let nearest: number | undefined; let best = Infinity;
      events.forEach((event, index) => {
        if (event.sequence < axis.start || event.sequence > axis.end) return;
        const distance = Math.abs(x(event.sequence) - px);
        if (distance < best) { best = distance; nearest = index; }
      });
      if (nearest !== undefined) onHover(nearest);
    }} onClick={() => onAscend ? onAscend() : hover !== undefined && onSelect(hover)}>
      {!compact && bands.filter((band) => band.end >= axis.start && band.start <= axis.end).map((band) => {
        const x0 = x(Math.max(axis.start, band.start)), x1 = x(Math.min(axis.end, band.end));
        return <g key={band.key} data-episode-key={band.key} data-episode-start={band.start} data-episode-end={band.end}><rect className="episode-band" x={x0} y="18" width={Math.max(1, x1 - x0)} height="23" /><text className="episode-label" x={x0 + 4} y="34">{x1 - x0 > 44 ? band.label : ""}</text></g>;
      })}
      {layout.mode === "binned" && layout.bins.map((bin, index) => bin.count ? <rect key={`b${index}`} className={`density-bin ${bin.tools * 2 > bin.count ? "tool-heavy" : ""}`} x={bin.x0} y={baseline - 4 - Math.round((bin.count / layout.peak) * 20)} width={Math.max(1, bin.x1 - bin.x0 - 1)} height={4 + Math.round((bin.count / layout.peak) * 20)} /> : null)}
      {layout.mode === "binned" ? layout.landmarks.map(markFor) : layout.marks.map(markFor)}
      {!compact && fidelity > 0 && <>{events.map((event) => event.context?.input_tokens !== undefined && event.context.capacity && event.sequence >= axis.start && event.sequence <= axis.end ? <rect key={`ctx:${event.id}`} className="context-pressure" x={x(event.sequence) - 2} y={153 - 25 * event.context.input_tokens / event.context.capacity} width="4" height={25 * event.context.input_tokens / event.context.capacity} /> : null)}<path className="reward-curve" d={rewardPath} /></>}
      {hover !== undefined && events[hover] && <line className="skimmer-line" x1={x(events[hover].sequence)} x2={x(events[hover].sequence)} y1="5" y2={height - 5} />}
      <line data-testid="playhead" className="playhead" x1={selectedX} x2={selectedX} y1="5" y2={height - 5} />
    </svg>
  </section>;
}

function OverviewSteps({ trajectory, axis, selected, onSelect }: { trajectory: Trajectory; axis: { start: number; end: number }; selected: number; onSelect: (index: number) => void }) {
  return <section className="overview-steps" aria-label="Rollout steps">
    {trajectory.events.map((event, index) => {
      if (event.sequence < axis.start || event.sequence > axis.end) return null;
      const toolNames = eventToolNames(event);
      return <button key={event.id} className={index === selected ? "selected" : ""} onClick={() => onSelect(index)}>
        <span className="address">#{event.sequence}</span><span className="kind-glyph">{glyphForKind(event.kind)}</span><span><small>{event.kind}</small><b>{toolNames.length ? `${toolNames.join(", ")} · ${eventText(event)}` : eventText(event)}</b></span>
      </button>;
    })}
  </section>;
}

function AxisNavigator({ trajectory, axis, selected, onChange }: { trajectory: Trajectory; axis: { start: number; end: number }; selected: number; onChange: (axis: { start: number; end: number }) => void }) {
  const drag = useRef<{ mode: "pan" | "start" | "end"; originX: number; start: number; end: number } | undefined>(undefined);
  const mapRef = useRef<HTMLDivElement>(null);
  const width = useMeasuredWidth(mapRef);
  const min = trajectory.events[0]?.sequence ?? 0;
  const max = trajectory.events.at(-1)?.sequence ?? min + 1;
  const span = Math.max(1, max - min);
  const minimumWindow = Math.max(1, span / 100);
  const start = Math.max(min, Math.min(axis.start, max));
  const end = Math.max(start, Math.min(axis.end, max));
  const startPct = ((start - min) / span) * 100;
  const widthPct = Math.max(0.75, ((end - start) / span) * 100);
  const layout = useMemo(() => layoutStrip(trajectory.events, { start: min, end: max }, width, { minSpacing: 3, binWidth: 3, preserveTools: true, preserveIndices: new Set([selected]) }), [max, min, selected, trajectory.events, width]);
  const sequenceAt = (clientX: number, rect: DOMRect) => min + Math.max(0, Math.min(1, (clientX - rect.left) / Math.max(1, rect.width))) * span;
  const panTo = (center: number, width: number) => {
    const boundedWidth = Math.min(span, Math.max(minimumWindow, width));
    const nextStart = Math.max(min, Math.min(max - boundedWidth, center - boundedWidth / 2));
    return { start: nextStart, end: nextStart + boundedWidth };
  };
  return <section className="axis-navigator" aria-label="Rollout timeline viewport" style={{ "--axis-start": `${startPct}%`, "--axis-width": `${widthPct}%` } as CSSProperties}>
    <div ref={mapRef} className="axis-map" aria-label="Timeline overview" data-axis-mode={layout.mode} data-axis-nodes={layout.mode === "marks" ? layout.marks.length : layout.bins.filter((bin) => bin.count).length + layout.landmarks.length} onPointerDown={(event) => {
      if (event.button !== undefined && event.button !== 0) return;
      const rect = event.currentTarget.getBoundingClientRect();
      const x = event.clientX - rect.left;
      const startX = ((start - min) / span) * rect.width;
      const endX = ((end - min) / span) * rect.width;
      const edge = Math.max(6, Math.min(12, rect.width * 0.02));
      let mode: "pan" | "start" | "end" = "pan";
      let origin = { start, end };
      if (Math.abs(x - startX) <= edge) mode = "start";
      else if (Math.abs(x - endX) <= edge) mode = "end";
      else if (x < startX || x > endX) {
        origin = panTo(sequenceAt(event.clientX, rect), end - start);
        onChange(origin);
      }
      drag.current = { mode, originX: event.clientX, ...origin };
      event.currentTarget.setPointerCapture?.(event.pointerId);
      event.preventDefault();
      event.currentTarget.closest<HTMLElement>("main")?.focus();
    }} onPointerMove={(event) => {
      const current = drag.current;
      if (!current) return;
      const rect = event.currentTarget.getBoundingClientRect();
      const delta = ((event.clientX - current.originX) / Math.max(1, rect.width)) * span;
      if (current.mode === "pan") {
        const width = current.end - current.start;
        const nextStart = Math.max(min, Math.min(max - width, current.start + delta));
        onChange({ start: nextStart, end: nextStart + width });
      } else if (current.mode === "start") onChange({ start: Math.max(min, Math.min(current.end - minimumWindow, current.start + delta)), end: current.end });
      else onChange({ start: current.start, end: Math.min(max, Math.max(current.start + minimumWindow, current.end + delta)) });
    }} onPointerUp={(event) => {
      drag.current = undefined;
      event.currentTarget.releasePointerCapture?.(event.pointerId);
      const surface = event.currentTarget.closest<HTMLElement>("main");
      surface?.focus();
      requestAnimationFrame(() => (surface?.isConnected ? surface : document.querySelector<HTMLElement>(".lane-track.active-zone"))?.focus());
    }} onPointerCancel={(event) => {
      drag.current = undefined;
      event.currentTarget.closest<HTMLElement>("main")?.focus();
    }}>
      {layout.mode === "marks" ? layout.marks.map((mark) => <i aria-hidden="true" key={trajectory.events[mark.index].id} className={`axis-mark ${mark.kind} ${mark.index === selected ? "selected" : ""}`} style={{ left: mark.x }} />) : <>
        {layout.bins.map((bin, index) => bin.count ? <i aria-hidden="true" key={`bin:${index}`} className={`axis-bin ${bin.tools * 2 > bin.count ? "tool-heavy" : ""}`} style={{ left: bin.x0, width: Math.max(1, bin.x1 - bin.x0), height: `${Math.max(2, Math.round((bin.count / layout.peak) * 9))}px` }} /> : null)}
        {layout.landmarks.map((mark) => <i aria-hidden="true" key={trajectory.events[mark.index].id} className={`axis-mark ${mark.kind} ${mark.index === selected ? "selected" : ""}`} style={{ left: mark.x }} />)}
      </>}
      <span className="axis-window"><i className="axis-handle start" /><i className="axis-handle end" /></span>
    </div>
    <input aria-label="Viewport start" type="range" min={min} max={max} step="any" value={start} onChange={(event) => onChange({ start: Math.min(Number(event.target.value), end - minimumWindow), end })} onPointerUp={(event) => event.currentTarget.closest<HTMLElement>("main")?.focus()} />
    <input aria-label="Viewport end" type="range" min={min} max={max} step="any" value={end} onChange={(event) => onChange({ start, end: Math.max(Number(event.target.value), start + minimumWindow) })} onPointerUp={(event) => event.currentTarget.closest<HTMLElement>("main")?.focus()} />
  </section>;
}

function EpisodeStrip({ trajectory, lane, episodes, selectedEpisode, onDescend }: { trajectory: Trajectory; lane: WorkspaceLane; episodes: Episode[]; selectedEpisode: number; onDescend: (episode: Episode) => void }) {
  const containerRef = useRef<HTMLElement>(null);
  const width = useMeasuredWidth(containerRef);
  const x = (sequence: number) => stripX(sequence, lane.axis, width);
  const percent = (px: number) => (px / width) * 100;
  const selectedSequence = trajectory.events[lane.selected]?.sequence ?? trajectory.events[0]?.sequence ?? 0;
  return <section ref={containerRef} className="episode-strip" aria-label="Trajectory shape" data-selected-x={x(selectedSequence).toFixed(4)} data-selected-episode={episodes[selectedEpisode]?.key ?? ""}>
    <div className="episode-axis">
      {episodes.filter((episode) => episode.end >= lane.axis.start && episode.start <= lane.axis.end).map((episode, index) => {
        const actualIndex = episodes.indexOf(episode);
        const nextStart = episodes[actualIndex + 1]?.start ?? Math.max(episode.end + 1, lane.axis.end);
        const left = percent(x(Math.max(lane.axis.start, episode.start)));
        const right = percent(x(Math.min(lane.axis.end, nextStart)));
        return <button key={episode.key} className={`episode-button ${actualIndex === selectedEpisode ? "selected" : ""}`} data-episode-index={actualIndex} data-episode-key={episode.key} data-episode-start={episode.start} data-episode-end={episode.end} style={{ left: `${left}%`, width: `${Math.max(0.8, right - left)}%` }} onClick={(event) => { event.stopPropagation(); onDescend(episode); }}>
          <b>{episode.label}</b><small>{episode.inferred ? "inferred" : "adapter"} · {episode.endIndex - episode.startIndex + 1} events</small>
        </button>;
      })}
      <i className="episode-playhead" style={{ left: `${percent(x(selectedSequence))}%` }} />
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

function LaneTrack({ lane, data, metadata, active, reference, hover, onActivate, onSelect, onHover, onDescend, onAscend, onAxisChange }: {
  lane: WorkspaceLane; data?: LaneData; metadata?: EditableMetadata; active: boolean; reference: boolean; hover?: number; onActivate: () => void; onSelect: (index: number) => void; onHover: (index?: number) => void; onDescend: (episode?: Episode) => void; onAscend: () => void; onAxisChange: (axis: { start: number; end: number }) => void;
}) {
  const trajectory = data?.trajectory;
  const depth = effectiveDepth(lane);
  const selected = trajectory ? Math.min(lane.selected, trajectory.events.length - 1) : 0;
  const episodes = trajectory ? episodesFor(trajectory.events) : [];
  const selectedEpisode = episodeIndexForEvent(episodes, selected);
  const episode = episodes[selectedEpisode];
  return <main tabIndex={0} aria-label={lane.band === "focus" ? "Read trajectory" : `Context lane ${lane.trajectoryId}`} className={`lane-track depth-${depth} fidelity-${lane.fidelity} ${lane.band}-lane ${active ? "active-zone" : ""} ${reference ? "reference-lane" : ""}`} data-lane-id={lane.id} data-trajectory={lane.trajectoryId} data-depth={depth} data-stored-depth={lane.depth} data-fidelity={fidelityNames[lane.fidelity]} data-episode={episode?.key ?? ""} data-axis-start={lane.axis.start.toFixed(4)} data-axis-end={lane.axis.end.toFixed(4)} onFocus={onActivate} onClick={onActivate}>
    <header><span title={lane.trajectoryId}><b>{metadata?.title ?? lane.trajectoryId}</b>{metadata?.description && <small>{metadata.description}</small>}{reference && <small>reference</small>}</span><span className="lane-state">{["", "overview", "episodes", "events", "raw"][depth]}{depth === 1 ? ` · ${fidelityNames[lane.fidelity]} · [ ]` : ""}</span></header>
    <div className="lane-body">{!trajectory ? <div className="lane-loading">loading trajectory…</div> : depth === 1 ? <><ShapeStrip trajectory={trajectory} selected={selected} hover={hover} axis={lane.axis} fidelity={lane.fidelity} compact={lane.band === "context"} label={lane.band === "focus" ? "Trajectory shape" : `Trajectory shape ${lane.trajectoryId}`} onSelect={onSelect} onHover={onHover} />{lane.fidelity === 2 && lane.band === "focus" && <OverviewSteps trajectory={trajectory} axis={lane.axis} selected={selected} onSelect={onSelect} />}</> : depth === 2 ? <EpisodeStrip trajectory={trajectory} lane={lane} episodes={episodes} selectedEpisode={selectedEpisode} onDescend={(target) => onDescend(target)} /> : <><ShapeStrip trajectory={trajectory} selected={selected} hover={hover} axis={lane.axis} compact label="Compressed trajectory shape" onSelect={onSelect} onHover={onHover} onAscend={onAscend} />{depth === 3 && episode && <ScopedEvents trajectory={trajectory} episode={episode} selected={selected} onSelect={onSelect} />}{depth === 4 && trajectory.events[selected] && <SourceRecord event={trajectory.events[selected]} />}</>}</div>
    {trajectory && <AxisNavigator trajectory={trajectory} axis={lane.axis} selected={selected} onChange={onAxisChange} />}
    {trajectory && hover !== undefined && depth === 1 && lane.band === "focus" && <aside className="skim-preview" role="status"><b>#{trajectory.events[hover].sequence} · {trajectory.events[hover].kind}</b><span>{eventText(trajectory.events[hover])}</span></aside>}
  </main>;
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

function Console({ workspace, lane, data, metadata, breadcrumb, resizeMode, dockPosition, pinned = false, active = false, onMetadata, onSelect, onHelp, onActivate }: {
  workspace: WorkspaceState; lane?: WorkspaceLane; data?: LaneData; metadata?: EditableMetadata; breadcrumb: string; resizeMode: boolean; dockPosition: "right" | "bottom"; pinned?: boolean; active?: boolean; onMetadata: (value: EditableMetadata) => void; onSelect: (index: number) => void; onHelp: () => void; onActivate: () => void;
}) {
  const trajectory = data?.trajectory; const current = trajectory?.events[Math.min(lane?.selected ?? 0, Math.max(0, trajectory.events.length - 1))];
  const around = 2;
  const detailRows = trajectory && current ? trajectory.events.slice(Math.max(0, trajectory.events.indexOf(current) - around), Math.min(trajectory.events.length, trajectory.events.indexOf(current) + around + 1)) : [];
  return <section className={`workspace-console ${active ? "active-zone" : ""}`} tabIndex={0} onFocus={() => { if (active) onActivate(); }} onPointerDown={onActivate} aria-label={pinned ? `Detail for ${lane?.trajectoryId ?? "rollout"}` : "Workspace console"} data-detail-lane-id={lane?.id} data-pinned={pinned ? "true" : "false"} data-resize-mode={resizeMode ? "true" : "false"} data-dock-position={dockPosition}>
    <header className="console-header">{trajectory ? <EditableHeader kind="trajectory" fallbackTitle={trajectory.name ?? trajectory.id} metadata={metadata} context={breadcrumb} onSave={onMetadata} /> : <div><h2>No lane selected</h2><p className="workspace-breadcrumb">{breadcrumb}</p></div>}
      {trajectory && <div className="judge-list">{judgesFor(trajectory).map((judge, index) => <button key={`${judge.label}:${index}`} className={/false|fail/i.test(judge.value) ? "failure" : judge.label === "verifier" && /true|pass/i.test(judge.value) ? "verifier-pass" : ""} onClick={() => { const found = trajectory.events.findIndex((event) => event.id === judge.eventId); if (found >= 0) onSelect(found); }}><small>{judge.label}</small><b>{judge.value}</b></button>)}</div>}
      <div className="console-meta">{pinned && <strong>pinned rollout</strong>}<span>reference: <b data-testid="reference-name">{workspace.reference ? workspace.lanes.find((item) => item.id === workspace.reference)?.trajectoryId ?? "none" : "none"}</b></span>{resizeMode && <strong>resize mode · arrows · Esc</strong>}<button onClick={onHelp}>?</button></div>
    </header>
    <section className="detail-region" aria-label="Selected moment">{trajectory ? detailRows.map((event) => <button key={event.id} className={`moment ${event.id === current?.id ? "selected" : ""}`} onClick={() => onSelect(trajectory.events.indexOf(event))}><span className="address">{event.sequence}</span><span className="kind-glyph">{glyphForKind(event.kind)}</span><span className="moment-copy"><small>{event.kind}</small><b>{eventText(event)}</b>{event.id === current?.id && <pre>{preview(eventDetail(event), 700)}</pre>}{event.id === current?.id && <em>source · {event.source?.path ?? "canonical record"}{event.source?.line ? `:${event.source.line}` : ""}</em>}</span></button>) : <div className="detail-empty"><b>Open a rollout from the collection.</b><small>Enter opens · A adds a lane</small></div>}</section>
  </section>;
}

type DockContent = { collection: ReactNode; guide: ReactNode; settings: ReactNode; detail: (id?: string) => ReactNode; lane: (id: string) => ReactNode };
const DockContentContext = createContext<DockContent | null>(null);

function WorkspacePanel({ params }: IDockviewPanelProps<{ kind: "collection" | "guide" | "settings" | "detail" | "lane"; laneId?: string }>) {
  const content = useContext(DockContentContext);
  if (!content) return null;
  if (params.kind === "collection") return content.collection;
  if (params.kind === "guide") return content.guide;
  if (params.kind === "settings") return content.settings;
  if (params.kind === "detail") return content.detail(params.laneId);
  return <div className="focus-slot">{content.lane(params.laneId ?? "")}</div>;
}

function MinimalTab({ api }: IDockviewPanelHeaderProps<{ label?: string }>) {
  const [title, setTitle] = useState(api.title);
  useEffect(() => {
    const disposable = api.onDidTitleChange((event) => setTitle(event.title));
    if (title !== api.title) setTitle(api.title);
    return () => disposable.dispose();
  }, [api]);
  return <span className="workspace-tab">{title || "module"}</span>;
}

const dockComponents = { workspace: WorkspacePanel };
const dockTabComponents = { minimal: MinimalTab };
const SHORTCUT_COLLECTION: CommandId[] = [commandIds.workspace.descend, commandIds.workspace.addLane, commandIds.view.fidelityUp, commandIds.group.search, commandIds.workspace.toggleRail, commandIds.workspace.toggleGuide, commandIds.workspace.toggleSettings, commandIds.workspace.cycleNext, commandIds.workspace.moveMode];
const SHORTCUT_LANE: CommandId[] = [commandIds.trajectory.next, commandIds.trajectory.previous, commandIds.trajectory.nextError, commandIds.view.fidelityUp, commandIds.workspace.descend, commandIds.view.zoomIn, commandIds.workspace.openDetail, commandIds.workspace.toggleGuide, commandIds.workspace.toggleSettings, commandIds.workspace.moveMode, commandIds.workspace.cycleNext, commandIds.workspace.closeLane];
const SHORTCUT_DETAIL: CommandId[] = [commandIds.trajectory.next, commandIds.trajectory.previous, commandIds.trajectory.nextError, commandIds.workspace.toggleGuide, commandIds.workspace.toggleSettings, commandIds.workspace.moveMode, commandIds.workspace.closeLane, commandIds.workspace.cycleNext];
const SHORTCUT_GUIDE: CommandId[] = [commandIds.workspace.toggleGuide, commandIds.workspace.toggleSettings, commandIds.workspace.cycleNext, commandIds.workspace.cyclePrevious, commandIds.workspace.moveMode];
const SHORTCUT_SETTINGS: CommandId[] = [commandIds.workspace.toggleSettings, commandIds.workspace.toggleGuide, commandIds.workspace.cycleNext, commandIds.workspace.cyclePrevious, commandIds.workspace.moveMode];

function KeyBar({ shortcuts, selection, mode, onModeArrow, onModeExit }: { shortcuts: Array<{ binding: string; label: string; id?: CommandId }>; selection?: string; mode?: "move" | "resize"; onModeArrow: (key: string) => void; onModeExit: () => void }) {
  return <footer className="keybar" role="region" aria-label="Active module shortcuts">
    {mode ? <>{["ArrowLeft", "ArrowUp", "ArrowDown", "ArrowRight"].map((key) => <button key={key} className="keybar-chip" tabIndex={-1} onMouseDown={(event) => event.preventDefault()} onClick={() => onModeArrow(key)}><kbd>{key.replace("Arrow", "")}</kbd><span>{mode === "move" ? "Move module" : "Resize seam"}</span></button>)}{shortcuts.slice(1).map((shortcut) => <button key={`${shortcut.binding}:${shortcut.label}`} className="keybar-chip" tabIndex={-1} onMouseDown={(event) => event.preventDefault()} onClick={shortcut.label === "Cancel" ? onModeExit : () => dispatchCommand(mode === "move" ? commandIds.workspace.moveMode : commandIds.workspace.resizeMode)}><kbd>{shortcut.binding}</kbd><span>{shortcut.label}</span></button>)}</> : shortcuts.map((shortcut) => <button key={`${shortcut.binding}:${shortcut.label}`} className="keybar-chip" tabIndex={-1} onMouseDown={(event) => event.preventDefault()} onClick={() => shortcut.id && dispatchCommand(shortcut.id)}><kbd>{shortcut.binding}</kbd><span>{shortcut.label}</span></button>)}
    {selection && <span className="selection-address">{selection}</span>}
  </footer>;
}

export function App({ initialTrajectory, provider = daemonProvider, setup = { mode: "cli" } }: { initialTrajectory?: Trajectory; provider?: ViewerProvider; setup?: ViewerSetup }) {
  useKeymapRevision();
  const { workspace, workspaceRef, breadcrumb, applyWorkspace, change, jump } = useWorkspaceController();
  const [browse, setBrowse] = useState<BrowseResponse>(() => fakeBrowse(initialTrajectory ?? sampleTrajectory));
  const [railFidelity, setRailFidelity] = useState(1);
  const [hover, setHover] = useState<Record<string, number | undefined>>({});
  const [resizeMode, setResizeMode] = useState(false); const [moveMode, setMoveMode] = useState(false); const [error, setError] = useState("");
  const [presentation, setPresentation] = useState<PresentationConfig>();
  const [theme, setTheme] = useState<"light" | "dark">(() => document.documentElement.getAttribute("data-theme") === "dark" || (!document.documentElement.getAttribute("data-theme") && window.matchMedia?.("(prefers-color-scheme: dark)").matches) ? "dark" : "light");
  const railRef = useRef<HTMLElement>(null);
  const lastFocus = useRef<string | undefined>(undefined);
  const lastGuideTarget = useRef("rail");
  const lastSettingsTarget = useRef("rail");
  const shouldOpenBrowserWelcome = useRef(setup.mode === "browser" && !new URLSearchParams(window.location.search).has("workspace") && !window.localStorage.getItem(workspaceStorageKey));
  const browserWelcomeOpened = useRef(false);
  const legacyReadIntent = useRef((() => { const params = new URLSearchParams(window.location.search); return (params.get("mode") === "read" || params.get("view") === "read") && !params.get("trajectory_id"); })());

  const { laneData, laneDataRef, putLaneData, deleteLaneData, pruneOffLaneData, loadForSlot, loadAnalysisForLane } = useLaneDataLoader({
    provider,
    initialTrajectory,
    workspace,
    workspaceRef,
    change,
    onError: setError,
    onPresentation: setPresentation,
  });
  const { metadata: viewerMetadata, updateCollection, updateTrajectory } = useViewerMetadata();

  const ordered = browse.trajectories;
  const collectionKey = useMemo(() => collectionMetadataKey(browse.sources.map((source) => source.id)), [browse.sources]);
  const trajectoryTitles = useMemo(() => Object.fromEntries(Object.entries(viewerMetadata.trajectories).map(([id, metadata]) => [id, metadata.title])), [viewerMetadata.trajectories]);
  const {
    apiRef: dockApiRef,
    detailPosition,
    onReady: onDockReady,
    persistLayout: persistDockLayout,
    arrangeLanes,
    arrangeWelcomeLayout,
    moveActiveModule,
    resizeNearest,
    activateTarget,
    activateAdjacent,
    beginPinnedOpen,
    canActivateContent,
  } = useWorkspaceDock({
    workspace,
    workspaceRef,
    railRef,
    change,
    collectionTitle: viewerMetadata.collections[collectionKey]?.title,
    trajectoryTitles,
  });
  const filtered = useMemo(() => ordered.filter((row) => !workspace.railQuery || rowSearchText(row, viewerMetadata.trajectories[rowKey(row)]).includes(workspace.railQuery.toLowerCase())), [ordered, viewerMetadata.trajectories, workspace.railQuery]);
  const boundedRail = Math.min(workspace.railSelected, Math.max(0, filtered.length - 1)); const selectedRow = filtered[boundedRail];
  const pinnedActiveLane = pinnedDetailLaneId(workspace.active);
  const activeLane = workspace.lanes.find((lane) => lane.id === workspace.active || lane.id === pinnedActiveLane) ?? (workspace.active === "detail" ? workspace.lanes.find((lane) => lane.id === lastFocus.current) ?? workspace.lanes.find((lane) => lane.band === "focus") : undefined);

  useEffect(() => {
    const controller = new AbortController();
    if (initialTrajectory) { setBrowse(fakeBrowse(initialTrajectory)); return () => controller.abort(); }
    if (workspaceRef.current.lanes.length) {
      provider.loadBrowse(controller.signal).then(setBrowse).catch((reason) => { if (!controller.signal.aborted && !(reason instanceof Error && reason.name === "AbortError")) setError(reason instanceof Error ? reason.message : "Could not load viewer"); });
      return () => controller.abort();
    }
    Promise.all([provider.loadInitial(controller.signal), provider.loadBrowse(controller.signal)]).then(([loaded, collection]) => {
      setBrowse(collection); setPresentation(loaded.presentation);
      const sourceId = collection.trajectories.find((row) => row.trajectory.id === loaded.trajectory.id)?.source_id;
      if (sourceId) putLaneData(laneId(sourceId, loaded.trajectory.id), { trajectory: loaded.trajectory, analysis: null, presentation: loaded.presentation });
      if (sourceId && legacyReadIntent.current && !workspaceRef.current.lanes.length) {
        const id = laneId(sourceId, loaded.trajectory.id);
        applyWorkspace({ ...workspaceRef.current, railExpanded: false, active: id, lanes: [{ id, sourceId, trajectoryId: loaded.trajectory.id, band: "focus", selected: firstAnomaly(loaded.trajectory), depth: 1, fidelity: 1, axis: { start: loaded.trajectory.events[0]?.sequence ?? 0, end: loaded.trajectory.events.at(-1)?.sequence ?? 1 }, descentStack: [] }] }, false);
      }
    }).catch((reason) => { if (!controller.signal.aborted && !(reason instanceof Error && reason.name === "AbortError")) setError(reason instanceof Error ? reason.message : "Could not load viewer"); });
    return () => controller.abort();
  }, [applyWorkspace, initialTrajectory, provider, putLaneData, workspaceRef]);

  useEffect(() => applyPresentationTheme(presentation), [presentation]);
  useEffect(() => { if (activeLane && laneData.has(activeLane.id)) setPresentation(laneData.get(activeLane.id)?.presentation); }, [activeLane, laneData]);
  useEffect(() => { document.documentElement.setAttribute("data-theme", theme); }, [theme]);
  useEffect(() => {
    const lane = workspace.lanes.find((item) => item.id === workspace.active);
    if (lane?.band === "focus") lastFocus.current = lane.id;
  }, [workspace.active, workspace.lanes]);

  const loadRowIntoLane = useCallback(async (row: BrowseTrajectory, add: boolean, preserve?: WorkspaceLane) => {
    const id = rowKey(row); const existing = workspaceRef.current.lanes.find((lane) => lane.id === id);
    if (existing) { change((current) => ({ ...current, active: existing.id })); return; }
    const slot = preserve?.id ?? (add ? `add:${id}` : "focus-open");
    const loaded = await loadForSlot(slot, row.source_id, row.trajectory.id);
    if (!loaded) return;
    if (workspaceRef.current.lanes.some((lane) => lane.id === id)) {
      if (!add) change((current) => ({ ...current, active: id }));
      return;
    }
    if (preserve && !workspaceRef.current.lanes.some((lane) => lane.id === preserve.id)) return;
    const focus = workspaceRef.current.lanes.filter((lane) => lane.band === "focus");
    const band = add && focus.length >= 2 ? "context" : "focus";
    const base: WorkspaceLane = { id, sourceId: row.source_id, trajectoryId: row.trajectory.id, band, selected: preserve?.selected ?? firstAnomaly(loaded.trajectory), depth: preserve?.depth ?? 1, fidelity: preserve?.fidelity ?? 1, axis: preserve?.axis ?? { start: loaded.trajectory.events[0]?.sequence ?? 0, end: loaded.trajectory.events.at(-1)?.sequence ?? 1 }, descentStack: preserve?.descentStack ?? [] };
    putLaneData(id, { trajectory: loaded.trajectory, analysis: null, presentation: loaded.presentation });
    change((current) => {
      // `a` piles lanes in without stealing focus from the collection;
      // opening (Enter) is the action that moves focus.
      if (add) return { ...current, lanes: [...current.lanes, base], railExpanded: current.railExpanded };
      if (!current.lanes.length) return { ...current, lanes: [base], active: id };
      if (preserve) return { ...current, lanes: current.lanes.map((lane) => lane.id === preserve.id ? { ...base, band: preserve.band } : lane), active: id, reference: current.reference === preserve.id ? undefined : current.reference };
      const replaceId = current.lanes.find((lane) => lane.id === current.active && lane.band === "focus")?.id ?? current.lanes.find((lane) => lane.id === lastFocus.current && lane.band === "focus")?.id ?? current.lanes.find((lane) => lane.band === "focus")?.id;
      if (!replaceId) return { ...current, lanes: [...current.lanes, base], active: id };
      return { ...current, lanes: current.lanes.map((lane) => lane.id === replaceId ? { ...base, band: "focus" } : lane), active: id, reference: current.reference === replaceId ? undefined : current.reference };
    });
    pruneOffLaneData();
    void loadAnalysisForLane(id, row.source_id, row.trajectory.id);
  }, [change, loadAnalysisForLane, loadForSlot, pruneOffLaneData, putLaneData, workspaceRef]);

  const openSelected = (add: boolean) => { if (selectedRow) void loadRowIntoLane(selectedRow, add).catch((reason) => setError(reason instanceof Error ? reason.message : "Could not load trajectory")); };
  useEffect(() => {
    if (!shouldOpenBrowserWelcome.current || browserWelcomeOpened.current || ordered.length < 2) return;
    browserWelcomeOpened.current = true;
    void (async () => {
      await loadRowIntoLane(ordered[0], true);
      await loadRowIntoLane(ordered[1], true);
      change((current) => ({ ...current, active: "guide", guideOpen: true, settingsOpen: true }), false);
      arrangeWelcomeLayout();
    })().catch((reason) => setError(reason instanceof Error ? reason.message : "Could not open example workspace"));
  }, [arrangeWelcomeLayout, change, loadRowIntoLane, ordered]);
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
  const cycleZone = (delta: number) => { const zones = [...(workspaceRef.current.railExpanded ? ["rail"] : []), ...(workspaceRef.current.guideOpen ? ["guide"] : []), ...(workspaceRef.current.settingsOpen ? ["settings"] : []), ...workspaceRef.current.lanes.map((lane) => lane.id), ...(workspaceRef.current.lanes.length ? ["detail"] : []), ...workspaceRef.current.details.map(pinnedDetailTarget)]; if (!zones.length) return; const index = zones.indexOf(workspaceRef.current.active); const active = zones[((index < 0 ? 0 : index) + delta + zones.length) % zones.length]; activateTarget(active); change((current) => ({ ...current, active })); };
  const sweep = (delta: number) => { if (!activeLane || !filtered.length) return; const occupied = new Set(workspaceRef.current.lanes.filter((lane) => lane.id !== activeLane.id).map((lane) => lane.id)); const candidates = filtered.filter((row) => !occupied.has(rowKey(row))); if (!candidates.length) return; const index = candidates.findIndex((row) => rowKey(row) === activeLane.id); const row = candidates[((index < 0 ? 0 : index) + delta + candidates.length) % candidates.length]; change((current) => ({ ...current, railSelected: filtered.indexOf(row) }), false); void loadRowIntoLane(row, false, activeLane); };
  const closeLane = () => { if (!activeLane) return; deleteLaneData(activeLane.id); change((current) => { const lanes = current.lanes.filter((lane) => lane.id !== activeLane.id); return { ...current, lanes, details: current.details.filter((id) => id !== activeLane.id), railExpanded: lanes.length ? current.railExpanded : true, active: lanes[0]?.id ?? "rail", reference: current.reference === activeLane.id ? undefined : current.reference }; }); };
  const openPinnedDetail = () => {
    if (!activeLane) return;
    beginPinnedOpen(activeLane.id);
  };
  const closeActiveModule = () => { const detailLane = pinnedDetailLaneId(workspaceRef.current.active); if (detailLane) { change((current) => ({ ...current, details: current.details.filter((id) => id !== detailLane), active: detailLane })); return; } closeLane(); };
  const promoteDemote = () => { if (!activeLane) return; const counterpart = workspaceRef.current.lanes.find((item) => item.id !== activeLane.id && item.band !== activeLane.band); if (!counterpart) return; const activePanel = dockApiRef.current?.getPanel(lanePanelId(activeLane.id)); const counterpartPanel = dockApiRef.current?.getPanel(lanePanelId(counterpart.id)); const promoting = activeLane.band === "context"; change((current) => ({ ...current, lanes: current.lanes.map((item) => item.id === activeLane.id ? { ...item, band: counterpart.band } : item.id === counterpart.id ? { ...item, band: activeLane.band } : item) })); requestAnimationFrame(() => { const api = dockApiRef.current; if (!api || !activePanel || !counterpartPanel) return; const promoted = promoting ? activePanel : counterpartPanel, demoted = promoting ? counterpartPanel : activePanel; const focusAnchor = workspaceRef.current.lanes.filter((lane) => lane.band === "focus" && lane.id !== (promoting ? activeLane.id : counterpart.id)).map((lane) => api.getPanel(lanePanelId(lane.id))).find(Boolean) ?? api.getPanel("collection"); if (focusAnchor) promoted.api.moveTo({ group: focusAnchor.api.group, position: "right" }); demoted.api.moveTo({ group: promoted.api.group, position: "bottom" }); persistDockLayout(api); }); };
  const adjustFidelity = (delta: number) => {
    if (workspaceRef.current.active === "rail") { setRailFidelity((value) => Math.max(0, Math.min(2, value + delta))); return; }
    if (!activeLane || effectiveDepth(activeLane) !== 1) return;
    updateLane(activeLane.id, (lane) => ({ ...lane, fidelity: Math.max(0, Math.min(2, lane.fidelity + delta)) }), false);
  };
  const adjustZoom = (factor: number | "fit", all: boolean) => change((current) => ({ ...current, lanes: current.lanes.map((lane) => { if (!all && lane.id !== current.active) return lane; const data = laneDataRef.current.get(lane.id); if (!data) return lane; const min = data.trajectory.events[0]?.sequence ?? 0, max = data.trajectory.events.at(-1)?.sequence ?? 1, sequence = data.trajectory.events[lane.selected]?.sequence ?? min; return { ...lane, axis: factor === "fit" ? { start: min, end: max } : zoomWindow(lane.axis, sequence, factor, min, max) }; }) }), false);
  const toggleSettings = () => {
    const current = workspaceRef.current;
    const openingWelcomeSettings = !current.settingsOpen && setup.mode === "browser" && current.guideOpen && current.details.length === 0 && current.lanes.length === 2 && current.lanes[0].trajectoryId === "checkout-rollout-01" && current.lanes[1].trajectoryId === "checkout-rollout-02";
    change((state) => { const settingsOpen = !state.settingsOpen; if (settingsOpen) lastSettingsTarget.current = state.active; return { ...state, settingsOpen, active: settingsOpen ? "settings" : state.active === "settings" ? lastSettingsTarget.current : state.active }; });
    if (openingWelcomeSettings) arrangeWelcomeLayout();
  };

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.target instanceof HTMLInputElement || event.target instanceof HTMLTextAreaElement) return;
      const mode = moveMode ? "move" : resizeMode ? "resize" : undefined;
      if (mode) {
        const ownToggle = event.ctrlKey && event.key.toLowerCase() === (mode === "move" ? "m" : "w");
        if (ownToggle) return;
        event.preventDefault(); event.stopImmediatePropagation();
        if (event.key === "Escape") { setMoveMode(false); setResizeMode(false); return; }
        if (["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"].includes(event.key)) mode === "move" ? moveActiveModule(event.key) : resizeNearest(event.key);
        return;
      }
      if (event.altKey && ["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"].includes(event.key)) {
        event.preventDefault();
        event.stopImmediatePropagation();
        const target = activateAdjacent(event.key);
        if (target) {
          change((current) => ({ ...current, active: target }), false);
        }
      }
    };
    window.addEventListener("keydown", onKey, true); return () => window.removeEventListener("keydown", onKey, true);
  }, [activateAdjacent, change, moveActiveModule, moveMode, resizeMode]);

  useCommands("workspace", {
    [commandIds.workspace.toggleRail]: () => change((current) => { const railExpanded = !current.railExpanded; return { ...current, railExpanded, active: !railExpanded && current.active === "rail" && current.lanes.length ? current.lanes[0].id : current.active }; }),
    [commandIds.workspace.addLane]: () => workspaceRef.current.active === "rail" ? openSelected(true) : false,
    [commandIds.workspace.closeLane]: () => activeLane ? closeActiveModule() : false,
    [commandIds.workspace.cycleNext]: () => cycleZone(1), [commandIds.workspace.cyclePrevious]: () => cycleZone(-1),
    [commandIds.workspace.nextRollout]: () => activeLane ? sweep(1) : false, [commandIds.workspace.previousRollout]: () => activeLane ? sweep(-1) : false,
    [commandIds.workspace.promoteDemote]: () => activeLane ? promoteDemote() : false,
    [commandIds.workspace.pinReference]: () => activeLane ? change((current) => ({ ...current, reference: current.reference === activeLane.id ? undefined : activeLane.id })) : false,
    [commandIds.workspace.directionRows]: () => arrangeLanes("rows"), [commandIds.workspace.directionColumns]: () => arrangeLanes("columns"),
    [commandIds.workspace.descend]: () => { if (!activeLane) { if (workspaceRef.current.active !== "rail") return false; openSelected(false); return; } if (activeLane.band === "context" || pinnedDetailLaneId(workspaceRef.current.active)) return false; descendLane(activeLane.id); },
    // Esc is structural (ascend, then close the lane, keeping current rail
    // state); history rewind is exclusively Ctrl+o, so backing out of a lane
    // never restores a stale rail selection.
    [commandIds.workspace.ascend]: () => { if (resizeMode) { setResizeMode(false); return; } if (!activeLane) return false; if (effectiveDepth(activeLane) > 1) ascendLane(activeLane.id); else closeLane(); },
    [commandIds.workspace.openDetail]: () => activeLane ? openPinnedDetail() : false,
    [commandIds.workspace.toggleGuide]: () => change((current) => { const guideOpen = !current.guideOpen; if (guideOpen) lastGuideTarget.current = current.active; return { ...current, guideOpen, active: guideOpen ? "guide" : current.active === "guide" ? lastGuideTarget.current : current.active }; }),
    [commandIds.workspace.toggleSettings]: toggleSettings,
    [commandIds.workspace.jumpBack]: () => jump(-1), [commandIds.workspace.jumpForward]: () => jump(1),
    [commandIds.workspace.resizeMode]: () => { setMoveMode(false); setResizeMode((current) => !current); },
    [commandIds.workspace.moveMode]: () => { setResizeMode(false); setMoveMode((current) => !current); },
    [commandIds.view.fidelityUp]: () => adjustFidelity(1), [commandIds.view.fidelityDown]: () => adjustFidelity(-1),
    [commandIds.view.zoomIn]: () => activeLane ? adjustZoom(2, false) : false, [commandIds.view.zoomOut]: () => activeLane ? adjustZoom(0.5, false) : false, [commandIds.view.zoomFit]: () => activeLane ? adjustZoom("fit", false) : false,
    [commandIds.view.zoomInAll]: () => activeLane ? adjustZoom(2, true) : false, [commandIds.view.zoomOutAll]: () => activeLane ? adjustZoom(0.5, true) : false, [commandIds.view.zoomFitAll]: () => activeLane ? adjustZoom("fit", true) : false,
    [commandIds.view.toggleHelp]: () => dispatchCommand(commandIds.workspace.toggleGuide),
  });
  useCommands("trajectory", {
    [commandIds.trajectory.next]: () => activeLane ? moveEvent(1) : workspaceRef.current.active === "rail" ? change((current) => ({ ...current, railSelected: Math.min(filtered.length - 1, current.railSelected + 1) }), false) : false,
    [commandIds.trajectory.previous]: () => activeLane ? moveEvent(-1) : workspaceRef.current.active === "rail" ? change((current) => ({ ...current, railSelected: Math.max(0, current.railSelected - 1) }), false) : false,
    [commandIds.trajectory.nextError]: () => jumpEvent((event) => event.kind === "error"), [commandIds.trajectory.nextContext]: () => jumpEvent((event) => !!event.context || !!event.alignment_key?.startsWith("context:")),
    [commandIds.trajectory.nextReward]: () => jumpEvent((event) => event.kind === "reward" || event.kind === "grader"), [commandIds.trajectory.nextFinding]: () => { if (!activeLane) return false; const ids = new Set((laneData.get(activeLane.id)?.analysis?.analysis.findings ?? []).flatMap((finding) => finding.event_ids ?? [])); jumpEvent((event) => ids.has(event.id)); },
  });

  const shortcutIDs = moveMode || resizeMode ? [] : workspace.active === "rail" ? SHORTCUT_COLLECTION : workspace.active === "guide" ? SHORTCUT_GUIDE : workspace.active === "settings" ? SHORTCUT_SETTINGS : workspace.active === "detail" || pinnedDetailLaneId(workspace.active) ? SHORTCUT_DETAIL : SHORTCUT_LANE;
  const shortcuts: Array<{ binding: string; label: string; id?: CommandId }> = moveMode || resizeMode
    ? [{ binding: "← ↑ ↓ →", label: `${moveMode ? "Move module" : "Resize seam"}` }, { binding: firstBindingLabel(moveMode ? commandIds.workspace.moveMode : commandIds.workspace.resizeMode), label: `Exit ${moveMode ? "move" : "resize"} mode` }, { binding: "Esc", label: "Cancel" }]
    : shortcutIDs.map((id) => ({ binding: firstBindingLabel(id), label: commandDefinition(id).label, id }));

  const dockContent: DockContent = {
    collection: <Rail root={railRef} rows={filtered} workspace={{ ...workspace, railSelected: boundedRail }} fidelity={railFidelity} metadata={viewerMetadata.collections[collectionKey]} trajectoryMetadata={viewerMetadata.trajectories} onMetadata={(value) => updateCollection(collectionKey, value)} onActivate={() => { if (canActivateContent("rail")) change((current) => ({ ...current, active: "rail" })); }} onSelect={(index) => change((current) => ({ ...current, railSelected: index, active: "rail" }))} onOpen={() => openSelected(false)} onAdd={() => openSelected(true)} onCollectionView={(collectionView) => change((current) => ({ ...current, collectionView }))} onQuery={(railQuery) => change((current) => { const next = ordered.filter((row) => !railQuery || rowSearchText(row, viewerMetadata.trajectories[rowKey(row)]).includes(railQuery.toLowerCase())); const kept = selectedRow ? next.findIndex((row) => rowKey(row) === rowKey(selectedRow)) : -1; return { ...current, railQuery, railSelected: kept >= 0 ? kept : 0 }; })} />,
    guide: <Guide active={workspace.active === "guide"} setup={setup} onActivate={() => { if (canActivateContent("guide")) change((current) => ({ ...current, active: "guide" })); }} onClose={() => change((current) => ({ ...current, guideOpen: false, active: current.active === "guide" ? lastGuideTarget.current : current.active }))} />,
    settings: <Settings active={workspace.active === "settings"} theme={theme} setup={setup} onTheme={setTheme} onActivate={() => { if (canActivateContent("settings")) change((current) => ({ ...current, active: "settings" })); }} onClose={() => change((current) => ({ ...current, settingsOpen: false, active: current.active === "settings" ? lastSettingsTarget.current : current.active }))} />,
    lane: (id) => { const lane = workspace.lanes.find((item) => item.id === id); return lane ? <LaneTrack lane={lane} data={laneData.get(lane.id)} metadata={viewerMetadata.trajectories[lane.id]} active={workspace.active === lane.id} reference={workspace.reference === lane.id} hover={hover[lane.id]} onActivate={() => { if (canActivateContent(lane.id)) change((current) => ({ ...current, active: lane.id })); }} onSelect={(value) => selectEvent(lane.id, value)} onHover={(value) => setHover((current) => ({ ...current, [lane.id]: value }))} onDescend={(episode) => descendLane(lane.id, episode)} onAscend={() => ascendLane(lane.id)} onAxisChange={(axis) => updateLane(lane.id, (current) => ({ ...current, axis }), false)} /> : null; },
    detail: (id) => { const lane = id ? workspace.lanes.find((item) => item.id === id) : activeLane ?? workspace.lanes.find((item) => item.id === lastFocus.current) ?? workspace.lanes.find((item) => item.band === "focus"); const target = id ? pinnedDetailTarget(id) : "detail"; return <Console workspace={workspace} lane={lane} data={lane ? laneData.get(lane.id) : undefined} metadata={lane ? viewerMetadata.trajectories[lane.id] : undefined} breadcrumb={id && lane ? `${lane.trajectoryId} · detail` : breadcrumb} resizeMode={resizeMode} dockPosition={detailPosition} pinned={!!id} active={workspace.active === target} onMetadata={(value) => lane && updateTrajectory(lane.id, value)} onSelect={(index) => lane && selectEvent(lane.id, index)} onHelp={() => dispatchCommand(commandIds.workspace.toggleGuide)} onActivate={() => { if (canActivateContent(target)) change((current) => ({ ...current, active: target })); }} />; },
  };
  return <ViewerProviderContext.Provider value={provider}><DockContentContext.Provider value={dockContent}><div className={`instrument-shell workspace-rack rail-${workspace.railExpanded ? "open" : "closed"}`} data-filter={workspace.railQuery} data-direction={workspace.direction} data-active-zone={workspace.active} data-move-mode={moveMode ? "true" : "false"} data-resize-mode={resizeMode ? "true" : "false"}>
    {error && <div className="instrument-error" role="alert">{error}</div>}{presentation?.notices?.map((notice) => <div className="presentation-notice" role="status" key={notice}>{notice}</div>)}
    <section className="workspace-stage" aria-label="Trajectory stage"><DockviewReact className="rlviz-dockview" components={dockComponents} tabComponents={dockTabComponents} defaultTabComponent={MinimalTab} onReady={onDockReady} disableFloatingGroups noPanelsOverlay="emptyGroup" keyboardNavigation={false} announcements={false} /></section>
    <KeyBar shortcuts={shortcuts} mode={moveMode ? "move" : resizeMode ? "resize" : undefined} onModeArrow={(key) => moveMode ? moveActiveModule(key) : resizeNearest(key)} onModeExit={() => { setMoveMode(false); setResizeMode(false); }} selection={activeLane && laneData.get(activeLane.id)?.trajectory.events[activeLane.selected] ? `#${laneData.get(activeLane.id)!.trajectory.events[Math.min(activeLane.selected, laneData.get(activeLane.id)!.trajectory.events.length - 1)].sequence}` : undefined} />
  </div></DockContentContext.Provider></ViewerProviderContext.Provider>;
}
