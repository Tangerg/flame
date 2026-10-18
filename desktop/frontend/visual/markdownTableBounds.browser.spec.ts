import { expect, test, type Locator } from "./test";

async function bounds(container: Locator) {
  return container.evaluate((node) => {
    const box = (element: Element) => {
      const { left, right, top, bottom, width, height } = element.getBoundingClientRect();
      return { left, right, top, bottom, width, height };
    };
    const markdown = node.closest(".md")!;
    const scroller = node.querySelector<HTMLElement>(".md-table-scroller")!;
    const table = node.querySelector("table")!;
    const actions = node.querySelector(".md-table-actions")!;
    const buttons = Array.from(actions.querySelectorAll("button"));
    const clip = box(scroller);
    const overlaps: { action: string | null; text: string }[] = [];
    const textNodes = document.createTreeWalker(table, NodeFilter.SHOW_TEXT);
    for (let text = textNodes.nextNode(); text; text = textNodes.nextNode()) {
      if (!text.textContent?.trim()) continue;
      const range = document.createRange();
      range.selectNodeContents(text);
      for (const rect of range.getClientRects()) {
        const left = Math.max(rect.left, clip.left);
        const right = Math.min(rect.right, clip.right);
        const top = Math.max(rect.top, clip.top);
        const bottom = Math.min(rect.bottom, clip.bottom);
        if (right <= left || bottom <= top) continue;
        for (const button of buttons) {
          const action = box(button);
          if (
            left < action.right &&
            right > action.left &&
            top < action.bottom &&
            bottom > action.top
          ) {
            overlaps.push({ action: button.getAttribute("aria-label"), text: text.textContent });
          }
        }
      }
    }
    return {
      markdown: box(markdown),
      container: box(node),
      scroller: clip,
      table: box(table),
      actions: box(actions),
      buttons: buttons.map(box),
      scrollWidth: scroller.scrollWidth,
      clientWidth: scroller.clientWidth,
      overflowX: getComputedStyle(scroller).overflowX,
      overlaps,
    };
  });
}

for (const [width, fontSize, theme] of [
  [1472, 14, "light"],
  [320, 18, "dark"],
] as const) {
  test(`Markdown table and revealed actions stay inside the answer measure ${width} ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      `/visual/?fixture=agent&state=long-content&theme=${theme}&font-size=${fontSize}&sidebar=collapsed`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    await page.evaluate(() => document.fonts.ready);
    const table = page.locator("[data-markdown-table]").filter({ hasText: "Boundary" });
    await table.evaluate((node) => node.scrollIntoView({ block: "center" }));
    await table.hover();
    const actions = table.locator(".md-table-actions");
    await expect(actions).toHaveCSS("opacity", "1");
    const readings = [{ state: "hover", ...(await bounds(table)) }];
    const copy = table.getByRole("button", { name: "Copy table", exact: true });
    await copy.focus();
    await page.mouse.move(0, 0);
    await expect(copy).toBeFocused();
    await expect(actions).toHaveCSS("opacity", "1");
    readings.push({ state: "focus", ...(await bounds(table)) });
    await test.info().attach("markdown-table-bounds", {
      body: JSON.stringify(readings, null, 2),
      contentType: "application/json",
    });
    for (const reading of readings) {
      for (const [name, box] of Object.entries({
        container: reading.container,
        scroller: reading.scroller,
        actions: reading.actions,
        ...Object.fromEntries(reading.buttons.map((button, index) => [`action ${index}`, button])),
      })) {
        expect
          .soft(box.left, `${reading.state} ${name} left`)
          .toBeGreaterThanOrEqual(reading.markdown.left);
        expect
          .soft(box.right, `${reading.state} ${name} right`)
          .toBeLessThanOrEqual(reading.markdown.right);
      }
      if (reading.scrollWidth <= reading.clientWidth) {
        expect.soft(reading.table.left).toBeGreaterThanOrEqual(reading.markdown.left);
        expect.soft(reading.table.right).toBeLessThanOrEqual(reading.markdown.right);
      } else {
        expect(reading.overflowX).toBe("auto");
      }
      expect.soft(reading.overlaps, `${reading.state} actions cover table text`).toEqual([]);
    }
  });
}
