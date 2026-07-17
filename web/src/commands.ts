import { useEffect, useState } from "react";

export type CommandScope = "trajectory" | "group" | "paths" | "comparison";

export const commandIds = {
  trajectory: {
    dismiss: "trajectory.dismiss", search: "trajectory.search", next: "trajectory.next", previous: "trajectory.previous",
    nextError: "trajectory.nextError", nextReward: "trajectory.nextReward", nextContext: "trajectory.nextContext", nextFinding: "trajectory.nextFinding",
    nextArtifact: "trajectory.nextArtifact", toggleRaw: "trajectory.toggleRaw", openGroup: "trajectory.openGroup",
    toggleHelp: "trajectory.toggleHelp", toggleExpanded: "trajectory.toggleExpanded",
    openTranscript: "trajectory.openTranscript", openTimeline: "trajectory.openTimeline", openOutcome: "trajectory.openOutcome",
  },
  group: {
    back: "group.back", togglePaths: "group.togglePaths", search: "group.search", next: "group.next",
    previous: "group.previous", open: "group.open", toggleCompare: "group.toggleCompare",
    compare: "group.compare", best: "group.best", median: "group.median", worst: "group.worst",
    rewardOutlier: "group.rewardOutlier", nextFailure: "group.nextFailure", nextInfraFailure: "group.nextInfraFailure",
    toggleColumns: "group.toggleColumns",
  },
  paths: {
    back: "paths.back", togglePaths: "paths.togglePaths", next: "paths.next", previous: "paths.previous", open: "paths.open",
  },
  comparison: {
    back: "comparison.back", next: "comparison.next", previous: "comparison.previous",
    firstDivergence: "comparison.firstDivergence", nextChange: "comparison.nextChange",
  },
} as const;

type NestedValue<T> = T extends Record<string, infer V> ? V extends string ? V : NestedValue<V> : never;
export type CommandId = NestedValue<typeof commandIds>;

export type CommandDefinition = {
  id: CommandId;
  scope: CommandScope;
  label: string;
  defaultBindings: readonly string[];
  allowInInput?: boolean;
};

