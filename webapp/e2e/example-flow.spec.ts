import { expect, test } from "@playwright/test";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

test("first paint stays on the viewer shell while the bundled cohort is delayed", async ({ page }) => {
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "dist");
  const contentTypes: Record<string, string> = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".wasm": "application/wasm", ".ndjson": "application/x-ndjson" };
  let releaseSample: (() => void) | undefined;
  const sampleReady = new Promise<void>((resolve) => { releaseSample = resolve; });
  await page.route("**/*", async (route) => {
    const url = new URL(route.request().url());
    if (url.origin !== "http://127.0.0.1:4174") return route.abort();
    const relative = url.pathname === "/" ? "index.html" : url.pathname.slice(1);
    if (relative.includes("checkout-cohort")) await sampleReady;
    try { await route.fulfill({ body: await readFile(path.join(root, relative)), contentType: contentTypes[path.extname(relative)] ?? "application/octet-stream" }); }
    catch { await route.fulfill({ status: 404, body: "not found" }); }
  });

  await page.goto("/", { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("status", { name: "Loading RLViz" })).toBeVisible();
  await expect(page.getByText("Inspect agent rollouts locally.")).toHaveCount(0);
  releaseSample?.();
  await expect(page.getByRole("main", { name: "Browse trajectories" })).toBeVisible({ timeout: 15_000 });
  await expect(page.getByText("Inspect agent rollouts locally.")).toHaveCount(0);
});

test("bundled sample opens automatically, keeps guide state, and walks Browse to Read", async ({ page }) => {
  const requests: Array<{ url: string; method: string; body: string | null }> = [];
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "dist");
  const contentTypes: Record<string, string> = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".wasm": "application/wasm" };
  await page.route("**/*", async (route) => {
    const request = route.request();
    requests.push({ url: request.url(), method: request.method(), body: request.postData() });
    const url = new URL(request.url());
    if (url.origin !== "http://127.0.0.1:4174") return route.abort();
    const relative = url.pathname === "/" ? "index.html" : url.pathname.slice(1);
    try {
      await route.fulfill({ body: await readFile(path.join(root, relative)), contentType: contentTypes[path.extname(relative)] ?? "application/octet-stream" });
    } catch {
      await route.fulfill({ status: 404, body: "not found" });
    }
  });

  await page.goto("/");
  await expect(page.getByText("Inspect agent rollouts locally.")).toHaveCount(0);
  await expect(page.getByRole("main", { name: "Browse trajectories" })).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole("main", { name: "Browse trajectories" }).getByRole("option").first()).toContainText("checkout-rollout-01");
  await expect(page.getByRole("article", { name: "RLViz guide" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Overview" })).toHaveAttribute("aria-current", "page");
  await expect(page.getByRole("region", { name: "RLViz settings" })).toBeVisible();
  await page.getByRole("button", { name: "trials" }).click();
  await expect(page.locator(".rail-evaluation-case")).toHaveCount(1);
  await expect(page.locator(".rail-evaluation-variant")).toHaveCount(2);
  await expect(page.getByRole("group", { name: "Deliberate · temperature 0.2" })).toContainText("8 rollouts");
  await expect(page.getByRole("group", { name: "Direct · temperature 0.8" })).toContainText("8 rollouts");
  await page.getByRole("button", { name: "rollouts" }).click();
  await page.getByRole("article", { name: "RLViz guide" }).getByRole("button", { name: "close" }).click();
  await page.getByRole("region", { name: "RLViz settings" }).getByRole("button", { name: "close" }).click();
  await expect(page.getByRole("article", { name: "RLViz guide" })).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole("main", { name: "Browse trajectories" })).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole("article", { name: "RLViz guide" })).toHaveCount(0);
  await expect(page.getByRole("region", { name: "RLViz settings" })).toHaveCount(0);
  await page.keyboard.press("?");
  await expect(page.getByRole("article", { name: "RLViz guide" })).toBeVisible();
  await page.keyboard.press("?");
  await page.keyboard.press("Enter");
  await expect(page.getByRole("main", { name: "Read trajectory" })).toHaveCount(2);
  await expect(page.locator(".workspace-console .moment.selected .address")).not.toBeEmpty();

  expect(requests.some((request) => new URL(request.url).origin !== "http://127.0.0.1:4174")).toBe(false);
  expect(requests.some((request) => request.method !== "GET")).toBe(false);
  expect(requests.some((request) => request.url.includes("/api/") || request.body?.includes("coding-event-0000"))).toBe(false);
  expect(requests.map((request) => new URL(request.url).pathname)).toContain("/rlviz.wasm");
});

test("checkout browse summary surfaces the known failed event", async ({ page }) => {
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
  await expect(page.getByRole("main", { name: "Browse trajectories" })).toBeVisible({ timeout: 15_000 });
  const rollout = page.getByRole("option").filter({ hasText: "checkout-rollout-06" });
  await expect(rollout.getByText("1 failed", { exact: true })).toBeVisible();
});

test("bundled viewer starts inside an opaque-origin sandbox", async ({ page }) => {
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "dist");
  const vercel = JSON.parse(await readFile(path.join(root, "vercel.json"), "utf8")) as { headers: Array<{ headers: Array<{ key: string; value: string }> }> };
  const securityHeaders = Object.fromEntries(vercel.headers[0].headers.map(({ key, value }) => [key, value]));
  const contentTypes: Record<string, string> = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".wasm": "application/wasm", ".ndjson": "application/x-ndjson" };
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.route("**/*", async (route) => {
    const url = new URL(route.request().url());
    if (url.origin === "https://chatblocks.test") return route.fulfill({ contentType: "text/html", body: '<iframe title="Block" sandbox="allow-scripts allow-forms" src="https://block.test/"></iframe>' });
    if (url.origin === "https://block.test") return route.fulfill({ contentType: "text/html", body: '<iframe title="Sandboxed RLViz" src="https://rlviz.test/"></iframe>' });
    if (url.origin !== "https://rlviz.test") return route.abort();
    const relative = url.pathname === "/" ? "index.html" : url.pathname.slice(1);
    try { await route.fulfill({ body: await readFile(path.join(root, relative)), contentType: contentTypes[path.extname(relative)] ?? "application/octet-stream", headers: securityHeaders }); }
    catch { await route.fulfill({ status: 404, body: "not found" }); }
  });

  await page.goto("https://chatblocks.test/");
  const viewer = page.frameLocator('iframe[title="Block"]').frameLocator('iframe[title="Sandboxed RLViz"]');
  await expect(viewer.getByRole("main", { name: "Browse trajectories" })).toBeVisible({ timeout: 15_000 });
  await expect(viewer.getByRole("option").filter({ hasText: "checkout-rollout-01" })).toBeVisible();
  expect(errors.filter((message) => /localStorage|SecurityError|Failed to read/i.test(message))).toEqual([]);
});
