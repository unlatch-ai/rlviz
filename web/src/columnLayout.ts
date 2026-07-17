export const groupColumnLayoutStorageKey = "rlviz.group-columns.v1";

export const optionalBuiltinColumns = ["reward", "pass", "status", "termination", "events", "errors", "tokens", "latency"] as const;
export type OptionalBuiltinColumn = typeof optionalBuiltinColumns[number];

export type GroupColumnLayout = {
  hiddenBuiltins: OptionalBuiltinColumn[];
  /** null means use the dataset's coverage-ranked defaults. */
  signalNames: string[] | null;
};

const Version = 1;
const MaxStoredBytes = 16 * 1024;
const MaxStoredSignals = 32;
const MaxSignalNameLength = 128;
const builtinSet = new Set<string>(optionalBuiltinColumns);
export const defaultGroupColumnLayout: GroupColumnLayout = { hiddenBuiltins: [], signalNames: null };

function browserStorage(): Storage | undefined {
  try { return typeof window === "undefined" ? undefined : window.localStorage; }
  catch { return undefined; }
}

function uniqueStrings(value: unknown, limit: number): string[] | undefined {
  if (!Array.isArray(value) || value.some((item) => typeof item !== "string")) return undefined;
  return [...new Set(value.filter((item) => item.length > 0 && item.length <= MaxSignalNameLength))].slice(0, limit);
}

export function loadGroupColumnLayout(storage?: Pick<Storage, "getItem">): GroupColumnLayout {
  try {
    const raw = (storage ?? browserStorage())?.getItem(groupColumnLayoutStorageKey);
    if (!raw || raw.length > MaxStoredBytes) return defaultGroupColumnLayout;
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    if (!parsed || Array.isArray(parsed) || parsed.version !== Version) return defaultGroupColumnLayout;
    const hidden = uniqueStrings(parsed.hiddenBuiltins, optionalBuiltinColumns.length);
    const signals = parsed.signalNames === null ? null : uniqueStrings(parsed.signalNames, MaxStoredSignals);
    if (!hidden || signals === undefined) return defaultGroupColumnLayout;
    return { hiddenBuiltins: hidden.filter((key): key is OptionalBuiltinColumn => builtinSet.has(key)), signalNames: signals };
  } catch { return defaultGroupColumnLayout; }
}

export function saveGroupColumnLayout(layout: GroupColumnLayout, storage?: Pick<Storage, "setItem">): void {
  try {
    const hiddenBuiltins = [...new Set(layout.hiddenBuiltins)].filter((key) => builtinSet.has(key));
    const signalNames = layout.signalNames === null ? null : [...new Set(layout.signalNames.filter((name) => name.length > 0 && name.length <= MaxSignalNameLength))].slice(0, MaxStoredSignals);
    (storage ?? browserStorage())?.setItem(groupColumnLayoutStorageKey, JSON.stringify({ version: Version, hiddenBuiltins, signalNames }));
  } catch { /* Storage is optional: private modes and embedded viewers can reject writes. */ }
}

export function resetGroupColumnLayout(storage?: Pick<Storage, "removeItem">): GroupColumnLayout {
  try { (storage ?? browserStorage())?.removeItem(groupColumnLayoutStorageKey); }
  catch { /* Keep the in-memory default usable. */ }
  return defaultGroupColumnLayout;
}