export const commands: readonly CommandDefinition[] = [
  { id: commandIds.trajectory.dismiss, scope: "trajectory", label: "Close search or dialog", defaultBindings: ["Escape"], allowInInput: true },
  { id: commandIds.trajectory.search, scope: "trajectory", label: "Search events", defaultBindings: ["/"] },
  { id: commandIds.trajectory.next, scope: "trajectory", label: "Next event", defaultBindings: ["j"] },
  { id: commandIds.trajectory.previous, scope: "trajectory", label: "Previous event", defaultBindings: ["k"] },
  { id: commandIds.trajectory.nextError, scope: "trajectory", label: "Jump to next error", defaultBindings: ["e"] },
  { id: commandIds.trajectory.nextReward, scope: "trajectory", label: "Jump to next reward or grader", defaultBindings: ["r"] },
  { id: commandIds.trajectory.nextContext, scope: "trajectory", label: "Jump to next context change", defaultBindings: ["c"] },
  { id: commandIds.trajectory.nextFinding, scope: "trajectory", label: "Jump through analyzer findings", defaultBindings: ["a"] },
  { id: commandIds.trajectory.nextArtifact, scope: "trajectory", label: "Open next artifact", defaultBindings: ["o"] },
  { id: commandIds.trajectory.toggleRaw, scope: "trajectory", label: "Toggle raw event record", defaultBindings: ["x"] },
  { id: commandIds.trajectory.openGroup, scope: "trajectory", label: "Compare trajectory group", defaultBindings: ["g"] },
  { id: commandIds.trajectory.toggleHelp, scope: "trajectory", label: "Toggle keyboard shortcuts", defaultBindings: ["?"] },
  { id: commandIds.trajectory.toggleExpanded, scope: "trajectory", label: "Expand selected event", defaultBindings: ["Enter", "Space"] },
  { id: commandIds.trajectory.openTranscript, scope: "trajectory", label: "Open transcript", defaultBindings: ["1"] },
  { id: commandIds.trajectory.openTimeline, scope: "trajectory", label: "Open event timeline", defaultBindings: ["2"] },
  { id: commandIds.trajectory.openOutcome, scope: "trajectory", label: "Open outcome", defaultBindings: ["3"] },

  { id: commandIds.group.back, scope: "group", label: "Back to trajectory", defaultBindings: ["Escape"], allowInInput: true },
  { id: commandIds.group.togglePaths, scope: "group", label: "Toggle behavioral paths", defaultBindings: ["p"] },
  { id: commandIds.group.search, scope: "group", label: "Filter trajectories", defaultBindings: ["/"] },
  { id: commandIds.group.next, scope: "group", label: "Next trajectory", defaultBindings: ["j", "ArrowDown"] },
  { id: commandIds.group.previous, scope: "group", label: "Previous trajectory", defaultBindings: ["k", "ArrowUp"] },
  { id: commandIds.group.open, scope: "group", label: "Open trajectory", defaultBindings: ["Enter", "o"] },
  { id: commandIds.group.toggleCompare, scope: "group", label: "Mark for comparison", defaultBindings: ["Space", "c"] },
  { id: commandIds.group.compare, scope: "group", label: "Compare selected trajectories", defaultBindings: ["v"] },
  { id: commandIds.group.best, scope: "group", label: "Select best trajectory", defaultBindings: ["b"] },
  { id: commandIds.group.median, scope: "group", label: "Select median-reward trajectory", defaultBindings: ["m"] },
  { id: commandIds.group.worst, scope: "group", label: "Select worst trajectory", defaultBindings: ["w"] },
  { id: commandIds.group.rewardOutlier, scope: "group", label: "Select reward outlier", defaultBindings: ["u"] },
  { id: commandIds.group.nextFailure, scope: "group", label: "Jump to next failed trajectory", defaultBindings: ["f"] },
  { id: commandIds.group.nextInfraFailure, scope: "group", label: "Jump to next infrastructure failure", defaultBindings: ["i"] },
  { id: commandIds.group.toggleColumns, scope: "group", label: "Configure table columns", defaultBindings: ["Shift+C"] },

  { id: commandIds.paths.back, scope: "paths", label: "Back to trajectory", defaultBindings: ["Escape"], allowInInput: true },
  { id: commandIds.paths.togglePaths, scope: "paths", label: "Back to trajectories", defaultBindings: ["p"] },
  { id: commandIds.paths.next, scope: "paths", label: "Next behavioral path", defaultBindings: ["j", "ArrowDown"] },
  { id: commandIds.paths.previous, scope: "paths", label: "Previous behavioral path", defaultBindings: ["k", "ArrowUp"] },
  { id: commandIds.paths.open, scope: "paths", label: "Open sample trajectory", defaultBindings: ["Enter", "o"] },

  { id: commandIds.comparison.back, scope: "comparison", label: "Back to group", defaultBindings: ["Escape"] },
  { id: commandIds.comparison.next, scope: "comparison", label: "Next alignment step", defaultBindings: ["j", "ArrowDown"] },
  { id: commandIds.comparison.previous, scope: "comparison", label: "Previous alignment step", defaultBindings: ["k", "ArrowUp"] },
  { id: commandIds.comparison.firstDivergence, scope: "comparison", label: "First meaningful divergence", defaultBindings: ["d"] },
  { id: commandIds.comparison.nextChange, scope: "comparison", label: "Next change", defaultBindings: ["n"] },
] as const;

const commandById = new Map<CommandId, CommandDefinition>(commands.map((command) => [command.id, command]));
export const keymapStorageKey = "rlviz.keybindings.v1";
export type KeymapOverrides = Partial<Record<CommandId, string[]>>;

function keyName(key: string): string {
  if (key === " " || key.toLowerCase() === "space") return "Space";
  if (key.toLowerCase() === "esc") return "Escape";
  if (key.length === 1) return key.toLowerCase();
  return key;
}

export function normalizeBinding(binding: string): string {
  const parts = binding.split("+").map((part) => part.trim()).filter(Boolean);
  const key = keyName(parts.pop() ?? "");
  const modifiers = new Set(parts.map((part) => part.toLowerCase()));
  const prefix = [modifiers.has("mod") ? "Mod" : "", modifiers.has("ctrl") ? "Ctrl" : "", modifiers.has("meta") ? "Meta" : "", modifiers.has("alt") ? "Alt" : "", modifiers.has("shift") ? "Shift" : ""].filter(Boolean);
  return [...prefix, key].join("+");
}

export function eventBinding(event: KeyboardEvent): string {
  const key = keyName(event.key);
  // `event.key` already encodes Shift for printable punctuation (for example
  // Shift+/ is "?"). Preserve Shift for named keys and alphanumerics, where it
  // is otherwise not represented by the key value.
  const shifted = event.shiftKey && (key.length > 1 || /^[a-z0-9]$/i.test(key));
  const prefix = [event.ctrlKey ? "Ctrl" : "", event.metaKey ? "Meta" : "", event.altKey ? "Alt" : "", shifted ? "Shift" : ""].filter(Boolean);
  return [...prefix, key].join("+");
}

export function matchesBinding(event: KeyboardEvent, binding: string): boolean {
  const actual = eventBinding(event);
  const normalized = normalizeBinding(binding);
  if (!normalized.startsWith("Mod+")) return actual === normalized;
  // Mod is portable primary-modifier syntax. An additional primary modifier
  // is still an extra modifier and must not accidentally execute the command.
  if (event.ctrlKey === event.metaKey) return false;
  return actual === normalized.replace("Mod+", event.ctrlKey ? "Ctrl+" : "Meta+");
}

