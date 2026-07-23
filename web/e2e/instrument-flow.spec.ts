import { expect, test } from "@playwright/test";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const rows = [
  { id: "candidate", pass: false, reward: -0.8, errors: 1 },
  { id: "reference", pass: true, reward: 1, errors: 0 },
  { id: "partial", pass: false, reward: 0.5, errors: 1 },
];

const events = (id: string) => [
  { id: `${id}-start`, sequence: 0, kind: "message", title: "Task prompt" },
  { id: `${id}-context`, sequence: 10, kind: "state", title: "Context compacted", alignment_key: "context:compaction", context: { operation: "compaction", input_tokens_before: 8000, input_tokens: 2100, capacity: 10000, provenance: "source_native" } },
  { id: `${id}-tool`, sequence: 20, kind: "tool", title: "Run tool", alignment_key: "tool:run", output: { ok: id !== "candidate" } },
  ...(id === "candidate" ? [{ id: `${id}-error`, sequence: 30, kind: "error", title: "Policy error", data: { class: "policy" } }] : []),
  { id: `${id}-reward`, sequence: 40, kind: "reward", title: "Final reward", data: { total: rows.find((row) => row.id === id)?.reward ?? 0 } },
  { id: `${id}-grader`, sequence: 50, kind: "grader", title: "Verifier", output: { verdict: id === "reference" ? "pass" : "fail", evidence: [`${id}-tool`] } },
];

const browse = {
  sources: [{ id: "source-1", path: "/tmp/demo.ndjson", index_state: "complete" }], count: rows.length,
  trajectories: rows.map((row) => ({ source_id: "source-1", source_name: "demo.ndjson", case_name: "policy demo", group_name: "demo group", trajectory: { id: row.id, group_id: "group", status: row.pass ? "completed" : "failed" }, metrics: { trajectory: { id: row.id, group_id: "group" }, event_count: events(row.id).length, error_count: row.errors, pass: row.pass, reward: row.reward } })),
};

const highContrastPresentation = {
  api_version: "rlviz.dev/v1alpha1",
  palette: {
    name: "high-contrast",
    light: { ctx: "#005fcc", failPolicy: "#b00020", failInfra: "#b54708", good: "#005a00", page: "#ffffff", surface: "#ffffff", ink: "#000000", inkSecondary: "#333333", muted: "#666666", hairline: "#a0a0a0" },
    dark: { ctx: "#66aaff", failPolicy: "#ff5c5c", failInfra: "#ff9a6c", good: "#42d642", page: "#000000", surface: "#101010", ink: "#ffffff", inkSecondary: "#dddddd", muted: "#a0a0a0", hairline: "#666666" },
  },
};

test.beforeEach(async ({ page }) => {
  await page.route("**/api/v1/trajectory**", (route) => route.fulfill({ json: { trajectory: { id: "candidate", group_id: "group", status: "failed" }, events: events("candidate"), signals: [{ trajectory_id: "candidate", event_id: "candidate-reward", name: "reward", value: -0.8 }], presentation: highContrastPresentation } }));
  await page.route("**/api/v1/indexed/browse", (route) => route.fulfill({ json: browse }));
  await page.route("**/api/v1/indexed/analysis**", (route) => route.fulfill({ json: { analysis: { api_version: "v1", provenance: { name: "test", version: "1", digest: "x", input_digest: "y" }, findings: [{ id: "finding", trajectory_id: "candidate", event_ids: ["candidate-error"], kind: "policy", severity: "error", title: "Policy violation" }], signals: [] }, cached: false, analyzed_at: "now" } }));
  await page.route("**/api/v1/indexed/compare**", async (route) => {
    const url = new URL(route.request().url());
    const left = url.searchParams.get("left") ?? "candidate", right = url.searchParams.get("right") ?? "reference";
    await route.fulfill({ json: { left: { trajectory: { id: left }, events: events(left) }, right: { trajectory: { id: right }, events: events(right) }, alignment: { steps: [], common_behavioral_prefix: 0, first_meaningful_divergence: 0 }, differences: { event_count: { left: events(left).length, right: events(right).length, delta: events(right).length - events(left).length }, status: { changed: true }, termination: { changed: false }, reward: { changed: true } } } });
  });
  await page.route("**/api/v1/indexed/trajectory**", async (route) => {
    const id = new URL(route.request().url()).searchParams.get("trajectory_id") ?? "candidate";
    const row = rows.find((item) => item.id === id)!;
    await route.fulfill({ json: { trajectory: { id, group_id: "group", status: row.pass ? "completed" : "failed", termination: row.pass ? "complete" : "grader_failed" }, events: events(id), signals: [{ id: `${id}-pass`, trajectory_id: id, event_id: `${id}-grader`, name: "pass", value: row.pass }, { id: `${id}-reward-signal`, trajectory_id: id, event_id: `${id}-reward`, name: "reward", value: row.reward }], presentation: highContrastPresentation, page: { count: events(id).length, total: events(id).length, limit: 200, has_more: false } } });
  });
  // Serve the built viewer through route interception rather than inlining:
  // the JS bundle can legally contain "</script>" inside string literals,
  // which terminates an inline script tag early.
  const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
  const contentTypes: Record<string, string> = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".map": "application/json" };
  await page.route("http://127.0.0.1:4173/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname.startsWith("/api/")) return route.fallback();
    const rel = url.pathname === "/" ? "index.html" : url.pathname.replace(/^\//, "");
    try {
      const body = await readFile(path.join(webRoot, "dist", rel));
      await route.fulfill({ body, contentType: contentTypes[path.extname(rel)] ?? "application/octet-stream" });
    } catch {
      await route.fulfill({ status: 404, body: "not found" });
    }
  });
  await page.goto("http://127.0.0.1:4173/", { waitUntil: "domcontentloaded" });
});

