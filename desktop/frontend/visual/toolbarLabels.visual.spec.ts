import { expect, test } from "./test";

const ROUTE = "/visual/?fixture=agent&state=idle&theme=light";

const SQUEEZED_PX = 340;

test("a toolbar with no room drops its labels rather than ellipsing them", async ({ page }) => {
  await page.setViewportSize({ width: 1120, height: 800 });
  await page.goto(ROUTE);
  await page.locator("html[data-visual-ready]").waitFor();
  await page.locator(".agent-composer-footer").waitFor();
  await page.waitForTimeout(400);

  const read = () =>
    page.evaluate(() => {
      const footer = document.querySelector(".agent-composer-footer") as HTMLElement | null;
      if (!footer) return null;
      const labels = [...footer.querySelectorAll('[data-slot="composer-chip-label"]')];
      footer.dataset.measuring = "";
      const natural = footer.scrollWidth;
      delete footer.dataset.measuring;
      return {
        labelled: footer.hasAttribute("data-labelled"),
        shown: labels.filter((label) => (label as HTMLElement).offsetParent !== null).length,
        natural,
        available: footer.clientWidth,
        overflow: footer.scrollWidth - footer.clientWidth,
        ellipsed: [...footer.querySelectorAll("*")]
          .filter((node) => {
            const style = getComputedStyle(node);
            return node.scrollWidth - node.clientWidth > 1 && style.textOverflow === "ellipsis";
          })
          .map((node) => (node.textContent ?? "").trim().slice(0, 24)),
      };
    });

  const roomy = await read();
  expect(roomy, "the composer has to render its toolbar").not.toBeNull();
  expect(roomy!.shown, "the toolbar has to have labels to give up").toBeGreaterThan(1);
  expect(roomy!.labelled).toBe(true);
  expect(roomy!.ellipsed, "nothing should be ellipsed while there is room").toEqual([]);

  await page.evaluate((width) => {
    const footer = document.querySelector(".agent-composer-footer") as HTMLElement | null;
    if (footer) footer.style.maxWidth = `${width}px`;
  }, SQUEEZED_PX);
  await page.waitForTimeout(400);

  const squeezed = await read();
  expect(
    squeezed!.natural,
    `the labelled row measured ${squeezed!.natural}px inside ${SQUEEZED_PX}px — either the ` +
      `squeeze is too wide, or the chips are shrinking while being measured`,
  ).toBeGreaterThan(SQUEEZED_PX);

  expect(squeezed!.labelled, "the toolbar kept claiming it had room").toBe(false);
  expect(squeezed!.shown, "labels that stayed to be ellipsed instead of dropping").toBe(0);
  expect(squeezed!.ellipsed, "chips ellipsed rather than the row dropping its labels").toEqual([]);
  expect(squeezed!.overflow, "the row still does not fit once the labels are gone").toBeLessThan(2);
});