function browserStorage(): Storage | undefined {
  try { return typeof window === "undefined" ? undefined : window.localStorage; }
  catch { return undefined; }
}

export function loadKeymapOverrides(storage?: Pick<Storage, "getItem">): KeymapOverrides {
  try {
    const parsed = JSON.parse((storage ?? browserStorage())?.getItem(keymapStorageKey) ?? "{}");
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return {};
    return Object.fromEntries(Object.entries(parsed).flatMap(([id, value]) => commandById.has(id as CommandId) && Array.isArray(value) && value.every((binding) => typeof binding === "string") ? [[id, value.map(normalizeBinding)]] : [])) as KeymapOverrides;
  } catch { return {}; }
}

export function saveKeymapOverrides(overrides: KeymapOverrides, storage?: Pick<Storage, "setItem">): void {
  (storage ?? browserStorage())?.setItem(keymapStorageKey, JSON.stringify(overrides));
  if (typeof window !== "undefined") window.dispatchEvent(new window.Event("rlviz:keymap-change"));
}

export function setCommandBindings(id: CommandId, bindings: readonly string[], storage?: Pick<Storage, "getItem" | "setItem">): KeymapOverrides {
  commandDefinition(id);
  const overrides = { ...loadKeymapOverrides(storage), [id]: [...new Set(bindings.map(normalizeBinding).filter(Boolean))] };
  saveKeymapOverrides(overrides, storage);
  return overrides;
}

export function resetKeymapOverrides(storage?: Pick<Storage, "removeItem">): void {
  (storage ?? browserStorage())?.removeItem(keymapStorageKey);
  if (typeof window !== "undefined") window.dispatchEvent(new window.Event("rlviz:keymap-change"));
}

export function bindingsFor(id: CommandId, overrides = loadKeymapOverrides()): readonly string[] {
  const command = commandById.get(id);
  return overrides[id] ?? command?.defaultBindings ?? [];
}

export type KeymapConflict = { scope: CommandScope; binding: string; commandIds: CommandId[] };
export function detectKeymapConflicts(overrides: KeymapOverrides = loadKeymapOverrides(), scope?: CommandScope): KeymapConflict[] {
  const seen = new Map<string, CommandId[]>();
  commands.filter((command) => !scope || command.scope === scope).forEach((command) => {
    new Set(bindingsFor(command.id, overrides).map(normalizeBinding)).forEach((binding) => {
      const key = `${command.scope}\0${binding}`;
      seen.set(key, [...(seen.get(key) ?? []), command.id]);
    });
  });
  return [...seen].flatMap(([key, commandIds]) => commandIds.length > 1 ? [{ scope: key.split("\0")[0] as CommandScope, binding: key.split("\0")[1], commandIds }] : []);
}

export type CommandHandlers = Partial<Record<CommandId, (event: KeyboardEvent) => void | boolean>>;
export function useCommands(scope: CommandScope, handlers: CommandHandlers, enabled = true): void {
  useEffect(() => {
    if (!enabled) return;
    const onKeyDown = (event: KeyboardEvent) => {
      const typing = event.target instanceof HTMLInputElement || event.target instanceof HTMLTextAreaElement || (event.target instanceof HTMLElement && event.target.isContentEditable);
      const overrides = loadKeymapOverrides();
      for (const command of commands) {
        const handler = handlers[command.id];
        if (command.scope !== scope || !handler || (typing && !command.allowInInput)) continue;
        if (!bindingsFor(command.id, overrides).some((candidate) => matchesBinding(event, candidate))) continue;
        if (handler(event) === false) continue;
        event.preventDefault();
        return;
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [enabled, handlers, scope]);
}

export function commandDefinition(id: CommandId): CommandDefinition {
  const command = commandById.get(id);
  if (!command) throw new Error(`Unknown command: ${id}`);
  return command;
}

export function bindingLabel(id: CommandId, overrides = loadKeymapOverrides()): string {
  return bindingsFor(id, overrides).map((binding) => binding === "Space" ? "Space" : binding === "Escape" ? "Esc" : binding).join(" / ");
}

/** Re-render a command surface when this tab or another tab edits the keymap. */
export function useKeymapRevision(): number {
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const changed = () => setRevision((value) => value + 1);
    const stored = (event: StorageEvent) => { if (event.key === keymapStorageKey) changed(); };
    window.addEventListener("rlviz:keymap-change", changed);
    window.addEventListener("storage", stored);
    return () => {
      window.removeEventListener("rlviz:keymap-change", changed);
      window.removeEventListener("storage", stored);
    };
  }, []);
  return revision;
}
