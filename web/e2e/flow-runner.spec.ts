import { expect, test, type Locator, type Page } from "@playwright/test";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { flows, type FlowAction, type Observable } from "./flows";

const rows = [
  { id: "candidate", pass: false, reward: -0.8, errors: 1 },
  { id: "partial", pass: false, reward: 0.5, errors: 1 },
  { id: "fourth", pass: false, reward: 0.8, errors: 0 },
  { id: "reference", pass: true, reward: 1, errors: 0 },
  // Keep review-only fixtures after the original attention queue so the
  // established a-p flows retain their deterministic row order.
  { id: "layered", pass: true, reward: 1, errors: 0 },
  { id: "long", pass: true, reward: 1, errors: 0 },
];

const events = (id: string) => [
  { id: `${id}-start`, sequence: 0, kind: "message", title: "Task prompt", alignment_key: "stage:setup" },
  { id: `${id}-context`, sequence: 10, kind: "state", title: "Context compacted", alignment_key: "stage:setup", context: { operation: "compaction", input_tokens_before: 8000, input_tokens: 2100, capacity: 10000, provenance: "source_native" } },
  { id: `${id}-tool`, sequence: 20, kind: "tool", title: "Run tool", alignment_key: "stage:act", output: { ok: id !== "candidate" } },
  { id: `${id}-error`, sequence: 30, kind: "error", title: "Policy error", alignment_key: "stage:verify", data: { class: "policy" } },
  { id: `${id}-reward`, sequence: 40, kind: "reward", title: "Final reward", alignment_key: "stage:outcome", data: { total: rows.find((row) => row.id === id)?.reward ?? 0 } },
  { id: `${id}-grader`, sequence: 50, kind: "grader", title: "Verifier", alignment_key: "stage:outcome", output: { verdict: id === "reference" ? "pass" : "fail", evidence: [`${id}-tool`] } },
];

const layeredEvents = [
  { id: "layered-start", sequence: 0, kind: "message", title: "Task prompt", alignment_key: "stage:setup" },
  { id: "layered-context", sequence: 10, kind: "state", title: "Context compacted", alignment_key: "stage:setup" },
  { id: "layered-tool", sequence: 20, kind: "tool", title: "Run tool", alignment_key: "stage:act" },
  { id: "layered-check-1", sequence: 29, kind: "observation", title: "Verification opened", alignment_key: "stage:verify" },
  { id: "layered-check-2", sequence: 30, kind: "tool", title: "Read result", alignment_key: "stage:verify" },
  { id: "layered-check-3", sequence: 31, kind: "observation", title: "Compare result", alignment_key: "stage:verify" },
  { id: "layered-check-4", sequence: 32, kind: "state", title: "Record verdict", alignment_key: "stage:verify" },
  { id: "layered-error", sequence: 34, kind: "error", title: "Policy error", alignment_key: "stage:verify" },
  { id: "layered-reward", sequence: 40, kind: "reward", title: "Final reward", alignment_key: "stage:outcome", data: { total: -0.2 } },
  { id: "layered-grader", sequence: 50, kind: "grader", title: "Verifier", alignment_key: "stage:outcome" },
];
const longEvents = Array.from({ length: 250 }, (_, sequence) => ({ id: `long-${sequence}`, sequence, kind: sequence === 249 ? "error" : "message", title: sequence === 249 ? "Terminal error" : `Event ${sequence}`, alignment_key: "stage:bulk" }));

const browse = {
  sources: [{ id: "source-1", path: "/tmp/demo.ndjson", index_state: "complete" }], count: rows.length,
  trajectories: rows.map((row) => ({ source_id: "source-1", source_name: "demo.ndjson", case_name: row.id, group_name: "demo group", trajectory: { id: row.id, group_id: "group", status: row.pass ? "completed" : "failed" }, metrics: { trajectory: { id: row.id, group_id: "group" }, event_count: row.id === "long" ? 250 : row.id === "layered" ? 10 : 6, error_count: row.errors, pass: row.pass, reward: row.reward } })),
};

const trajectoryResponse = (id: string) => {
  const row = rows.find((item) => item.id === id)!;
  const allEvents = id === "layered" ? layeredEvents : id === "long" ? longEvents : events(id);
  const loadedEvents = id === "long" ? allEvents.slice(0, 200) : allEvents;
  return { trajectory: { id, group_id: "group", status: row.pass ? "completed" : "failed", termination: row.pass ? "complete" : "grader_failed" }, events: loadedEvents, signals: [{ id: `${id}-pass`, trajectory_id: id, event_id: `${id}-grader`, name: "pass", value: row.pass }, { id: `${id}-reward-signal`, trajectory_id: id, event_id: `${id}-reward`, name: "reward", value: row.reward }], page: { count: loadedEvents.length, total: allEvents.length, limit: 200, next_sequence: id === "long" ? 199 : undefined, has_more: id === "long" } };
};

