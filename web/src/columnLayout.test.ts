import { describe, expect, it, vi } from "vitest";
import { defaultGroupColumnLayout, groupColumnLayoutStorageKey, loadGroupColumnLayout, loadStoredGroupColumnLayout, resetGroupColumnLayout, saveGroupColumnLayout } from "./columnLayout";

describe("group column layout persistence", () => {
  it("round trips a bounded versioned layout", () => {
    let stored = "";
    const storage = { getItem: () => stored, setItem: (_key: string, value: string) => { stored = value; } };
    saveGroupColumnLayout({ hiddenBuiltins: ["errors", "latency"], signalNames: ["grader", "advantage", "grader"] }, storage);
    expect(JSON.parse(stored)).toEqual({ version: 1, hiddenBuiltins: ["errors", "latency"], signalNames: ["grader", "advantage"] });
    expect(loadGroupColumnLayout(storage)).toEqual({ hiddenBuiltins: ["errors", "latency"], signalNames: ["grader", "advantage"] });
  });

  it.each(["not json", "[]", '{"version":2}', '{"version":1,"hiddenBuiltins":"reward","signalNames":[]}'])
  ("falls back for corrupt or unsupported data: %s", (raw) => {
    expect(loadGroupColumnLayout({ getItem: () => raw })).toEqual(defaultGroupColumnLayout);
  });

  it("survives unavailable storage and ignores unknown built-ins", () => {
    const unavailable = { getItem: vi.fn(() => { throw new Error("denied"); }), setItem: vi.fn(() => { throw new Error("denied"); }), removeItem: vi.fn(() => { throw new Error("denied"); }) };
    expect(loadGroupColumnLayout(unavailable)).toEqual(defaultGroupColumnLayout);
    expect(() => saveGroupColumnLayout({ hiddenBuiltins: ["reward", "unknown" as "reward"], signalNames: ["score"] }, unavailable)).not.toThrow();
    expect(resetGroupColumnLayout(unavailable)).toEqual(defaultGroupColumnLayout);
  });

  it("uses one explicit versioned storage key", () => {
    const removeItem = vi.fn();
    resetGroupColumnLayout({ removeItem });
    expect(removeItem).toHaveBeenCalledWith(groupColumnLayoutStorageKey);
  });

  it("distinguishes no saved preference from the built-in default", () => {
    expect(loadStoredGroupColumnLayout({ getItem: () => null })).toBeNull();
    expect(loadGroupColumnLayout({ getItem: () => null })).toEqual(defaultGroupColumnLayout);
  });
});
