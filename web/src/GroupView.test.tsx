import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { GroupView } from "./GroupView";
import type { GroupPathsResponse, GroupResponse } from "./types";

export const mixedGroup: GroupResponse = {
  group_id: "group-eval-7",
  aggregates: { reward: { mean: 0.4 } },
  trajectories: [
    { trajectory: { value: { id: "attempt-good", group_id: "group-eval-7", status: "completed", termination: "answer" } }, normalized_metrics: { reward: 1, pass: true, outcome: "correct", event_count: 12, error_count: 0, token_count: 840, latency_ms: 1200 }, signals: { advantage: 0.8, policy_reward: 0.65, grader_label: "accepted", checkpoint: "ckpt-42", nested_debug: { hidden: true } } },
    { trajectory: { value: { id: "attempt-bad", group_id: "group-eval-7" } }, reward: -0.2, success: false, outcome: "incorrect", event_count: 8, error_count: 2, token_count: 420, latency_ms: 900, status: "failed", termination: "tool_error", signals: { advantage: -0.1, policy_reward: -0.4, grader_label: "rejected", checkpoint: "ckpt-41" } },
    { trajectory: { id: "attempt-unknown", group_id: "group-eval-7", status: "stopped", termination: "max_steps" }, event_count: 20, signals: { advantage: 0.2, grader_label: "review", checkpoint: "ckpt-43" } },
  ],
};

const mixedPaths: GroupPathsResponse = {
  group_id: "group-eval-7", source_native_branches: false, source_native_branch_count: 0, count: 3, total_events: 9,
  tree: {
    trajectory_count: 3, terminal_count: 0, narrative_only_count: 0, behavioral_event_count: 6, narrative_event_count: 3, root_narrative_event_count: 1,
    children: [{
      fingerprint: { kind: "tool", class: "action", alignment_key: "search", behavioral: true }, count: 3, terminal_count: 0,
      trajectory_ids: ["attempt-good", "attempt-bad", "attempt-unknown"], narrative_event_count: 1, depth: 0,
      children: [
        { fingerprint: { kind: "observation", class: "observation", alignment_key: "result", behavioral: true }, count: 2, terminal_count: 2, trajectory_ids: ["attempt-good", "attempt-unknown"], narrative_event_count: 1, depth: 1, children: [] },
        { fingerprint: { kind: "error", class: "error", alignment_key: "timeout", behavioral: true }, count: 1, terminal_count: 1, trajectory_ids: ["attempt-bad"], narrative_event_count: 0, depth: 1, children: [] },
      ],
    }],
  },
};

