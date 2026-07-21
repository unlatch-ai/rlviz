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
];

const events = (id: string) => [
  { id: `${id}-start`, sequence: 0, kind: "message", title: "Task prompt", alignment_key: "stage:setup" },
  { id: `${id}-context`, sequence: 10, kind: "state", title: "Context compacted", alignment_key: "stage:setup", context: { operation: "compaction", input_tokens_before: 8000, input_tokens: 2100, capacity: 10000, provenance: "source_native" } },
  { id: `${id}-tool`, sequence: 20, kind: "tool", title: "Run tool", alignment_key: "stage:act", output: { ok: id !== "candidate" } },
  { id: `${id}-error`, sequence: 30, kind: "error", title: "Policy error", alignment_key: "stage:verify", data: { class: "policy" } },
  { id: `${id}-reward`, sequence: 40, kind: "reward", title: "Final reward", alignment_key: "stage:outcome", data: { total: rows.find((row) => row.id === id)?.reward ?? 0 } },
  { id: `${id}-grader`, sequence: 50, kind: "grader", title: "Verifier", alignment_key: "stage:outcome", output: { verdict: id === "reference" ? "pass" : "fail", evidence: [`${id}-tool`] } },
];

const browse = {
  sources: [{ id: "source-1", path: "/tmp/demo.ndjson", index_state: "complete" }], count: rows.length,
  trajectories: rows.map((row) => ({ source_id: "source-1", source_name: "demo.ndjson", case_name: "policy demo", group_name: "demo group", trajectory: { id: row.id, group_id: "group", status: row.pass ? "completed" : "failed" }, metrics: { trajectory: { id: row.id, group_id: "group" }, event_count: 6, error_count: row.errors, pass: row.pass, reward: row.reward } })),
};

const trajectoryResponse = (id: string) => {
  const row = rows.find((item) => item.id === id)!;
  return { trajectory: { id, group_id: "group", status: row.pass ? "completed" : "failed", termination: row.pass ? "complete" : "grader_failed" }, events: events(id), signals: [{ id: `${id}-pass`, trajectory_id: id, event_id: `${id}-grader`, name: "pass", value: row.pass }, { id: `${id}-reward-signal`, trajectory_id: id, event_id: `${id}-reward`, name: "reward", value: row.reward }], page: { count: 6, total: 6, limit: 200, has_more: false } };
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
  }
}

async function act(page: Page, action: FlowAction) {
  if (action.kind === "key") return page.keyboard.press(action.value);
  if (action.kind === "filter") return page.locator("#browse-filter").fill(action.value);
  if (action.kind === "click") return page.locator(action.target).first().click({ clickCount: action.clicks ?? 1 });
  const shape = page.locator(`[data-event-index="${action.eventIndex}"]`);
  await shape.hover();
  return shape.click();
}

async function observe(page: Page, observable: Observable) {
  const locator = target(page, observable);
  if (observable.absent) return expect(locator).toHaveCount(0);
  if (observable.count !== undefined) return expect(locator).toHaveCount(observable.count);
  await expect(locator).toBeVisible();
  if (observable.attribute && observable.equals !== undefined) await expect(locator).toHaveAttribute(observable.attribute, observable.equals);
  if (observable.attribute && observable.notEquals !== undefined) await expect(locator).not.toHaveAttribute(observable.attribute, observable.notEquals);
  if (observable.attribute && observable.contains !== undefined) await expect(locator).toHaveAttribute(observable.attribute, new RegExp(observable.contains));
  if (!observable.attribute && observable.equals !== undefined) await expect(locator).toHaveText(observable.equals);
  if (!observable.attribute && observable.contains !== undefined) await expect(locator).toContainText(observable.contains);
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
    if (flow.keyboardOnly) expect(flow.steps.every((step) => step.action.kind !== "click" && step.action.kind !== "strip-click")).toBe(true);
    for (const step of flow.steps) {
      expect(step.expect.length).toBeGreaterThan(0);
      await act(page, step.action);
      for (const observable of step.expect) await observe(page, observable);
    }
    await invariants(page);
  });
}
