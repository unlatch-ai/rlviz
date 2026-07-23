import { describe, expect, it } from "vitest";
import { effectiveDepth, emptyWorkspace, laneId, legacyWorkspace, normalizeWorkspace, serializeWorkspace, workspaceFromSearch, workspaceURL } from "./workspace";

describe("workspace arrangements", () => {
  it("opens the guide by default and migrates older workspaces into the guide", () => {
    expect(emptyWorkspace().guideOpen).toBe(true);
    const older = emptyWorkspace() as Partial<ReturnType<typeof emptyWorkspace>>;
    delete older.guideOpen;
    expect(normalizeWorkspace(older)?.guideOpen).toBe(true);
    const lane = { sourceId: "source", trajectoryId: "one", band: "focus", axis: { start: 0, end: 1 } };
    expect(normalizeWorkspace({ ...older, lanes: [lane] })?.guideOpen).toBe(false);
    expect(normalizeWorkspace({ ...emptyWorkspace(), guideOpen: false, active: "guide" })?.active).toBe("rail");
  });

  it("round-trips lanes, per-lane view state, and dockview layout JSON", () => {
    const workspace = emptyWorkspace();
    workspace.collectionView = "trials";
    workspace.lanes = [{ id: laneId("source", "one"), sourceId: "source", trajectoryId: "one", band: "focus", selected: 4, depth: 4, fidelity: 2, axis: { start: 10, end: 20 }, descentStack: [{ depth: 3, axis: { start: 0, end: 30 } }] }];
    workspace.active = workspace.lanes[0].id;
    workspace.layout = { grid: { root: { type: "leaf", data: { id: "group" }, size: 100 }, width: 1000, height: 700, orientation: 0 }, panels: {} } as never;
    const encoded = serializeWorkspace(workspace);
    expect(workspaceFromSearch(`?workspace=${encodeURIComponent(encoded)}`)).toEqual(workspace);
    const url = workspaceURL(workspace, { pathname: "/view", search: "?trajectory=old&mode=read", hash: "#token=x" } as Location);
    expect(url).toContain("workspace=");
    expect(JSON.parse(new URL(url, "http://local").searchParams.get("workspace")!)).not.toHaveProperty("layout");
  });

  it("round-trips rollout-pinned detail modules and drops orphaned details", () => {
    const workspace = emptyWorkspace();
    const id = laneId("source", "one");
    workspace.lanes = [{ id, sourceId: "source", trajectoryId: "one", band: "focus", selected: 0, depth: 1, fidelity: 1, axis: { start: 0, end: 1 }, descentStack: [] }];
    workspace.details = [id, "missing"];
    workspace.active = `detail:${id}`;
    const normalized = normalizeWorkspace(workspace)!;
    expect(normalized.details).toEqual([id]);
    expect(normalized.active).toBe(`detail:${id}`);
  });

  it("normalizes the shared detail state and clears orphaned pins", () => {
    const id = laneId("source", "one");
    const lane = { id, sourceId: "source", trajectoryId: "one", band: "focus" as const, selected: 0, depth: 1, fidelity: 1, axis: { start: 0, end: 1 }, descentStack: [] };
    const normalized = normalizeWorkspace({ ...emptyWorkspace(), lanes: [lane], detailOpen: true, detailCompact: true, detailPinned: id, active: "detail" })!;
    expect(normalized).toMatchObject({ detailOpen: true, detailCompact: true, detailPinned: id, active: "detail" });

    const orphaned = normalizeWorkspace({ ...normalized, lanes: [], detailPinned: id })!;
    expect(orphaned.detailOpen).toBe(false);
    expect(orphaned.detailPinned).toBeUndefined();
    expect(orphaned.active).toBe("rail");
  });

  it("uses Surface as context's effective depth without discarding stored focus depth", () => {
    const lane = { id: "lane", sourceId: "source", trajectoryId: "one", band: "context" as const, selected: 0, depth: 3, fidelity: 3, axis: { start: 0, end: 10 }, descentStack: [] };
    expect(effectiveDepth(lane)).toBe(1);
    expect(effectiveDepth({ ...lane, band: "focus" })).toBe(3);
  });

  it("ignores legacy rail projection fields and omits them when serializing", () => {
    const legacy = { ...emptyWorkspace(), railProjection: "caterpillar" };
    const normalized = normalizeWorkspace(legacy)!;
    expect(normalized).not.toHaveProperty("railProjection");
    expect(serializeWorkspace(normalized)).not.toContain("railProjection");
  });

  it("migrates v2 state and keeps at most two focus lanes", () => {
    const normalized = normalizeWorkspace({ ...emptyWorkspace(), version: 2, seams: { rail: 9, focusContext: -1, focusLane: 9, console: 0 }, lanes: ["a", "b", "c"].map((trajectoryId) => ({ sourceId: "s", trajectoryId, band: "focus", axis: { start: 0, end: 1 } })) })!;
    expect(normalized.lanes.filter((lane) => lane.band === "focus")).toHaveLength(2);
    expect(normalized.lanes[2].band).toBe("context");
    expect(normalized.version).toBe(3);
    expect(normalized.layout).toBeUndefined();
  });

  it("falls back to the default layout for corrupt workspace and dockview input", () => {
    expect(normalizeWorkspace(undefined)).toBeUndefined();
    expect(normalizeWorkspace({ version: 99 })).toBeUndefined();
    const normalized = normalizeWorkspace({ ...emptyWorkspace(), layout: { grid: "bad", panels: [] } })!;
    expect(normalized.layout).toBeUndefined();
  });

  it("rejects dock layouts beyond the local restore bounds", () => {
    const panels = Object.fromEntries(Array.from({ length: 65 }, (_, index) => [`panel-${index}`, {}]));
    const normalized = normalizeWorkspace({
      ...emptyWorkspace(),
      layout: { grid: { root: { type: "leaf", data: { id: "group" }, size: 100 }, width: 1000, height: 700, orientation: 0 }, panels },
    })!;
    expect(normalized.layout).toBeUndefined();
  });

  it("keeps the first duplicate lane before clamping the focus band", () => {
    const normalized = normalizeWorkspace({ ...emptyWorkspace(), lanes: [
      { sourceId: "s", trajectoryId: "one", band: "focus", depth: 2, axis: { start: 1, end: 2 } },
      { sourceId: "s", trajectoryId: "one", band: "context", depth: 3, axis: { start: 3, end: 4 } },
      { sourceId: "s", trajectoryId: "two", band: "focus", axis: { start: 0, end: 1 } },
      { sourceId: "s", trajectoryId: "three", band: "focus", axis: { start: 0, end: 1 } },
    ] })!;
    expect(normalized.lanes.map((lane) => [lane.trajectoryId, lane.band, lane.depth])).toEqual([
      ["one", "focus", 2], ["two", "focus", 1], ["three", "context", 1],
    ]);
  });

  it("maps legacy read and compare URLs into arrangements", () => {
    const read = legacyWorkspace("?trajectory=source&trajectory_id=one&mode=read")!;
    expect(read.lanes.map((lane) => lane.trajectoryId)).toEqual(["one"]);
    const compare = legacyWorkspace("?trajectory=source&left=one&right=two&view=compare")!;
    expect(compare.lanes.map((lane) => lane.trajectoryId)).toEqual(["one", "two"]);
    expect(compare.reference).toBe(compare.lanes[0].id);
  });
});
