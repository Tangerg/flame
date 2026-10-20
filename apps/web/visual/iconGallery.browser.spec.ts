import { expect, test, type Locator, type Page } from "./test";
async function openGallery(page: Page, theme: string, fontSize: number, width: number) {
  await page.setViewportSize({ width, height: 720 });
  const query = new URLSearchParams({
    fixture: "workspace",
    state: "full-view",
    "full-view": "package:visual:icon-gallery",
    "font-size": String(fontSize),
    theme,
  });
  await page.goto(`/visual/?${query}`);
  await page.locator("html[data-visual-ready]").waitFor();
  if (width === 320)
    await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
  return page.getByRole("region", { name: "Flame icon gallery", exact: true });
}

async function readTile(tile: Locator) {
  return tile.evaluate((node) => {
    const box = node.getBoundingClientRect();
    const style = getComputedStyle(node);
    const lines = (element: Element) => {
      const range = document.createRange();
      range.selectNodeContents(element);
      return [...range.getClientRects()].map((rect) => ({
        left: rect.left,
        right: rect.right,
        top: rect.top,
        bottom: rect.bottom,
      }));
    };
    const drawing = node.querySelector("svg")!.getBoundingClientRect();
    return {
      width: box.width,
      gridWidth: node.parentElement!.getBoundingClientRect().width,
      overflow: node.scrollWidth - node.clientWidth,
      contentLeft: box.left + parseFloat(style.borderLeftWidth) + parseFloat(style.paddingLeft),
      contentRight: box.right - parseFloat(style.borderRightWidth) - parseFloat(style.paddingRight),
      label: lines(node.querySelector("strong")!),
      codes: [...node.querySelectorAll("code")].map((element) => ({
        text: element.textContent,
        lines: lines(element),
      })),
      drawing: { width: drawing.width, height: drawing.height },
    };
  });
}

test("gallery adjusts column density with the chosen font scale", async ({ page }) => {
  const regular = await openGallery(page, "light", 14, 1120);
  const before = await readTile(
    regular.getByRole("article", { name: "Strikethrough", exact: true }),
  );
  const enlarged = await openGallery(page, "light", 18, 1120);
  const after = await readTile(
    enlarged.getByRole("article", { name: "Strikethrough", exact: true }),
  );
  expect(after.gridWidth).toBe(before.gridWidth);
  expect(after.width).toBeGreaterThan(before.width);
});

for (const theme of ["light", "dark"]) {
  for (const fontSize of [14, 18]) {
    test(`gallery names stay readable without widening filtered cards ${theme} ${fontSize}`, async ({
      page,
    }) => {
      const gallery = await openGallery(page, theme, fontSize, 1120);
      const tile = gallery.getByRole("article", { name: "Strikethrough", exact: true });
      await expect(tile).toBeVisible();
      const count = await gallery.getByRole("article").count();
      expect(count).toBeGreaterThan(1);
      const before = await readTile(tile);
      expect(before.label).toHaveLength(1);
      expect(before.codes.map((code) => code.lines.length)).toEqual([1, 1]);
      expect(before.codes.map((code) => code.text)).toEqual(["Strikethrough", "strikethrough"]);
      expect(before.drawing).toEqual({ width: 24, height: 24 });

      const search = gallery.getByRole("searchbox", { name: "Filter icons", exact: true });
      await search.fill("strikethrough");
      await expect(gallery.getByRole("article")).toHaveCount(1);
      const after = await readTile(tile);
      expect(Math.abs(after.width - before.width)).toBeLessThanOrEqual(1);
      expect(after.width).toBeLessThan(after.gridWidth / 2);
      expect(after.label).toHaveLength(1);
      expect(after.codes.map((code) => code.lines.length)).toEqual([1, 1]);
      expect(after.codes[0]!.lines[0]!.bottom).toBeLessThanOrEqual(after.codes[1]!.lines[0]!.top);
      expect(after.overflow).toBe(0);
      await expect(search).toBeFocused();
      await search.fill("");
      await expect(gallery.getByRole("article")).toHaveCount(count);
      expect(Math.abs((await readTile(tile)).width - before.width)).toBeLessThanOrEqual(1);
    });
  }

  test(`gallery preserves long identifiers in a narrow window ${theme}`, async ({ page }) => {
    const gallery = await openGallery(page, theme, 18, 320);
    const search = gallery.getByRole("searchbox", { name: "Filter icons", exact: true });
    for (const [label, component, name] of [
      ["Rectangle Horizontal", "RectangleHorizontal", "rectangle-horizontal"],
      ["arrow right from line", "ArrowRightFromLine", "arrow-right-from-line"],
    ] as const) {
      await search.fill(name);
      const tile = gallery.getByRole("article", { name: label, exact: true });
      await expect(tile).toBeVisible();
      const measured = await readTile(tile);
      expect(measured.codes.map((code) => code.text)).toEqual([component, name]);
      expect(measured.overflow).toBe(0);
      expect(measured.drawing).toEqual({ width: 24, height: 24 });
      for (const code of measured.codes) {
        expect(code.lines.length).toBeLessThanOrEqual(2);
        for (const line of code.lines) {
          expect(line.left).toBeGreaterThanOrEqual(measured.contentLeft - 1);
          expect(line.right).toBeLessThanOrEqual(measured.contentRight + 1);
        }
      }
      expect(measured.codes[0]!.lines.at(-1)!.bottom).toBeLessThanOrEqual(
        measured.codes[1]!.lines[0]!.top,
      );
    }
    expect(await gallery.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  });
}
