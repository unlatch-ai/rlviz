import { fireEvent, render } from "@testing-library/react";
import { createElement, useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { applyPresentationKeymap, bindingsFor, commandIds, commands, detectKeymapConflicts, eventBinding, keymapStorageKey, loadKeymapOverrides, matchesBinding, normalizeBinding, presentationKeymapOverrides, resetKeymapOverrides, saveKeymapOverrides, setCommandBindings } from "./commands";
import { useCommands } from "./commands";

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
    expect(detectKeymapConflicts({ [commandIds.trajectory.next]: ["Mod+k"], [commandIds.trajectory.previous]: ["Ctrl+k"] }, "trajectory")).toEqual(expect.arrayContaining([expect.objectContaining({ binding: "Ctrl+k" })]));
  });

  it("applies portable project defaults below browser-local overrides", () => {
    const cleanup = applyPresentationKeymap({ api_version: "rlviz.dev/v1alpha1", keymap: { bindings: { [commandIds.trajectory.next]: ["n", "ArrowDown"] } } });
    expect(bindingsFor(commandIds.trajectory.next, {})).toEqual(["n", "ArrowDown"]);
    expect(bindingsFor(commandIds.trajectory.next, { [commandIds.trajectory.next]: ["j"] })).toEqual(["j"]);
    cleanup();
    expect(bindingsFor(commandIds.trajectory.next, {})).toEqual(["j"]);
  });

  it("fails malformed or conflicting portable keymaps to shipped defaults", () => {
    expect(presentationKeymapOverrides({ api_version: "rlviz.dev/v1alpha1", keymap: { bindings: { [commandIds.trajectory.next]: ["k"] } } })).toEqual({});
    expect(presentationKeymapOverrides({ api_version: "rlviz.dev/v1alpha1", keymap: { bindings: { unknown: ["q"] } } })).toEqual({});
    expect(presentationKeymapOverrides({ api_version: "rlviz.dev/v1alpha1", keymap: { bindings: { [commandIds.trajectory.next]: ["Hyper+j"] } } })).toEqual({});
  });

  it("keeps one listener bound while using the latest command handler", () => {
    const first = vi.fn();
    const second = vi.fn();
    function Harness() {
      const [latest, setLatest] = useState(false);
      useCommands("trajectory", { [commandIds.trajectory.next]: latest ? second : first });
      return createElement("button", { onClick: () => setLatest(true) }, "Update");
    }
    const { getByRole } = render(createElement(Harness));
    fireEvent.keyDown(window, { key: "j" });
    fireEvent.click(getByRole("button", { name: "Update" }));
    fireEvent.keyDown(window, { key: "j" });
    expect(first).toHaveBeenCalledTimes(1);
    expect(second).toHaveBeenCalledTimes(1);
  });

  it("suppresses commands in text entry, selects, and dialogs", () => {
    const next = vi.fn();
    function Harness() {
      useCommands("trajectory", { [commandIds.trajectory.next]: next });
      return createElement("div", null,
        createElement("input", { "aria-label": "input" }),
        createElement("select", { "aria-label": "select" }, createElement("option", null, "one")),
        createElement("div", { role: "dialog" }, createElement("button", null, "Dialog action")),
        createElement("button", null, "Outside"),
      );
    }
    const { getByRole, getByLabelText } = render(createElement(Harness));
    fireEvent.keyDown(getByLabelText("input"), { key: "j" });
    fireEvent.keyDown(getByLabelText("select"), { key: "j" });
    fireEvent.keyDown(getByRole("button", { name: "Dialog action" }), { key: "j" });
    fireEvent.keyDown(getByRole("button", { name: "Outside" }), { key: "j" });
    expect(next).toHaveBeenCalledTimes(1);
  });
});