describe("trajectory group", () => {
  it("summarizes mixed outcomes and only shows reported metric columns", () => {
    render(<GroupView group={mixedGroup} onClose={() => {}} onOpen={() => {}} />);
    expect(screen.getByText("50%", { selector: ".group-summary strong" })).toBeInTheDocument();
    expect(screen.getAllByText("correct").length).toBeGreaterThan(0);
    expect(screen.getAllByText("incorrect").length).toBeGreaterThan(0);
    expect(screen.getByRole("button", { name: /^Reward/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Errors/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Artifacts/ })).not.toBeInTheDocument();
  });

  it("shows, sorts, and filters scalar canonical research signals", () => {
    render(<GroupView group={mixedGroup} onClose={() => {}} onOpen={() => {}} />);
    const reward = screen.getByRole("button", { name: /^Reward/ });
    const advantage = screen.getByRole("button", { name: /Advantage/ });
    const headers = screen.getAllByRole("columnheader");
    expect(headers.indexOf(reward.closest("th")!)).toBeLessThan(headers.indexOf(advantage.closest("th")!));
    expect(screen.getByRole("button", { name: /Policy Reward/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Grader Label/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Checkpoint/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Nested Debug/ })).not.toBeInTheDocument();

    fireEvent.click(advantage);
    let rows = screen.getAllByRole("row").slice(1);
    expect(rows[0]).toHaveTextContent("attempt-good");
    expect(rows[1]).toHaveTextContent("attempt-unknown");
    expect(rows[2]).toHaveTextContent("attempt-bad");
    fireEvent.click(advantage);
    rows = screen.getAllByRole("row").slice(1);
    expect(rows[0]).toHaveTextContent("attempt-bad");

    fireEvent.change(screen.getByLabelText("Filter trajectories"), { target: { value: "rejected" } });
    expect(screen.getByText("attempt-bad")).toBeInTheDocument();
    expect(screen.queryByText("attempt-good")).not.toBeInTheDocument();
  });

  it("keeps case-variant canonical signals in canonical metric columns", () => {
    const group: GroupResponse = { group_id: "case", trajectories: [{ trajectory: { id: "attempt", group_id: "case" }, signals: { ReWaRd: 0.75, PASS: true, TOKEN_COUNT: 42 } }] };
    render(<GroupView group={group} onClose={() => {}} onOpen={() => {}} />);
    expect(screen.getByRole("button", { name: /^Reward/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^Pass/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^Tokens/ })).toBeInTheDocument();
    expect(screen.getByRole("row", { name: /attempt/ })).toHaveTextContent("0.75");
    expect(screen.queryByText(/signals$/)).not.toBeInTheDocument();
  });

  it("caps very wide signal schemas by coverage then name", () => {
    const signalNames = Array.from({ length: 11 }, (_, index) => `custom_${String(index).padStart(2, "0")}`);
    const group: GroupResponse = {
      group_id: "wide",
      trajectories: [
        { trajectory: { id: "one", group_id: "wide" }, signals: Object.fromEntries(signalNames.map((name, index) => [name, index])) },
        { trajectory: { id: "two", group_id: "wide" }, signals: { custom_10: 10, custom_09: 9 } },
      ],
    };
    render(<GroupView group={group} onClose={() => {}} onOpen={() => {}} />);
    expect(screen.getByText("8/11 signals")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Custom 09/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Custom 10/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Custom 08/ })).not.toBeInTheDocument();
  });

  it("navigates best and worst trajectories from the keyboard and opens the selection", () => {
    const open = vi.fn();
    render(<GroupView group={mixedGroup} onClose={() => {}} onOpen={open} />);
    fireEvent.keyDown(window, { key: "w" });
    expect(screen.getByText("attempt-bad").closest("tr")).toHaveAttribute("aria-selected", "true");
    fireEvent.keyDown(window, { key: "Enter" });
    expect(open).toHaveBeenCalledWith("attempt-bad");
    fireEvent.keyDown(window, { key: "b" });
    expect(screen.getByText("attempt-good").closest("tr")).toHaveAttribute("aria-selected", "true");
  });

  it("filters rows and supports j/k navigation", () => {
    render(<GroupView group={mixedGroup} onClose={() => {}} onOpen={() => {}} />);
    fireEvent.keyDown(window, { key: "/" });
    const filter = screen.getByLabelText("Filter trajectories");
    expect(filter).toHaveFocus();
    fireEvent.change(filter, { target: { value: "unknown" } });
    expect(screen.getByText("attempt-unknown")).toBeInTheDocument();
    expect(screen.queryByText("attempt-good")).not.toBeInTheDocument();
  });

  it("ANDs reproducible outcome, metric, and signal filters", () => {
    render(<GroupView group={mixedGroup} onClose={() => {}} onOpen={() => {}} />);
    const filter = screen.getByLabelText("Filter trajectories");
    fireEvent.change(filter, { target: { value: "pass:false reward<0 signal.policy_reward<0" } });
    expect(screen.getByText("attempt-bad")).toBeInTheDocument();
    expect(screen.queryByText("attempt-good")).not.toBeInTheDocument();
    expect(screen.queryByText("attempt-unknown")).not.toBeInTheDocument();
    expect(screen.getByText("1/3")).toBeInTheDocument();
  });

  it("fails malformed structured filters closed with a visible diagnostic", () => {
    render(<GroupView group={mixedGroup} onClose={() => {}} onOpen={() => {}} />);
    const filter = screen.getByLabelText("Filter trajectories");
    fireEvent.change(filter, { target: { value: "reward:high" } });
    expect(filter).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByRole("status")).toHaveTextContent("reward requires a numeric comparison");
    expect(screen.getByText(/Invalid filter/)).toBeInTheDocument();
    expect(screen.queryByText("attempt-good")).not.toBeInTheDocument();
  });

  it("selects exactly two trajectories by mouse and keyboard, then compares them", () => {
    const compare = vi.fn();
    render(<GroupView group={mixedGroup} onClose={() => {}} onOpen={() => {}} onCompare={compare} />);
    fireEvent.click(screen.getByRole("checkbox", { name: "Select attempt-good for comparison" }));
    fireEvent.keyDown(window, { key: "j" });
    fireEvent.keyDown(window, { key: " " });
    expect(screen.getByText("2/2 selected")).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "v" });
    expect(compare).toHaveBeenCalledWith("attempt-good", "attempt-bad");

    fireEvent.click(screen.getByRole("checkbox", { name: "Select attempt-unknown for comparison" }));
    expect(screen.getByRole("checkbox", { name: "Select attempt-good for comparison" })).not.toBeChecked();
    expect(screen.getAllByRole("checkbox", { checked: true })).toHaveLength(2);
  });

  it("shows compact shared prefixes and divergence with keyboard navigation", () => {
    const open = vi.fn();
    render(<GroupView group={mixedGroup} paths={mixedPaths} onClose={() => {}} onOpen={open} />);
    fireEvent.click(screen.getByRole("button", { name: /Behavioral paths/ }));
    expect(screen.getByRole("tree", { name: "Compact behavioral path tree" })).toBeInTheDocument();
    expect(screen.getByText("shared ×3")).toBeInTheDocument();
    expect(screen.getByText("splits 2")).toBeInTheDocument();
    expect(screen.getByText(/1 narrative events before/)).toBeInTheDocument();
    const items = screen.getAllByRole("treeitem");
    expect(items[0]).toHaveAttribute("aria-selected", "true");
    fireEvent.keyDown(window, { key: "j" });
    expect(items[1]).toHaveAttribute("aria-selected", "true");
    fireEvent.keyDown(window, { key: "Enter" });
    expect(open).toHaveBeenCalledWith("attempt-good");
    fireEvent.keyDown(window, { key: "p" });
    expect(screen.getByRole("table")).toBeInTheDocument();
  });
});