test.beforeEach(async ({ page }) => {
  await page.route("**/api/v1/trajectory**", (route) => route.fulfill({ json: trajectoryResponse("candidate") }));
  await page.route("**/api/v1/indexed/browse", (route) => route.fulfill({ json: browse }));
  await page.route("**/api/v1/indexed/analysis**", (route) => route.fulfill({ json: { analysis: { api_version: "v1", provenance: { name: "test", version: "1", digest: "x", input_digest: "y" }, findings: [], signals: [] }, cached: false, analyzed_at: "now" } }));
  await page.route("**/api/v1/indexed/compare**", async (route) => {
    const url = new URL(route.request().url());
    const left = url.searchParams.get("left") ?? "candidate", right = url.searchParams.get("right") ?? "partial";
    await route.fulfill({ json: { left: trajectoryResponse(left), right: trajectoryResponse(right), alignment: { steps: [], common_behavioral_prefix: 0, first_meaningful_divergence: 0 }, differences: { event_count: { left: 6, right: 6, delta: 0 }, status: { changed: true }, termination: { changed: false }, reward: { changed: true } } } });
  });
  await page.route("**/api/v1/indexed/trajectory**", (route) => {
    const id = new URL(route.request().url()).searchParams.get("trajectory_id") ?? "candidate";
    return route.fulfill({ json: trajectoryResponse(id) });
  });
  await page.route("**/api/v1/indexed/events**", (route) => route.fulfill({ json: { events: longEvents.slice(200), page: { count: 50, total: 250, limit: 200, after_sequence: 199, has_more: false } } }));
  const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
  const contentTypes: Record<string, string> = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".map": "application/json" };
  await page.route("http://127.0.0.1:4173/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname.startsWith("/api/")) return route.fallback();
    const relative = url.pathname === "/" ? "index.html" : url.pathname.slice(1);
    try { await route.fulfill({ body: await readFile(path.join(webRoot, "dist", relative)), contentType: contentTypes[path.extname(relative)] ?? "application/octet-stream" }); }
    catch { await route.fulfill({ status: 404, body: "not found" }); }
  });
  await page.goto("http://127.0.0.1:4173/", { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("main", { name: "Browse trajectories" })).toBeVisible();
});

function target(page: Page, observable: Observable): Locator {
  if (observable.selector) return page.locator(observable.selector);
  switch (observable.target) {
    case "shell": return page.locator(".instrument-shell");
    case "browse": return page.getByRole("main", { name: "Browse trajectories" });
    case "read": return page.getByRole("main", { name: "Read trajectory" });
    case "compare": return page.getByRole("main", { name: "Pair Compare" });
    case "selected-row": return page.locator("[role=option][aria-selected=true]");
    case "selected-event": return page.locator(".moment.selected");
    case "filter": return page.locator("#browse-filter");
    case "strip": return page.getByRole("region", { name: "Trajectory shape" });
    case "marked-rows": return page.locator("[role=option].marked");
    case "alert": return page.getByRole("alert");
    case "rail": return page.locator(".workspace-rail");
    case "stage": return page.locator(".workspace-stage");
    case "focus-lane": return page.locator(".lane-track.focus-lane");
    case "context-lane": return page.locator(".lane-track.context-lane");
    case "console": return page.locator(".workspace-console");
    case "breadcrumb": return page.locator(".workspace-breadcrumb");
    case "reference": return page.getByTestId("reference-name");
    case "seam": return page.locator(".workspace-sash");
  }
}

