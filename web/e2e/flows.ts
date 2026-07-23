export type Observable = {
  target: "shell" | "browse" | "read" | "compare" | "selected-row" | "selected-event" | "filter" | "strip" | "marked-rows" | "alert" | "rail" | "stage" | "focus-lane" | "context-lane" | "console" | "breadcrumb" | "reference" | "seam";
  selector?: string;
  attribute?: string;
  equals?: string;
  notEquals?: string;
  contains?: string;
  value?: string;
  absent?: boolean;
  count?: number;
  boxEquals?: string;
  boxNotEquals?: string;
  boxBelow?: string;
  boxFills?: string;
  attributeEqualsCapture?: string;
  attributeNotEqualsCapture?: string;
  attributeNumberLte?: number;
  attributeNumberGte?: number;
  relativeXGte?: number;
  relativeXLte?: number;
  withinViewport?: boolean;
  pageFitsViewport?: boolean;
};

export type FlowAction =
  | { kind: "key"; value: string }
  | { kind: "filter"; value: string }
  | { kind: "fill"; target: string; value: string }
  | { kind: "click"; target: string; clicks?: number }
  | { kind: "strip-click"; eventIndex: number }
  | { kind: "capture-box"; target: string; key: string }
  | { kind: "capture-attribute"; target: string; attribute: string; key: string }
  | { kind: "seam-drag"; name: "rail" | "focusContext" | "focusLane" | "console"; dx: number; dy: number }
  | { kind: "timeline-click"; ratio: number }
  | { kind: "timeline-drag"; part: "window" | "start" | "end"; dx: number }
  | { kind: "viewport"; width: number; height: number }
  | { kind: "reload" }
  | { kind: "history-back" };

export type FlowStep = { action: FlowAction; expect: Observable[] };
export type Flow = { id: string; name: string; keyboardOnly: boolean; surfaces: Array<"daemon" | "webapp">; steps: FlowStep[]; webappSteps?: FlowStep[]; webappExample?: string };

const mode = (target: Observable["target"]): Observable => ({ target });
const selectedRow = (id: string): Observable => ({ target: "selected-row", contains: id });
const selectedEvent = (text: string): Observable => ({ target: "selected-event", contains: text });
const attr = (target: Observable["target"], attribute: string, equals: string): Observable => ({ target, attribute, equals });
const depth = (level: 1 | 2 | 3 | 4): Observable[] => [attr("read", "data-depth", String(level)), { target: "rail", selector: ".lane-track.active-zone .lane-state", ...(level === 1 ? { contains: "overview" } : { equals: ["", "overview", "episodes", "events", "raw"][level] }) }];

