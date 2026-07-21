import { expect, test, type Locator, type Page } from "@playwright/test";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { flows, type FlowAction, type Observable } from "../../web/e2e/flows";

test.beforeEach(async ({ page }) => {
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "dist");
  const vercel = JSON.parse(await readFile(path.join(root, "vercel.json"), "utf8")) as { headers: Array<{ headers: Array<{ key: string; value: string }> }> };
  const securityHeaders = Object.fromEntries(vercel.headers[0].headers.map(({ key, value }) => [key, value]));
  const contentTypes: Record<string, string> = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".wasm": "application/wasm" };
  await page.route("**/*", async (route) => {
    const url = new URL(route.request().url());
    if (url.origin !== "http://127.0.0.1:4174") return route.abort();
    const relative = url.pathname === "/" ? "index.html" : url.pathname.slice(1);
    try { await route.fulfill({ body: await readFile(path.join(root, relative)), contentType: contentTypes[path.extname(relative)] ?? "application/octet-stream", headers: securityHeaders }); }
    catch { await route.fulfill({ status: 404, body: "not found" }); }
  });
  await page.goto("/");
});

async function loadExample(page: Page, name: string) {
  await page.getByRole("button", { name }).click();
  await expect(page.getByRole("main", { name: "Browse trajectories" })).toBeVisible({ timeout: 15_000 });
}

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

for (const flow of flows.filter((item) => item.surfaces.includes("webapp"))) {
  test(`${flow.id}. ${flow.name} through bundled in-browser provider`, async ({ page }) => {
    const boxes = new Map<string, Awaited<ReturnType<Locator["boundingBox"]>>>();
    const attributes = new Map<string, string | null>();
    await loadExample(page, flow.webappExample ?? "checkout cohort");
    const steps = flow.webappSteps ?? flow.steps;
    expect(steps.every((step) => step.action.kind !== "click" && step.action.kind !== "strip-click")).toBe(true);
    for (const step of steps) {
      expect(step.expect.length).toBeGreaterThan(0);
      await act(page, step.action, boxes, attributes);
      for (const observable of step.expect) await observe(page, observable, boxes, attributes);
    }
    const selected = page.locator("[role=option][aria-selected=true], .moment.selected").first();
    const text = await selected.textContent();
    await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => resolve())));
    expect(await selected.textContent()).toBe(text);
    await expect(page.locator("main:focus")).toBeVisible();
    await expect(page.getByRole("alert")).toHaveCount(0);
  });
}
