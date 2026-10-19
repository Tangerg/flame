import { expect, test } from "./test";

for (const [width, fontSize, theme] of [
  [1472, 14, "light"],
  [320, 18, "dark"],
] as const) {
  test(`a cold Markdown reader keeps source syntax out of the loading surface ${width}`, async ({
    page,
  }) => {
    let release!: () => void;
    let requested!: () => void;
    const rendererReady = new Promise<void>((resolve) => {
      release = resolve;
    });
    const rendererRequested = new Promise<void>((resolve) => {
      requested = resolve;
    });
    await page.route(/\/MarkdownRenderer\.tsx(?:\?.*)?$/, async (route) => {
      requested();
      await rendererReady;
      await route.continue();
    });
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      `/visual/?fixture=workspace&state=dock-light&agent-state=prose-blocks&theme=${theme}&font-size=${fontSize}&locale=en`,
      { waitUntil: "commit" },
    );
    await rendererRequested;
    if (width === 320) {
      await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
    }
    const answer = page.getByRole("heading", { name: "Assistant", exact: true }).locator("..");
    const pending = answer.locator("[data-surface-pending]");
    try {
      await expect(pending).toBeVisible();
      const screenshot = test.info().outputPath("markdown-loading.png");
      await page.screenshot({ path: screenshot });
      await test.info().attach("markdown-loading", { path: screenshot });
      await expect(pending).not.toContainText("###");
      await expect(pending).not.toContainText("[protocol]");
      await expect(pending).toContainText("Loading");
      await expect(pending.locator('[aria-busy="true"]')).toBeVisible();
      await expect(answer.getByRole("button", { name: "Copy message", exact: true })).toHaveCount(
        0,
      );
      expect((await pending.boundingBox())!.height).toBeLessThan(fontSize * 6);
      expect(await pending.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
      const userBubble = (await page.locator("[data-user-message-bubble]").boundingBox())!;
      expect(userBubble.width).toBeGreaterThan(fontSize * 8);
      expect(userBubble.height).toBeLessThan(fontSize * 6);
    } finally {
      release();
      await page.locator("html[data-visual-ready]").waitFor();
    }
    await expect(pending).toHaveCount(0);
    await expect(
      answer.getByRole("heading", { name: "Quoted evidence", exact: true }),
    ).toBeVisible();
    await expect(answer.locator("blockquote > p").first()).toHaveText(
      "The Runtime owns the durable facts and their transitions.",
    );
    await expect(
      answer.getByRole("link", { name: "the protocol guide", exact: true }),
    ).toHaveAttribute("href", "https://example.test/runtime/protocol");
    await expect(answer.getByRole("button", { name: "Copy message", exact: true })).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  });
}