async function act(page: Page, action: FlowAction, boxes: Map<string, Awaited<ReturnType<Locator["boundingBox"]>>>, attributes: Map<string, string | null>) {
  if (action.kind === "key") return page.keyboard.press(action.value === "+" ? "Shift+Equal" : action.value);
  if (action.kind === "filter") return page.locator("#browse-filter").fill(action.value);
  if (action.kind === "click") return page.locator(action.target).first().click({ clickCount: action.clicks ?? 1 });
  if (action.kind === "capture-box") { boxes.set(action.key, await page.locator(action.target).first().boundingBox()); return; }
  if (action.kind === "capture-attribute") { attributes.set(action.key, await page.locator(action.target).first().getAttribute(action.attribute)); return; }
  if (action.kind === "seam-drag") {
    const seam = page.locator(`[data-seam="${action.name}"]`); const box = await seam.boundingBox(); if (!box) throw new Error(`missing ${action.name} seam`);
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2); await page.mouse.down(); await page.mouse.move(box.x + box.width / 2 + action.dx, box.y + box.height / 2 + action.dy); await page.mouse.up(); return;
  }
  if (action.kind === "reload") { await page.reload({ waitUntil: "domcontentloaded" }); await expect(page.locator(".workspace-rack")).toBeVisible(); return; }
  if (action.kind === "history-back") { await page.goBack(); return; }
  const shape = page.locator(`[data-event-index="${action.eventIndex}"]`);
  await shape.hover();
  return shape.click();
}

async function observe(page: Page, observable: Observable, boxes: Map<string, Awaited<ReturnType<Locator["boundingBox"]>>>, attributes: Map<string, string | null>) {
  const locator = target(page, observable);
  if (observable.absent) return expect(locator).toHaveCount(0);
  if (observable.count !== undefined) return expect(locator).toHaveCount(observable.count);
  await expect(locator.first()).toBeVisible();
  if (observable.attribute && observable.equals !== undefined) await expect(locator.first()).toHaveAttribute(observable.attribute, observable.equals);
  if (observable.attribute && observable.notEquals !== undefined) await expect(locator.first()).not.toHaveAttribute(observable.attribute, observable.notEquals);
  if (observable.attribute && observable.contains !== undefined) await expect(locator.first()).toHaveAttribute(observable.attribute, new RegExp(observable.contains));
  if (!observable.attribute && observable.equals !== undefined) await expect(locator).toHaveText(observable.equals);
  if (!observable.attribute && observable.contains !== undefined) await expect(locator.first()).toContainText(observable.contains);
  if (observable.boxEquals) expect(await locator.first().boundingBox()).toEqual(boxes.get(observable.boxEquals));
  if (observable.boxNotEquals) expect(await locator.first().boundingBox()).not.toEqual(boxes.get(observable.boxNotEquals));
  if (observable.attribute && observable.attributeEqualsCapture) expect(await locator.first().getAttribute(observable.attribute)).toBe(attributes.get(observable.attributeEqualsCapture));
  if (observable.attribute && observable.attributeNotEqualsCapture) expect(await locator.first().getAttribute(observable.attribute)).not.toBe(attributes.get(observable.attributeNotEqualsCapture));
  if (observable.attribute && observable.attributeNumberLte !== undefined) expect(Number(await locator.first().getAttribute(observable.attribute))).toBeLessThanOrEqual(observable.attributeNumberLte);
  if (observable.attribute && observable.attributeNumberGte !== undefined) expect(Number(await locator.first().getAttribute(observable.attribute))).toBeGreaterThanOrEqual(observable.attributeNumberGte);
}

async function invariants(page: Page) {
  const selected = await page.locator("[role=option][aria-selected=true], .moment.selected, .stage-row.selected").first().getAttribute("class");
  const selectedText = await page.locator("[role=option][aria-selected=true], .moment.selected, .stage-row.selected").first().textContent();
  await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => resolve())));
  expect(await page.locator("[role=option][aria-selected=true], .moment.selected, .stage-row.selected").first().getAttribute("class")).toBe(selected);
  expect(await page.locator("[role=option][aria-selected=true], .moment.selected, .stage-row.selected").first().textContent()).toBe(selectedText);
  await expect(page.locator("main:focus")).toBeVisible();
  await expect(page.getByRole("alert")).toHaveCount(0);
}

for (const flow of flows.filter((item) => item.surfaces.includes("daemon"))) {
  test(`${flow.id}. ${flow.name}`, async ({ page }) => {
    const boxes = new Map<string, Awaited<ReturnType<Locator["boundingBox"]>>>();
    const attributes = new Map<string, string | null>();
    if (flow.keyboardOnly) expect(flow.steps.every((step) => step.action.kind !== "click" && step.action.kind !== "strip-click")).toBe(true);
    for (const step of flow.steps) {
      expect(step.expect.length).toBeGreaterThan(0);
      await act(page, step.action, boxes, attributes);
      for (const observable of step.expect) await observe(page, observable, boxes, attributes);
    }
    await invariants(page);
  });
}
