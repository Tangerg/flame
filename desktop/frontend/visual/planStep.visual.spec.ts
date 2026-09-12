import { expect, test } from "./test";

const ROUTE = "/visual/?fixture=workspace&state=dock-light&theme=light";

const LONG =
  "Verify that every boundary owner keeps its invariant when the run is resumed after a " +
  "restart, and that the projection never advances the fact on its own account";

test("a plan step that wraps keeps its mark on the first line", async ({ page }) => {
  await page.setViewportSize({ width: 1472, height: 900 });
  await page.goto(ROUTE);
  await page.locator("html[data-visual-ready]").waitFor();
  await page.waitForTimeout(300);

  const measured = await page.evaluate((long) => {
    const rows: HTMLElement[] = [];
    for (const row of document.querySelectorAll("div")) {
      if (row.closest("[data-fixture-chrome]")) continue;
      const kids = [...row.children];
      if (kids.length !== 2) continue;
      const [mark, label] = kids;
      if (!mark || !label) continue;
      if (mark.tagName !== "DIV" || label.tagName !== "SPAN") continue;
      if (getComputedStyle(mark).display !== "grid") continue;
      if (getComputedStyle(row).display !== "flex") continue;
      if (Math.abs(mark.getBoundingClientRect().width - 16) > 1) continue;
      rows.push(row as HTMLElement);
    }

    const read = (row: HTMLElement) => {
      const mark = row.children[0] as HTMLElement;
      const label = row.children[1] as HTMLElement;
      const m = mark.getBoundingClientRect();
      const l = label.getBoundingClientRect();
      const leading = Number.parseFloat(getComputedStyle(label).lineHeight);
      return {
        lines: Math.max(1, Math.round(l.height / leading)),
        drift: Math.round((m.top + m.height / 2 - (l.top + leading / 2)) * 10) / 10,
      };
    };

    const found = rows.length;
    const short = rows.map(read);
    for (const row of rows) (row.children[1] as HTMLElement).textContent = long;
    const wrapped = rows.map(read);
    return { found, short, wrapped };
  }, LONG);

  expect(measured.found, "the plan pane has to render steps").toBeGreaterThan(1);
  expect(
    measured.wrapped.filter((one) => one.lines < 2).length,
    "the long label has to wrap, or this measured the easy case twice",
  ).toBe(0);

  expect(
    measured.short.filter((one) => Math.abs(one.drift) > 1),
    "one-line steps whose mark left the line",
  ).toEqual([]);
  expect(
    measured.wrapped.filter((one) => Math.abs(one.drift) > 1),
    "wrapped steps whose mark drifted off the first line",
  ).toEqual([]);
});
