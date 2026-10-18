import { expect, test } from "./test";

for (const [theme, fontSize, width] of [
  ["light", 14, 1472],
  ["dark", 18, 800],
] as const) {
  test(`completed work opens directly into aligned activity rows in ${theme}`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(`/visual/?fixture=agent&state=waves&theme=${theme}&font-size=${fontSize}`);
    await page.locator("html[data-visual-ready]").waitFor();
    const summary = page.getByRole("button", { name: /3 steps/ });
    await summary.click();
    const wave = page
      .locator('[data-slot="agent-activity-disclosure"]')
      .filter({ has: summary })
      .first();
    await expect(wave.getByText("1 read · 1 search", { exact: true })).toHaveCount(1);
    await expect(wave.getByRole("button", { name: /calls/ })).toHaveCount(0);
    const thought = wave.getByRole("button", { name: "Thought", exact: true });
    const read = wave.locator('[data-tool="read"]');
    const search = wave.locator('[data-tool="grep"]');
    await expect(read).toBeVisible();
    await expect(search).toBeVisible();
    await page.evaluate(() => document.fonts.ready);
    const starts = await Promise.all([
      thought
        .locator('[data-slot="agent-activity-label"]')
        .evaluate((node) => node.getBoundingClientRect().left),
      read.getByText("Read", { exact: true }).evaluate((node) => node.getBoundingClientRect().left),
      search
        .getByText("Searched", { exact: true })
        .evaluate((node) => node.getBoundingClientRect().left),
    ]);
    expect(Math.max(...starts) - Math.min(...starts)).toBeLessThanOrEqual(1);

    await read.click();
    const result = wave.getByRole("region", { name: "Result", exact: true });
    await expect(result.locator("pre")).toHaveCount(6);
    await expect(result).toContainText("flushLocked");
    await expect(result).toBeInViewport();
    await summary.click();
    await expect(summary).toHaveAttribute("aria-expanded", "false");
    await expect(result).not.toBeInViewport();
    await result.focus();
    await expect(result).not.toBeFocused();
    await summary.click();
    await expect(read).toHaveAttribute("aria-expanded", "true");
    await expect(result.locator("pre")).toHaveCount(6);
    await expect(result).toContainText("flushLocked");
    await expect(result).toBeInViewport();
    expect(await wave.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  });
}
