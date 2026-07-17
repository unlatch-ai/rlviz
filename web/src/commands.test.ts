import { describe, expect, it } from "vitest";
import { bindingsFor, commandIds, commands, detectKeymapConflicts, eventBinding, keymapStorageKey, loadKeymapOverrides, matchesBinding, normalizeBinding, resetKeymapOverrides, saveKeymapOverrides, setCommandBindings } from "./commands";

describe("command registry", () => {
  it("has stable unique IDs and conflict-free defaults in each scope", () => {
    expect(new Set(commands.map(({ id }) => id))).toHaveProperty("size", commands.length);
    expect(detectKeymapConflicts({})).toEqual([]);
    expect(commandIds.trajectory.next).toBe("trajectory.next");
  });

  it("normalizes browser keys and common binding aliases", () => {
    expect(normalizeBinding("space")).toBe("Space");
    expect(normalizeBinding("Ctrl+K")).toBe("Ctrl+k");
    expect(eventBinding(new KeyboardEvent("keydown", { key: " ", ctrlKey: true }))).toBe("Ctrl+Space");
  });

  it("matches portable and shifted modifiers without accepting extras", () => {
    expect(matchesBinding(new KeyboardEvent("keydown", { key: "k", ctrlKey: true }), "Mod+k")).toBe(true);
    expect(matchesBinding(new KeyboardEvent("keydown", { key: "k", metaKey: true }), "Mod+k")).toBe(true);
    expect(matchesBinding(new KeyboardEvent("keydown", { key: "k", ctrlKey: true, metaKey: true }), "Mod+k")).toBe(false);
    expect(matchesBinding(new KeyboardEvent("keydown", { key: "k", ctrlKey: true }), "k")).toBe(false);
    expect(matchesBinding(new KeyboardEvent("keydown", { key: "k", altKey: true }), "k")).toBe(false);
    expect(matchesBinding(new KeyboardEvent("keydown", { key: "ArrowDown", shiftKey: true }), "Shift+ArrowDown")).toBe(true);
    expect(matchesBinding(new KeyboardEvent("keydown", { key: "?", shiftKey: true }), "?")).toBe(true);
    expect(matchesBinding(new KeyboardEvent("keydown", { key: "J", shiftKey: true }), "j")).toBe(false);
  });

  it("persists valid local overrides and ignores unknown or malformed entries", () => {
    const values = new Map<string, string>();
    const storage = { getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => { values.set(key, value); }, removeItem: (key: string) => { values.delete(key); } };
    saveKeymapOverrides({ [commandIds.trajectory.next]: ["ArrowDown"] }, storage);
    values.set(keymapStorageKey, JSON.stringify({ ...JSON.parse(values.get(keymapStorageKey)!), unknown: ["q"], [commandIds.trajectory.previous]: "bad" }));
    const overrides = loadKeymapOverrides(storage);
    expect(bindingsFor(commandIds.trajectory.next, overrides)).toEqual(["ArrowDown"]);
    expect(overrides).not.toHaveProperty("unknown");
    expect(overrides).not.toHaveProperty(commandIds.trajectory.previous);
    setCommandBindings(commandIds.trajectory.next, ["n", "N"], storage);
    expect(loadKeymapOverrides(storage)[commandIds.trajectory.next]).toEqual(["n"]);
    resetKeymapOverrides(storage);
    expect(loadKeymapOverrides(storage)).toEqual({});
  });

  it("reports conflicts introduced by overrides only within the active scope", () => {
    const overrides = { [commandIds.trajectory.previous]: ["j"], [commandIds.group.next]: ["j"] };
    expect(detectKeymapConflicts(overrides, "trajectory")).toEqual([{ scope: "trajectory", binding: "j", commandIds: [commandIds.trajectory.next, commandIds.trajectory.previous] }]);
    expect(detectKeymapConflicts(overrides, "group")).toEqual([]);
  });
});
