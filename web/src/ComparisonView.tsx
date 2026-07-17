import { useEffect, useMemo, useRef, useState } from "react";
import { InlineArtifacts } from "./ArtifactPanel";
import { bindingLabel, commandIds, useCommands, useKeymapRevision } from "./commands";
import { json, preview, title } from "./format";
import type { ComparisonResponse, TrajectoryEvent } from "./types";
import { VirtualList } from "./VirtualList";

function differenceValue(value: unknown): string {
  if (value === undefined || value === "") return "—";
  return typeof value === "object" ? preview(value, 60) : String(value);
}

function EventLane({ event, side }: { event?: TrajectoryEvent; side: "left" | "right" }) {
  if (!event) return <div className={`compare-lane ${side} gap`}><span>— gap —</span></div>;
  const detail = event.output ?? event.input ?? event.content ?? event.data;
  return <div className={`compare-lane ${side}`}>
    <header><span>{event.kind.replaceAll("_", " ")}</span><b>#{event.sequence}</b></header>
    <strong>{title(event)}</strong>
    {detail !== undefined && <pre>{preview(detail, 180)}</pre>}
  </div>;
}

function metric(label: string, left: unknown, right: unknown, changed = false) {
  return <div className={changed ? "changed" : ""} key={label}><span>{label}</span><b>{differenceValue(left)}</b><i>→</i><b>{differenceValue(right)}</b></div>;
}

export function ComparisonView({ comparison, onClose, initialStep, onStepChange }: { comparison: ComparisonResponse; onClose: () => void; initialStep?: number; onStepChange?: (step: number) => void }) {
  useKeymapRevision();
  const { alignment, left, right, differences } = comparison;
  const first = alignment.first_meaningful_divergence;
  const realignment = alignment.later_realignment;
  const changes = useMemo(() => alignment.steps.map((step, index) => ({ step, index })).filter(({ step }) => step.operation !== "match" || step.meaningful), [alignment.steps]);
  const validInitialStep = initialStep !== undefined && initialStep >= 0 && initialStep < alignment.steps.length ? initialStep : (first ?? 0);
  const [selected, setSelected] = useState(validInitialStep);
  const alignmentRef = useRef<HTMLElement>(null);
  const prefixEnd = first ?? alignment.steps.length;
  const hiddenPrefix = prefixEnd > 2 ? prefixEnd - 1 : 0;
  const displayed = useMemo(() => alignment.steps.map((step, index) => ({ step, index })).filter(({ index }) => !hiddenPrefix || index >= prefixEnd - 1), [alignment.steps, hiddenPrefix, prefixEnd]);

  const selectAndReveal = (index: number) => {
    if (index < 0 || index >= alignment.steps.length) return;
    setSelected(index);
    onStepChange?.(index);
  };
  const move = (delta: number) => selectAndReveal((selected + delta + alignment.steps.length) % alignment.steps.length);
  const nextChange = () => {
    const next = changes.find(({ index }) => index > selected) ?? changes[0];
    if (next) selectAndReveal(next.index);
  };

  useEffect(() => {
    setSelected(validInitialStep);
  }, [comparison, validInitialStep]);

  useCommands("comparison", {
    [commandIds.comparison.back]: onClose,
    [commandIds.comparison.next]: () => move(1),
    [commandIds.comparison.previous]: () => move(-1),
    [commandIds.comparison.firstDivergence]: () => first !== undefined ? void selectAndReveal(first) : false,
    [commandIds.comparison.nextChange]: nextChange,
  });

  const selectedStep = alignment.steps[selected];
  const selectedLeft = selectedStep?.left_index === undefined ? undefined : left.events[selectedStep.left_index];
  const selectedRight = selectedStep?.right_index === undefined ? undefined : right.events[selectedStep.right_index];

  return <main className="comparison-view" aria-label="Trajectory comparison">
    <header className="comparison-heading">
      <div><span className="eyebrow">Pair comparison</span><h1><b>{left.trajectory.id}</b><i>vs</i><b>{right.trajectory.id}</b></h1></div>
      <button onClick={onClose}>Back to group <kbd>{bindingLabel(commandIds.comparison.back)}</kbd></button>
    </header>
    <section className="comparison-metrics" aria-label="Trajectory differences">
      {metric("REWARD", differences.reward.left, differences.reward.right, differences.reward.changed)}
      {metric("STATUS", differences.status.left, differences.status.right, differences.status.changed)}
      {metric("TERMINATION", differences.termination.left, differences.termination.right, differences.termination.changed)}
      {metric("EVENTS", differences.event_count.left, differences.event_count.right, differences.event_count.delta !== 0)}
    </section>
    <div className="comparison-workspace">
      <section ref={alignmentRef} className="comparison-alignment" aria-label="Aligned events">
        <div className="lane-headings"><b>{left.trajectory.id}</b><span>alignment</span><b>{right.trajectory.id}</b></div>
        {hiddenPrefix > 0 && <button className="prefix-compression" onClick={() => selectAndReveal(0)}><span>{hiddenPrefix} aligned prefix events compressed</span><small>{alignment.common_behavioral_prefix} shared behavioral anchors</small></button>}
        <VirtualList items={displayed} estimateSize={97} overscan={5} selectedIndex={displayed.findIndex(({ index }) => index === selected)} scrollRef={alignmentRef} className="comparison-rows" itemKey={({ index }) => String(index)} renderItem={({ step, index }) => <div>
          {index === first && <div className="divergence-marker" role="note">First meaningful divergence <kbd>{bindingLabel(commandIds.comparison.firstDivergence)}</kbd></div>}
          {index === realignment && <div className="realignment-marker" role="note">Later behavioral realignment</div>}
          <button id={`compare-step-${index}`} className={`alignment-row operation-${step.operation} ${selected === index ? "selected" : ""}`} onClick={() => selectAndReveal(index)} aria-label={`Alignment step ${index + 1}: ${step.operation}`}>
            <EventLane event={step.left_index === undefined ? undefined : left.events[step.left_index]} side="left" />
            <span className="operation"><b>{step.operation}</b><small>{index + 1}</small></span>
            <EventLane event={step.right_index === undefined ? undefined : right.events[step.right_index]} side="right" />
          </button>
        </div>} />
      </section>
      <aside className="comparison-inspector">
        <div className="panel-heading"><span>Raw event payloads</span><span>step {selected + 1}</span></div>
        <InlineArtifacts artifacts={left.artifacts} eventId={selectedLeft?.id} label={left.trajectory.id} />
        <section><h3>{left.trajectory.id}</h3>{selectedLeft ? <pre>{json(selectedLeft.raw ?? selectedLeft)}</pre> : <div className="raw-gap">No event on this side</div>}</section>
        <InlineArtifacts artifacts={right.artifacts} eventId={selectedRight?.id} label={right.trajectory.id} />
        <section><h3>{right.trajectory.id}</h3>{selectedRight ? <pre>{json(selectedRight.raw ?? selectedRight)}</pre> : <div className="raw-gap">No event on this side</div>}</section>
      </aside>
    </div>
    <footer className="group-keybar"><span><kbd>{bindingLabel(commandIds.comparison.next)}</kbd><kbd>{bindingLabel(commandIds.comparison.previous)}</kbd> step</span><span><kbd>{bindingLabel(commandIds.comparison.firstDivergence)}</kbd> first divergence</span><span><kbd>{bindingLabel(commandIds.comparison.nextChange)}</kbd> next change</span><span><kbd>{bindingLabel(commandIds.comparison.back)}</kbd> group</span></footer>
  </main>;
}
