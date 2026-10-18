import { CODE_READING_SOURCE } from "./agentSessionSnapshots";
import { expect, test } from "./test";

for (const [theme, width, fontSize, accent] of [
  ["light", 1440, 14, ""],
  ["dark", 1120, 18, "&accent=%238b5cf6"],
] as const) {
  test(`long transcript code keeps a bounded, stable reading region and shares Dock wrap in ${theme}`, async ({
    page,
  }) => {
    let requested = false;
    let release!: () => void;
    const highlight = new Promise<void>((resolve) => (release = resolve));
    await page.route("**/node_modules/.vite/deps/shiki.js*", async (route) => {
      requested = true;
      await highlight;
      await route.continue();
    });
    await page.setViewportSize({ width, height: 900 });
    try {
      await page.goto(
        `/visual/?fixture=workspace&state=dock-trajectory&agent-state=code-reading&theme=${theme}&font-size=${fontSize}${accent}`,
      );
      await page.locator("html[data-visual-ready]").waitFor();
      const frame = page.locator('[data-markdown-copy="code-block"]');
      const source = frame.getByRole("region", { name: "Code content", exact: true });
      await expect.poll(() => requested).toBe(true);
      await expect(source).toHaveText(CODE_READING_SOURCE);
      await expect(source.locator("code .line")).toHaveCount(0);
      await source.evaluate((node) => node.scrollIntoView({ block: "center" }));
      await source.focus();
      const before = await source.evaluate((node) => ({
        width: node.clientWidth,
        height: node.clientHeight,
        contentHeight: node.scrollHeight,
        limit: Math.min(
          28 * parseFloat(getComputedStyle(document.documentElement).fontSize),
          innerHeight * 0.6,
        ),
      }));
      expect(before.contentHeight).toBeGreaterThan(before.height);
      expect(before.height).toBeLessThanOrEqual(before.limit);
      await expect(frame.getByRole("button", { name: "Copy code" })).toBeInViewport();
      release();
      await expect(source.locator("code .line")).toHaveCount(48);
      await expect(source).toBeFocused();
      const after = await source.evaluate((node) => ({
        width: node.clientWidth,
        height: node.clientHeight,
      }));
      expect(after).toEqual({ width: before.width, height: before.height });
      await expect(source).toHaveText(CODE_READING_SOURCE);

      const transcript = page.locator(".msg-scroll-viewport");
      const outerTop = await transcript.evaluate((node) => node.scrollTop);
      await page.keyboard.down("ArrowDown");
      try {
        await expect.poll(() => source.evaluate((node) => node.scrollTop)).toBeGreaterThan(0);
      } finally {
        await page.keyboard.up("ArrowDown");
      }
      expect(await transcript.evaluate((node) => node.scrollTop)).toBe(outerTop);

      const dock = page.locator('[data-dock-view-id="package:visual:trajectory"]');
      const record = dock.locator('[data-trajectory-record="item:item_code_reading"]');
      await record.getByRole("button").first().click();
      await record.getByRole("button", { name: "Recorded evidence", exact: true }).click();
      const evidence = record.getByRole("region", { name: "Recorded evidence", exact: true });
      await expect(evidence.locator("code .line").first()).toBeAttached();
      await frame.getByRole("button", { name: "Enable word wrap", exact: true }).click();
      await expect(source).toHaveAttribute("data-wrap", "true");
      await expect(evidence).toHaveAttribute("data-wrap", "true");
      await expect(frame.getByRole("button", { name: "Disable word wrap" })).toHaveAttribute(
        "aria-pressed",
        "true",
      );
      await expect(source).toHaveText(CODE_READING_SOURCE);
      expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
      const continuation = page.getByText(
        "The explanation continues with the same reading rhythm.",
        { exact: true },
      );
      await continuation.scrollIntoViewIfNeeded();
      await expect(continuation).toBeInViewport();
    } finally {
      release();
    }
  });
}
