export type Observable = {
  target: "shell" | "browse" | "read" | "compare" | "selected-row" | "selected-event" | "filter" | "strip" | "marked-rows" | "alert" | "rail" | "stage" | "focus-lane" | "context-lane" | "console" | "breadcrumb" | "reference" | "seam";
  selector?: string;
  attribute?: string;
  equals?: string;
  notEquals?: string;
  contains?: string;
  absent?: boolean;
  count?: number;
  boxEquals?: string;
  boxNotEquals?: string;
  attributeEqualsCapture?: string;
  attributeNotEqualsCapture?: string;
  attributeNumberLte?: number;
  attributeNumberGte?: number;
};

export type FlowAction =
  | { kind: "key"; value: string }
  | { kind: "filter"; value: string }
  | { kind: "click"; target: string; clicks?: number }
  | { kind: "strip-click"; eventIndex: number }
  | { kind: "capture-box"; target: string; key: string }
  | { kind: "capture-attribute"; target: string; attribute: string; key: string }
  | { kind: "seam-drag"; name: "rail" | "focusContext" | "focusLane" | "console"; dx: number; dy: number }
  | { kind: "reload" }
  | { kind: "history-back" };

export type FlowStep = { action: FlowAction; expect: Observable[] };
export type Flow = { id: string; name: string; keyboardOnly: boolean; surfaces: Array<"daemon" | "webapp">; steps: FlowStep[]; webappSteps?: FlowStep[]; webappExample?: string };

const mode = (target: Observable["target"]): Observable => ({ target });
const selectedRow = (id: string): Observable => ({ target: "selected-row", contains: id });
const selectedEvent = (text: string): Observable => ({ target: "selected-event", contains: text });
const attr = (target: Observable["target"], attribute: string, equals: string): Observable => ({ target, attribute, equals });

