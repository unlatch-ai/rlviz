# Use the workspace

The GUI is the only trajectory display. Collection, rollout, Detail, Guide, and Settings are movable modules. Additional rollouts open as rows by default.

## Collection

- `j` / `k` or arrow keys select a rollout and keep it in view.
- `Enter` opens the selected rollout. `a` adds it without replacing an open rollout.
- `[` / `]` change collection fidelity. The highest level names tool calls.
- Switch between rollout and trial grouping in the Collection header.

## Rollout and detail

- `j` / `k` move event by event. `e` jumps to the next error and `r` to the next reward or grader.
- `Enter` opens the selected section in place: overview, episode, events, then source. `Escape` goes back up. An unpinned side Detail closes as you descend so the same event is not shown twice.
- `d` opens or closes the shared Detail module. `Shift+D` pins it to the current rollout; `Shift+C` compacts or expands it.
- Drag the timeline window to pan, click to center it, or drag either edge to resize it.

## Modules and shortcuts

- `Tab` / `Shift+Tab` cycle modules. `Alt` plus an arrow activates the spatial neighbor.
- `Ctrl+m` toggles module move mode. `Ctrl+w` toggles seam resize mode. The same chord or `Escape` exits.
- `?` toggles Guide. `Shift+S` toggles Settings.
- The bottom bar shows the active module's current shortcuts. Guide includes the full default keybinding reference.
