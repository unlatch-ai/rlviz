# Default keybindings

The bottom bar always shows the commands available in the active module. These are the complete defaults; configured remaps replace them in the bar.

## Workspace

- Arrow keys activate the module in that direction. `Tab` / `Shift+Tab` cycles through modules.
- `Ctrl+m` toggles module move mode. Arrow keys move the active module; `Ctrl+m` or `Escape` exits.
- `Ctrl+w` toggles seam resize mode. Arrow keys resize the nearest seam; `Ctrl+w` or `Escape` exits.
- `?` toggles Guide. `Shift+S` toggles Settings. `x` closes either when active.
- `Shift+R` resets the workspace to its default modules and layout.

## Collection

- `j` / `k` selects the next or previous rollout.
- `Enter` opens the selected rollout. `a` adds it as another rollout module.
- `[` / `]` decreases or increases fidelity.
- `/` focuses the collection filter. `t` toggles the Collection module.

## Rollout

- `j` / `k` moves to the next or previous event.
- `e`, `r`, and `c` jump to the next error, reward or grader, and context change.
- `Enter` opens the selected section in place. `Escape` ascends one layer, then closes the rollout.
- `[` / `]` changes overview fidelity. `+`, `-`, and `0` zoom or fit the axis.
- `n` / `p` replaces the active rollout with the next or previous collection item.
- `d` opens or closes Detail. `Shift+D` pins it to this rollout. `z` spotlights the rollout; press `z` again to restore the workspace. `x` closes the active module.

## Detail

- `j` / `k` moves through the detail module's rollout.
- `e` jumps to its next error.
- `d` or `x` closes Detail. `Shift+D` pins or unpins it. `Shift+C` compacts or expands it.
