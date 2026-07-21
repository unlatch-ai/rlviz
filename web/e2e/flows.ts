export type Observable = {
  target: "shell" | "browse" | "read" | "compare" | "selected-row" | "selected-event" | "filter" | "strip" | "marked-rows" | "alert";
  attribute?: string;
  equals?: string;
  notEquals?: string;
  contains?: string;
  absent?: boolean;
  count?: number;
};

export type FlowAction =
  | { kind: "key"; value: string }
  | { kind: "filter"; value: string }
  | { kind: "click"; target: string; clicks?: number }
  | { kind: "strip-click"; eventIndex: number };

export type FlowStep = { action: FlowAction; expect: Observable[] };
export type Flow = { id: "a" | "b" | "c" | "d" | "e" | "f"; name: string; keyboardOnly: boolean; surfaces: Array<"daemon" | "webapp">; steps: FlowStep[]; webappSteps?: FlowStep[] };

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
      { action: { kind: "key", value: "2" }, expect: [selectedRow("reference"), { target: "browse", contains: "tag 2" }] },
      { action: { kind: "filter", value: "reference" }, expect: [selectedRow("reference"), attr("browse", "data-filter", "reference")] },
      { action: { kind: "filter", value: "" }, expect: [selectedRow("reference"), attr("browse", "data-filter", "")] },
      { action: { kind: "key", value: "Escape" }, expect: [mode("browse"), selectedRow("reference")] },
    ],
    webappSteps: [
      { action: { kind: "key", value: "j" }, expect: [{ target: "selected-row", contains: "checkout-rollout" }] },
      { action: { kind: "key", value: "j" }, expect: [{ target: "selected-row", contains: "checkout-rollout" }] },
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
      { action: { kind: "key", value: "Enter" }, expect: [mode("read"), attr("read", "data-trajectory", "candidate"), attr("read", "data-fidelity", "glyphs"), { target: "read", contains: "Read · 1/4" }] },
      { action: { kind: "key", value: "j" }, expect: [selectedEvent("Final reward")] },
      { action: { kind: "key", value: "k" }, expect: [selectedEvent("Policy error")] },
      { action: { kind: "key", value: "e" }, expect: [selectedEvent("Policy error")] },
      { action: { kind: "key", value: "Escape" }, expect: [mode("browse"), selectedRow("candidate"), attr("browse", "data-filter", "demo"), attr("browse", "data-fidelity", "glyphs")] },
    ],
    webappSteps: [
      { action: { kind: "filter", value: "checkout" }, expect: [attr("browse", "data-filter", "checkout")] },
      { action: { kind: "key", value: "Escape" }, expect: [mode("browse"), attr("browse", "data-filter", "checkout")] },
      { action: { kind: "key", value: "Enter" }, expect: [mode("read"), attr("read", "data-fidelity", "glyphs")] },
      { action: { kind: "key", value: "j" }, expect: [mode("read"), { target: "selected-event" }] },
      { action: { kind: "key", value: "k" }, expect: [mode("read"), { target: "selected-event" }] },
      { action: { kind: "key", value: "e" }, expect: [mode("read"), { target: "selected-event" }] },
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
    id: "d", name: "compare-loop", keyboardOnly: true, surfaces: ["daemon"], steps: [
      { action: { kind: "key", value: "Space" }, expect: [{ target: "selected-row", attribute: "class", contains: "marked" }] },
      { action: { kind: "key", value: "j" }, expect: [selectedRow("partial")] },
      { action: { kind: "key", value: "Space" }, expect: [{ target: "selected-row", attribute: "class", contains: "marked" }] },
      { action: { kind: "key", value: "v" }, expect: [mode("compare")] },
      { action: { kind: "key", value: "d" }, expect: [{ target: "compare", contains: "first divergence" }] },
      { action: { kind: "key", value: "Enter" }, expect: [mode("read"), attr("read", "data-trajectory", "candidate")] },
      { action: { kind: "key", value: "Escape" }, expect: [mode("compare"), attr("compare", "data-selected-stage", "stage:act")] },
      { action: { kind: "key", value: "Escape" }, expect: [mode("browse"), { target: "marked-rows", count: 2 }] },
    ],
  },
  {
    id: "e", name: "zoom-depth", keyboardOnly: true, surfaces: ["daemon", "webapp"], steps: [
      { action: { kind: "key", value: "Enter" }, expect: [mode("read")] },
      { action: { kind: "key", value: "+" }, expect: [{ target: "strip", attribute: "data-visible-events", notEquals: "6" }] },
      { action: { kind: "key", value: "+" }, expect: [{ target: "strip", attribute: "data-visible-events", notEquals: "6" }] },
      { action: { kind: "key", value: "c" }, expect: [selectedEvent("Context compacted"), attr("read", "data-axis-start", "8.1250")] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "2")] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "3")] },
      { action: { kind: "key", value: "Escape" }, expect: [attr("read", "data-depth", "2")] },
      { action: { kind: "key", value: "Escape" }, expect: [attr("read", "data-depth", "1")] },
      { action: { kind: "key", value: "0" }, expect: [{ target: "strip", attribute: "data-visible-events", equals: "6" }] },
    ],
    webappSteps: [
      { action: { kind: "key", value: "Enter" }, expect: [mode("read")] },
      { action: { kind: "key", value: "+" }, expect: [{ target: "strip" }] },
      { action: { kind: "key", value: "+" }, expect: [{ target: "strip" }] },
      { action: { kind: "key", value: "e" }, expect: [{ target: "selected-event" }] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "2")] },
      { action: { kind: "key", value: "Enter" }, expect: [attr("read", "data-depth", "3")] },
      { action: { kind: "key", value: "Escape" }, expect: [attr("read", "data-depth", "2")] },
      { action: { kind: "key", value: "Escape" }, expect: [attr("read", "data-depth", "1")] },
      { action: { kind: "key", value: "0" }, expect: [{ target: "strip" }] },
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
];
