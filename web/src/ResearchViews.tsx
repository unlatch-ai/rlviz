import { useMemo } from "react";
import type { RefObject } from "react";
import { preview } from "./format";
import { deriveMessage, deriveOutcome, deriveTool, isContextEvent } from "./research";
import type { OutcomeSummary, ResearchMessage } from "./research";
import type { Trajectory, TrajectoryEvent } from "./types";
import { VirtualList } from "./VirtualList";
import type { VisibleRange } from "./VirtualList";

function DisplayValue({ value, className = "" }: { value: unknown; className?: string }) {
  if (value === undefined) return null;
  if (typeof value === "string") return <div className={`research-prose ${className}`}>{value}</div>;
  return <pre className={`research-data ${className}`}>{preview(value, 8_000)}</pre>;
}

function TranscriptEvent({ event, selected, onSelect }: { event: TrajectoryEvent; selected: boolean; onSelect: () => void }) {
  const message = deriveMessage(event);
  const tool = deriveTool(event);
  if (message) {
    const role = message.role?.value ?? "message";
    return <article id={`event-${event.id}`} className={`transcript-entry transcript-${role} ${selected ? "selected" : ""}`} onClick={onSelect}>
      <header><span className="transcript-role">{role}</span><span>#{event.sequence}</span>{message.role?.provenance === "inferred" && <span className="provenance" title={message.role.source}>inferred</span>}</header>
      <DisplayValue value={message.content} />
    </article>;
  }
  if (tool) {
    return <article id={`event-${event.id}`} className={`transcript-entry transcript-tool ${selected ? "selected" : ""}`} onClick={onSelect}>
      <header><span className="transcript-role">tool</span><strong>{tool.name?.value ?? "unnamed tool"}</strong><span>#{event.sequence}</span>{tool.name?.provenance === "inferred" && <span className="provenance" title={tool.name.source}>inferred</span>}</header>
      {tool.call !== undefined && <section><span>Call</span><DisplayValue value={tool.call} /></section>}
      {tool.result !== undefined && <section><span>Result</span><DisplayValue value={tool.result} /></section>}
    </article>;
  }
  const context = isContextEvent(event);
  return <article id={`event-${event.id}`} className={`transcript-entry transcript-${context ? "context" : event.kind} ${selected ? "selected" : ""}`} onClick={onSelect}>
    <header><span className="transcript-role">{context ? "context change" : event.kind.replaceAll("_", " ")}</span><strong>{event.title ?? event.summary ?? event.kind}</strong><span>#{event.sequence}</span></header>
    <DisplayValue value={event.content ?? event.data ?? event.output ?? event.input} />
  </article>;
}

export function TranscriptView({ events, selectedId, selectedIndex, scrollRef, onSelect, onVisibleRangeChange }: { events: TrajectoryEvent[]; selectedId: string; selectedIndex: number; scrollRef: RefObject<HTMLElement | null>; onSelect: (id: string) => void; onVisibleRangeChange?: (range: VisibleRange) => void }) {
  const turnNumbers = useMemo(() => {
    let turn = 0;
    return events.map((event, index) => {
      if (deriveMessage(event)?.role?.value === "user") turn += 1;
      return turn || (index === 0 ? 0 : -1);
    });
  }, [events]);
  return <div className="transcript-view" aria-label="Trajectory transcript">
    <VirtualList items={events} estimateSize={150} overscan={4} selectedIndex={selectedIndex} scrollRef={scrollRef} className="transcript-events" itemKey={(event) => event.id} renderItem={(event, index) => <div className="transcript-row">
      {(index === 0 || deriveMessage(event)?.role?.value === "user") && <div className="turn-marker"><span>{turnNumbers[index] > 0 ? `Turn ${turnNumbers[index]}` : "Preamble"}</span><small>grouping inferred from message roles</small></div>}
      <TranscriptEvent event={event} selected={event.id === selectedId} onSelect={() => onSelect(event.id)} />
    </div>} onVisibleRangeChange={onVisibleRangeChange} />
  </div>;
}

function OutcomeStat({ label, value, tone = "" }: { label: string; value: unknown; tone?: string }) {
  return <div className={`outcome-stat ${tone}`}><span>{label}</span><strong>{value === undefined || value === "" ? "—" : String(value)}</strong></div>;
}

function FinalOutput({ message, outcome, onSelect }: { message: ResearchMessage; outcome: OutcomeSummary; onSelect: (id: string) => void }) {
  return <section className="outcome-section final-output">
    <header><div><span className="eyebrow">Final output</span><h2>{message.role?.value ?? "assistant"}</h2></div><button type="button" onClick={() => onSelect(message.eventId)}>Event {message.eventId}</button></header>
    <DisplayValue value={message.content} />
    {outcome.finalOutputSelection?.provenance === "inferred" && <p className="provenance-note">Selected as the last assistant message because the source did not identify a final output.</p>}
  </section>;
}

export function OutcomeView({ trajectory, onSelect }: { trajectory: Trajectory; onSelect: (id: string) => void }) {
  const outcome = deriveOutcome(trajectory, trajectory.signals);
  const pass = outcome.pass?.value;
  return <div className="outcome-view" aria-label="Trajectory outcome">
    <div className="outcome-summary">
      <OutcomeStat label="Result" value={pass === undefined ? outcome.status : pass ? "Pass" : "Fail"} tone={pass === true ? "success" : pass === false ? "danger" : ""} />
      <OutcomeStat label="Reward" value={outcome.reward.total?.value} />
      <OutcomeStat label="Termination" value={outcome.termination} />
      <OutcomeStat label="Errors" value={outcome.errorEventIds.length} tone={outcome.errorEventIds.length ? "danger" : ""} />
    </div>
    {outcome.finalOutput && <FinalOutput message={outcome.finalOutput} outcome={outcome} onSelect={onSelect} />}
    {outcome.graders.length > 0 && <section className="outcome-section"><header><div><span className="eyebrow">Evaluation</span><h2>Graders</h2></div><span>{outcome.graders.length}</span></header><div className="grader-list">
      {outcome.graders.map((grader) => <article key={grader.eventId}><button type="button" onClick={() => onSelect(grader.eventId)}><strong>{String(grader.verdict ?? "grader")}</strong>{grader.score !== undefined && <span>score {String(grader.score)}</span>}</button>{grader.reason !== undefined && <DisplayValue value={grader.reason} />}{grader.evidenceEventIds.length > 0 && <div className="evidence"><span>Evidence</span>{grader.evidenceEventIds.map((id) => <button type="button" key={id} onClick={() => onSelect(id)}>{id}</button>)}</div>}</article>)}
    </div></section>}
    {Object.keys(outcome.reward.components).length > 0 && <section className="outcome-section"><header><div><span className="eyebrow">Reward</span><h2>Components</h2></div></header><dl className="reward-components">{Object.entries(outcome.reward.components).map(([name, value]) => <div key={name}><dt>{name.replaceAll("_", " ")}</dt><dd className={value < 0 ? "negative" : value > 0 ? "positive" : ""}>{value}</dd></div>)}</dl></section>}
    {outcome.errorEventIds.length > 0 && <section className="outcome-section error-evidence"><header><div><span className="eyebrow">Failure evidence</span><h2>Errors</h2></div></header><div className="evidence">{outcome.errorEventIds.map((id) => <button type="button" key={id} onClick={() => onSelect(id)}>{id}</button>)}</div></section>}
  </div>;
}