export const flows: Flow[] = [
  {
    id: "a", name: "triage-sweep", keyboardOnly: true, surfaces: ["daemon", "webapp"], steps: [
      { action: { kind: "key", value: "j" }, expect: [selectedRow("partial")] },
      { action: { kind: "key", value: "j" }, expect: [selectedRow("fourth")] },
      { action: { kind: "key", value: "1" }, expect: [selectedRow("reference"), { target: "browse", contains: "tag 1" }] },
      { action: { kind: "key", value: "2" }, expect: [selectedRow("layered"), { target: "browse", contains: "tag 2" }] },
      { action: { kind: "filter", value: "reference" }, expect: [selectedRow("reference"), attr("browse", "data-filter", "reference")] },
      { action: { kind: "filter", value: "" }, expect: [selectedRow("reference"), attr("browse", "data-filter", "")] },
      { action: { kind: "key", value: "Escape" }, expect: [mode("browse"), selectedRow("reference")] },
    ],
    webappSteps: [
      { action: { kind: "key", value: "j" }, expect: [selectedRow("checkout-rollout-06")] },
      { action: { kind: "key", value: "j" }, expect: [selectedRow("checkout-rollout-10")] },
      { action: { kind: "key", value: "1" }, expect: [{ target: "browse", contains: "tag 1" }] },
      { action: { kind: "key", value: "2" }, expect: [{ target: "browse", contains: "tag 2" }] },
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
      { action: { kind: "key", value: "Enter" }, expect: [mode("read"), attr("read", "data-trajectory", "candidate"), attr("read", "data-fidelity", "glyphs")] },
      { action: { kind: "key", value: "j" }, expect: [selectedEvent("Final reward")] },
      { action: { kind: "key", value: "k" }, expect: [selectedEvent("Policy error")] },
      { action: { kind: "key", value: "e" }, expect: [selectedEvent("Policy error")] },
      { action: { kind: "key", value: "Escape" }, expect: [mode("browse"), selectedRow("candidate"), attr("browse", "data-filter", "demo"), attr("browse", "data-fidelity", "glyphs")] },
    ],
    webappSteps: [
      { action: { kind: "filter", value: "checkout" }, expect: [attr("browse", "data-filter", "checkout")] },
      { action: { kind: "key", value: "Escape" }, expect: [mode("browse"), attr("browse", "data-filter", "checkout")] },
      { action: { kind: "key", value: "Enter" }, expect: [mode("read"), attr("read", "data-fidelity", "glyphs")] },
      { action: { kind: "key", value: "j" }, expect: [selectedEvent("payment · step 46")] },
      { action: { kind: "key", value: "k" }, expect: [selectedEvent("Recoverable submit timeout · retry 1")] },
      { action: { kind: "key", value: "e" }, expect: [selectedEvent("Recoverable submit timeout · retry 2")] },
      { action: { kind: "key", value: "Escape" }, expect: [mode("browse"), attr("browse", "data-filter", "checkout"), attr("browse", "data-fidelity", "glyphs")] },
    ],
  },
  {
    id: "c", name: "read-sweep", keyboardOnly: true, surfaces: ["daemon"], steps: [
      { action: { kind: "filter", value: "demo" }, expect: [attr("browse", "data-filter", "demo")] },
      { action: { kind: "key", value: "Escape" }, expect: [mode("browse"), attr("browse", "data-filter", "demo")] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-trajectory", "candidate")] },
      { action: { kind: "key", value: "+" }, expect: [{ target: "strip", attribute: "data-visible-events", equals: "3" }, attr("read", "data-axis-start", "15.0000"), attr("read", "data-axis-end", "40.0000")] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "2")] },
      { action: { kind: "key", value: "n" }, expect: [attr("read", "data-trajectory", "partial"), attr("read", "data-depth", "2"), attr("read", "data-fidelity", "glyphs"), attr("read", "data-axis-start", "15.0000"), attr("read", "data-axis-end", "40.0000"), attr("shell", "data-filter", "demo")] },
      { action: { kind: "key", value: "n" }, expect: [attr("read", "data-trajectory", "fourth"), attr("read", "data-depth", "2"), attr("read", "data-axis-start", "15.0000"), attr("shell", "data-filter", "demo")] },
      { action: { kind: "key", value: "p" }, expect: [attr("read", "data-trajectory", "partial"), attr("read", "data-depth", "2"), attr("read", "data-axis-start", "15.0000"), attr("shell", "data-filter", "demo")] },
      { action: { kind: "key", value: "Escape" }, expect: [attr("read", "data-depth", "1"), attr("read", "data-trajectory", "partial")] },
      { action: { kind: "key", value: "Escape" }, expect: [mode("browse"), selectedRow("partial"), attr("browse", "data-filter", "demo")] },
    ],
  },
  {
    id: "d", name: "lane-add-close-swap", keyboardOnly: true, surfaces: ["daemon"], steps: [
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-trajectory", "candidate")] },
      { action: { kind: "key", value: "Tab" }, expect: [{ target: "shell", attribute: "data-active-zone", equals: "rail" }] },
      { action: { kind: "key", value: "j" }, expect: [selectedRow("partial")] },
      { action: { kind: "key", value: "a" }, expect: [{ target: "focus-lane", count: 2 }] },
      { action: { kind: "key", value: "]" }, expect: [{ target: "focus-lane", selector: ".focus-lane[data-trajectory='candidate']", attribute: "data-fidelity", equals: "glyphs" }, { target: "focus-lane", selector: ".focus-lane[data-trajectory='partial']", attribute: "data-fidelity", equals: "previews" }] },
      { action: { kind: "key", value: "}" }, expect: [{ target: "focus-lane", selector: ".focus-lane[data-trajectory='candidate']", attribute: "data-fidelity", equals: "previews" }, { target: "focus-lane", selector: ".focus-lane[data-trajectory='partial']", attribute: "data-fidelity", equals: "full" }] },
      { action: { kind: "key", value: "+" }, expect: [{ target: "focus-lane", selector: ".focus-lane[data-trajectory='candidate']", attribute: "data-axis-start", equals: "0.0000" }, { target: "focus-lane", selector: ".focus-lane[data-trajectory='partial']", attribute: "data-axis-start", equals: "15.0000" }] },
      { action: { kind: "key", value: ">" }, expect: [{ target: "focus-lane", selector: ".focus-lane[data-trajectory='candidate']", attribute: "data-axis-start", equals: "15.0000" }] },
      { action: { kind: "key", value: "Shift+V" }, expect: [attr("shell", "data-direction", "columns")] },
      { action: { kind: "key", value: "Shift+H" }, expect: [attr("shell", "data-direction", "rows")] },
      { action: { kind: "key", value: "Tab" }, expect: [{ target: "shell", attribute: "data-active-zone", equals: "rail" }] },
      { action: { kind: "key", value: "j" }, expect: [selectedRow("fourth")] },
      { action: { kind: "key", value: "a" }, expect: [{ target: "context-lane", count: 1 }, attr("context-lane", "data-trajectory", "fourth")] },
      { action: { kind: "key", value: "Shift+Enter" }, expect: [{ target: "focus-lane", selector: ".lane-track.active-zone", attribute: "data-trajectory", equals: "fourth" }, { target: "context-lane", count: 1 }] },
      { action: { kind: "key", value: "Shift+A" }, expect: [{ target: "reference", equals: "fourth" }] },
      { action: { kind: "key", value: "x" }, expect: [{ target: "focus-lane", count: 1 }, { target: "reference", equals: "none" }] },
    ],
  },
  {
    id: "e", name: "zoom-depth", keyboardOnly: true, surfaces: ["daemon", "webapp"], steps: [
      { action: { kind: "key", value: "Enter" }, expect: [mode("read")] },
      { action: { kind: "key", value: "+" }, expect: [{ target: "strip", attribute: "data-visible-events", notEquals: "6" }, attr("read", "data-axis-start", "15.0000"), attr("read", "data-axis-end", "40.0000")] },
      { action: { kind: "key", value: "+" }, expect: [{ target: "strip", attribute: "data-visible-events", notEquals: "6" }, attr("read", "data-axis-start", "22.5000"), attr("read", "data-axis-end", "35.0000")] },
      { action: { kind: "key", value: "c" }, expect: [selectedEvent("Context compacted"), attr("read", "data-axis-start", "8.1250")] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "2")] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "3")] },
      { action: { kind: "key", value: "Escape" }, expect: [attr("read", "data-depth", "2")] },
      { action: { kind: "key", value: "Escape" }, expect: [attr("read", "data-depth", "1")] },
      { action: { kind: "key", value: "0" }, expect: [{ target: "strip", attribute: "data-visible-events", equals: "6" }, attr("read", "data-axis-start", "0.0000"), attr("read", "data-axis-end", "50.0000")] },
    ],
    webappSteps: [
      { action: { kind: "key", value: "Enter" }, expect: [mode("read")] },
      { action: { kind: "key", value: "+" }, expect: [attr("read", "data-axis-start", "225.0000"), attr("read", "data-axis-end", "570.0000")] },
      { action: { kind: "key", value: "+" }, expect: [attr("read", "data-axis-start", "337.5000"), attr("read", "data-axis-end", "510.0000")] },
      { action: { kind: "key", value: "e" }, expect: [selectedEvent("Recoverable submit timeout · retry 2")] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "2")] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "3")] },
      { action: { kind: "key", value: "Escape" }, expect: [attr("read", "data-depth", "2")] },
      { action: { kind: "key", value: "Escape" }, expect: [attr("read", "data-depth", "1")] },
      { action: { kind: "key", value: "0" }, expect: [attr("read", "data-axis-start", "0.0000"), attr("read", "data-axis-end", "690.0000")] },
    ],
  },
  {
    id: "f", name: "open-read-return-clicks", keyboardOnly: false, surfaces: ["daemon"], steps: [
      { action: { kind: "filter", value: "demo" }, expect: [attr("browse", "data-filter", "demo")] },
      { action: { kind: "click", target: "[role=option]", clicks: 2 }, expect: [mode("read"), attr("read", "data-trajectory", "candidate")] },
      { action: { kind: "strip-click", eventIndex: 1 }, expect: [selectedEvent("Context compacted")] },
      { action: { kind: "click", target: ".moment:has-text('Policy error')" }, expect: [selectedEvent("Policy error")] },
      { action: { kind: "key", value: "Escape" }, expect: [mode("browse"), selectedRow("candidate"), attr("browse", "data-filter", "demo"), attr("browse", "data-fidelity", "glyphs")] },
    ],
  },
  {
    id: "g", name: "anti-jitter-depth-fidelity-zoom", keyboardOnly: true, surfaces: ["daemon", "webapp"], steps: [
      { action: { kind: "key", value: "Enter" }, expect: [{ target: "focus-lane", count: 1 }] },
      { action: { kind: "capture-box", target: ".lane-track.active-zone", key: "lane-track" }, expect: [attr("read", "data-depth", "1")] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "2"), { target: "focus-lane", selector: ".lane-track.active-zone", boxEquals: "lane-track" }] },
      { action: { kind: "key", value: "]" }, expect: [{ target: "focus-lane", selector: ".lane-track.active-zone", boxEquals: "lane-track" }] },
      { action: { kind: "key", value: "+" }, expect: [{ target: "focus-lane", selector: ".lane-track.active-zone", boxEquals: "lane-track" }] },
    ],
  },
  {
    id: "h", name: "seam-resize-pointer-keyboard-persistence", keyboardOnly: false, surfaces: ["daemon"], steps: [
      { action: { kind: "key", value: "Enter" }, expect: [{ target: "focus-lane", count: 1 }] },
      { action: { kind: "key", value: "Tab" }, expect: [{ target: "shell", attribute: "data-active-zone", equals: "rail" }] },
      { action: { kind: "key", value: "j" }, expect: [selectedRow("partial")] },
      { action: { kind: "key", value: "a" }, expect: [{ target: "focus-lane", count: 2 }] },
      { action: { kind: "capture-box", target: ".workspace-rail", key: "rail-before" }, expect: [{ target: "rail" }] },
      { action: { kind: "seam-drag", name: "rail", dx: 70, dy: 0 }, expect: [{ target: "rail", boxNotEquals: "rail-before" }] },
      { action: { kind: "seam-drag", name: "focusContext", dx: 0, dy: -35 }, expect: [{ target: "stage" }] },
      { action: { kind: "seam-drag", name: "focusLane", dx: 0, dy: 30 }, expect: [{ target: "focus-lane", count: 2 }] },
      { action: { kind: "seam-drag", name: "console", dx: 0, dy: -30 }, expect: [{ target: "console" }] },
      { action: { kind: "key", value: "x" }, expect: [{ target: "focus-lane", count: 1 }] },
      { action: { kind: "capture-box", target: ".workspace-console", key: "console-pointer" }, expect: [{ target: "console" }] },
      { action: { kind: "key", value: "Control+w" }, expect: [attr("console", "data-resize-mode", "true")] },
      { action: { kind: "key", value: "ArrowUp" }, expect: [{ target: "console", boxNotEquals: "console-pointer" }] },
      { action: { kind: "key", value: "Escape" }, expect: [attr("console", "data-resize-mode", "false")] },
      { action: { kind: "capture-box", target: ".workspace-console", key: "console-persist" }, expect: [{ target: "console" }] },
      { action: { kind: "reload" }, expect: [{ target: "console", boxEquals: "console-persist" }, { target: "focus-lane", count: 1 }] },
      { action: { kind: "click", target: "[data-seam='console']", clicks: 2 }, expect: [{ target: "console", boxNotEquals: "console-persist" }] },
      { action: { kind: "key", value: "Tab" }, expect: [{ target: "shell", attribute: "data-active-zone", equals: "rail" }] },
    ],
  },
  {
    id: "i", name: "jumplist-restoration", keyboardOnly: true, surfaces: ["daemon", "webapp"], steps: [
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "1")] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "2")] },
      { action: { kind: "key", value: "Control+o" }, expect: [attr("read", "data-depth", "1")] },
      { action: { kind: "key", value: "Control+i" }, expect: [attr("read", "data-depth", "2")] },
      { action: { kind: "key", value: "x" }, expect: [{ target: "focus-lane", count: 0 }] },
      { action: { kind: "history-back" }, expect: [{ target: "focus-lane", count: 1 }, attr("read", "data-depth", "2")] },
      { action: { kind: "key", value: "Control+o" }, expect: [attr("read", "data-depth", "1")] },
    ],
  },
  {
    id: "j", name: "rail-fidelity-ladder-with-lanes", keyboardOnly: true, surfaces: ["daemon", "webapp"], steps: [
      { action: { kind: "key", value: "Enter" }, expect: [{ target: "focus-lane", count: 1 }] },
      { action: { kind: "key", value: "Tab" }, expect: [{ target: "shell", attribute: "data-active-zone", equals: "rail" }] },
      { action: { kind: "key", value: "[" }, expect: [{ target: "rail", selector: ".browse-list", attribute: "data-fidelity-level", equals: "L2" }, { target: "rail", selector: ".cat-texture" }] },
      { action: { kind: "key", value: "[" }, expect: [{ target: "rail", selector: ".browse-list", attribute: "data-fidelity-level", equals: "L1" }, { target: "rail", selector: ".cat-marks" }] },
      { action: { kind: "key", value: "[" }, expect: [{ target: "rail", selector: ".browse-list", attribute: "data-fidelity-level", equals: "L0" }, { target: "rail", selector: ".cat-line" }, { target: "rail", selector: ".browse-row .verdict", absent: true }, { target: "rail", selector: ".browse-row .identity small", absent: true }] },
      { action: { kind: "key", value: "]" }, expect: [{ target: "rail", selector: ".browse-list", attribute: "data-fidelity-level", equals: "L1" }, { target: "rail", selector: ".browse-row .verdict" }] },
      { action: { kind: "key", value: "]" }, expect: [{ target: "rail", selector: ".browse-list", attribute: "data-fidelity-level", equals: "L2" }, { target: "rail", selector: ".cat-texture" }] },
      { action: { kind: "key", value: "]" }, expect: [{ target: "rail", selector: ".browse-list", attribute: "data-fidelity-level", equals: "L3" }, { target: "rail", selector: ".cat-glyphs" }] },
      { action: { kind: "key", value: "]" }, expect: [{ target: "rail", selector: ".browse-list", attribute: "data-fidelity-level", equals: "L4" }, { target: "rail", selector: ".browse-row", attribute: "data-columns", equals: "true" }, { target: "rail", selector: ".events-column" }, { target: "rail", selector: ".browse-row .identity small" }] },
      { action: { kind: "key", value: "]" }, expect: [{ target: "rail", selector: ".browse-list", attribute: "data-fidelity-level", equals: "L5" }, { target: "rail", selector: ".row-state" }, { target: "focus-lane", count: 1 }] },
      { action: { kind: "key", value: "Shift+Tab" }, expect: [{ target: "focus-lane", selector: ".lane-track.active-zone" }] },
    ],
  },
  {
    id: "k", name: "tab-cycle-rail-and-lanes", keyboardOnly: true, surfaces: ["daemon", "webapp"], steps: [
      { action: { kind: "key", value: "Enter" }, expect: [{ target: "focus-lane", count: 1 }] },
      { action: { kind: "key", value: "Tab" }, expect: [{ target: "shell", attribute: "data-active-zone", equals: "rail" }] },
      { action: { kind: "key", value: "j" }, expect: [{ target: "selected-row" }] },
      { action: { kind: "key", value: "a" }, expect: [{ target: "focus-lane", count: 2 }] },
      { action: { kind: "key", value: "t" }, expect: [{ target: "rail", absent: true }, { target: "focus-lane", count: 2 }] },
      { action: { kind: "key", value: "Tab" }, expect: [{ target: "focus-lane", selector: ".focus-slot:first-child .lane-track.active-zone" }] },
      { action: { kind: "key", value: "Tab" }, expect: [{ target: "focus-lane", selector: ".focus-slot + .focus-slot .lane-track.active-zone" }] },
      { action: { kind: "key", value: "Shift+Tab" }, expect: [{ target: "focus-lane", selector: ".focus-slot:first-child .lane-track.active-zone" }] },
      { action: { kind: "key", value: "x" }, expect: [{ target: "focus-lane", count: 1 }, { target: "rail", absent: true }] },
      { action: { kind: "key", value: "x" }, expect: [{ target: "focus-lane", count: 0 }, { target: "rail" }, attr("shell", "data-active-zone", "rail")] },
    ],
  },
  {
    id: "l", name: "context-lane-sweep-preserves-focus", keyboardOnly: true, surfaces: ["daemon"], steps: [
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-trajectory", "candidate")] },
      { action: { kind: "key", value: "Tab" }, expect: [attr("shell", "data-active-zone", "rail")] },
      { action: { kind: "key", value: "j" }, expect: [selectedRow("partial")] },
      { action: { kind: "key", value: "a" }, expect: [{ target: "focus-lane", count: 2 }] },
      { action: { kind: "key", value: "Tab" }, expect: [attr("shell", "data-active-zone", "rail")] },
      { action: { kind: "key", value: "j" }, expect: [selectedRow("fourth")] },
      { action: { kind: "key", value: "a" }, expect: [attr("context-lane", "data-trajectory", "fourth")] },
      { action: { kind: "key", value: "n" }, expect: [attr("context-lane", "data-trajectory", "reference"), { target: "focus-lane", selector: ".focus-lane[data-trajectory='candidate']" }, { target: "focus-lane", selector: ".focus-lane[data-trajectory='partial']" }] },
    ],
  },
  {
    id: "m", name: "episode-pointer-descend-strip-ascend", keyboardOnly: false, surfaces: ["daemon"], steps: [
      { action: { kind: "filter", value: "layered" }, expect: [selectedRow("layered")] },
      { action: { kind: "key", value: "Escape" }, expect: [selectedRow("layered")] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "1"), selectedEvent("Policy error")] },
      { action: { kind: "key", value: "+" }, expect: [attr("read", "data-axis-start", "17.0000")] },
      { action: { kind: "key", value: "+" }, expect: [attr("read", "data-axis-start", "25.5000")] },
      { action: { kind: "key", value: "+" }, expect: [attr("read", "data-axis-start", "29.2400")] },
      { action: { kind: "capture-attribute", target: ".lane-track.active-zone", attribute: "data-axis-start", key: "pre-descend-start" }, expect: [attr("read", "data-axis-start", "29.2400")] },
      { action: { kind: "capture-attribute", target: ".lane-track.active-zone", attribute: "data-axis-end", key: "pre-descend-end" }, expect: [attr("read", "data-axis-end", "36.2400")] },
      { action: { kind: "capture-attribute", target: ".shape-strip", attribute: "data-selected-x", key: "layer-anchor" }, expect: [selectedEvent("Policy error")] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "2"), attr("read", "data-episode", "stage:verify#1"), { target: "rail", selector: ".episode-strip", attribute: "data-selected-x", attributeEqualsCapture: "layer-anchor" }, { target: "rail", selector: ".episode-button.selected", attribute: "data-episode-key", equals: "stage:verify#1" }] },
      { action: { kind: "click", target: ".episode-button.selected" }, expect: [attr("read", "data-depth", "3"), attr("read", "data-episode", "stage:verify#1"), attr("read", "data-axis-start", "29.0000"), attr("read", "data-axis-end", "36.3529"), { target: "read", attribute: "data-axis-start", attributeNumberLte: 29 }, { target: "read", attribute: "data-axis-end", attributeNumberGte: 34 }, { target: "rail", selector: ".shape-strip.compact", attribute: "data-selected-x", attributeEqualsCapture: "layer-anchor" }] },
      { action: { kind: "click", target: ".shape-strip.compact svg" }, expect: [attr("read", "data-depth", "2"), attr("read", "data-episode", "stage:verify#1"), { target: "read", attribute: "data-axis-start", attributeEqualsCapture: "pre-descend-start" }, { target: "read", attribute: "data-axis-end", attributeEqualsCapture: "pre-descend-end" }, { target: "rail", selector: ".episode-strip", attribute: "data-selected-x", attributeEqualsCapture: "layer-anchor" }] },
    ],
  },
  {
    id: "n", name: "episode-keyboard-descend-ascend-twin", keyboardOnly: true, surfaces: ["daemon", "webapp"], steps: [
      { action: { kind: "filter", value: "layered" }, expect: [selectedRow("layered")] },
      { action: { kind: "key", value: "Escape" }, expect: [selectedRow("layered")] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "1"), selectedEvent("Policy error")] },
      { action: { kind: "key", value: "+" }, expect: [attr("read", "data-axis-start", "17.0000")] },
      { action: { kind: "key", value: "+" }, expect: [attr("read", "data-axis-start", "25.5000")] },
      { action: { kind: "key", value: "+" }, expect: [attr("read", "data-axis-start", "29.2400")] },
      { action: { kind: "capture-attribute", target: ".lane-track.active-zone", attribute: "data-axis-start", key: "pre-descend-start" }, expect: [attr("read", "data-axis-start", "29.2400")] },
      { action: { kind: "capture-attribute", target: ".lane-track.active-zone", attribute: "data-axis-end", key: "pre-descend-end" }, expect: [attr("read", "data-axis-end", "36.2400")] },
      { action: { kind: "capture-attribute", target: ".shape-strip", attribute: "data-selected-x", key: "layer-anchor" }, expect: [selectedEvent("Policy error")] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "2"), attr("read", "data-episode", "stage:verify#1"), { target: "rail", selector: ".episode-strip", attribute: "data-selected-x", attributeEqualsCapture: "layer-anchor" }, { target: "rail", selector: ".episode-button.selected", attribute: "data-episode-key", equals: "stage:verify#1" }] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "3"), attr("read", "data-episode", "stage:verify#1"), attr("read", "data-axis-start", "29.0000"), attr("read", "data-axis-end", "36.3529"), { target: "read", attribute: "data-axis-start", attributeNumberLte: 29 }, { target: "read", attribute: "data-axis-end", attributeNumberGte: 34 }, { target: "rail", selector: ".shape-strip.compact", attribute: "data-selected-x", attributeEqualsCapture: "layer-anchor" }] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "4"), { target: "rail", selector: ".lane-source" }, { target: "rail", selector: ".shape-strip.compact", attribute: "data-selected-x", attributeEqualsCapture: "layer-anchor" }] },
      { action: { kind: "key", value: "Escape" }, expect: [attr("read", "data-depth", "3"), { target: "rail", selector: ".shape-strip.compact", attribute: "data-selected-x", attributeEqualsCapture: "layer-anchor" }] },
      { action: { kind: "key", value: "Escape" }, expect: [attr("read", "data-depth", "2"), attr("read", "data-episode", "stage:verify#1"), { target: "read", attribute: "data-axis-start", attributeEqualsCapture: "pre-descend-start" }, { target: "read", attribute: "data-axis-end", attributeEqualsCapture: "pre-descend-end" }, { target: "rail", selector: ".episode-strip", attribute: "data-selected-x", attributeEqualsCapture: "layer-anchor" }] },
    ],
    webappSteps: [
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "1")] },
      { action: { kind: "capture-attribute", target: ".shape-strip", attribute: "data-selected-x", key: "layer-anchor" }, expect: [{ target: "selected-event" }] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "2"), { target: "rail", selector: ".episode-strip", attribute: "data-selected-x", attributeEqualsCapture: "layer-anchor" }] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "3"), { target: "rail", selector: ".shape-strip.compact", attribute: "data-selected-x", attributeEqualsCapture: "layer-anchor" }, { target: "rail", selector: ".lane-events" }] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "4"), { target: "rail", selector: ".lane-source" }, { target: "rail", selector: ".shape-strip.compact", attribute: "data-selected-x", attributeEqualsCapture: "layer-anchor" }] },
      { action: { kind: "key", value: "Escape" }, expect: [attr("read", "data-depth", "3"), { target: "rail", selector: ".shape-strip.compact", attribute: "data-selected-x", attributeEqualsCapture: "layer-anchor" }] },
      { action: { kind: "key", value: "Escape" }, expect: [attr("read", "data-depth", "2"), { target: "rail", selector: ".episode-strip", attribute: "data-selected-x", attributeEqualsCapture: "layer-anchor" }] },
    ],
  },
  {
    id: "o", name: "episode-j-k-traversal", keyboardOnly: true, surfaces: ["daemon", "webapp"], steps: [
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "1")] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "2"), attr("read", "data-episode", "stage:verify#1")] },
      { action: { kind: "key", value: "j" }, expect: [attr("read", "data-episode", "stage:outcome#1"), selectedEvent("Final reward"), { target: "rail", selector: ".episode-button.selected", attribute: "data-episode-key", equals: "stage:outcome#1" }] },
      { action: { kind: "key", value: "k" }, expect: [attr("read", "data-episode", "stage:verify#1"), selectedEvent("Policy error")] },
    ],
    webappSteps: [
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "1")] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "2")] },
      { action: { kind: "capture-attribute", target: ".lane-track.active-zone", attribute: "data-episode", key: "episode-before" }, expect: [{ target: "rail", selector: ".episode-button.selected" }] },
      { action: { kind: "key", value: "j" }, expect: [{ target: "read", attribute: "data-episode", attributeNotEqualsCapture: "episode-before" }, { target: "rail", selector: ".episode-button.selected" }] },
      { action: { kind: "key", value: "k" }, expect: [{ target: "read", attribute: "data-episode", attributeEqualsCapture: "episode-before" }] },
    ],
  },
  {
    id: "p", name: "landmark-crosses-episode-boundary", keyboardOnly: true, surfaces: ["daemon", "webapp"], webappExample: "300-event coding trace", steps: [
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "1")] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "2")] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "3"), attr("read", "data-episode", "stage:verify#1")] },
      { action: { kind: "capture-attribute", target: ".lane-track.active-zone", attribute: "data-axis-start", key: "episode-axis" }, expect: [selectedEvent("Policy error")] },
      { action: { kind: "key", value: "c" }, expect: [attr("read", "data-episode", "stage:setup#1"), selectedEvent("Context compacted"), { target: "read", attribute: "data-axis-start", attributeNotEqualsCapture: "episode-axis" }, { target: "rail", selector: ".lane-events", attribute: "data-episode-key", equals: "stage:setup#1" }] },
    ],
    webappSteps: [
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "1")] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "2")] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "3")] },
      { action: { kind: "capture-attribute", target: ".lane-track.active-zone", attribute: "data-episode", key: "episode-before" }, expect: [{ target: "rail", selector: ".lane-events" }] },
      { action: { kind: "capture-attribute", target: ".lane-track.active-zone", attribute: "data-axis-start", key: "episode-axis" }, expect: [{ target: "selected-event" }] },
      // in the real bugfix trace the compaction sits INSIDE stage:diagnose, so
      // `c` moves selection without switching episodes (that is correct
      // behavior); the reward/grader landmark genuinely crosses into verify.
      { action: { kind: "key", value: "c" }, expect: [{ target: "selected-event", contains: "ompact" }, { target: "read", attribute: "data-episode", attributeEqualsCapture: "episode-before" }] },
      { action: { kind: "key", value: "r" }, expect: [{ target: "read", attribute: "data-episode", attributeNotEqualsCapture: "episode-before" }, { target: "read", attribute: "data-axis-start", attributeNotEqualsCapture: "episode-axis" }, { target: "rail", selector: ".lane-events" }] },
    ],
  },
  {
    id: "q", name: "daemon-completes-paginated-episode", keyboardOnly: true, surfaces: ["daemon"], steps: [
      { action: { kind: "filter", value: "long" }, expect: [selectedRow("long")] },
      { action: { kind: "key", value: "Escape" }, expect: [selectedRow("long")] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-trajectory", "long"), attr("read", "data-axis-start", "0.0000"), attr("read", "data-axis-end", "249.0000"), { target: "strip", attribute: "data-visible-events", equals: "250" }] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "2"), attr("read", "data-episode", "stage:bulk#1"), { target: "rail", selector: ".episode-summary", contains: "message 249" }, { target: "rail", selector: ".episode-summary", contains: "error 1" }, { target: "rail", selector: ".episode-summary", contains: "#0–#249" }] },
    ],
  },
];
