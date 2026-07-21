import { useEffect, useId, useMemo, useRef, useState } from "react";
import { bindingsFor, commands, detectKeymapConflicts, loadKeymapOverrides, normalizeBinding, resetKeymapOverrides, setCommandBindings } from "./commands";
import type { CommandId, CommandScope, KeymapOverrides } from "./commands";

const scopeLabels: Record<CommandScope, string> = {
  trajectory: "Trajectory",
  group: "Rollout group",
  paths: "Behavioral paths",
  comparison: "Comparison",
  overlay: "Overlays",
};

type Drafts = Record<CommandId, string>;

function currentDrafts(overrides = loadKeymapOverrides()): Drafts {
  return Object.fromEntries(commands.map((command) => [command.id, bindingsFor(command.id, overrides).join(", ")])) as Drafts;
}

function parseBindings(value: string): string[] {
  return [...new Set(value.split(",").map((binding) => normalizeBinding(binding.trim())).filter(Boolean))];
}

function draftOverrides(drafts: Drafts): KeymapOverrides {
  return Object.fromEntries(commands.map((command) => [command.id, parseBindings(drafts[command.id])])) as KeymapOverrides;
}

export function KeymapDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const titleId = useId();
  const descriptionId = useId();
  const closeRef = useRef<HTMLButtonElement>(null);
  const [drafts, setDrafts] = useState<Drafts>(() => currentDrafts());
  const [saved, setSaved] = useState<Drafts>(() => currentDrafts());

  useEffect(() => {
    if (!open) return;
    const next = currentDrafts();
    setDrafts(next);
    setSaved(next);
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    closeRef.current?.focus();
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") { event.preventDefault(); onClose(); }
    };
    window.addEventListener("keydown", closeOnEscape);
    return () => { window.removeEventListener("keydown", closeOnEscape); previousFocus?.focus(); };
  }, [onClose, open]);

  const conflicts = useMemo(() => detectKeymapConflicts(draftOverrides(drafts)), [drafts]);
  const conflictsByCommand = useMemo(() => {
    const result = new Map<CommandId, string[]>();
    conflicts.forEach((conflict) => conflict.commandIds.forEach((id) => {
      const peers = conflict.commandIds.filter((peer) => peer !== id).map((peer) => commands.find((command) => command.id === peer)?.label ?? peer);
      result.set(id, [...(result.get(id) ?? []), `${conflict.binding} is also assigned to ${peers.join(", ")}`]);
    }));
    return result;
  }, [conflicts]);

  if (!open) return null;

  const save = (id: CommandId) => {
    const normalized = parseBindings(drafts[id]);
    setCommandBindings(id, normalized);
    const value = normalized.join(", ");
    setDrafts((current) => ({ ...current, [id]: value }));
    setSaved((current) => ({ ...current, [id]: value }));
  };
  const reset = () => {
    resetKeymapOverrides();
    const defaults = currentDrafts({});
    setDrafts(defaults);
    setSaved(defaults);
  };

  return <div className="modal-backdrop keymap-backdrop" onMouseDown={onClose}>
    <section className="keymap-dialog" role="dialog" aria-modal="true" aria-labelledby={titleId} aria-describedby={descriptionId} onMouseDown={(event) => event.stopPropagation()}>
      <header>
        <div><span className="eyebrow">Preferences</span><h2 id={titleId}>Keyboard shortcuts</h2><p id={descriptionId}>Separate bindings with commas. Browser changes override project defaults.</p></div>
        <button ref={closeRef} type="button" onClick={onClose} aria-label="Close keyboard shortcut settings">×</button>
      </header>
      <div className="keymap-scopes">
        {(Object.keys(scopeLabels) as CommandScope[]).map((scope) => <section className="keymap-scope" key={scope} aria-labelledby={`${titleId}-${scope}`}>
          <h3 id={`${titleId}-${scope}`}>{scopeLabels[scope]}</h3>
          <div className="keymap-commands">
            {commands.filter((command) => command.scope === scope).map((command) => {
              const inputId = `${titleId}-${command.id.replaceAll(".", "-")}`;
              const errors = conflictsByCommand.get(command.id) ?? [];
              const errorId = `${inputId}-conflict`;
              const changed = drafts[command.id] !== saved[command.id];
              return <div className={`keymap-command ${errors.length ? "has-conflict" : ""}`} key={command.id}>
                <label htmlFor={inputId}><strong>{command.label}</strong><code>{command.id}</code></label>
                <div className="keymap-edit">
                  <input id={inputId} value={drafts[command.id]} onChange={(event) => setDrafts((current) => ({ ...current, [command.id]: event.target.value }))} aria-label={`${command.label} bindings`} aria-invalid={errors.length > 0 || undefined} aria-describedby={errors.length ? errorId : undefined} placeholder="Unbound" />
                  <button type="button" onClick={() => save(command.id)} disabled={!changed || errors.length > 0} aria-label={`Save ${command.label} bindings`}>Save</button>
                </div>
                {errors.length > 0 && <div id={errorId} className="keymap-conflict" role="alert">{errors.join(". ")}</div>}
              </div>;
            })}
          </div>
        </section>)}
      </div>
      <footer><button type="button" onClick={reset}>Reset browser overrides</button><button type="button" onClick={onClose}>Done</button></footer>
    </section>
  </div>;
}
