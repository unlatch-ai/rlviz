import { expect, test } from "@playwright/test";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const browse = {
  sources: [{ id: "source", path: "/tmp/recovered.ndjson", index_state: "complete" }],
  trajectories: [{
    source_id: "source", source_name: "recovered.ndjson", case_name: "token recovery",
    trajectory: { id: "recovered", group_id: "group", status: "completed" },
    metrics: { trajectory: { id: "recovered", group_id: "group" }, event_count: 1, error_count: 0, pass: true, reward: 1 },
  }],
  count: 1,
};

test("a stale stored token recovers when rlviz supplies a fresh hash token", async ({ page }) => {
  const authorizations: Array<string | undefined> = [];
  await page.addInitScript(() => {
    if (!sessionStorage.getItem("rlviz.auth-test-bootstrapped")) {
      localStorage.setItem("rlviz.daemon-token", "stale");
      sessionStorage.setItem("rlviz.auth-test-bootstrapped", "1");
    }
  });
  await page.route("**/api/v1/indexed/browse", async (route) => {
    const authorization = await route.request().headerValue("authorization") ?? undefined;
    authorizations.push(authorization);
    if (authorization !== "Bearer fresh") {
      await route.fulfill({ status: 401, json: { code: "unauthorized" } });
      return;
    }
    await route.fulfill({ json: browse });
  });
  await page.route("**/api/v1/trajectory**", (route) => route.fulfill({ json: {
    trajectory: { id: "recovered", group_id: "group", status: "completed" },
    events: [{ id: "done", sequence: 0, kind: "grader", output: { verdict: "pass" } }],
  } }));

  const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
  const contentTypes: Record<string, string> = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".map": "application/json" };
  await page.route("http://127.0.0.1:4173/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname.startsWith("/api/")) return route.fallback();
    const rel = url.pathname === "/" ? "index.html" : url.pathname.replace(/^\//, "");
    try {
      await route.fulfill({ body: await readFile(path.join(webRoot, "dist", rel)), contentType: contentTypes[path.extname(rel)] ?? "application/octet-stream" });
    } catch {
      await route.fulfill({ status: 404, body: "not found" });
    }
  });

  await page.goto("http://127.0.0.1:4173/", { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("alert")).toContainText("rlviz open");
  expect(authorizations).toEqual(["Bearer stale", undefined]);

  await page.goto("http://127.0.0.1:4173/#token=fresh", { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("main", { name: "Browse trajectories" })).toContainText("recovered");
  expect(authorizations).toEqual(["Bearer stale", undefined, "Bearer fresh"]);
  await expect.poll(() => page.evaluate(() => localStorage.getItem("rlviz.daemon-token"))).toBe("fresh");
});