export const flows: Flow[] = [
  {
    id: "a", name: "source-order-sweep", keyboardOnly: true, surfaces: ["daemon", "webapp"], steps: [
      { action: { kind: "key", value: "j" }, expect: [selectedRow("partial"), { target: "browse", contains: "7 trajectories" }] },
      { action: { kind: "key", value: "j" }, expect: [selectedRow("fourth")] },
      { action: { kind: "filter", value: "reference" }, expect: [selectedRow("reference"), attr("browse", "data-filter", "reference")] },
      { action: { kind: "filter", value: "" }, expect: [selectedRow("reference"), attr("browse", "data-filter", "")] },
      { action: { kind: "key", value: "Escape" }, expect: [mode("browse"), selectedRow("reference")] },
    ],
    webappSteps: [
      { action: { kind: "key", value: "j" }, expect: [selectedRow("checkout-rollout-02"), { target: "browse", contains: "16 trajectories" }] },
      { action: { kind: "key", value: "j" }, expect: [selectedRow("checkout-rollout-03")] },
      { action: { kind: "filter", value: "checkout-rollout-01" }, expect: [selectedRow("checkout-rollout-01"), attr("browse", "data-filter", "checkout-rollout-01")] },
      { action: { kind: "filter", value: "" }, expect: [{ target: "selected-row", contains: "checkout-rollout" }, attr("browse", "data-filter", "")] },
      { action: { kind: "key", value: "Escape" }, expect: [mode("browse")] },
    ],
  },
  {
    id: "b", name: "open-read-return", keyboardOnly: true, surfaces: ["daemon", "webapp"], steps: [
      { action: { kind: "filter", value: "demo" }, expect: [mode("browse"), attr("browse", "data-filter", "demo")] },
      // typing leaves focus in the filter input where commands are
      // (correctly) suppressed; Escape returns focus to the reading surface
      // without clearing the filter.
      { action: { kind: "key", value: "Escape" }, expect: [mode("browse"), attr("browse", "data-filter", "demo")] },
      { action: { kind: "key", value: "Enter" }, expect: [mode("read"), attr("read", "data-trajectory", "candidate"), ...depth(1)] },
      { action: { kind: "key", value: "j" }, expect: [selectedEvent("Final reward")] },
      { action: { kind: "key", value: "k" }, expect: [selectedEvent("Policy error")] },
      { action: { kind: "key", value: "e" }, expect: [selectedEvent("Policy error")] },
      { action: { kind: "key", value: "Escape" }, expect: [mode("browse"), selectedRow("candidate"), attr("browse", "data-filter", "demo"), attr("browse", "data-fidelity", "signals")] },
    ],
    webappSteps: [
      { action: { kind: "filter", value: "checkout-rollout-14" }, expect: [attr("browse", "data-filter", "checkout-rollout-14")] },
      { action: { kind: "key", value: "Escape" }, expect: [mode("browse"), attr("browse", "data-filter", "checkout-rollout-14")] },
      { action: { kind: "key", value: "Enter" }, expect: [mode("read"), ...depth(1)] },
      { action: { kind: "key", value: "j" }, expect: [selectedEvent("payment · step 46")] },
      { action: { kind: "key", value: "k" }, expect: [selectedEvent("Recoverable submit timeout · retry 1")] },
      { action: { kind: "key", value: "e" }, expect: [selectedEvent("Recoverable submit timeout · retry 2")] },
      { action: { kind: "key", value: "Escape" }, expect: [mode("browse"), attr("browse", "data-filter", "checkout-rollout-14"), attr("browse", "data-fidelity", "signals")] },
    ],
  },
  {
    id: "c", name: "read-sweep", keyboardOnly: true, surfaces: ["daemon"], steps: [
      { action: { kind: "filter", value: "demo" }, expect: [attr("browse", "data-filter", "demo")] },
      { action: { kind: "key", value: "Escape" }, expect: [mode("browse"), attr("browse", "data-filter", "demo")] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-trajectory", "candidate")] },
      { action: { kind: "capture-box", target: ".focus-lane[data-trajectory='candidate']", key: "first-lane" }, expect: [{ target: "focus-lane", count: 1 }] },
      { action: { kind: "key", value: "+" }, expect: [{ target: "strip", attribute: "data-visible-events", equals: "3" }, attr("read", "data-axis-start", "15.0000"), attr("read", "data-axis-end", "40.0000")] },
      { action: { kind: "key", value: "Enter" }, expect: [...depth(2)] },
      { action: { kind: "key", value: "n" }, expect: [attr("read", "data-trajectory", "partial"), ...depth(2), attr("read", "data-axis-start", "15.0000"), attr("read", "data-axis-end", "40.0000"), attr("shell", "data-filter", "demo")] },
      { action: { kind: "key", value: "n" }, expect: [attr("read", "data-trajectory", "fourth"), ...depth(2), attr("read", "data-axis-start", "15.0000"), attr("shell", "data-filter", "demo")] },
      { action: { kind: "key", value: "p" }, expect: [attr("read", "data-trajectory", "partial"), ...depth(2), attr("read", "data-axis-start", "15.0000"), attr("shell", "data-filter", "demo")] },
      { action: { kind: "key", value: "Escape" }, expect: [...depth(1), attr("read", "data-trajectory", "partial")] },
      { action: { kind: "key", value: "Escape" }, expect: [mode("browse"), selectedRow("partial"), attr("browse", "data-filter", "demo")] },
    ],
  },
  {
    id: "d", name: "lane-add-close-swap", keyboardOnly: true, surfaces: ["daemon"], steps: [
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-trajectory", "candidate")] },
      { action: { kind: "capture-box", target: ".focus-lane[data-trajectory='candidate']", key: "first-lane" }, expect: [{ target: "focus-lane", count: 1 }] },
      { action: { kind: "key", value: "Tab" }, expect: [attr("shell", "data-active-zone", "detail")] },
      { action: { kind: "key", value: "Tab" }, expect: [attr("shell", "data-active-zone", "rail")] },
      { action: { kind: "key", value: "j" }, expect: [selectedRow("partial")] },
      { action: { kind: "key", value: "a" }, expect: [{ target: "focus-lane", count: 2 }, { target: "focus-lane", selector: ".focus-lane[data-trajectory='partial']", boxBelow: "first-lane" }] },
      { action: { kind: "key", value: "Shift+Tab" }, expect: [attr("shell", "data-active-zone", "detail")] },
      { action: { kind: "key", value: "Shift+Tab" }, expect: [{ target: "focus-lane", selector: ".lane-track.active-zone", attribute: "data-trajectory", equals: "partial" }] },
      { action: { kind: "key", value: "+" }, expect: [{ target: "focus-lane", selector: ".focus-lane[data-trajectory='candidate']", attribute: "data-axis-start", equals: "0.0000" }, { target: "focus-lane", selector: ".focus-lane[data-trajectory='partial']", attribute: "data-axis-start", equals: "15.0000" }] },
      { action: { kind: "key", value: ">" }, expect: [{ target: "focus-lane", selector: ".focus-lane[data-trajectory='candidate']", attribute: "data-axis-start", equals: "15.0000" }] },
      { action: { kind: "key", value: "Shift+V" }, expect: [attr("shell", "data-direction", "columns")] },
      { action: { kind: "key", value: "Shift+H" }, expect: [attr("shell", "data-direction", "rows")] },
      { action: { kind: "key", value: "Tab" }, expect: [attr("shell", "data-active-zone", "detail")] },
      { action: { kind: "key", value: "Tab" }, expect: [attr("shell", "data-active-zone", "rail")] },
      { action: { kind: "key", value: "j" }, expect: [selectedRow("fourth")] },
      { action: { kind: "key", value: "a" }, expect: [{ target: "context-lane", count: 1 }, attr("context-lane", "data-trajectory", "fourth")] },
      { action: { kind: "key", value: "Shift+Tab" }, expect: [attr("shell", "data-active-zone", "detail")] },
      { action: { kind: "key", value: "Shift+Tab" }, expect: [{ target: "context-lane", selector: ".context-lane.active-zone[data-trajectory='fourth']" }] },
      { action: { kind: "key", value: "Shift+Enter" }, expect: [{ target: "focus-lane", selector: ".lane-track.active-zone", attribute: "data-trajectory", equals: "fourth" }, { target: "context-lane", count: 1 }] },
      { action: { kind: "key", value: "Shift+A" }, expect: [{ target: "reference", equals: "fourth" }] },
      { action: { kind: "key", value: "x" }, expect: [{ target: "focus-lane", count: 1 }, { target: "reference", equals: "none" }] },
    ],
  },
  {
    id: "e", name: "zoom-depth", keyboardOnly: true, surfaces: ["daemon", "webapp"], steps: [
      { action: { kind: "key", value: "Enter" }, expect: [mode("read"), ...depth(1), attr("strip", "data-strip-mode", "marks")] },
      { action: { kind: "key", value: "+" }, expect: [{ target: "strip", attribute: "data-visible-events", notEquals: "6" }, attr("read", "data-axis-start", "15.0000"), attr("read", "data-axis-end", "40.0000")] },
      { action: { kind: "key", value: "+" }, expect: [{ target: "strip", attribute: "data-visible-events", notEquals: "6" }, attr("read", "data-axis-start", "22.5000"), attr("read", "data-axis-end", "35.0000")] },
      { action: { kind: "key", value: "c" }, expect: [selectedEvent("Context compacted"), attr("read", "data-axis-start", "8.1250")] },
      { action: { kind: "key", value: "Enter" }, expect: [...depth(2)] },
      { action: { kind: "key", value: "Enter" }, expect: [...depth(3)] },
      { action: { kind: "key", value: "Escape" }, expect: [...depth(2)] },
      { action: { kind: "key", value: "Escape" }, expect: [...depth(1)] },
      { action: { kind: "key", value: "0" }, expect: [{ target: "strip", attribute: "data-visible-events", equals: "6" }, attr("read", "data-axis-start", "0.0000"), attr("read", "data-axis-end", "50.0000")] },
    ],
    webappSteps: [
      { action: { kind: "filter", value: "checkout-rollout-14" }, expect: [selectedRow("checkout-rollout-14"), attr("browse", "data-filter", "checkout-rollout-14")] },
      { action: { kind: "key", value: "Escape" }, expect: [selectedRow("checkout-rollout-14")] },
      { action: { kind: "key", value: "Enter" }, expect: [mode("read"), ...depth(1)] },
      { action: { kind: "key", value: "+" }, expect: [attr("read", "data-axis-start", "225.0000"), attr("read", "data-axis-end", "570.0000")] },
      { action: { kind: "key", value: "+" }, expect: [attr("read", "data-axis-start", "337.5000"), attr("read", "data-axis-end", "510.0000"), attr("strip", "data-strip-mode", "marks")] },
      { action: { kind: "key", value: "e" }, expect: [selectedEvent("Recoverable submit timeout · retry 2")] },
      { action: { kind: "key", value: "Enter" }, expect: [...depth(2)] },
      { action: { kind: "key", value: "Enter" }, expect: [...depth(3)] },
      { action: { kind: "key", value: "Escape" }, expect: [...depth(2)] },
      { action: { kind: "key", value: "Escape" }, expect: [...depth(1)] },
      { action: { kind: "key", value: "0" }, expect: [attr("read", "data-axis-start", "0.0000"), attr("read", "data-axis-end", "690.0000")] },
    ],
  },
  {
    id: "f", name: "open-read-return-clicks", keyboardOnly: false, surfaces: ["daemon"], steps: [
      { action: { kind: "filter", value: "demo" }, expect: [attr("browse", "data-filter", "demo")] },
      { action: { kind: "click", target: "[role=option]", clicks: 2 }, expect: [mode("read"), attr("read", "data-trajectory", "candidate")] },
      { action: { kind: "strip-click", eventIndex: 1 }, expect: [selectedEvent("Context compacted")] },
      { action: { kind: "click", target: ".moment:has-text('Policy error')" }, expect: [selectedEvent("Policy error")] },
      { action: { kind: "key", value: "Escape" }, expect: [mode("browse"), selectedRow("candidate"), attr("browse", "data-filter", "demo"), attr("browse", "data-fidelity", "signals")] },
    ],
  },
  {
    id: "g", name: "anti-jitter-depth-zoom", keyboardOnly: true, surfaces: ["daemon", "webapp"], steps: [
      { action: { kind: "key", value: "Enter" }, expect: [{ target: "focus-lane", count: 1 }] },
      { action: { kind: "capture-box", target: ".lane-track.active-zone", key: "lane-track" }, expect: [...depth(1)] },
      // Enter moves into the selected section and dismisses an unpinned
      // Detail, so the lane intentionally expands once. It must then remain
      // stable while navigating inside that section.
      { action: { kind: "key", value: "Enter" }, expect: [...depth(2), { target: "focus-lane", selector: ".lane-track.active-zone", boxNotEquals: "lane-track" }, { target: "console", absent: true }] },
      { action: { kind: "capture-box", target: ".lane-track.active-zone", key: "expanded-lane-track" }, expect: [...depth(2)] },
      { action: { kind: "key", value: "+" }, expect: [{ target: "focus-lane", selector: ".lane-track.active-zone", boxEquals: "expanded-lane-track" }] },
    ],
  },
  {
    id: "h", name: "seam-resize-pointer-keyboard-persistence", keyboardOnly: false, surfaces: ["daemon"], steps: [
      // Pointer sash-dragging is dockview's own library-tested behavior; our
      // contract covers keyboard resize (real input) and persistence.
      { action: { kind: "key", value: "Enter" }, expect: [{ target: "focus-lane", count: 1 }] },
      { action: { kind: "capture-box", target: ".workspace-console", key: "console-before" }, expect: [{ target: "console" }] },
      { action: { kind: "key", value: "Control+w" }, expect: [attr("console", "data-resize-mode", "true")] },
      { action: { kind: "key", value: "ArrowLeft" }, expect: [{ target: "console" }] },
      { action: { kind: "key", value: "ArrowLeft" }, expect: [{ target: "console", boxNotEquals: "console-before" }] },
      { action: { kind: "key", value: "Control+w" }, expect: [attr("console", "data-resize-mode", "false")] },
      { action: { kind: "capture-box", target: ".workspace-console", key: "console-persist" }, expect: [{ target: "console" }] },
      { action: { kind: "reload" }, expect: [{ target: "console", boxEquals: "console-persist" }, { target: "focus-lane", count: 1 }] },
    ],
  },
  {
    id: "i", name: "jumplist-restoration", keyboardOnly: true, surfaces: ["daemon", "webapp"], steps: [
      { action: { kind: "key", value: "Enter" }, expect: [...depth(1)] },
      { action: { kind: "key", value: "Enter" }, expect: [...depth(2)] },
      { action: { kind: "key", value: "Control+o" }, expect: [...depth(1)] },
      { action: { kind: "key", value: "Control+i" }, expect: [...depth(2)] },
      { action: { kind: "key", value: "x" }, expect: [{ target: "focus-lane", count: 0 }] },
      { action: { kind: "history-back" }, expect: [{ target: "focus-lane", count: 1 }, ...depth(2)] },
      { action: { kind: "key", value: "Control+o" }, expect: [...depth(1)] },
    ],
  },
  {
    id: "j", name: "rail-fidelity-ladder-with-lanes", keyboardOnly: true, surfaces: ["daemon", "webapp"], steps: [
      { action: { kind: "key", value: "Enter" }, expect: [{ target: "focus-lane", count: 1 }] },
      { action: { kind: "key", value: "Tab" }, expect: [attr("shell", "data-active-zone", "detail")] },
      { action: { kind: "key", value: "Tab" }, expect: [attr("shell", "data-active-zone", "rail")] },
      { action: { kind: "key", value: "[" }, expect: [{ target: "rail", selector: ".browse-list", attribute: "data-fidelity-level", equals: "L0" }, { target: "rail", selector: ".collection-signals", absent: true }, { target: "rail", selector: ".browse-row", attribute: "data-columns", equals: "false" }] },
      { action: { kind: "key", value: "]" }, expect: [{ target: "rail", selector: ".browse-list", attribute: "data-fidelity-level", equals: "L1" }, { target: "rail", selector: ".browse-row .collection-signals" }, { target: "rail", selector: ".browse-row .verdict:not(:empty)" }, { target: "rail", selector: ".browse-row", attribute: "data-columns", equals: "false" }] },
      { action: { kind: "key", value: "]" }, expect: [{ target: "rail", selector: ".browse-list", attribute: "data-fidelity-level", equals: "L2" }, { target: "rail", selector: ".browse-row", attribute: "data-columns", equals: "true" }, { target: "rail", selector: ".browse-row .numeric" }, { target: "focus-lane", count: 1 }] },
      { action: { kind: "key", value: "Shift+Tab" }, expect: [attr("shell", "data-active-zone", "detail")] },
      { action: { kind: "key", value: "Shift+Tab" }, expect: [{ target: "focus-lane", selector: ".lane-track.active-zone" }] },
    ],
  },
  {
    id: "k", name: "tab-cycle-rail-and-lanes", keyboardOnly: true, surfaces: ["daemon", "webapp"], steps: [
      { action: { kind: "key", value: "Enter" }, expect: [{ target: "focus-lane", count: 1 }] },
      { action: { kind: "key", value: "Tab" }, expect: [attr("shell", "data-active-zone", "detail")] },
      { action: { kind: "key", value: "Tab" }, expect: [attr("shell", "data-active-zone", "rail")] },
      { action: { kind: "key", value: "j" }, expect: [{ target: "selected-row" }] },
      { action: { kind: "key", value: "a" }, expect: [{ target: "focus-lane", count: 2 }] },
      { action: { kind: "capture-attribute", target: ".instrument-shell", attribute: "data-active-zone", key: "zone-rail" }, expect: [attr("shell", "data-active-zone", "rail")] },
      { action: { kind: "key", value: "Shift+Tab" }, expect: [attr("shell", "data-active-zone", "detail")] },
      // Shift+Tab again: some lane becomes active (dockview group order is
      // not the contract; identity-by-capture is).
      { action: { kind: "key", value: "Shift+Tab" }, expect: [{ target: "focus-lane", selector: ".lane-track.active-zone" }] },
      { action: { kind: "capture-attribute", target: ".lane-track.active-zone", attribute: "data-lane-id", key: "lane-b" }, expect: [{ target: "focus-lane", selector: ".lane-track.active-zone" }] },
      { action: { kind: "key", value: "t" }, expect: [{ target: "rail", absent: true }, { target: "focus-lane", count: 2 }] },
      { action: { kind: "key", value: "Tab" }, expect: [attr("shell", "data-active-zone", "detail")] },
      { action: { kind: "key", value: "Tab" }, expect: [{ target: "focus-lane", selector: ".lane-track.active-zone" }] },
      { action: { kind: "capture-attribute", target: ".lane-track.active-zone", attribute: "data-lane-id", key: "lane-a" }, expect: [{ target: "focus-lane", selector: ".lane-track.active-zone", attribute: "data-lane-id", attributeNotEqualsCapture: "lane-b" }] },
      { action: { kind: "key", value: "Tab" }, expect: [{ target: "focus-lane", selector: ".lane-track.active-zone", attribute: "data-lane-id", attributeEqualsCapture: "lane-b" }] },
      { action: { kind: "key", value: "Shift+Tab" }, expect: [{ target: "focus-lane", selector: ".lane-track.active-zone", attribute: "data-lane-id", attributeEqualsCapture: "lane-a" }] },
      { action: { kind: "key", value: "x" }, expect: [{ target: "focus-lane", count: 1 }, { target: "rail", absent: true }] },
      { action: { kind: "key", value: "x" }, expect: [{ target: "focus-lane", count: 0 }, { target: "rail" }, attr("shell", "data-active-zone", "rail")] },
    ],
  },
  {
    id: "l", name: "context-lane-sweep-preserves-focus", keyboardOnly: true, surfaces: ["daemon"], steps: [
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-trajectory", "candidate")] },
      { action: { kind: "key", value: "Tab" }, expect: [attr("shell", "data-active-zone", "detail")] },
      { action: { kind: "key", value: "Tab" }, expect: [attr("shell", "data-active-zone", "rail")] },
      { action: { kind: "key", value: "j" }, expect: [selectedRow("partial")] },
      { action: { kind: "key", value: "a" }, expect: [{ target: "focus-lane", count: 2 }] },
      { action: { kind: "key", value: "j" }, expect: [selectedRow("fourth")] },
      { action: { kind: "key", value: "a" }, expect: [attr("context-lane", "data-trajectory", "fourth")] },
      { action: { kind: "key", value: "Shift+Tab" }, expect: [attr("shell", "data-active-zone", "detail")] },
      { action: { kind: "key", value: "Shift+Tab" }, expect: [{ target: "context-lane", selector: ".context-lane.active-zone[data-trajectory='fourth']" }] },
      { action: { kind: "key", value: "n" }, expect: [attr("context-lane", "data-trajectory", "reference"), { target: "focus-lane", selector: ".focus-lane[data-trajectory='candidate']" }, { target: "focus-lane", selector: ".focus-lane[data-trajectory='partial']" }] },
    ],
  },
  {
    id: "m", name: "episode-pointer-descend-strip-ascend", keyboardOnly: false, surfaces: ["daemon"], steps: [
      { action: { kind: "filter", value: "layered" }, expect: [selectedRow("layered")] },
      { action: { kind: "key", value: "Escape" }, expect: [selectedRow("layered")] },
      { action: { kind: "key", value: "Enter" }, expect: [...depth(1), selectedEvent("Policy error")] },
      { action: { kind: "key", value: "+" }, expect: [attr("read", "data-axis-start", "17.0000")] },
      { action: { kind: "key", value: "+" }, expect: [attr("read", "data-axis-start", "25.5000")] },
      { action: { kind: "key", value: "+" }, expect: [attr("read", "data-axis-start", "29.2400")] },
      { action: { kind: "capture-attribute", target: ".lane-track.active-zone", attribute: "data-axis-start", key: "pre-descend-start" }, expect: [attr("read", "data-axis-start", "29.2400")] },
      { action: { kind: "capture-attribute", target: ".lane-track.active-zone", attribute: "data-axis-end", key: "pre-descend-end" }, expect: [attr("read", "data-axis-end", "36.2400")] },
      { action: { kind: "key", value: "Enter" }, expect: [...depth(2), attr("read", "data-episode", "stage:verify#1"), { target: "console", absent: true }, { target: "rail", selector: ".episode-button.selected", attribute: "data-episode-key", equals: "stage:verify#1" }] },
      { action: { kind: "capture-attribute", target: ".episode-strip", attribute: "data-selected-x", key: "layer-anchor" }, expect: [{ target: "rail", selector: ".episode-button.selected" }] },
      { action: { kind: "click", target: ".episode-button.selected" }, expect: [...depth(3), attr("read", "data-episode", "stage:verify#1"), attr("read", "data-axis-start", "29.0000"), attr("read", "data-axis-end", "36.3529"), { target: "read", attribute: "data-axis-start", attributeNumberLte: 29 }, { target: "read", attribute: "data-axis-end", attributeNumberGte: 34 }, { target: "rail", selector: ".shape-strip.compact", attribute: "data-selected-x", attributeEqualsCapture: "layer-anchor" }] },
      { action: { kind: "click", target: ".shape-strip.compact svg" }, expect: [...depth(2), attr("read", "data-episode", "stage:verify#1"), { target: "read", attribute: "data-axis-start", attributeEqualsCapture: "pre-descend-start" }, { target: "read", attribute: "data-axis-end", attributeEqualsCapture: "pre-descend-end" }, { target: "rail", selector: ".episode-strip", attribute: "data-selected-x", attributeEqualsCapture: "layer-anchor" }] },
    ],
  },
  {
    id: "n", name: "episode-keyboard-descend-ascend-twin", keyboardOnly: true, surfaces: ["daemon", "webapp"], webappExample: "300-event coding trace", steps: [
      { action: { kind: "filter", value: "layered" }, expect: [selectedRow("layered")] },
      { action: { kind: "key", value: "Escape" }, expect: [selectedRow("layered")] },
      { action: { kind: "key", value: "Enter" }, expect: [...depth(1), selectedEvent("Policy error")] },
      { action: { kind: "key", value: "+" }, expect: [attr("read", "data-axis-start", "17.0000")] },
      { action: { kind: "key", value: "+" }, expect: [attr("read", "data-axis-start", "25.5000")] },
      { action: { kind: "key", value: "+" }, expect: [attr("read", "data-axis-start", "29.2400")] },
      { action: { kind: "capture-attribute", target: ".lane-track.active-zone", attribute: "data-axis-start", key: "pre-descend-start" }, expect: [attr("read", "data-axis-start", "29.2400")] },
      { action: { kind: "capture-attribute", target: ".lane-track.active-zone", attribute: "data-axis-end", key: "pre-descend-end" }, expect: [attr("read", "data-axis-end", "36.2400")] },
      { action: { kind: "key", value: "Enter" }, expect: [...depth(2), attr("read", "data-episode", "stage:verify#1"), { target: "console", absent: true }, { target: "rail", selector: ".episode-button.selected", attribute: "data-episode-key", equals: "stage:verify#1" }] },
      { action: { kind: "capture-attribute", target: ".episode-strip", attribute: "data-selected-x", key: "layer-anchor" }, expect: [{ target: "rail", selector: ".episode-button.selected" }] },
      { action: { kind: "key", value: "Enter" }, expect: [...depth(3), attr("read", "data-episode", "stage:verify#1"), attr("read", "data-axis-start", "29.0000"), attr("read", "data-axis-end", "36.3529"), { target: "read", attribute: "data-axis-start", attributeNumberLte: 29 }, { target: "read", attribute: "data-axis-end", attributeNumberGte: 34 }, { target: "rail", selector: ".shape-strip.compact", attribute: "data-selected-x", attributeEqualsCapture: "layer-anchor" }] },
      { action: { kind: "key", value: "Enter" }, expect: [...depth(4), { target: "rail", selector: ".lane-source" }, { target: "rail", selector: ".shape-strip.compact", attribute: "data-selected-x", attributeEqualsCapture: "layer-anchor" }] },
      { action: { kind: "key", value: "Escape" }, expect: [...depth(3), { target: "rail", selector: ".shape-strip.compact", attribute: "data-selected-x", attributeEqualsCapture: "layer-anchor" }] },
      { action: { kind: "key", value: "Escape" }, expect: [...depth(2), attr("read", "data-episode", "stage:verify#1"), { target: "read", attribute: "data-axis-start", attributeEqualsCapture: "pre-descend-start" }, { target: "read", attribute: "data-axis-end", attributeEqualsCapture: "pre-descend-end" }, { target: "rail", selector: ".episode-strip", attribute: "data-selected-x", attributeEqualsCapture: "layer-anchor" }] },
    ],
    webappSteps: [
      { action: { kind: "key", value: "Enter" }, expect: [...depth(1)] },
      // anchor mid-trace: boundary anchors shift ~1px under min-span clamps
      { action: { kind: "key", value: "e" }, expect: [{ target: "selected-event" }] },
      { action: { kind: "key", value: "Enter" }, expect: [...depth(2), { target: "console", absent: true }] },
      { action: { kind: "capture-attribute", target: ".episode-strip", attribute: "data-selected-x", key: "layer-anchor" }, expect: [{ target: "rail", selector: ".episode-button.selected" }] },
      { action: { kind: "key", value: "Enter" }, expect: [...depth(3), { target: "rail", selector: ".shape-strip.compact", attribute: "data-selected-x", attributeEqualsCapture: "layer-anchor" }, { target: "rail", selector: ".lane-events" }] },
      { action: { kind: "key", value: "Enter" }, expect: [...depth(4), { target: "rail", selector: ".lane-source" }, { target: "rail", selector: ".shape-strip.compact", attribute: "data-selected-x", attributeEqualsCapture: "layer-anchor" }] },
      { action: { kind: "key", value: "Escape" }, expect: [...depth(3), { target: "rail", selector: ".shape-strip.compact", attribute: "data-selected-x", attributeEqualsCapture: "layer-anchor" }] },
      { action: { kind: "key", value: "Escape" }, expect: [...depth(2), { target: "rail", selector: ".episode-strip", attribute: "data-selected-x", attributeEqualsCapture: "layer-anchor" }] },
    ],
  },
  {
    id: "o", name: "episode-j-k-traversal", keyboardOnly: true, surfaces: ["daemon", "webapp"], steps: [
      { action: { kind: "key", value: "Enter" }, expect: [...depth(1)] },
      { action: { kind: "key", value: "Enter" }, expect: [...depth(2), attr("read", "data-episode", "stage:verify#1")] },
      { action: { kind: "key", value: "j" }, expect: [attr("read", "data-episode", "stage:outcome#1"), { target: "rail", selector: ".episode-button.selected", attribute: "data-episode-key", equals: "stage:outcome#1" }] },
      { action: { kind: "key", value: "k" }, expect: [attr("read", "data-episode", "stage:verify#1"), { target: "rail", selector: ".episode-button.selected", attribute: "data-episode-key", equals: "stage:verify#1" }] },
    ],
    webappSteps: [
      { action: { kind: "key", value: "Enter" }, expect: [...depth(1)] },
      { action: { kind: "key", value: "Enter" }, expect: [...depth(2)] },
      { action: { kind: "capture-attribute", target: ".lane-track.active-zone", attribute: "data-episode", key: "episode-before" }, expect: [{ target: "rail", selector: ".episode-button.selected" }] },
      { action: { kind: "key", value: "j" }, expect: [{ target: "read", attribute: "data-episode", attributeNotEqualsCapture: "episode-before" }, { target: "rail", selector: ".episode-button.selected" }] },
      { action: { kind: "key", value: "k" }, expect: [{ target: "read", attribute: "data-episode", attributeEqualsCapture: "episode-before" }] },
    ],
  },
  {
    id: "p", name: "landmark-crosses-episode-boundary", keyboardOnly: true, surfaces: ["daemon", "webapp"], webappExample: "300-event coding trace", steps: [
      { action: { kind: "key", value: "Enter" }, expect: [...depth(1)] },
      { action: { kind: "key", value: "Enter" }, expect: [...depth(2)] },
      { action: { kind: "key", value: "Enter" }, expect: [...depth(3), attr("read", "data-episode", "stage:verify#1")] },
      { action: { kind: "capture-attribute", target: ".lane-track.active-zone", attribute: "data-axis-start", key: "episode-axis" }, expect: [{ target: "rail", selector: ".lane-event.selected", contains: "Policy error" }] },
      { action: { kind: "key", value: "c" }, expect: [attr("read", "data-episode", "stage:setup#1"), { target: "rail", selector: ".lane-event.selected", contains: "Context compacted" }, { target: "read", attribute: "data-axis-start", attributeNotEqualsCapture: "episode-axis" }, { target: "rail", selector: ".lane-events", attribute: "data-episode-key", equals: "stage:setup#1" }] },
    ],
    webappSteps: [
      { action: { kind: "key", value: "Enter" }, expect: [...depth(1)] },
      { action: { kind: "key", value: "Enter" }, expect: [...depth(2)] },
      { action: { kind: "key", value: "Enter" }, expect: [...depth(3)] },
      { action: { kind: "capture-attribute", target: ".lane-track.active-zone", attribute: "data-episode", key: "episode-before" }, expect: [{ target: "rail", selector: ".lane-events" }] },
      { action: { kind: "capture-attribute", target: ".lane-track.active-zone", attribute: "data-axis-start", key: "episode-axis" }, expect: [{ target: "rail", selector: ".lane-event.selected" }] },
      // in the real bugfix trace the compaction sits INSIDE stage:diagnose, so
      // `c` moves selection without switching episodes (that is correct
      // behavior); the reward/grader landmark genuinely crosses into verify.
      { action: { kind: "key", value: "c" }, expect: [{ target: "rail", selector: ".lane-event.selected", contains: "ompact" }, { target: "read", attribute: "data-episode", attributeEqualsCapture: "episode-before" }] },
      { action: { kind: "key", value: "r" }, expect: [{ target: "read", attribute: "data-episode", attributeNotEqualsCapture: "episode-before" }, { target: "read", attribute: "data-axis-start", attributeNotEqualsCapture: "episode-axis" }, { target: "rail", selector: ".lane-events" }] },
    ],
  },
  {
    id: "q", name: "daemon-completes-paginated-episode", keyboardOnly: true, surfaces: ["daemon"], steps: [
      { action: { kind: "filter", value: "long" }, expect: [selectedRow("long")] },
      { action: { kind: "key", value: "Escape" }, expect: [selectedRow("long")] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-trajectory", "long"), ...depth(1), attr("read", "data-axis-start", "0.0000"), attr("read", "data-axis-end", "249.0000"), { target: "strip", attribute: "data-visible-events", equals: "250" }] },
      { action: { kind: "key", value: "Enter" }, expect: [...depth(2), attr("read", "data-episode", "stage:bulk#1"), { target: "rail", selector: ".episode-summary", contains: "message 249" }, { target: "rail", selector: ".episode-summary", contains: "error 1" }, { target: "rail", selector: ".episode-summary", contains: "#0–#249" }] },
    ],
  },
  {
    id: "r", name: "dense-strip-bins-with-true-error-landmark", keyboardOnly: true, surfaces: ["daemon"], steps: [
      { action: { kind: "filter", value: "long" }, expect: [selectedRow("long")] },
      { action: { kind: "key", value: "Escape" }, expect: [selectedRow("long")] },
      { action: { kind: "key", value: "Enter" }, expect: [...depth(1), attr("strip", "data-strip-mode", "binned"), { target: "strip", selector: ".event-shape.error[data-event-index='173']" }] },
    ],
  },
  {
    id: "s", name: "keybar-shortcuts-switch-with-active-module", keyboardOnly: true, surfaces: ["daemon"], steps: [
      { action: { kind: "key", value: "k" }, expect: [selectedRow("candidate"), { target: "rail", selector: ".keybar", contains: "Increase fidelity" }] },
      { action: { kind: "key", value: "Enter" }, expect: [...depth(1), selectedEvent("Policy error"), { target: "rail", selector: ".keybar", contains: "Next event" }] },
      { action: { kind: "key", value: "?" }, expect: [{ target: "rail", selector: ".workspace-guide" }, { target: "rail", selector: ".keybar", contains: "Toggle guide" }] },
      { action: { kind: "key", value: "?" }, expect: [{ target: "rail", selector: ".workspace-guide", absent: true }, { target: "rail", selector: ".keybar", contains: "Next event" }] },
      { action: { kind: "key", value: "j" }, expect: [selectedEvent("Final reward")] },
    ],
  },
  {
    id: "t", name: "daemon-gallery-shape-keeps-errors-mid-strip", keyboardOnly: true, surfaces: ["daemon"], steps: [
      { action: { kind: "filter", value: "coding-bugfix" }, expect: [selectedRow("coding-bugfix-rollout-01")] },
      { action: { kind: "key", value: "Escape" }, expect: [selectedRow("coding-bugfix-rollout-01")] },
      { action: { kind: "key", value: "]" }, expect: [{ target: "rail", selector: ".browse-row .collection-signals .signal-fail", contains: "failed" }] },
    ],
  },
  {
    id: "u", name: "keyboard-module-move-persists", keyboardOnly: true, surfaces: ["daemon", "webapp"], steps: [
      { action: { kind: "key", value: "Enter" }, expect: [{ target: "focus-lane", count: 1 }] },
      { action: { kind: "key", value: "Tab" }, expect: [attr("shell", "data-active-zone", "detail"), attr("console", "data-dock-position", "right")] },
      { action: { kind: "key", value: "Control+m" }, expect: [attr("shell", "data-move-mode", "true")] },
      { action: { kind: "key", value: "ArrowDown" }, expect: [attr("console", "data-dock-position", "bottom")] },
      { action: { kind: "key", value: "Control+m" }, expect: [attr("shell", "data-move-mode", "false"), attr("console", "data-dock-position", "bottom")] },
      { action: { kind: "reload" }, expect: [attr("console", "data-dock-position", "bottom"), { target: "focus-lane", count: 1 }] },
    ],
    // The webapp holds the trace in memory only (privacy design): reload
    // returns to the landing screen, so persistence is daemon-only.
    webappSteps: [
      { action: { kind: "key", value: "Enter" }, expect: [{ target: "focus-lane", count: 1 }] },
      { action: { kind: "key", value: "Tab" }, expect: [attr("shell", "data-active-zone", "detail"), attr("console", "data-dock-position", "right")] },
      { action: { kind: "key", value: "Control+m" }, expect: [attr("shell", "data-move-mode", "true")] },
      { action: { kind: "key", value: "ArrowDown" }, expect: [attr("console", "data-dock-position", "bottom")] },
      { action: { kind: "key", value: "Control+m" }, expect: [attr("shell", "data-move-mode", "false"), attr("console", "data-dock-position", "bottom")] },
    ],
  },
  {
    id: "v", name: "empty-dock-group-collapses", keyboardOnly: true, surfaces: ["daemon", "webapp"], steps: [
      { action: { kind: "key", value: "Enter" }, expect: [{ target: "focus-lane", count: 1 }] },
      { action: { kind: "key", value: "x" }, expect: [{ target: "focus-lane", count: 0 }, { target: "console", selector: ".detail-empty", absent: true }, { target: "stage", selector: ".dv-groupview:has(.lane-track)", absent: true }] },
    ],
  },
  {
    id: "w", name: "spatial-navigation-overview-fidelity-timeline-and-pinned-detail", keyboardOnly: true, surfaces: ["daemon"], steps: [
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-trajectory", "candidate"), attr("read", "data-fidelity", "glyphs"), { target: "read", selector: ".axis-navigator" }] },
      { action: { kind: "key", value: "ArrowLeft" }, expect: [attr("shell", "data-active-zone", "rail")] },
      { action: { kind: "key", value: "ArrowRight" }, expect: [attr("shell", "data-active-zone", "source-1:candidate")] },
      { action: { kind: "key", value: "]" }, expect: [attr("read", "data-fidelity", "detail"), { target: "read", selector: ".run-facts", contains: "steps" }, { target: "read", selector: ".timeline-event-label:has-text('Run tool')", contains: "Run tool" }, { target: "read", selector: ".overview-steps", contains: "Run tool" }] },
      { action: { kind: "key", value: "d" }, expect: [attr("shell", "data-active-zone", "detail"), { target: "console", selector: ".workspace-console[data-pinned='false']", attribute: "data-detail-lane-id", equals: "source-1:candidate" }] },
      { action: { kind: "key", value: "Shift+d" }, expect: [{ target: "console", selector: ".workspace-console[data-pinned='true']", attribute: "data-detail-lane-id", equals: "source-1:candidate" }, { target: "console", selector: ".workspace-console[data-pinned='true'] .detail-region", attribute: "data-event-count", equals: "6" }, { target: "console", selector: ".workspace-console[data-pinned='true'] .moment", count: 6 }] },
      { action: { kind: "key", value: "j" }, expect: [{ target: "selected-event", selector: ".workspace-console[data-pinned='true'] .moment.selected", contains: "Final reward" }] },
      { action: { kind: "key", value: "x" }, expect: [{ target: "console", selector: ".workspace-console[data-pinned='true']", absent: true }, attr("read", "data-trajectory", "candidate")] },
    ],
  },
  {
    id: "x", name: "timeline-pointer-center-pan-and-resize", keyboardOnly: false, surfaces: ["daemon", "webapp"], steps: [
      { action: { kind: "key", value: "Enter" }, expect: [{ target: "read", selector: ".axis-navigator" }] },
      { action: { kind: "key", value: "+" }, expect: [{ target: "read", selector: ".axis-navigator" }] },
      { action: { kind: "capture-attribute", target: ".lane-track.active-zone", attribute: "data-axis-start", key: "timeline-start-before-click" }, expect: [{ target: "read" }] },
      { action: { kind: "timeline-click", ratio: 0.95 }, expect: [{ target: "read", attribute: "data-axis-start", attributeNotEqualsCapture: "timeline-start-before-click" }] },
      { action: { kind: "capture-attribute", target: ".lane-track.active-zone", attribute: "data-axis-start", key: "timeline-start-before-pan" }, expect: [{ target: "read" }] },
      { action: { kind: "timeline-drag", part: "window", dx: -40 }, expect: [{ target: "read", attribute: "data-axis-start", attributeNotEqualsCapture: "timeline-start-before-pan" }] },
      { action: { kind: "capture-attribute", target: ".lane-track.active-zone", attribute: "data-axis-start", key: "timeline-start-before-resize" }, expect: [{ target: "read" }] },
      { action: { kind: "capture-attribute", target: ".lane-track.active-zone", attribute: "data-axis-end", key: "timeline-end-before-resize" }, expect: [{ target: "read" }] },
      { action: { kind: "timeline-drag", part: "start", dx: 20 }, expect: [{ target: "read", attribute: "data-axis-start", attributeNotEqualsCapture: "timeline-start-before-resize" }, { target: "read", attribute: "data-axis-end", attributeEqualsCapture: "timeline-end-before-resize" }] },
    ],
  },
  {
    id: "y", name: "local-metadata-edit-filter-and-restore", keyboardOnly: false, surfaces: ["daemon", "webapp"], steps: [
      { action: { kind: "click", target: ".workspace-rail .metadata-edit" }, expect: [{ target: "rail", selector: ".metadata-editor.collection" }] },
      { action: { kind: "fill", target: "[aria-label='collection title']", value: "Checkout reliability" }, expect: [{ target: "rail", selector: "[aria-label='collection title']", value: "Checkout reliability" }] },
      { action: { kind: "fill", target: "[aria-label='collection description']", value: "Saved-card confirmation rollouts" }, expect: [{ target: "rail", selector: "[aria-label='collection description']", value: "Saved-card confirmation rollouts" }] },
      { action: { kind: "click", target: ".workspace-rail .metadata-editor button[type='submit']" }, expect: [{ target: "rail", selector: ".editable-metadata h1", equals: "Checkout reliability" }, { target: "rail", selector: ".editable-metadata p", equals: "Saved-card confirmation rollouts" }] },
      { action: { kind: "key", value: "Enter" }, expect: [{ target: "read" }, { target: "console", selector: ".metadata-edit" }] },
      { action: { kind: "click", target: ".workspace-console .metadata-edit" }, expect: [{ target: "console", selector: ".metadata-editor.trajectory" }] },
      { action: { kind: "fill", target: "[aria-label='trajectory title']", value: "Reviewed checkout" }, expect: [{ target: "console", selector: "[aria-label='trajectory title']", value: "Reviewed checkout" }] },
      { action: { kind: "fill", target: "[aria-label='trajectory description']", value: "Confirmation path under review" }, expect: [{ target: "console", selector: "[aria-label='trajectory description']", value: "Confirmation path under review" }] },
      { action: { kind: "click", target: ".workspace-console .metadata-editor button[type='submit']" }, expect: [{ target: "read", selector: ".lane-track header b", equals: "Reviewed checkout" }, { target: "console", selector: ".workspace-console .editable-metadata h2", equals: "Reviewed checkout" }, { target: "console", selector: ".workspace-console .editable-metadata p", equals: "Confirmation path under review" }] },
      { action: { kind: "key", value: "Escape" }, expect: [{ target: "browse" }, { target: "rail", selector: ".editable-metadata h1", equals: "Checkout reliability" }] },
      { action: { kind: "filter", value: "Reviewed checkout" }, expect: [{ target: "selected-row", contains: "Reviewed checkout" }] },
      { action: { kind: "filter", value: "" }, expect: [{ target: "browse" }] },
      { action: { kind: "key", value: "Escape" }, expect: [{ target: "browse" }] },
      { action: { kind: "key", value: "Enter" }, expect: [{ target: "read", selector: ".lane-track header b", equals: "Reviewed checkout" }] },
      { action: { kind: "reload" }, expect: [{ target: "rail", selector: ".editable-metadata h1", equals: "Checkout reliability" }, { target: "read", selector: ".lane-track header b", equals: "Reviewed checkout" }] },
    ],
    webappSteps: [
      { action: { kind: "click", target: ".workspace-rail .metadata-edit" }, expect: [{ target: "rail", selector: ".metadata-editor.collection" }] },
      { action: { kind: "fill", target: "[aria-label='collection title']", value: "Checkout reliability" }, expect: [{ target: "rail", selector: "[aria-label='collection title']", value: "Checkout reliability" }] },
      { action: { kind: "fill", target: "[aria-label='collection description']", value: "Saved-card confirmation rollouts" }, expect: [{ target: "rail", selector: "[aria-label='collection description']", value: "Saved-card confirmation rollouts" }] },
      { action: { kind: "click", target: ".workspace-rail .metadata-editor button[type='submit']" }, expect: [{ target: "rail", selector: ".editable-metadata h1", equals: "Checkout reliability" }] },
      { action: { kind: "key", value: "Enter" }, expect: [{ target: "read" }, { target: "console", selector: ".metadata-edit" }] },
      { action: { kind: "click", target: ".workspace-console .metadata-edit" }, expect: [{ target: "console", selector: ".metadata-editor.trajectory" }] },
      { action: { kind: "fill", target: "[aria-label='trajectory title']", value: "Reviewed checkout" }, expect: [{ target: "console", selector: "[aria-label='trajectory title']", value: "Reviewed checkout" }] },
      { action: { kind: "fill", target: "[aria-label='trajectory description']", value: "Confirmation path under review" }, expect: [{ target: "console", selector: "[aria-label='trajectory description']", value: "Confirmation path under review" }] },
      { action: { kind: "click", target: ".workspace-console .metadata-editor button[type='submit']" }, expect: [{ target: "read", selector: ".lane-track header b", equals: "Reviewed checkout" }, { target: "console", selector: ".workspace-console .editable-metadata p", equals: "Confirmation path under review" }] },
      { action: { kind: "key", value: "Escape" }, expect: [{ target: "browse" }] },
      { action: { kind: "filter", value: "Reviewed checkout" }, expect: [{ target: "selected-row", contains: "Reviewed checkout" }] },
      { action: { kind: "key", value: "Escape" }, expect: [{ target: "browse" }, { target: "selected-row", contains: "Reviewed checkout" }] },
    ],
  },
  {
    id: "z", name: "timeline-fits-desktop-and-compact-viewports", keyboardOnly: false, surfaces: ["daemon", "webapp"], steps: [
      { action: { kind: "key", value: "Enter" }, expect: [{ target: "read" }] },
      { action: { kind: "viewport", width: 1440, height: 900 }, expect: [{ target: "read", selector: ".lane-track.active-zone .axis-navigator", withinViewport: true }, { target: "shell", pageFitsViewport: true }] },
      { action: { kind: "viewport", width: 1024, height: 700 }, expect: [{ target: "read", selector: ".lane-track.active-zone .axis-navigator", withinViewport: true }, { target: "shell", pageFitsViewport: true }] },
    ],
  },
];
