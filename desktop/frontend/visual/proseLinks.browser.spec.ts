import { expect, test } from "./test";

for (const [fontSize, theme, width] of [
  [14, "light", 1472],
  [18, "dark", 800],
] as const) {
  test(`captured links keep readable destinations in ${theme}`, async ({ page }) => {
    await page.setViewportSize({ width: 1120, height: 900 });
    await page.goto(
      `/visual/?fixture=workspace&state=dock-files&agent-state=prose-blocks&theme=${theme}&font-size=${fontSize}`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    await page.getByRole("treeitem", { name: "reading-blocks.md", exact: true }).click();
    const view = page.locator('[data-dock-view-id="file"]');
    const captured = view.locator('[data-slot="document-body"]');
    await expect(
      captured.getByRole("heading", { name: "Reference material", exact: true }),
    ).toBeVisible();
    const release = captured.locator("p").filter({ hasText: "The recorded release" });
    expect(
      (await release.textContent())?.match(/https:\/\/example\.test\/runtime\/releases/g),
    ).toHaveLength(1);
    const annotation = captured.locator('span[title="Accepted evidence and follow-up"]');
    await expect(annotation.locator("strong")).toHaveText("Review");
    await expect(annotation.locator("code")).toHaveText("guide");
    await expect(annotation).toContainText("https://example.test/runtime/review");
    await expect(captured.locator("a, [href], [onclick]")).toHaveCount(0);

    await view.getByRole("tab", { name: "Source", exact: true }).click();
    const source = view.locator('[data-quote-source="file"]');
    await expect(source).toContainText(
      '[protocol]: https://example.test/runtime/protocol "Accepted protocol contract"',
    );
    await expect(source).toContainText(
      "[implementation notes]: https://example.test/runtime/implementation",
    );
    await expect(source).toContainText("[unavailable source](javascript:alert%281%29)");
  });

  test(`answer links preserve their reading context in ${theme}`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      `/visual/?fixture=agent&state=prose-blocks&theme=${theme}&font-size=${fontSize}`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    const reading = page.locator(".md").filter({ hasText: "Quoted evidence" });
    const protocol = reading.getByRole("link", { name: "the protocol guide", exact: true });
    await expect(protocol).toHaveAttribute("href", "https://example.test/runtime/protocol");
    await expect(protocol).toHaveAttribute("title", "Accepted protocol contract");
    await expect(protocol).toHaveAttribute("target", "_blank");
    await expect(
      reading.getByRole("link", { name: "implementation notes", exact: true }),
    ).toHaveAttribute("href", "https://example.test/runtime/implementation");
    const unavailable = reading.locator("a").filter({ hasText: "unavailable source" });
    await expect(unavailable).not.toHaveAttribute("href");
    await unavailable.hover();
    await expect(unavailable).toHaveCSS("text-decoration-line", "none");
    expect(await unavailable.evaluate((node) => getComputedStyle(node).color)).toBe(
      await unavailable.locator("..").evaluate((node) => getComputedStyle(node).color),
    );
    expect(await reading.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  });
}
