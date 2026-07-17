import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { deriveOverviewBins, stableOverviewEvents, TrajectoryOverview } from "./TrajectoryOverview";
import type { TrajectoryEvent } from "./types";

const events: TrajectoryEvent[] = [
  { id: "model", sequence: 1, kind: "generation" },
  { id: "message", sequence: 2, kind: "message" },
  { id: "tool", sequence: 3, kind: "tool" },
  { id: "state", sequence: 4, kind: "state", alignment_key: "context:compaction" },
  { id: "reward", sequence: 5, kind: "reward" },
  { id: "error", sequence: 6, kind: "error" },
];

describe("TrajectoryOverview", () => {
  it("aggregates stable model, interaction, and evaluation lanes", () => {
    expect(deriveOverviewBins(events, 8, 4)).toEqual([
      { model: 2, interaction: 0, evaluation: 0 },
      { model: 0, interaction: 2, evaluation: 0 },
      { model: 0, interaction: 0, evaluation: 2 },
      { model: 0, interaction: 0, evaluation: 0 },
    ]);
    const duplicateSequences = [events[2], events[0], { ...events[1], sequence: 1 }];
    expect(stableOverviewEvents(duplicateSequences).map((event) => event.id)).toEqual(["model", "message", "tool"]);
  });

  it("shows partial extent, viewport, and selection without context landmarks", () => {
    const { container } = render(<TrajectoryOverview events={events} eventTotal={12} visibleEvents={events} selectedId="tool" visibleRange={{ start: 1, end: 4 }} onSelect={() => {}} />);
    expect(screen.getByText("6/12 loaded")).toBeInTheDocument();
    expect(container.querySelector(".overview-unavailable")).toBeInTheDocument();
    expect(container.querySelector(".overview-window")).toBeInTheDocument();
    expect(container.querySelector(".overview-selection")).toBeInTheDocument();
    expect(container.querySelector(".context-marker")).not.toBeInTheDocument();
  });

  it("selects the nearest loaded event that survives filtering", () => {
    const onSelect = vi.fn();
    render(<TrajectoryOverview events={events} eventTotal={events.length} visibleEvents={[events[0], events[5]]} selectedId="model" visibleRange={{ start: 0, end: 1 }} onSelect={onSelect} />);
    fireEvent.change(screen.getByRole("slider", { name: "Trajectory overview position" }), { target: { value: "4" } });
    expect(onSelect).toHaveBeenCalledWith("error");
  });

  it("bounds overview DOM for a 10,000-event run", () => {
    const large = Array.from({ length: 10_000 }, (_, index) => ({ id: `event-${index}`, sequence: index + 1, kind: "generation" }));
    const { container } = render(<TrajectoryOverview events={large} eventTotal={large.length} visibleEvents={large} selectedId="event-0" onSelect={() => {}} />);
    expect(container.querySelectorAll(".overview-lane i")).toHaveLength(192);
  });
});
