import { expect, test } from "@playwright/test";

function trajectoryResponse(count = 250) {
  return {
    trajectory: {
      id: "browser-long-run",
      name: "Browser navigation fixture",
      status: "completed",
      events: Array.from({ length: count }, (_, index) => ({
        id: `event-${index + 1}`,
        sequence: index + 1,
        kind: index === count - 1 ? "reward" : "generation",
        title: `Event ${index + 1}`,
        content: `Payload ${index + 1}`,
      })),
    },
  };
}

test.beforeEach(async ({ page }) => {
  await page.route("**/api/v1/trajectory**", async (route) => route.fulfill({ json: trajectoryResponse() }));
});

test("keyboard navigation is deterministic and manual rail scroll is retained", async ({ page }) => {
  await page.goto("/");
  const search = page.getByLabel("Search events");
  await expect(search).toBeVisible();

  await search.focus();
  await page.keyboard.type("j");
  await expect(search).toHaveValue("j");
  await expect(page.locator(".selected-heading h3")).toHaveText("Event 1");

  await search.fill("Event");
  await page.getByRole("heading", { name: "Browser navigation fixture" }).click();
  const rail = page.getByRole("navigation", { name: "Trajectory search results" });
  await rail.evaluate((element) => { element.scrollTop = 3000; element.dispatchEvent(new Event("scroll")); });
  await page.waitForTimeout(250);
  expect(await rail.evaluate((element) => element.scrollTop)).toBeGreaterThan(2500);

  for (let index = 0; index < 100; index += 1) await page.keyboard.press("j");
  await expect(page).toHaveURL(/event=event-101/);
  await expect(page.locator(".selected-heading h3")).toHaveText("Event 101");
});

test("deep-link reveal and Help focus return work", async ({ page }) => {
  await page.goto("/?event=event-120");
  await expect(page.locator(".selected-heading h3")).toHaveText("Event 120");
  await expect(page.locator("#event-event-120")).toBeInViewport();

  const help = page.getByRole("button", { name: "Keyboard shortcuts" });
  await help.click();
  await expect(page.getByRole("dialog", { name: "Keyboard shortcuts" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog", { name: "Keyboard shortcuts" })).toBeHidden();
  await expect(help).toBeFocused();
});
