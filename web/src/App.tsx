import { useEffect, useMemo, useRef, useState } from "react";
import { loadAnalysis, loadChildPage, loadComparison, loadEventPage, loadGroup, loadGroupPaths, loadTrajectory } from "./api";
import { ComparisonView } from "./ComparisonView";
import { applyPresentationKeymap, bindingLabel, commandIds, useCommands, useKeymapRevision } from "./commands";
import { ContextTrack } from "./ContextTrack";
import { duration, eventText, payload, preview, time, title } from "./format";
import { GroupView } from "./GroupView";
import { Inspector } from "./Inspector";
import { KeymapDialog } from "./KeymapDialog";
import { Kind } from "./Kind";
import { applyPresentationTheme } from "./presentation";
import { deriveLandmark, deriveLandmarkRail, isContextEvent } from "./research";
import { OutcomeView, TranscriptView } from "./ResearchViews";
import { sampleTrajectory } from "./sample";
import { TrajectoryTabs } from "./TrajectoryTabs";
import { TrajectoryOverview } from "./TrajectoryOverview";
import type { TrajectorySurface } from "./TrajectoryTabs";
import type { AnalysisResponse, ComparisonResponse, GroupPathsResponse, GroupResponse, IndexedSource, PresentationConfig, Trajectory, TrajectoryArtifact, TrajectoryEvent } from "./types";
import { VirtualList } from "./VirtualList";
import type { VisibleRange } from "./VirtualList";

const filterKinds = ["all", "generation", "tool", "observation", "reward", "grader", "error"];
const eventKey = (event: TrajectoryEvent) => event.id;

const viewerKeys = ["view", "left", "right", "step", "cohort_filter"];
function validID(value: string | null, max = 512): string | null {
  return value && value.length <= max && !/[\u0000-\u001f\u007f]/.test(value) ? value : null;
}
function validSurface(value: string | null): TrajectorySurface {
  return value === "timeline" || value === "outcome" ? value : "transcript";
}
function replaceParams(update: (params: URLSearchParams) => void) {
  const params = new URLSearchParams(globalThis.location?.search ?? "");
  update(params);
  const query = params.toString();
  globalThis.history?.replaceState({}, "", `${globalThis.location.pathname}${query ? `?${query}` : ""}${globalThis.location.hash}`);
}
function resolvedStep(value: string | null, comparison: ComparisonResponse): number {
  const fallback = comparison.alignment.first_meaningful_divergence ?? 0;
  if (value === "divergence") return fallback;
  if (!value || !/^\d+$/.test(value)) return fallback;
  const step = Number(value);
  return Number.isSafeInteger(step) && step < comparison.alignment.steps.length ? step : fallback;
}

function Value({ value }: { value: unknown }) {
  if (value === undefined) return null;
  return <pre className="payload">{preview(value)}</pre>;
}

function TimelineCard({ event, selected, expanded, position, total, onSelect, onExpand }: {
  event: TrajectoryEvent; selected: boolean; expanded: boolean; position: number; total: number; onSelect: () => void; onExpand: () => void;
}) {
  const hasIO = event.input !== undefined || event.output !== undefined;
  return (
    <article id={`event-${event.id}`} className={`event-card kind-border-${event.kind} ${selected ? "selected" : ""}`} onClick={onSelect} aria-posinset={position} aria-setsize={total}>
      <div className="event-rail"><Kind kind={event.kind} /><span className="seq">{String(event.sequence).padStart(3, "0")}</span></div>
      <div className="event-body">
        <header>
          <div><span className="event-kind-label">{event.kind.replaceAll("_", " ")}</span><h2>{title(event)}</h2></div>
          <div className="event-stats">{event.reward !== undefined && <span className={event.reward < 0 ? "negative" : "positive"}>{event.reward > 0 ? "+" : ""}{event.reward}</span>}{event.duration_ms !== undefined && <span>{duration(event.duration_ms)}</span>}<span>{time(event.timestamp)}</span></div>
        </header>
        {event.summary && event.summary !== event.title && <p className="summary">{event.summary}</p>}
        {expanded ? (
          <div className="expanded-content">
            {hasIO ? <><div className="io-label">Input</div><Value value={event.input} /><div className="io-label">Output</div><Value value={event.output} /></> : <Value value={event.content} />}
          </div>
        ) : (payload(event) !== event && <pre className="preview">{preview(payload(event), 210)}</pre>)}
        <button className="expand" onClick={(e) => { e.stopPropagation(); onExpand(); }} aria-label={`${expanded ? "Collapse" : "Expand"} event ${event.sequence}`}>{expanded ? "Collapse" : "Expand"} <kbd>{bindingLabel(commandIds.trajectory.toggleExpanded)}</kbd></button>
      </div>
    </article>
  );
}