test("Browse into a multi-lane workspace preserves the instrument invariants", async ({ page }) => {
  await expect(page.getByRole("main", { name: "Browse trajectories" })).toBeVisible();
  await expect(page.getByRole("option").first()).toContainText("candidate");
  await expect(page.getByRole("listbox", { name: "Trajectory collection" })).toHaveAttribute("data-fidelity-level", "L1");
  await expect(page.locator(".fidelity-readout b")).toHaveText("signals");
  await expect(page.getByRole("option").first()).toHaveAttribute("data-columns", "false");

  await page.keyboard.press("]");
  await expect(page.locator(".fidelity-readout b")).toHaveText("summary");
  await expect(page.getByRole("option").first()).toHaveAttribute("data-columns", "true");
  await page.keyboard.press("[");
  await expect(page.locator(".fidelity-readout b")).toHaveText("signals");
  await page.keyboard.press("[");
  await expect(page.locator(".fidelity-readout b")).toHaveText("compact");
  await page.keyboard.press("]");
  await expect(page.locator(".fidelity-readout b")).toHaveText("signals");
  await page.keyboard.press("Space");
  await expect(page.getByRole("main", { name: "Read trajectory" })).toHaveAttribute("data-trajectory", "candidate");
  await page.keyboard.press("Tab");
  await page.keyboard.press("Tab");
  await page.keyboard.press("j");
  await page.keyboard.press("a");
  await expect(page.getByRole("main", { name: "Read trajectory" })).toHaveCount(2);
  await page.keyboard.press("Shift+Tab");
  await page.keyboard.press("Shift+A");
  await expect(page.getByTestId("reference-name")).toHaveText("candidate");
  await page.keyboard.press("Shift+Tab");
  await expect(page.locator(".lane-track.active-zone")).toHaveAttribute("data-trajectory", "reference");
  await expect(page.locator(".moment.selected b")).toHaveText("Task prompt");

  const strip = page.locator(".lane-track.active-zone .shape-strip");
  const anchor = await strip.getAttribute("data-selected-x");
	const initialVisible = Number(await strip.getAttribute("data-visible-events"));
	await page.keyboard.press("+");
	await expect(strip).toHaveAttribute("data-selected-x", anchor!);
	expect(Number(await strip.getAttribute("data-visible-events"))).toBeLessThan(initialVisible);
	for (const key of ["-", "0"]) {
	  await page.keyboard.press(key);
	  await expect(strip).toHaveAttribute("data-selected-x", anchor!);
	}
  await page.keyboard.press("c");
  await expect(page.locator(".moment.selected b")).toHaveText("Context compacted");
  await page.keyboard.press("r");
  await expect(page.locator(".moment.selected b")).toHaveText("Final reward");

  const selection = await page.locator(".selection-address").textContent();
  await strip.locator("svg").hover({ position: { x: 30, y: 80 } });
  await expect(page.getByRole("status")).toBeVisible();
  await expect(page.locator(".selection-address")).toHaveText(selection!);
});

test("theme control switches computed high-contrast palette values", async ({ page }) => {
  const root = page.locator(":root");
  const initial = await root.getAttribute("data-theme");
  expect(initial === "light" || initial === "dark").toBeTruthy();
  const target = initial === "light" ? "dark" : "light";
  const settings = page.getByRole("region", { name: "RLViz settings" });
  await settings.getByRole("group", { name: "Color theme" }).getByRole("button", { name: target === "dark" ? "Dark" : "Light" }).click();
  await expect(root).toHaveAttribute("data-theme", target);
  const computed = await root.evaluate((element) => {
    const style = getComputedStyle(element);
    return { page: style.getPropertyValue("--page").trim(), ctx: style.getPropertyValue("--ctx").trim() };
  });
  expect(computed).toEqual(target === "dark"
    ? { page: "#000000", ctx: "#66aaff" }
    : { page: "#ffffff", ctx: "#005fcc" });
});

test("collection trial groups keep rollout options and the keybar in view", async ({ page }) => {
  await page.getByRole("button", { name: "trials" }).click();
  await expect(page.getByRole("main", { name: "Browse trajectories" })).toHaveAttribute("data-collection-view", "trials");
  await expect(page.locator(".collection-evaluation-summary")).toHaveAttribute("aria-label", "Evaluation summary");
  await expect(page.locator(".rail-evaluation-case")).toHaveCount(1);
  await expect(page.locator(".rail-evaluation-variant")).toHaveCount(1);
  await expect(page.locator(".rail-evaluation-variant")).toContainText("3 rollouts");
  await expect(page.getByRole("option")).toHaveCount(3);
  const layout = await page.locator(".workspace-rack").evaluate((rack) => {
    const keybar = rack.querySelector(".keybar")!.getBoundingClientRect();
    return { keybarBottom: keybar.bottom, viewport: window.innerHeight, scrollbar: getComputedStyle(rack).scrollbarColor };
  });
  expect(layout.keybarBottom).toBeLessThanOrEqual(layout.viewport);
  expect(layout.scrollbar).not.toBe("auto");
});
