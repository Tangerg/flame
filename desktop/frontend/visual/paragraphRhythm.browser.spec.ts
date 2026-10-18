import { expect, test } from "./test";

for (const [theme, fontSize, width] of [
  ["light", 14, 1472],
  ["dark", 18, 800],
] as const) {
  test(`Chinese paragraphs preserve semantic separation at ${fontSize}px in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      `/visual/?fixture=agent&state=long-content&theme=${theme}&font-size=${fontSize}`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    await page.evaluate(() => document.fonts.ready);
    const section = page.getByRole("heading", { name: "中文长回答", exact: true });
    await section.scrollIntoViewIfNeeded();
    const paragraphs = await section.evaluate((heading) => {
      const rows = [];
      for (
        let node = heading.nextElementSibling;
        node?.tagName === "P";
        node = node.nextElementSibling
      ) {
        const bounds = node.getBoundingClientRect();
        rows.push({
          text: node.textContent,
          direction: node.getAttribute("dir"),
          top: bounds.top,
          bottom: bounds.bottom,
          fontSize: Number.parseFloat(getComputedStyle(node).fontSize),
        });
      }
      return rows;
    });
    expect(paragraphs).toHaveLength(3);
    for (const paragraph of paragraphs) {
      expect(paragraph.direction).toBe("auto");
      expect(paragraph.fontSize).toBe(fontSize);
    }
    expect(paragraphs[1]!.text).toContain("runtime.Commit");
    for (let index = 1; index < paragraphs.length; index++) {
      expect(paragraphs[index]!.top - paragraphs[index - 1]!.bottom).toBe(fontSize);
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  });
}
