import { expect, test, type Locator } from "./test";

async function expectBlockRhythm(root: Locator) {
  const reading = await root.evaluate((node) => {
    const quotes = [...node.querySelectorAll("blockquote")];
    const startsWith = (element: Element, text: string) =>
      element.querySelector(":scope > p")?.textContent?.startsWith(text);
    const evidence = quotes.find((quote) => startsWith(quote, "The Runtime owns"));
    const nextSteps = quotes.find((quote) => startsWith(quote, "Before publishing"));
    const nested = quotes.find((quote) => startsWith(quote, "中文引用"));
    const surrounding = quotes.find((quote) => startsWith(quote, "Preserve the original"));
    const looseItem = [...node.querySelectorAll("li")].find((item) =>
      startsWith(item, "Capture the result"),
    );
    const task = node
      .querySelector('[role="checkbox"][aria-label="Preserve the accepted result"]')
      ?.closest("li");
    if (!evidence || !nextSteps || !nested || !surrounding || !looseItem || !task) return null;
    const flow = (container: Element) => {
      const blocks = [...container.children].filter((child) => !child.matches('[role="checkbox"]'));
      return {
        tags: blocks.map((child) => child.tagName),
        gaps: blocks.slice(1).map((child, index) => {
          const previous = blocks[index]!.getBoundingClientRect();
          return child.getBoundingClientRect().top - previous.bottom;
        }),
      };
    };
    const taskParagraphs = task.querySelectorAll(":scope > p");
    return {
      fontSize: Number.parseFloat(getComputedStyle(node).fontSize),
      evidence: flow(evidence),
      nextSteps: flow(nextSteps),
      nested: flow(nested),
      surrounding: flow(surrounding),
      looseItem: flow(looseItem),
      task: flow(task),
      taskBodyAlignment: Math.abs(
        taskParagraphs[0]!.getBoundingClientRect().left -
          taskParagraphs[1]!.getBoundingClientRect().left,
      ),
    };
  });
  expect(reading).not.toBeNull();
  const fontSize = reading!.fontSize;
  expect(reading!.evidence).toEqual({ tags: ["P", "P"], gaps: [fontSize / 2] });
  expect(reading!.nextSteps).toEqual({
    tags: ["P", "UL", "P"],
    gaps: [fontSize / 2, fontSize / 2],
  });
  expect(reading!.nested).toEqual({ tags: ["P", "P"], gaps: [fontSize / 2] });
  expect(reading!.surrounding).toEqual({
    tags: ["P", "BLOCKQUOTE", "P"],
    gaps: [fontSize, fontSize],
  });
  expect(reading!.looseItem).toEqual({
    tags: ["P", "UL", "P"],
    gaps: [fontSize / 2, fontSize / 2],
  });
  expect(reading!.task).toEqual({ tags: ["P", "P"], gaps: [fontSize] });
  expect(reading!.taskBodyAlignment).toBeLessThanOrEqual(1);
}

for (const [fontSize, theme, narrow] of [
  [14, "light", false],
  [18, "dark", true],
] as const) {
  test(`nested prose preserves paragraph boundaries at ${fontSize}px in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1472, height: 900 });
    await page.goto(
      `/visual/?fixture=workspace&state=dock-files&theme=${theme}&font-size=${fontSize}`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    await page.getByRole("treeitem", { name: "reading-blocks.md", exact: true }).click();
    if (narrow) {
      const resizer = page.locator(".agent-pane-resizer");
      await resizer.focus();
      await resizer.press("Home");
    }
    await expectBlockRhythm(page.locator('[data-slot="document-body"]'));

    await page.setViewportSize({ width: narrow ? 320 : 1472, height: 900 });
    await page.goto(
      `/visual/?fixture=agent&state=prose-blocks&theme=${theme}&font-size=${fontSize}`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    await expectBlockRhythm(page.locator(".md").filter({ hasText: "Quoted evidence" }));
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  });
}
