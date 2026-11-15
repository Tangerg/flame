import { expect, test } from "./test";
import { CONTROL } from "./controls";
import { expectLabReady } from "../readiness";
const MINIMUM_TARGET_SIZE = 24;
const MAX_CONTROL_LABEL_LENGTH = 24;
const ROUTES = [
  "fixture=atoms",
  "fixture=shell&state=populated",
  "fixture=agent&state=narrative",
  "fixture=agent&state=waiting",
  "fixture=agent&state=tool-shells",
  "fixture=agent&state=delegated",
  "fixture=workspace&state=dock-light",
  "fixture=workspace&state=dock-trajectory",
  "fixture=workspace&state=settings",
];

for (const route of ROUTES) {
  test(`undersized controls keep ${MINIMUM_TARGET_SIZE}px of clearance in ${route}`, async ({
    page,
  }) => {
    await page.goto(`/lab/fixture.html?${route}&theme=light`);
    await expectLabReady(page);
    await page.evaluate(() => document.fonts.ready);

    const result = await page.evaluate(
      ({ selector, minimumTargetSize, maxLabelLength }) => {
        const boxes = [...document.querySelectorAll(selector)]
          .filter((el) => !el.closest("[data-fixture-chrome]"))
          .filter((el) => {
            const style = getComputedStyle(el);
            const r = el.getBoundingClientRect();
            return r.width > 0 && r.height > 0 && style.visibility !== "hidden";
          })
          .map((el) => ({ el, r: el.getBoundingClientRect() }));
        const label = (el: Element) =>
          (el.getAttribute("aria-label") || el.textContent || el.tagName)
            .trim()
            .replace(/\s+/g, " ")
            .slice(0, maxLabelLength);

        const out: string[] = [];
        for (const { el, r } of boxes) {
          if (r.width >= minimumTargetSize && r.height >= minimumTargetSize) continue;
          if (el.closest('[data-slot="chat-rail"]')) continue;
          const cx = r.left + r.width / 2;
          const cy = r.top + r.height / 2;
          for (const other of boxes) {
            if (other.el === el || other.el.contains(el) || el.contains(other.el)) continue;
            const nx = Math.max(other.r.left, Math.min(cx, other.r.right));
            const ny = Math.max(other.r.top, Math.min(cy, other.r.bottom));
            if (Math.hypot(nx - cx, ny - cy) < minimumTargetSize / 2) {
              out.push(`"${label(el)}" <-> "${label(other.el)}"`);
              break;
            }
          }
        }
        return { controls: boxes.length, crowded: out };
      },
      {
        selector: CONTROL,
        minimumTargetSize: MINIMUM_TARGET_SIZE,
        maxLabelLength: MAX_CONTROL_LABEL_LENGTH,
      },
    );
    expect(result.controls, "the fixture must expose real controls").toBeGreaterThan(0);
    expect(
      result.crowded,
      `undersized controls closer than ${MINIMUM_TARGET_SIZE}px to a neighbour`,
    ).toEqual([]);
  });
}
