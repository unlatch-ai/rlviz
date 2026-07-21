import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import type { BrowseResponse, ComparisonResponse } from "./types";

const browse: BrowseResponse = {
  sources: [{ id: "source-1", path: "/tmp/demo.ndjson", index_state: "complete" }], count: 2,
  trajectories: [
    { source_id: "source-1", source_name: "demo.ndjson", case_name: "demo", trajectory: { id: "reference", group_id: "group", status: "completed" }, metrics: { trajectory: { id: "reference", group_id: "group" }, event_count: 2, error_count: 0, pass: true, reward: 1 } },
    { source_id: "source-1", source_name: "demo.ndjson", case_name: "demo", trajectory: { id: "candidate", group_id: "group", status: "failed" }, metrics: { trajectory: { id: "candidate", group_id: "group" }, event_count: 2, error_count: 1, pass: false, reward: -1 } },
  ],
};
const trajectoryPayload = (id: string) => ({ trajectory: { id, group_id: "group", status: id === "candidate" ? "failed" : "completed" }, events: [{ id: `${id}-start`, sequence: 0, kind: "tool", alignment_key: "stage:work", output: { ok: true } }, { id: `${id}-end`, sequence: 10, kind: id === "candidate" ? "error" : "grader", alignment_key: "stage:outcome", output: { verdict: id === "candidate" ? "fail" : "pass" } }], page: { count: 2, total: 2, limit: 200, has_more: false } });
const comparison: ComparisonResponse = {
  left: { trajectory: { id: "candidate" }, events: trajectoryPayload("candidate").events },
  right: { trajectory: { id: "reference" }, events: trajectoryPayload("reference").events },
  alignment: { common_behavioral_prefix: 0, first_meaningful_divergence: 0, steps: [] },
  differences: { event_count: { left: 2, right: 2, delta: 0 }, status: { changed: true }, termination: { changed: false }, reward: { changed: true } },
};

