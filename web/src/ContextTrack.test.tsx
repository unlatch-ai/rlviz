import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ContextDetails } from "./ContextDetails";
import { ContextTrack } from "./ContextTrack";
import type { TrajectoryEvent } from "./types";

const contextEvents: TrajectoryEvent[] = [
  { id: "after", sequence: 20, kind: "state", context: { operation: "compaction", input_tokens_before: 8000, input_tokens: 2000, capacity: 10000, provenance: "source_native", retained_event_ids: ["before"], summarized_event_ids: ["prompt"], summary: "Earlier turns summarized." } },
  { id: "prompt", sequence: 1, kind: "message", input: { role: "user", content: "Task" } },
  { id: "before", sequence: 10, kind: "state", context: { input_tokens: 6000, provenance: "adapter_derived", derivation: "tokenizer count" } },
  { id: "legacy", sequence: 30, kind: "state", alignment_key: "context:truncation" },
];

describe("ContextTrack", () => {
  it("stays absent when the trajectory has no context evidence", () => {
    render(<ContextTrack events={[contextEvents[1]]} eventTotal={1} selectedId="prompt" onSelect={() => {}} />);
    expect(screen.queryByRole("navigation", { name: "Context events" })).not.toBeInTheDocument();
  });

  it("renders discrete source-ordered markers without interpolating unknown gaps", () => {
    const { container } = render(<ContextTrack events={contextEvents} eventTotal={9} selectedId="after" onSelect={() => {}} />);
    const track = screen.getByRole("navigation", { name: "Context events" });
    const markers = within(track).getAllByRole("button");
    expect(markers).toHaveLength(3);
    expect(markers.map((marker) => marker.getAttribute("aria-label"))).toEqual([
      expect.stringContaining("Event 10, context observation"),
      expect.stringContaining("Event 20, compaction"),
      expect.stringContaining("Event 30, legacy context marker"),
    ]);
    expect(markers[0]).toHaveAccessibleName(/6,000 input tokens after.*capacity not reported.*adapter-derived.*derivation: tokenizer count/i);
    expect(markers[1]).toHaveAccessibleName(/8,000 input tokens before.*2,000 input tokens after.*10,000 capacity.*20% occupancy/i);
    expect(track).toHaveAttribute("data-state", "partial");
    expect(track).toHaveTextContent("4/9 events loaded · partial");
    expect(track).toHaveTextContent("gaps unobserved");
    expect(container.querySelector("path, polyline")).not.toBeInTheDocument();
  });

  it("supports a roving keyboard stop and exact marker selection", () => {
    const onSelect = vi.fn();
    render(<ContextTrack events={contextEvents} eventTotal={4} selectedId="after" onSelect={onSelect} />);
    const markers = within(screen.getByRole("navigation", { name: "Context events" })).getAllByRole("button");
    expect(markers[1]).toHaveAttribute("tabindex", "0");
    expect(markers[1]).toHaveAttribute("aria-current", "true");
    markers[1].focus();
    fireEvent.keyDown(markers[1], { key: "ArrowLeft" });
    expect(markers[0]).toHaveFocus();
    expect(onSelect).toHaveBeenLastCalledWith("before");
    fireEvent.keyDown(markers[0], { key: "End" });
    expect(markers[2]).toHaveFocus();
    expect(onSelect).toHaveBeenLastCalledWith("legacy");
    fireEvent.click(markers[1]);
    expect(onSelect).toHaveBeenLastCalledWith("after");
  });

  it("shows exact context details and jumps through explicit references", () => {
    const onJump = vi.fn();
    render(<ContextDetails event={contextEvents[0]} onJump={onJump} />);
    expect(screen.getByRole("heading", { name: "Context" })).toBeInTheDocument();
    expect(screen.getByText("2,000")).toBeInTheDocument();
    expect(screen.getByText("20%")).toBeInTheDocument();
    expect(screen.getByText("Earlier turns summarized.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "before" }));
    expect(onJump).toHaveBeenCalledWith("before");
    fireEvent.click(screen.getByRole("button", { name: "prompt" }));
    expect(onJump).toHaveBeenCalledWith("prompt");
  });

});
