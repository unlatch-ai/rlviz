import { expect, test, type Locator, type Page } from "@playwright/test";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { flows, type FlowAction, type Observable } from "../../web/e2e/flows";

test.beforeEach(async ({ page }) => {
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "dist");
  const contentTypes: Record<string, string> = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".wasm": "application/wasm" };
  await page.route("**/*", async (route) => {
    const url = new URL(route.request().url());
    if (url.origin !== "http://127.0.0.1:4174") return route.abort();
    const relative = url.pathname === "/" ? "index.html" : url.pathname.slice(1);
    try { await route.fulfill({ body: await readFile(path.join(root, relative)), contentType: contentTypes[path.extname(relative)] ?? "application/octet-stream" }); }
    catch { await route.fulfill({ status: 404, body: "not found" }); }
  });
  await page.goto("/");
  await page.getByRole("button", { name: "checkout cohort" }).click();
  await expect(page.getByRole("main", { name: "Browse trajectories" })).toBeVisible({ timeout: 15_000 });
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

for (const flow of flows.filter((item) => item.surfaces.includes("webapp"))) {
  test(`${flow.id}. ${flow.name} through bundled in-browser provider`, async ({ page }) => {
    const steps = flow.webappSteps ?? flow.steps;
    expect(steps.every((step) => step.action.kind !== "click" && step.action.kind !== "strip-click")).toBe(true);
    for (const step of steps) {
      expect(step.expect.length).toBeGreaterThan(0);
      await act(page, step.action);
      for (const observable of step.expect) await observe(page, observable);
    }
    const selected = page.locator("[role=option][aria-selected=true], .moment.selected").first();
    const text = await selected.textContent();
    await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => resolve())));
    expect(await selected.textContent()).toBe(text);
    await expect(page.locator("main:focus")).toBeVisible();
    await expect(page.getByRole("alert")).toHaveCount(0);
  });
}
