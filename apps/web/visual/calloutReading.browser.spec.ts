import { expect, test } from "./test";
test("callout titles keep their icons on the same line and inherit the theme color", async ({
  page,
}) => {
  await page.setViewportSize({ width: 800, height: 1000 });
  await page.goto("/visual/?fixture=agent&state=callouts&theme=dark&font-size=18");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.evaluate(() => document.fonts.ready);
  const markdown = page.locator(".md").filter({ hasText: "Notes that belong to the answer" });
  const alerts = markdown.locator('[data-slot="callout"]');
  await expect(alerts).toHaveCount(2);
  for (const alert of await alerts.all()) {
    const title = alert.locator('[data-slot="callout-title"]');
    await expect(title).toBeVisible();
    const layout = await title.evaluate((node) => {
      const icon = node.querySelector("svg")!;
      const text = [...node.childNodes].find((child) => child.nodeType === Node.TEXT_NODE)!;
      const range = document.createRange();
      range.selectNodeContents(text);
      const iconBox = icon.getBoundingClientRect();
      const textBox = range.getBoundingClientRect();
      return {
        iconRight: iconBox.right,
        iconTop: iconBox.top,
        iconBottom: iconBox.bottom,
        textLeft: textBox.left,
        textTop: textBox.top,
        textBottom: textBox.bottom,
        color: getComputedStyle(node).color,
        stroke: getComputedStyle(icon).stroke,
        bodyColor: getComputedStyle(node.nextElementSibling!).color,
      };
    });
    expect(layout.textLeft - layout.iconRight).toBeGreaterThanOrEqual(4);
    expect(layout.textLeft - layout.iconRight).toBeLessThanOrEqual(12);
    expect(layout.iconBottom).toBeGreaterThan(layout.textTop);
    expect(layout.textBottom).toBeGreaterThan(layout.iconTop);
    expect(layout.stroke).toBe(layout.color);
    expect(layout.color).not.toBe(layout.bodyColor);
  }
  await expect(markdown.locator("blockquote > p")).toHaveText(
    "A plain quotation remains an ordinary part of the review.",
  );
  expect(await markdown.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
});
