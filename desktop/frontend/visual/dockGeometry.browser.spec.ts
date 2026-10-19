import { expect, test } from "./test";

const NAMING_VIEWS = ["dock-skills", "dock-review", "dock-runs", "dock-trajectory"] as const;

const LEGIBLE_PX = 12;

test("a dock near its floor never renders a name at zero width", async ({ page }) => {
  test.setTimeout(180_000);
  await page.setViewportSize({ width: 1120, height: 820 });

  const gone: string[] = [];
  let examined = 0;

  for (const state of NAMING_VIEWS) {
    await page.goto(`/visual/?fixture=workspace&state=${state}&theme=light`);
    await page.locator("html[data-visual-ready]").waitFor();

    const seen = await page.evaluate((legible) => {
      const out = { examined: 0, gone: [] as string[] };
      for (const element of document.querySelectorAll<HTMLElement>("main *")) {
        if (getComputedStyle(element).textOverflow !== "ellipsis") continue;
        if (!element.checkVisibility({ visibilityProperty: true, contentVisibilityAuto: true })) {
          continue;
        }
        const text = [...element.childNodes]
          .filter((node) => node.nodeType === Node.TEXT_NODE)
          .map((node) => node.textContent ?? "")
          .join("")
          .trim();
        if (text.length < 3) continue;
        out.examined += 1;
        const width = element.getBoundingClientRect().width;
        if (width < legible) out.gone.push(`${Math.round(width)}px "${text.slice(0, 32)}"`);
      }
      return out;
    }, LEGIBLE_PX);

    examined += seen.examined;
    for (const hit of seen.gone) gone.push(`${state}: ${hit}`);
  }

  expect(examined, "no truncating name was found in any dock view").toBeGreaterThan(20);
  expect(gone, "a name the row exists to identify was squeezed out of the row").toEqual([]);
});
