import { expect, test, type Locator } from "./test";

async function formulaGeometry(formula: Locator) {
  return formula.evaluate((node) => {
    const viewport = node.getBoundingClientRect();
    const bases = [...node.querySelectorAll<HTMLElement>(".katex-html .katex-base")].filter(
      (base) => !base.closest(".katex-tag"),
    );
    const first = bases[0]!.getBoundingClientRect();
    const last = bases.at(-1)!.getBoundingClientRect();
    const tag = node.querySelector(".katex-html .katex-tag")?.getBoundingClientRect();
    return {
      left: viewport.left,
      right: viewport.right,
      width: viewport.width,
      firstLeft: first.left,
      lastRight: last.right,
      tagLeft: tag?.left,
      tagRight: tag?.right,
      scrollLeft: node.scrollLeft,
      horizontalOverflow: node.scrollWidth - node.clientWidth,
      verticalOverflow: node.scrollHeight - node.clientHeight,
    };
  });
}

test("wide formulas stay in their reading region with keyboard access to both ends", async ({
  page,
}) => {
  await page.setViewportSize({ width: 320, height: 900 });
  await page.goto("/visual/?fixture=agent&state=math-content&theme=dark&font-size=18");
  await page.locator("html[data-visual-ready]").waitFor();
  const markdown = page.locator(".md").filter({ hasText: "A readable derivation" });
  const formulas = markdown.locator(".katex-display");
  await expect(formulas).toHaveCount(4);
  await expect(markdown.locator(".katex-error")).toHaveCount(0);
  await expect(formulas.first().locator(".katex")).toHaveCSS("font-family", /KaTeX/);
  await page.evaluate(() => document.fonts.ready);

  const short = formulas.nth(0);
  const objective = formulas.nth(1);
  const matrix = formulas.nth(2);
  const tall = formulas.nth(3);
  await expect(short).not.toHaveAttribute("tabindex");
  await expect(tall).not.toHaveAttribute("tabindex");
  for (const formula of [objective, matrix]) {
    await expect(formula).toHaveAttribute("tabindex", "0");
    await expect(formula).toHaveAttribute("role", "region");
    await expect(formula).toHaveAttribute("aria-label", "Mathematical expression");
    await expect(formula).toHaveAttribute("dir", "ltr");
    await expect(formula).toHaveCSS("overflow-x", "auto");
  }

  await objective.scrollIntoViewIfNeeded();
  const beginning = await formulaGeometry(objective);
  expect(beginning.horizontalOverflow).toBeGreaterThan(0);
  expect(beginning.firstLeft).toBeGreaterThanOrEqual(beginning.left - 1);
  expect(beginning.tagLeft!).toBeGreaterThanOrEqual(beginning.lastRight);
  await objective.focus();
  await expect(objective).toBeFocused();
  await page.keyboard.down("ArrowRight");
  try {
    await expect.poll(() => objective.evaluate((node) => node.scrollLeft)).toBeGreaterThan(0);
  } finally {
    await page.keyboard.up("ArrowRight");
  }
  await objective.evaluate((node) => {
    node.scrollLeft = node.scrollWidth;
  });
  const ending = await formulaGeometry(objective);
  expect(ending.tagLeft!).toBeGreaterThanOrEqual(ending.left - 1);
  expect(ending.tagRight!).toBeLessThanOrEqual(ending.right + 1);
  await matrix.evaluate((node) => {
    node.scrollLeft = node.scrollWidth;
  });
  const matrixEnd = await formulaGeometry(matrix);
  expect(matrixEnd.scrollLeft).toBeGreaterThan(0);
  expect(matrixEnd.lastRight).toBeLessThanOrEqual(matrixEnd.right + 1);

  for (const formula of await formulas.all()) {
    const size = await formulaGeometry(formula);
    expect(size.verticalOverflow).toBeLessThanOrEqual(1);
    const reading = (await markdown.boundingBox())!;
    expect(size.left).toBeGreaterThanOrEqual(reading.x - 1);
    expect(size.right).toBeLessThanOrEqual(reading.x + reading.width + 1);
  }
  expect(await markdown.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  await expect(markdown.locator("p .katex")).toHaveCount(1);
  await expect(objective.locator('annotation[encoding="application/x-tex"]')).toContainText(
    "\\tag{1}",
  );

  await page.setViewportSize({ width: 1472, height: 900 });
  await expect(matrix).not.toHaveAttribute("tabindex");
  const centered = await formulaGeometry(short);
  expect(
    Math.abs((centered.firstLeft + centered.lastRight) / 2 - (centered.left + centered.right) / 2),
  ).toBeLessThanOrEqual(1);
});
