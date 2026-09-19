import { expect, test } from "./test";

test("the composer preserves the model and lets secondary labels yield without measurement", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/visual/?fixture=agent&state=idle&theme=light");
  await page.locator("html[data-visual-ready]").waitFor();
  const composer = page.locator('[data-slot="composer-root"]');
  const model = page.getByRole("button", { name: "Switch model" });
  const approval = page.getByRole("button", { name: "Approval mode" });
  await expect(model.locator('[data-slot="composer-chip-label"]')).toBeVisible();
  await expect(approval.locator('[data-slot="composer-chip-label"]')).toBeVisible();
  await composer.evaluate((node) => {
    node.style.width = "320px";
  });
  await expect(model.locator('[data-slot="composer-chip-label"]')).toBeVisible();
  await expect(approval.locator('[data-slot="composer-chip-label"]')).toBeHidden();
  await model.locator('[data-slot="composer-chip-label"]').evaluate((node) => {
    node.textContent = "A much longer model label changed without resizing the window";
  });
  expect(await composer.evaluate((node) => node.scrollWidth - node.clientWidth)).toBeLessThan(2);
  await model.click();
  await expect(page.getByRole("button", { name: "Switch reasoning effort" })).toBeVisible();
});

test("high-risk approval remains explicit in a narrow composer", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&state=idle&theme=light");
  await page.locator("html[data-visual-ready]").waitFor();
  const approval = page.getByRole("button", { name: "Approval mode" });
  await approval.click();
  await page.getByRole("menuitem", { name: "Auto Run everything without asking." }).click();
  const composer = page.locator('[data-slot="composer-root"]');
  await composer.evaluate((node) => {
    node.style.width = "280px";
  });
  await expect(approval.locator('[data-slot="composer-chip-label"]')).toBeVisible();
  await expect(approval).toHaveText("Auto");
  expect(await composer.evaluate((node) => node.scrollWidth - node.clientWidth)).toBeLessThan(2);
});

test("navigation density does not rescale reading or input geometry", async ({ page }) => {
  const measures = [];
  for (const density of ["compact", "spacious"]) {
    await page.goto(`/visual/?fixture=agent&state=idle&theme=light&density=${density}`);
    await page.locator("html[data-visual-ready]").waitFor();
    measures.push(
      await page.evaluate(() => {
        const root = getComputedStyle(document.documentElement);
        const input = document.querySelector('[data-slot="composer-root"]')!;
        return {
          row: root.getPropertyValue("--density-row-height"),
          reading: root.getPropertyValue("--reading-gutter-wide"),
          input: input.getBoundingClientRect().height,
        };
      }),
    );
  }
  expect(measures[0]!.row).not.toBe(measures[1]!.row);
  expect(measures[0]!.reading).toBe(measures[1]!.reading);
  expect(measures[0]!.input).toBe(measures[1]!.input);
});
