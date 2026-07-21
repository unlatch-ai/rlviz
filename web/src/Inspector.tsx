import { Fragment } from "react";
import { AnalysisPanel } from "./AnalysisPanel";
import { ArtifactPanel } from "./ArtifactPanel";
import { bindingLabel, commandIds } from "./commands";
import { ContextDetails } from "./ContextDetails";
import { duration, json } from "./format";
import { Kind } from "./Kind";
import { presentationInspectorSections } from "./presentation";
import { deriveLandmark } from "./research";
import type { AnalysisResponse, PresentationConfig, PresentationInspectorSectionID, TrajectoryArtifact, TrajectoryEvent } from "./types";

interface InspectorProps {
  event: TrajectoryEvent;
  raw: boolean;
  presentation?: PresentationConfig;
  analysis: AnalysisResponse | null;
  analysisLoading: boolean;
  analysisError: string;
  onRetryAnalysis: () => void;
  onJump: (id: string) => void;
  artifacts: TrajectoryArtifact[];
  sourceId: string;
  trajectoryId: string;
  selectedArtifactId: string;
  onSelectArtifact: (artifact: TrajectoryArtifact) => void;
}

export function Inspector({ event, raw, presentation, analysis, analysisLoading, analysisError, onRetryAnalysis, onJump, artifacts, sourceId, trajectoryId, selectedArtifactId, onSelectArtifact }: InspectorProps) {
  const landmark = deriveLandmark(event);
  const linkedArtifacts = artifacts.filter((artifact) => artifact.event_id === event.id);
  const trajectoryArtifacts = artifacts.filter((artifact) => artifact.event_id !== event.id);
  const entries = [
    ["Event ID", event.id], ["Sequence", event.sequence], ["Kind", event.kind], ["Time", event.timestamp],
    ["Duration", event.duration_ms === undefined ? undefined : duration(event.duration_ms)], ["Tokens", event.token_count],
    ["Reward", event.reward], ["Parent", event.parent_id], ["Alignment", event.alignment_key], ["State hash", event.state_hash],
  ].filter((entry) => entry[1] !== undefined) as [string, unknown][];

  const renderSection = (section: PresentationInspectorSectionID) => {
    switch (section) {
      case "properties": return <section><h4>Properties</h4><dl>{entries.map(([key, value]) => <div key={key}><dt>{key}</dt><dd>{String(value)}</dd></div>)}</dl></section>;
      case "context": return <ContextDetails event={event} onJump={onJump} />;
      case "source": return event.source ? <section><h4>Source</h4><div className="source-path">{event.source.path || "Unknown source"}</div><div className="source-detail">{event.source.line && `line ${event.source.line}`}{(event.source.byte_offset ?? event.source.byte_start) !== undefined && ` · bytes ${event.source.byte_offset ?? event.source.byte_start}–${event.source.byte_length !== undefined ? (event.source.byte_offset ?? 0) + event.source.byte_length : (event.source.byte_end ?? "?")}`}</div></section> : null;
      case "input": return event.input !== undefined ? <section><h4>Input</h4><pre className="raw-json compact">{json(event.input)}</pre></section> : null;
      case "output": return event.output !== undefined ? <section><h4>Output</h4><pre className="raw-json compact">{json(event.output)}</pre></section> : null;
      case "content": return (event.content ?? event.data) !== undefined ? <section><h4>Content</h4><pre className="raw-json compact">{typeof (event.content ?? event.data) === "string" ? String(event.content ?? event.data) : json(event.content ?? event.data)}</pre></section> : null;
      case "metadata": return event.metadata ? <section><h4>Metadata</h4><pre className="raw-json compact">{json(event.metadata)}</pre></section> : null;
      case "linked_artifacts": return <ArtifactPanel artifacts={linkedArtifacts} sourceId={sourceId} trajectoryId={trajectoryId} selectedId={selectedArtifactId} onSelect={onSelectArtifact} label="Linked artifacts" />;
      case "analysis": return <AnalysisPanel analysis={analysis} loading={analysisLoading} error={analysisError} onRetry={onRetryAnalysis} onJump={onJump} />;
      case "other_artifacts": return <ArtifactPanel artifacts={trajectoryArtifacts} sourceId={sourceId} trajectoryId={trajectoryId} selectedId={selectedArtifactId} onSelect={onSelectArtifact} label="Other artifacts" />;
    }
  };

  return <aside className="inspector">
    <div className="panel-heading"><span>Details</span>{bindingLabel(commandIds.trajectory.toggleRaw) && <span className="panel-hint">{bindingLabel(commandIds.trajectory.toggleRaw)} raw</span>}</div>
    <div className="selected-heading"><Kind kind={event.kind} /><div><h3>{landmark.label}</h3><span>event {event.sequence}</span></div></div>
    <div className="inspector-scroll">
      {raw
        ? <section><h4>Raw normalized record</h4><pre className="raw-json">{json(event.raw ?? event)}</pre></section>
        : presentationInspectorSections(presentation).map((section) => <Fragment key={section}>{renderSection(section)}</Fragment>)}
    </div>
  </aside>;
}
