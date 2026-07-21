import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { StructuredToolDiff } from "./StructuredDiffView";
import type { TrajectoryEvent } from "./types";

const tool = (id: string, name: string, args: unknown, output: unknown): TrajectoryEvent => ({
  id, sequence: id === "left" ? 1 : 2, kind: "tool", title: name,
  input: { name, arguments: args }, output,
});

describe("structured tool differences", () => {
  it("shows only changed argument and result fields with selected-event provenance", () => {
    render(<StructuredToolDiff
      left={tool("left", "search", { query: "agent", limit: 5, mode: "fast" }, { ok: true, hits: 3 })}
      right={tool("right", "search", { query: "agent", limit: 10, extra: true }, { ok: false, error: "timeout" })}
    />);
    expect(screen.getByRole("region", { name: "Structured tool payload differences" })).toHaveTextContent("search vs search");
    expect(screen.getByRole("table", { name: "Arguments field differences" })).toHaveTextContent("$.arguments.limit");
    expect(screen.getByRole("table", { name: "Arguments field differences" })).toHaveTextContent("$.arguments.extra");
    expect(screen.queryByText("$.arguments.query")).not.toBeInTheDocument();
    expect(screen.getByRole("table", { name: "Results field differences" })).toHaveTextContent("$.result.error");
    expect(screen.getByText("3 changed · 1 equal")).toBeInTheDocument();
  });

  it("reports structurally equal payloads without raw JSON noise", () => {
    const left = tool("left", "read", { path: "a" }, { ok: true });
    const right = tool("right", "read", { path: "a" }, { ok: true });
    render(<StructuredToolDiff left={left} right={right} />);
    expect(screen.getAllByText("No field changes")).toHaveLength(2);
  });

  it("marks bounded differences as mild attention", () => {
    const many = Object.fromEntries(Array.from({ length: 70 }, (_, index) => [`field_${index}`, index]));
    render(<StructuredToolDiff left={tool("left", "bounded", many, {})} right={tool("right", "bounded", {}, {})} />);
    expect(screen.getByText(/Bounded view/)).toHaveAttribute("data-state", "truncated");
  });

  it("stays absent for gaps, non-tool events, and payload-free events", () => {
    const event: TrajectoryEvent = { id: "message", sequence: 1, kind: "message", content: "hello" };
    const { rerender } = render(<StructuredToolDiff left={event} right={event} />);
    expect(screen.queryByRole("region", { name: "Structured tool payload differences" })).not.toBeInTheDocument();
    rerender(<StructuredToolDiff left={tool("left", "empty", undefined, undefined)} right={undefined} />);
    expect(screen.queryByRole("region", { name: "Structured tool payload differences" })).not.toBeInTheDocument();
  });
});
