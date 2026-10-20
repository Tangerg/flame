import { expect, test } from "./test";
for (const [theme, fontSize, width] of [
  ["light", 14, 1472],
  ["dark", 18, 1120],
] as const) {
  test(`Memory keeps secondary filters discoverable without occupying the reading view in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      `/visual/?fixture=workspace&state=dock-agent-memory&theme=${theme}&font-size=${fontSize}`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    const panel = page.getByRole("region", { name: "Agent memory", exact: true });
    const search = panel.getByRole("searchbox", { name: "Search this page", exact: true });
    const filters = panel.getByRole("button", { name: /^Filters\b/ });
    const status = panel.getByRole("combobox", { name: "Memory status", exact: true });
    const origin = panel.getByRole("combobox", { name: "Memory origin", exact: true });
    const pending = panel.getByText("Prefers the diff read worst-risk-first.", { exact: true });
    const active = panel.getByText(
      "The compaction cutpoint is chosen by the Runtime, never by the client.",
      { exact: true },
    );
    await expect(search).toBeVisible();
    await expect(filters).toHaveAttribute("aria-expanded", "false");
    await expect(status).toHaveCount(0);
    await expect(origin).toHaveCount(0);
    await expect(pending).toBeVisible();
    await expect(active).toBeVisible();
    await filters.press("Enter");
    await expect(filters).toHaveAttribute("aria-expanded", "true");
    await expect(status).toBeVisible();
    await expect(origin).toBeVisible();
    await expect(panel.getByRole("checkbox", { name: "Pinned only", exact: true })).toBeVisible();
    await status.click();
    await page.getByRole("option", { name: "Pending review", exact: true }).click();
    await expect(pending).toBeVisible();
    await expect(active).toHaveCount(0);
    await filters.click();
    await expect(filters).toHaveAttribute("aria-expanded", "false");
    await expect(filters).toContainText("1");
    const clear = panel.getByRole("button", { name: "Clear filters", exact: true });
    await expect(clear).toBeVisible();
    await clear.click();
    await expect(filters).toHaveAttribute("aria-expanded", "false");
    await expect(pending).toBeVisible();
    await expect(active).toBeVisible();
    await expect(clear).toHaveCount(0);
    await search.fill("compaction");
    await expect(active).toBeVisible();
    await expect(pending).toHaveCount(0);
    await expect(panel.getByRole("button", { name: "Add memory", exact: true })).toBeEnabled();
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  });
}
