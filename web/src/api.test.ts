import { describe, expect, it } from "vitest";
import { daemonToken, normalizeTrajectoryResponse, trajectoryEndpoint } from "./api";

describe("trajectory API normalization", () => {
	it("uses the stable trajectory ID from the viewer URL", () => {
		expect(trajectoryEndpoint("?trajectory=abc/123")).toBe("/api/v1/trajectory?trajectory=abc%2F123");
		expect(trajectoryEndpoint("")).toBe("/api/v1/trajectory");
	});

	it("reads the daemon token from the URL fragment", () => {
		expect(daemonToken("#token=abc%2F123")).toBe("abc/123");
		expect(daemonToken("")).toBeNull();
	});

  it("merges sibling events and canonical event data", () => {
    const trajectory = normalizeTrajectoryResponse({
      trajectory: { id: "traj-1", group_id: "group-1" },
      run: { id: "run-1", started_at: "2026-01-01T00:00:00Z", metadata: { model: "checkpoint-42" } },
      case: { id: "case-1", run_id: "run-1", name: "example case" },
      group: { id: "group-1", case_id: "case-1" },
      signals: [{ trajectory_id: "traj-1", name: "reward", value: 0.75 }],
      events: [{
        id: "evt-1", sequence: 0, kind: "reward", data: { reward: 0.75 },
        source: { path: "trace.jsonl", byte_offset: 100, byte_length: 42 },
      }],
    });
    expect(trajectory.events).toHaveLength(1);
    expect(trajectory.events[0].reward).toBe(0.75);
    expect(trajectory.events[0].source?.byte_offset).toBe(100);
    expect(trajectory.run_id).toBe("run-1");
    expect(trajectory.case_id).toBe("case-1");
    expect(trajectory.model).toBe("checkpoint-42");
    expect(trajectory.total_reward).toBe(0.75);
  });
});