export function App({ initialTrajectory }: { initialTrajectory?: Trajectory }) {
  useKeymapRevision();
  const [trajectory, setTrajectory] = useState(initialTrajectory || sampleTrajectory);
  const [isSample, setIsSample] = useState(!initialTrajectory);
  const [loading, setLoading] = useState(!initialTrajectory);
  const [eventTotal, setEventTotal] = useState(trajectory.events.length);
  const [indexSource, setIndexSource] = useState<IndexedSource | null>(null);
  const [presentation, setPresentation] = useState<PresentationConfig | undefined>();
  const [selectedId, setSelectedId] = useState(validID(new URLSearchParams(globalThis.location?.search ?? "").get("event")) ?? trajectory.events[0]?.id ?? "");
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [filter, setFilter] = useState("all");
  const [query, setQuery] = useState("");
  const [searchOpen, setSearchOpen] = useState(false);
  const [raw, setRaw] = useState(false);
  const [help, setHelp] = useState(false);
  const [surface, setSurface] = useState<TrajectorySurface>(() => validSurface(new URLSearchParams(globalThis.location?.search ?? "").get("surface")));
  const [group, setGroup] = useState<GroupResponse | null>(null);
  const [groupPaths, setGroupPaths] = useState<GroupPathsResponse | null>(null);
  const [groupPathsError, setGroupPathsError] = useState("");
  const [groupLoading, setGroupLoading] = useState(false);
  const [groupError, setGroupError] = useState("");
  const [comparison, setComparison] = useState<ComparisonResponse | null>(null);
  const [comparisonStep, setComparisonStep] = useState(0);
  const [analysis, setAnalysis] = useState<AnalysisResponse | null>(null);
  const [analysisLoading, setAnalysisLoading] = useState(false);
  const [analysisError, setAnalysisError] = useState("");
  const [analysisVersion, setAnalysisVersion] = useState(0);
  const [selectedArtifactId, setSelectedArtifactId] = useState(trajectory.artifacts?.[0]?.id ?? "");
  const [visibleRange, setVisibleRange] = useState<VisibleRange>();
  const [routeVersion, setRouteVersion] = useState(0);
  const searchRef = useRef<HTMLInputElement>(null);
  const outlineRef = useRef<HTMLElement>(null);
  const timelineRef = useRef<HTMLElement>(null);
  const restoredView = useRef("");
  const sourceId = new URLSearchParams(globalThis.location?.search ?? "").get("trajectory") ?? "";
  const isDemo = new URLSearchParams(globalThis.location?.search ?? "").get("demo") === "1";

  useEffect(() => {
    if (initialTrajectory && routeVersion === 0) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const params = new URLSearchParams(globalThis.location?.search ?? "");
    const sourceId = params.get("trajectory") ?? "";
    const indexed = params.get("indexed") === "1";
    const wait = () => new Promise<void>((resolve) => { timer = setTimeout(resolve, 1000); });
    const run = async () => {
      const result = await loadTrajectory(controller.signal);
      const next = result.trajectory;
      const requestedEvent = validID(params.get("event"));
      setTrajectory(next); setIsSample(result.isSample); setSelectedId(requestedEvent ?? next.events[0]?.id ?? ""); setLoading(false); setIndexSource(result.source ?? null); setPresentation(result.presentation);
      setEventTotal(result.page?.total ?? next.events.length);
      if (!indexed || result.isSample || !sourceId) return;
      let after = result.page?.next_sequence ?? next.events.at(-1)?.sequence;
      let hasMore = result.page?.has_more ?? false;
      let indexState = result.source?.index_state;
      while (!controller.signal.aborted && after !== undefined && (hasMore || indexState === "indexing" || indexState === "refreshing")) {
        if (!hasMore) await wait();
        if (controller.signal.aborted) return;
        const page = await loadEventPage(sourceId, next.id, after, controller.signal);
        if (controller.signal.aborted) return;
        setEventTotal(page.page.total);
        if (page.source) { setIndexSource(page.source); indexState = page.source.index_state; }
        if (page.events.length) {
          setTrajectory((current) => {
            const existing = new Set(current.events.map((event) => event.id));
            const appended = page.events.filter((event) => !existing.has(event.id));
            return appended.length ? { ...current, events: [...current.events, ...appended] } : current;
          });
          after = page.events.at(-1)?.sequence ?? after;
        }
        hasMore = page.page.has_more;
        if (page.page.next_sequence !== undefined) after = page.page.next_sequence;
      }
      const shouldLoadChildren = (page?: import("./types").PageMetadata) => page?.has_more || result.source?.index_state === "indexing" || result.source?.index_state === "refreshing";
      const loadRemainingSignals = async () => {
        if (!shouldLoadChildren(result.signalPage)) return;
        let offset = result.signalPage?.next_offset ?? next.signals?.length ?? 0;
        while (!controller.signal.aborted) {
          const child = await loadChildPage("signals", sourceId, next.id, offset, controller.signal);
          if (controller.signal.aborted) return;
          setTrajectory((current) => {
            const existing = new Set((current.signals ?? []).map((item) => item.id ?? `${item.name}\u0000${item.event_id ?? ""}`));
            const appended = child.items.filter((item) => !existing.has(item.id ?? `${item.name}\u0000${item.event_id ?? ""}`));
            if (!appended.length) return current;
            const signals = [...(current.signals ?? []), ...appended];
            const reward = [...signals].reverse().find((item) => item.name.toLowerCase() === "reward" && typeof item.value === "number");
            return { ...current, signals, total_reward: typeof reward?.value === "number" ? reward.value : current.total_reward };
          });
          if (!child.page.has_more || child.page.next_offset === undefined || child.page.next_offset <= offset) return;
          offset = child.page.next_offset;
        }
      };
      const loadRemainingArtifacts = async () => {
        if (!shouldLoadChildren(result.artifactPage)) return;
        let offset = result.artifactPage?.next_offset ?? next.artifacts?.length ?? 0;
        while (!controller.signal.aborted) {
          const child = await loadChildPage("artifacts", sourceId, next.id, offset, controller.signal);
          if (controller.signal.aborted) return;
          setTrajectory((current) => {
            const existing = new Set((current.artifacts ?? []).map((item) => item.id));
            const appended = child.items.filter((item) => !existing.has(item.id));
            return appended.length ? { ...current, artifacts: [...(current.artifacts ?? []), ...appended] } : current;
          });
          if (!child.page.has_more || child.page.next_offset === undefined || child.page.next_offset <= offset) return;
          offset = child.page.next_offset;
        }
      };
      await Promise.all([
        loadRemainingSignals(),
        loadRemainingArtifacts(),
      ]);
    };
    run().catch(() => setLoading(false));
    return () => controller.abort();
  }, [initialTrajectory, routeVersion]);

  useEffect(() => applyPresentationTheme(presentation), [presentation]);
  useEffect(() => applyPresentationKeymap(presentation), [presentation]);

  useEffect(() => {
    const params = new URLSearchParams(globalThis.location?.search ?? "");
    const view = params.get("view");
    const source = validID(params.get("trajectory"), 256);
    const key = `${routeVersion}:${trajectory.id}:${view}:${params.get("left")}:${params.get("right")}`;
    if (!source || !trajectory.id || (view !== "group" && view !== "compare") || restoredView.current === key) return;
    restoredView.current = key;
    const controller = new AbortController();
    const restoreGroup = async () => {
      if (!trajectory.group_id) return;
      try {
        const restored = await loadGroup(source, trajectory.group_id, controller.signal);
        if (!controller.signal.aborted) setGroup(restored);
        try {
          const paths = await loadGroupPaths(source, trajectory.group_id, controller.signal);
          if (!controller.signal.aborted) setGroupPaths(paths);
        } catch (error) {
          if (!controller.signal.aborted) setGroupPathsError(error instanceof Error ? error.message : "Could not load compact paths");
        }
      } catch (error) {
        if (!controller.signal.aborted) setGroupError(error instanceof Error ? error.message : "Could not load group");
      }
    };
    if (view === "group") void restoreGroup();
    else {
      const left = validID(params.get("left"), 256);
      const right = validID(params.get("right"), 256);
      if (!left || !right || left === right) {
        replaceParams((next) => viewerKeys.forEach((item) => next.delete(item)));
        return () => controller.abort();
      }
      void restoreGroup();
      setGroupLoading(true); setGroupError("");
      loadComparison(source, left, right, controller.signal).then((result) => {
        if (controller.signal.aborted) return;
        const step = resolvedStep(params.get("step"), result);
        setComparisonStep(step); setComparison(result);
        replaceParams((next) => next.set("step", String(step)));
      }).catch((error) => {
        if (!controller.signal.aborted) setGroupError(error instanceof Error ? error.message : "Could not compare trajectories");
      }).finally(() => { if (!controller.signal.aborted) setGroupLoading(false); });
    }
    return () => controller.abort();
  }, [routeVersion, trajectory.group_id, trajectory.id]);

  useEffect(() => {
    const params = new URLSearchParams(globalThis.location?.search ?? "");
    const sourceId = params.get("trajectory") ?? "";
    if (params.get("indexed") !== "1" || !sourceId || isSample || !trajectory.id) {
      setAnalysis(null); setAnalysisLoading(false); setAnalysisError("");
      return;
    }
    if (indexSource?.index_state === "indexing" || indexSource?.index_state === "refreshing") {
      setAnalysis(null); setAnalysisLoading(true); setAnalysisError("");
      return;
    }
    const controller = new AbortController();
    setAnalysis(null); setAnalysisLoading(true); setAnalysisError("");
    loadAnalysis(sourceId, trajectory.id, controller.signal).then(setAnalysis).catch((error) => {
      if (!controller.signal.aborted) setAnalysisError(error instanceof Error ? error.message : "Could not analyze trajectory");
    }).finally(() => { if (!controller.signal.aborted) setAnalysisLoading(false); });
    return () => controller.abort();
  }, [analysisVersion, indexSource?.index_state, isSample, trajectory.id]);

  useEffect(() => {
    setSelectedArtifactId((current) => trajectory.artifacts?.some((artifact) => artifact.id === current) ? current : (trajectory.artifacts?.[0]?.id ?? ""));
  }, [trajectory.id, trajectory.artifacts]);

  const counts = useMemo(() => trajectory.events.reduce<Record<string, number>>((acc, e) => ({ ...acc, [e.kind]: (acc[e.kind] || 0) + 1 }), {}), [trajectory]);
  const visible = useMemo(() => trajectory.events.filter((event) => (filter === "all" || event.kind === filter) && (!query || eventText(event).includes(query.toLowerCase()))), [trajectory, filter, query]);
  const selected = trajectory.events.find((event) => event.id === selectedId) || visible[0] || trajectory.events[0];
  const selectedVisibleIndex = visible.findIndex((event) => event.id === selected.id);
  const analysisEventIds = useMemo(() => [...new Set((analysis?.analysis.findings ?? []).flatMap((finding) => finding.event_ids ?? []))], [analysis]);
  const resultsActive = filter !== "all" || query.length > 0;
  const railEntries = useMemo(() => {
    if (resultsActive) {
      const entries: Array<{ event: TrajectoryEvent; landmark: ReturnType<typeof deriveLandmark>; reason: string; pinned: boolean }> = visible.map((event) => ({ event, landmark: deriveLandmark(event), reason: "result", pinned: false }));
      if (!visible.some((event) => event.id === selected.id)) entries.unshift({ event: selected, landmark: deriveLandmark(selected), reason: "outside filter", pinned: true });
      return entries;
    }
    const byID = new Map(trajectory.events.map((event) => [event.id, event]));
    const complete = indexSource === null || indexSource.index_state === "complete";
    return deriveLandmarkRail(trajectory.events, trajectory.artifacts ?? [], analysis?.analysis.findings ?? [], [...(trajectory.signals ?? []), ...(analysis?.analysis.signals ?? [])], { selectedId: selected.id, complete })
      .flatMap((landmark) => {
        const event = byID.get(landmark.eventId);
        return event ? [{ event, landmark, reason: landmark.reason.replaceAll("_", " "), pinned: landmark.reason === "selected" }] : [];
      });
  }, [analysis, indexSource, resultsActive, selected, trajectory.artifacts, trajectory.events, trajectory.signals, visible]);
  const selectedRailIndex = railEntries.findIndex(({ event }) => event.id === selected.id);

  const selectEvent = (id: string) => {
    if (!trajectory.events.some((event) => event.id === id)) return;
    setSelectedId(id);
    replaceParams((params) => params.set("event", id));
  };
  const openSurface = (next: TrajectorySurface) => {
    setSurface(next);
    replaceParams((params) => next === "transcript" ? params.delete("surface") : params.set("surface", next));
  };

  useEffect(() => {
    if (surface !== "transcript") return;
    requestAnimationFrame(() => document.getElementById(`event-${selectedId}`)?.scrollIntoView({ block: "nearest", behavior: "instant" }));
  }, [selectedId, surface, trajectory.id]);

  const move = (delta: number, predicate?: (event: TrajectoryEvent) => boolean) => {
    const candidates = predicate ? visible.filter(predicate) : visible;
    if (!candidates.length) return;
    const index = candidates.findIndex((event) => event.id === selectedId);
    const next = candidates[index < 0 ? 0 : (index + delta + candidates.length) % candidates.length];
    selectEvent(next.id);
  };
  const toggleExpand = (id: string) => setExpanded((current) => { const next = new Set(current); next.has(id) ? next.delete(id) : next.add(id); return next; });
  const jumpToEvent = (id: string) => { setFilter("all"); setQuery(""); setSearchOpen(false); selectEvent(id); };
  const moveAnalysis = () => {
    if (!analysisEventIds.length) return;
    const index = analysisEventIds.indexOf(selectedId);
    jumpToEvent(analysisEventIds[(index + 1 + analysisEventIds.length) % analysisEventIds.length]);
  };
  const selectArtifact = (artifact: TrajectoryArtifact) => {
    setSelectedArtifactId(artifact.id);
    if (artifact.event_id && trajectory.events.some((event) => event.id === artifact.event_id)) jumpToEvent(artifact.event_id);
  };
  const moveArtifact = () => {
    const artifacts = trajectory.artifacts ?? [];
    if (!artifacts.length) return;
    const index = artifacts.findIndex((artifact) => artifact.id === selectedArtifactId);
    selectArtifact(artifacts[(index + 1 + artifacts.length) % artifacts.length]);
  };
  const openGroup = async () => {
    const params = new URLSearchParams(globalThis.location?.search ?? "");
    const sourceId = params.get("trajectory") ?? "";
    if (!sourceId || !trajectory.group_id) return;
    setGroupLoading(true); setGroupError(""); setGroupPaths(null); setGroupPathsError("");
    try {
      setGroup(await loadGroup(sourceId, trajectory.group_id));
      replaceParams((next) => { next.set("view", "group"); for (const key of ["left", "right", "step"]) next.delete(key); });
      try { setGroupPaths(await loadGroupPaths(sourceId, trajectory.group_id)); }
      catch (error) { setGroupPathsError(error instanceof Error ? error.message : "Could not load compact paths"); }
    }
    catch (error) { setGroupError(error instanceof Error ? error.message : "Could not load group"); }
    finally { setGroupLoading(false); }
  };
  const openGroupTrajectory = (id: string) => {
    const params = new URLSearchParams(globalThis.location?.search ?? "");
    params.set("indexed", "1"); params.set("trajectory_id", id);
    params.delete("event"); viewerKeys.forEach((key) => params.delete(key));
    globalThis.history?.replaceState({}, "", `${globalThis.location.pathname}?${params}${globalThis.location.hash}`);
    setGroup(null); setLoading(true);
    setRouteVersion((version) => version + 1);
  };
  const openComparison = async (left: string, right: string) => {
    const sourceId = new URLSearchParams(globalThis.location?.search ?? "").get("trajectory") ?? "";
    if (!sourceId) return;
    setGroupLoading(true); setGroupError("");
    try {
      const result = await loadComparison(sourceId, left, right);
      const step = result.alignment.first_meaningful_divergence ?? 0;
      setComparisonStep(step); setComparison(result);
      replaceParams((next) => { next.set("view", "compare"); next.set("left", left); next.set("right", right); next.set("step", String(step)); });
    }
    catch (error) { setGroupError(error instanceof Error ? error.message : "Could not compare trajectories"); }
    finally { setGroupLoading(false); }
  };

  useCommands("trajectory", {
    [commandIds.trajectory.dismiss]: () => { setSearchOpen(false); setHelp(false); searchRef.current?.blur(); },
    [commandIds.trajectory.search]: () => { setSearchOpen(true); requestAnimationFrame(() => searchRef.current?.focus()); },
    [commandIds.trajectory.next]: () => move(1),
    [commandIds.trajectory.previous]: () => move(-1),
    [commandIds.trajectory.nextError]: () => move(1, (item) => item.kind === "error"),
    [commandIds.trajectory.nextReward]: () => move(1, (item) => item.kind === "reward" || item.kind === "grader"),
    [commandIds.trajectory.nextContext]: () => move(1, isContextEvent),
    [commandIds.trajectory.nextFinding]: moveAnalysis,
    [commandIds.trajectory.nextArtifact]: moveArtifact,
    [commandIds.trajectory.toggleRaw]: () => setRaw((value) => !value),
    [commandIds.trajectory.openGroup]: () => trajectory.group_id ? void openGroup() : false,
    [commandIds.trajectory.toggleHelp]: () => setHelp((value) => !value),
    [commandIds.trajectory.toggleExpanded]: () => selected ? toggleExpand(selected.id) : false,
    [commandIds.trajectory.openTranscript]: () => openSurface("transcript"),
    [commandIds.trajectory.openTimeline]: () => openSurface("timeline"),
    [commandIds.trajectory.openOutcome]: () => openSurface("outcome"),
  }, !group && !comparison && !help);

  if (comparison) return <div className="app-shell group-shell comparison-shell">
    <header className="topbar"><div className="brand"><span className="brand-mark">RV</span><span>RLViz</span></div><div className="crumb"><span>{trajectory.run_id || "local run"}</span><b>/</b><strong>comparison</strong></div>{isDemo && <div className="top-actions"><span className="demo-pill">Synthetic demo</span></div>}</header>
    <ComparisonView comparison={comparison} initialStep={comparisonStep} onStepChange={(step) => { setComparisonStep(step); replaceParams((params) => params.set("step", String(step))); }} onClose={() => { setComparison(null); replaceParams((params) => { params.set("view", "group"); for (const key of ["left", "right", "step"]) params.delete(key); }); }} />
  </div>;
  if (group) return <div className="app-shell group-shell">
    <header className="topbar"><div className="brand"><span className="brand-mark">RV</span><span>RLViz</span></div><div className="crumb"><span>{trajectory.run_id || "local run"}</span><b>/</b><strong>{group.group_id}</strong></div>{(groupError || isDemo) && <div className="top-actions">{groupError && <span className="group-error">{groupError}</span>}{isDemo && <span className="demo-pill">Synthetic demo</span>}</div>}</header>
    <GroupView group={group} presentation={presentation} paths={groupPaths} pathsError={groupPathsError} initialQuery={validID(new URLSearchParams(globalThis.location?.search ?? "").get("cohort_filter"), 1024) ?? ""} onQueryChange={(value) => replaceParams((params) => value ? params.set("cohort_filter", value) : params.delete("cohort_filter"))} onClose={() => { setGroup(null); replaceParams((params) => viewerKeys.forEach((key) => params.delete(key))); }} onOpen={openGroupTrajectory} onCompare={(left, right) => void openComparison(left, right)} />
  </div>;
  if (!selected) return <main className="empty">No events in this trajectory.</main>;
  return (
    <div className="app-shell">
      <header className="topbar">
        <div className="brand"><span className="brand-mark">RV</span><span>RLViz</span></div>
        <div className="crumb"><span>{trajectory.run_id || "local run"}</span><b>/</b><strong>{trajectory.name || trajectory.id}</strong></div>
        <div className="top-actions">{groupError && <span className="group-error">{groupError}</span>}{indexSource?.index_state === "failed" && <span className="index-state failed" title={indexSource.index_error}>Index failed{indexSource.index_error ? `: ${indexSource.index_error}` : ""}</span>}{(indexSource?.index_state === "indexing" || indexSource?.index_state === "refreshing") && <span className="index-state"><i></i>{indexSource.index_state === "refreshing" ? "Refreshing" : "Indexing"}</span>}{loading && <span className="loading">Connecting…</span>}{isDemo && <span className="demo-pill">Synthetic demo</span>}{isSample && !loading && <span className="demo-pill" title="The local API was unavailable">Sample data</span>}<button onClick={() => setHelp(true)} className="icon-button" aria-label="Keyboard shortcuts">?</button></div>
      </header>
      <div className="contextbar">
        <div className="status"><span className={`status-dot ${trajectory.status}`}></span>{trajectory.status || "complete"}</div>
        <div><span>MODEL</span>{trajectory.model || "—"}</div><div><span>EVENTS</span>{trajectory.events.length < eventTotal ? `${trajectory.events.length}/${eventTotal}` : eventTotal}</div><div><span>DURATION</span>{duration(trajectory.duration_ms)}</div><div><span>REWARD</span><strong className={(trajectory.total_reward || 0) < 0 ? "negative" : "positive"}>{trajectory.total_reward ?? "—"}</strong></div>
        <div className="context-spacer"></div><div><span>CASE</span>{trajectory.case_id || "—"}</div><div><span>GROUP</span>{trajectory.group_id ? <button className="context-link" onClick={() => void openGroup()} disabled={groupLoading}>{groupLoading ? "loading…" : trajectory.group_id} <kbd>{bindingLabel(commandIds.trajectory.openGroup)}</kbd></button> : "—"}</div>
      </div>
      <div className="workspace">
        <aside className="outline">
          <div className="panel-heading"><span>{resultsActive ? "Results" : "Landmarks"}</span><span>{resultsActive ? visible.length : railEntries.length} · {trajectory.events.length < eventTotal ? `${trajectory.events.length}/${eventTotal}` : trajectory.events.length} events</span></div>
          <div className={`search ${searchOpen ? "open" : ""}`}><span>⌕</span><input ref={searchRef} value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search all events" aria-label="Search events" /><kbd>{bindingLabel(commandIds.trajectory.search)}</kbd></div>
          <div className="filters">{filterKinds.filter((kind) => kind === "all" || counts[kind]).map((kind) => <button key={kind} className={filter === kind ? "active" : ""} onClick={() => setFilter(kind)}><span>{kind}</span><b>{kind === "all" ? trajectory.events.length : counts[kind]}</b></button>)}</div>
          <nav ref={outlineRef} className="event-outline" aria-label={resultsActive ? "Trajectory search results" : "Trajectory landmarks"}>
            <VirtualList items={railEntries} estimateSize={47} overscan={6} selectedIndex={selectedRailIndex} scrollRef={outlineRef} className="outline-virtual" itemKey={({ event }) => event.id} renderItem={({ event, landmark, reason, pinned }, index) => <button className={`${selected.id === event.id ? "active" : ""} ${pinned ? "pinned" : ""}`} aria-current={selected.id === event.id ? "true" : undefined} aria-posinset={index + 1} aria-setsize={railEntries.length} onClick={() => selectEvent(event.id)}><Kind kind={event.kind} /><span className="outline-text"><b>{landmark.label ?? title(event)}</b><small>{reason} · {landmark.category}</small></span><span className="outline-seq">{event.sequence}</span></button>} />
          </nav>
          {!visible.length && <div className="no-results">No matching events</div>}
        </aside>
        <main ref={timelineRef} className="timeline" aria-label="Trajectory workspace">
          <div className="trajectory-header">
            <div className="timeline-heading"><div><h1>{trajectory.name || trajectory.id}</h1><p>{trajectory.id} · {trajectory.started_at ? new Date(trajectory.started_at).toLocaleString() : "local trajectory"}</p></div><TrajectoryTabs active={surface} onChange={openSurface} /></div>
            <ContextTrack events={trajectory.events} eventTotal={eventTotal} selectedId={selected.id} onSelect={jumpToEvent} />
          </div>
          {surface === "transcript" && <TranscriptView events={visible} selectedId={selected.id} selectedIndex={selectedVisibleIndex} scrollRef={timelineRef} onSelect={selectEvent} onVisibleRangeChange={setVisibleRange} />}
          {surface === "timeline" && <VirtualList items={visible} estimateSize={118} overscan={4} selectedIndex={selectedVisibleIndex} scrollRef={timelineRef} className="timeline-events" itemKey={eventKey} renderItem={(event, index) => <TimelineCard event={event} selected={selected.id === event.id} expanded={expanded.has(event.id)} position={index + 1} total={visible.length} onSelect={() => selectEvent(event.id)} onExpand={() => toggleExpand(event.id)} />} onVisibleRangeChange={setVisibleRange} />}
          {surface === "outcome" && <OutcomeView trajectory={trajectory} onSelect={(id) => { selectEvent(id); openSurface("transcript"); }} />}
        </main>
        <Inspector event={selected} raw={raw} presentation={presentation} analysis={analysis} analysisLoading={analysisLoading} analysisError={analysisError} onRetryAnalysis={() => setAnalysisVersion((version) => version + 1)} onJump={jumpToEvent} artifacts={trajectory.artifacts ?? []} sourceId={sourceId} trajectoryId={trajectory.id} selectedArtifactId={selectedArtifactId} onSelectArtifact={selectArtifact} />
      </div>
      <TrajectoryOverview events={trajectory.events} eventTotal={eventTotal} visibleEvents={visible} selectedId={selected.id} visibleRange={surface === "outcome" ? undefined : visibleRange} onSelect={selectEvent} />
      <footer className="keybar"><span><kbd>{bindingLabel(commandIds.trajectory.next)}</kbd><kbd>{bindingLabel(commandIds.trajectory.previous)}</kbd> navigate</span>{surface === "timeline" && <span><kbd>{bindingLabel(commandIds.trajectory.toggleExpanded)}</kbd> expand</span>}{trajectory.group_id && <span><kbd>{bindingLabel(commandIds.trajectory.openGroup)}</kbd> group</span>}{analysisEventIds.length > 0 && <span><kbd>{bindingLabel(commandIds.trajectory.nextFinding)}</kbd> finding</span>}{(trajectory.artifacts?.length ?? 0) > 0 && <span><kbd>{bindingLabel(commandIds.trajectory.nextArtifact)}</kbd> artifact</span>}{trajectory.events.some(isContextEvent) && <span><kbd>{bindingLabel(commandIds.trajectory.nextContext)}</kbd> context</span>}<span><kbd>{bindingLabel(commandIds.trajectory.nextError)}</kbd> error</span><span><kbd>{bindingLabel(commandIds.trajectory.nextReward)}</kbd> reward</span><span><kbd>{bindingLabel(commandIds.trajectory.toggleRaw)}</kbd> raw</span><span><kbd>{bindingLabel(commandIds.trajectory.toggleHelp)}</kbd> shortcuts</span></footer>
      <KeymapDialog open={help} onClose={() => setHelp(false)} />
    </div>
  );
}
