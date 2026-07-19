# Design system

## Goal

RLViz should feel like a durable technical instrument: quiet, legible, dense
when useful, and consistent under long sessions. Visual polish comes from a
small coherent system, not a large collection of one-off CSS values.

## Principles

1. **Hierarchy before decoration.** One primary reading surface, supporting
   navigation, and contextual detail.
2. **Density without illegibility.** Default body text remains readable; compact
   mode reduces spacing before it reduces type size.
3. **Color carries meaning.** Use color for selection, state, severity, and
   semantic landmarks, not to make every event kind unique.
4. **Code looks like code.** Use monospace for payloads, IDs, paths, timestamps,
   numeric tables, and keycaps. Use the UI face for prose and controls.
5. **Interaction is visible.** Hover, focus, selected, active, disabled, loading,
   stale, and error states have consistent treatments.
6. **The source stays trustworthy.** Derived and inferred information is labeled
   differently from source-native facts.

## Token layers

The implementation should expose CSS custom properties through these layers:

- primitive color ramp: neutral surfaces and restrained accent ramps
- semantic color: text, muted text, border, focus, selection, success, warning,
  error, reward, grader, context change, and inferred data
- typography: display, heading, body, small, label, code, and numeric table
- spacing: 2, 4, 8, 12, 16, 24, 32
- size: control heights, rail widths, inspector widths, readable line lengths
- shape: radii and border widths
- elevation: flat, raised, overlay
- motion: short state transition and reduced-motion behavior

Components must not introduce literal colors or arbitrary font sizes when an
appropriate token exists.

## Typography baseline

- UI body: 13–14px depending on density
- Primary transcript/payload prose: 13–15px with 1.45–1.6 line height
- Small metadata: 11–12px
- Labels: 10–11px, sparing uppercase and letter spacing
- Code/data: 11–13px depending on context
- Avoid essential text below 10px

The current viewer's frequent 7–9px labels should be removed. Monospace should
not be the default for headings, explanatory copy, or buttons.

## Density

Support two deliberate density modes using tokens:

- Comfortable: default for transcript reading and first use
- Compact: smaller row heights and spacing for experienced users and large
  tables, while keeping essential text readable

Density is a user setting and must not fork component behavior.

## Core primitives

Build and test these before redesigning feature screens:

- application header and breadcrumb
- tabs and segmented controls
- panel and resizable/collapsible panel header
- button, icon button, menu item, and command item
- badge, status, semantic landmark, and provenance label
- metric cell and distribution legend
- search/filter field
- table header, row, selected row, and empty row
- code/payload block with expand, wrap, copy, and raw controls
- transcript message and tool span
- inspector section and property list
- tooltip, popover, dialog, toast, and empty/error/loading state
- keycap and shortcut sequence

Primitives should expose semantic props and state attributes rather than require
feature components to know internal CSS structure.

## Layout

- Primary content uses available width but keeps prose at a readable measure.
- Navigation and inspector widths are adjustable within tested bounds.
- The app must remain usable without the inspector open.
- A fixed minimum width is acceptable for the first desktop release, but a
  narrower window must explain or gracefully collapse rather than clip silently.
- Sticky headers and footers must not hide deep-linked content.

## Themes and customization

Theme mappings must use the same semantic roles without changing component
behavior. User themes may override validated token values. Do not allow plugins
to inject selectors or arbitrary CSS.

Selection and keyboard focus are separate semantic states. Components must
remain understandable when both states appear at once and must not rely on a
specific theme palette to preserve that distinction.

## Accessibility

- Meet WCAG AA contrast for text and interactive states.
- Keep a visible focus indicator distinct from selection.
- Ensure every mouse action has a keyboard route.
- Do not communicate pass/fail, change type, or severity by color alone.
- Respect reduced motion.
- Maintain semantic headings, landmarks, tables, dialogs, and accessible names.
- Return focus correctly after dialogs, palettes, and temporary panels close.

## Visual review contract

Every new primitive or visible workflow is reviewed at the fixed desktop test
viewport in comfortable and compact density. Review selection, keyboard focus,
hover, disabled, empty, loading, partial, stale, and failure states. Stable
trajectory, group, and comparison screenshots act as regression evidence, not
as a substitute for interaction tests.
