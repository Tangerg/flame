import { expect, test } from "./test";

const REFERENCE =
  "desktop/frontend/src/plugins/builtin/chat/message/ui/markdown/markdownComponents.tsx:12:3";

for (const [width, fontSize, theme] of [
  [1472, 14, "light"],
  [800, 18, "dark"],
] as const) {
  test(`file references read as part of their sentence at ${width}px in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 1000 });
    await page.goto(
      `/visual/?fixture=agent&state=prose-blocks&theme=${theme}&font-size=${fontSize}`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    const reading = page.locator(".md").filter({ hasText: "Source references" });
    const reference = reading.getByRole("button", { name: REFERENCE, exact: true });
    await reference.scrollIntoViewIfNeeded();
    await page.evaluate(() => document.fonts.ready);

    const flow = await reference.evaluate((node) => {
      const paragraph = node.parentElement!;
      const prefix = document.createRange();
      prefix.selectNodeContents(paragraph);
      prefix.setEndBefore(node);
      const before = [...prefix.getClientRects()].at(-1)!;
      const fragments = [...node.getClientRects()];
      const bounds = paragraph.getBoundingClientRect();
      const selection = window.getSelection()!;
      const range = document.createRange();
      range.selectNodeContents(paragraph);
      selection.removeAllRanges();
      selection.addRange(range);
      const selected = selection.toString();
      selection.removeAllRanges();
      return {
        firstLineOffset: Math.abs(fragments[0]!.top - before.top),
        hasWrapped: fragments.some((rect) => rect.top > fragments[0]!.top + 1),
        fits: fragments.every(
          (rect) => rect.left >= bounds.left - 1 && rect.right <= bounds.right + 1,
        ),
        selected,
        text: paragraph.textContent,
        overflow: paragraph.scrollWidth - paragraph.clientWidth,
      };
    });
    expect(flow.firstLineOffset).toBeLessThan(fontSize / 2);
    expect(flow.hasWrapped).toBe(true);
    expect(flow.fits).toBe(true);
    expect(flow.overflow).toBe(0);
    expect(flow.selected).toBe(flow.text);
    expect(flow.selected).toContain(`${REFERENCE}. Continue reading`);
    await expect(
      reading.getByRole("button", { name: "src/main.ts:12:3", exact: true }),
    ).toBeVisible();
    await expect(
      reading.getByRole("button", { name: "desktop/frontend/package.json", exact: true }),
    ).toHaveText("desktop/frontend/package.json");
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  });
}

test("inline references retain keyboard navigation to the recorded file and line", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1472, height: 1000 });
  await page.goto(
    "/visual/?fixture=workspace&state=dock-files&agent-state=prose-blocks&theme=light",
  );
  await page.locator("html[data-visual-ready]").waitFor();
  const reading = page.locator(".md").filter({ hasText: "Source references" });
  const reference = reading.getByRole("button", { name: "README.md:15:2", exact: true });
  await reference.focus();
  await expect(reference).toBeFocused();
  await reference.press("Enter");
  const view = page.locator('[data-dock-view-id="file"]');
  await expect(view.getByRole("tab", { name: "Source", exact: true })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  const source = view.locator('[data-quote-source="file"][data-quote-path="README.md"]');
  await expect(source.locator('[data-quote-line="15"]')).toBeVisible();
  const next = reading.getByRole("button", {
    name: "desktop/frontend/package.json",
    exact: true,
  });
  await next.focus();
  await next.press("Space");
  await expect(
    view.locator('[data-quote-source="file"][data-quote-path="desktop/frontend/package.json"]'),
  ).toBeVisible();
});
