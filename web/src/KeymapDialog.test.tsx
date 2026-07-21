import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { KeymapDialog } from "./KeymapDialog";
import { commandIds, keymapStorageKey } from "./commands";

const values = new Map<string, string>();
const storage = {
  getItem: vi.fn((key: string) => values.get(key) ?? null),
  setItem: vi.fn((key: string, value: string) => { values.set(key, value); }),
  removeItem: vi.fn((key: string) => { values.delete(key); }),
};

describe("keymap settings", () => {
  beforeEach(() => {
    values.clear(); storage.getItem.mockClear(); storage.setItem.mockClear(); storage.removeItem.mockClear();
    vi.stubGlobal("localStorage", storage);
  });
  afterEach(() => vi.unstubAllGlobals());

  it("groups commands by scope in an accessible dialog", () => {
    render(<KeymapDialog open onClose={() => {}} />);
    expect(screen.getByRole("dialog", { name: "Keyboard shortcuts" })).toHaveAttribute("aria-modal", "true");
    expect(screen.getByRole("button", { name: "Close keyboard shortcut settings" })).toHaveFocus();
    expect(screen.getByRole("heading", { name: "Trajectory" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Rollout group" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Behavioral paths" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Comparison" })).toBeInTheDocument();
    expect(screen.getByLabelText("Next event bindings")).toHaveValue("j");
  });

  it("keeps edits as drafts until a per-command save", () => {
    render(<KeymapDialog open onClose={() => {}} />);
    const input = screen.getByLabelText("Next event bindings");
    fireEvent.change(input, { target: { value: "l, ArrowDown" } });
    expect(storage.setItem).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Save Next event bindings" })).toBeEnabled();

    fireEvent.click(screen.getByRole("button", { name: "Save Next event bindings" }));
    expect(storage.setItem).toHaveBeenCalledOnce();
    expect(JSON.parse(values.get(keymapStorageKey)!)[commandIds.trajectory.next]).toEqual(["l", "ArrowDown"]);
    expect(input).toHaveValue("l, ArrowDown");
  });

  it("reports same-scope conflicts inline and blocks the conflicting save", () => {
    render(<KeymapDialog open onClose={() => {}} />);
    fireEvent.change(screen.getByLabelText("Next event bindings"), { target: { value: "k" } });
    expect(screen.getByText("k is also assigned to Previous event")).toHaveAttribute("role", "alert");
    expect(screen.getByLabelText("Next event bindings")).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByRole("button", { name: "Save Next event bindings" })).toBeDisabled();
    expect(storage.setItem).not.toHaveBeenCalled();
  });

  it("resets persisted overrides only after the explicit reset action", () => {
    values.set(keymapStorageKey, JSON.stringify({ [commandIds.trajectory.next]: ["n"] }));
    render(<KeymapDialog open onClose={() => {}} />);
    expect(screen.getByLabelText("Next event bindings")).toHaveValue("n");
    fireEvent.change(screen.getByLabelText("Previous event bindings"), { target: { value: "p" } });
    expect(storage.removeItem).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Reset browser overrides" }));
    expect(storage.removeItem).toHaveBeenCalledWith(keymapStorageKey);
    expect(screen.getByLabelText("Next event bindings")).toHaveValue("j");
    expect(screen.getByLabelText("Previous event bindings")).toHaveValue("k");
  });

  it("closes from the close button, backdrop, and Escape", () => {
    const close = vi.fn();
    const { container } = render(<KeymapDialog open onClose={close} />);
    fireEvent.click(screen.getByRole("button", { name: "Close keyboard shortcut settings" }));
    fireEvent.mouseDown(container.querySelector(".keymap-backdrop")!);
    fireEvent.keyDown(window, { key: "Escape" });
    expect(close).toHaveBeenCalledTimes(3);
  });
});