describe("Browse Read Compare flow", () => {
  afterEach(() => { vi.unstubAllGlobals(); window.history.replaceState({}, "", "/"); });

  it("loads the daemon collection, marks a pair, and opens stage-aligned Compare", async () => {
    window.history.replaceState({}, "", "/?trajectory=source-1&trajectory_id=candidate&indexed=1#token=secret");
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === "/api/v1/indexed/browse") return new Response(JSON.stringify(browse));
      if (url.includes("/indexed/compare")) return new Response(JSON.stringify(comparison));
      if (url.includes("/indexed/analysis")) return new Response(JSON.stringify({ analysis: { api_version: "v1", provenance: { name: "test", version: "1", digest: "x", input_digest: "y" }, findings: [], signals: [] }, cached: false, analyzed_at: "now" }));
      if (url.includes("/indexed/trajectory")) {
        const id = new URL(url, "http://local").searchParams.get("trajectory_id") ?? "candidate";
        return new Response(JSON.stringify(trajectoryPayload(id)));
      }
      throw new Error(`unexpected request ${url}`);
    }));
    render(<App />);
    await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(2));
		expect(screen.getAllByRole("option")[0]).toHaveTextContent("candidate");
		expect(screen.getByRole("option", { selected: true })).toHaveTextContent("candidate");
		expect(screen.getByText(/2 unresolved/)).toBeInTheDocument();
		fireEvent.keyDown(window, { key: "3" });
		expect(screen.getByText(/1 unresolved/)).toBeInTheDocument();
		expect(screen.getByRole("option", { selected: true })).toHaveTextContent("reference");
		await waitFor(() => expect(screen.getByRole("option", { name: /candidate/ })).toHaveTextContent("tag 3"));
		fireEvent.keyDown(window, { key: "k" });
    fireEvent.keyDown(window, { key: " " });
    fireEvent.keyDown(window, { key: "j" });
    fireEvent.keyDown(window, { key: " " });
    fireEvent.keyDown(window, { key: "v" });
    expect(await screen.findByRole("main", { name: "Pair Compare" })).toHaveTextContent("aligned by annotated stages");
    expect(screen.getByText(/first divergence/)).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "d" });
    expect(screen.getByRole("button", { name: /outcome/ })).toHaveClass("selected");
  });

	it("does not apply late analysis to another rollout", async () => {
		let resolveCandidateAnalysis!: (value: Response) => void;
		vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
			const url = String(input);
			if (url === "/api/v1/indexed/browse") return new Response(JSON.stringify(browse));
			if (url === "/api/v1/trajectory") return new Response(JSON.stringify(trajectoryPayload("candidate")));
			if (url.includes("/indexed/trajectory")) {
				const id = new URL(url, "http://local").searchParams.get("trajectory_id") ?? "candidate";
				return new Response(JSON.stringify(trajectoryPayload(id)));
			}
			if (url.includes("/indexed/analysis")) {
				const id = new URL(url, "http://local").searchParams.get("trajectory_id");
				if (id === "candidate") return new Promise<Response>((resolve) => { resolveCandidateAnalysis = resolve; });
				return new Response(JSON.stringify({ analysis: { provenance: { name: "test" }, findings: [], signals: [] } }));
			}
			throw new Error(`unexpected request ${url}`);
		}));
		render(<App />);
		await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(2));
		fireEvent.keyDown(window, { key: "Enter" });
		expect(await screen.findByRole("main", { name: "Read trajectory" })).toHaveTextContent("candidate");
		fireEvent.keyDown(window, { key: "Escape" });
		fireEvent.keyDown(window, { key: "j" });
		fireEvent.keyDown(window, { key: "Enter" });
		await waitFor(() => expect(screen.getByRole("main", { name: "Read trajectory" })).toHaveTextContent("reference"));
		resolveCandidateAnalysis(new Response(JSON.stringify({ analysis: { provenance: { name: "test" }, findings: [{ event_ids: ["candidate-start"] }], signals: [] } })));
		await waitFor(() => expect(screen.getByRole("main", { name: "Read trajectory" })).toHaveTextContent("reference"));
	});

	it("does not move a deliberate event-zero selection when initial analysis arrives", async () => {
		const plain = { trajectory: { id: "plain" }, events: [
			{ id: "plain-start", sequence: 0, kind: "message", title: "start" },
			{ id: "plain-finding", sequence: 1, kind: "tool", title: "finding" },
		] };
		const collection: BrowseResponse = { sources: [{ id: "source" }], count: 1, trajectories: [{ source_id: "source", source_name: "plain", trajectory: { id: "plain" }, metrics: { event_count: 2 } }] };
		let resolveAnalysis!: (value: Response) => void;
		vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
			const url = String(input);
			if (url === "/api/v1/indexed/browse") return new Response(JSON.stringify(collection));
			if (url.includes("/indexed/analysis")) return new Promise<Response>((resolve) => { resolveAnalysis = resolve; });
			return new Response(JSON.stringify(plain));
		}));
		render(<App />);
		await screen.findByRole("option");
		fireEvent.keyDown(window, { key: "Enter" });
		await screen.findByRole("main", { name: "Read trajectory" });
		fireEvent.click(screen.getByRole("button", { name: /start/ }));
		resolveAnalysis(new Response(JSON.stringify({ analysis: { provenance: { name: "test" }, findings: [{ event_ids: ["plain-finding"] }], signals: [] } })));
		await waitFor(() => expect(screen.getByText("start", { selector: ".moment.selected b" })).toBeInTheDocument());
	});

	it("walks the filtered attention queue with n and p", async () => {
		const collection: BrowseResponse = {
			...browse,
			count: 3,
			trajectories: [
				{ ...browse.trajectories[0], case_name: "keep" },
				{ ...browse.trajectories[1], case_name: "keep" },
				{ source_id: "source-1", source_name: "demo.ndjson", case_name: "drop", trajectory: { id: "excluded" }, metrics: { event_count: 1, error_count: 0 } },
			],
		};
		vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
			const url = String(input);
			if (url === "/api/v1/indexed/browse") return new Response(JSON.stringify(collection));
			if (url.includes("/indexed/analysis")) return new Response(JSON.stringify({ analysis: { provenance: { name: "test" }, findings: [], signals: [] } }));
			if (url.includes("/indexed/trajectory")) {
				const id = new URL(url, "http://local").searchParams.get("trajectory_id") ?? "candidate";
				return new Response(JSON.stringify(trajectoryPayload(id)));
			}
			return new Response(JSON.stringify(trajectoryPayload("candidate")));
		}));
		render(<App />);
		await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(3));
		fireEvent.change(screen.getByLabelText("Filter"), { target: { value: "keep" } });
		expect(screen.getAllByRole("option")).toHaveLength(2);
		fireEvent.keyDown(window, { key: "Enter" });
		await screen.findByRole("main", { name: "Read trajectory" });
		expect(screen.getByRole("main", { name: "Read trajectory" })).toHaveTextContent("candidate");
		fireEvent.keyDown(window, { key: "n" });
		await waitFor(() => expect(screen.getByRole("main", { name: "Read trajectory" })).toHaveTextContent("reference"));
		fireEvent.keyDown(window, { key: "p" });
		await waitFor(() => expect(screen.getByRole("main", { name: "Read trajectory" })).toHaveTextContent("candidate"));
	});
});
