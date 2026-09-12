import { expect, test } from "./test";
import { CONTROL } from "./controls";

const ROUTES = [
  "fixture=agent&state=narrative",
  "fixture=agent&state=tool-shells",
  "fixture=shell&state=populated",
  "fixture=workspace&state=dock-light",
  "fixture=workspace&state=settings",
];

const ROUTE_BUDGET_MS = 90_000;

const MAX_DISTINCT_ANSWERS = 8;

const MAY_ANSWER_NOTHING = 'input, textarea, [role="switch"], [role="checkbox"]';

const VISIBLE_STATE = `(el) => {
  const s = getComputedStyle(el);
  return [s.backgroundColor, s.color, s.opacity, s.borderColor, s.boxShadow,
          s.textDecorationColor, s.scale, s.translate, s.visibility].join("|");
}`;

const wholeBox = `(node, read) => {
  const parts = [read(node)];
  for (const kid of [...node.querySelectorAll("*")].slice(0, 12)) parts.push(read(kid));
  for (let p = node.parentElement, d = 0; p && d < 6; p = p.parentElement, d += 1) {
    parts.push(read(p));
    const ps = getComputedStyle(p);
    if (ps.overflowY === "auto" || ps.overflowY === "scroll") break;
  }
  return parts.join("~");
}`;

const ARGS = { visible: VISIBLE_STATE, box: wholeBox, allowed: MAY_ANSWER_NOTHING };

test("hover always adds ink, and never replaces the fill it lands on", async ({ page }) => {
  test.setTimeout(ROUTES.length * ROUTE_BUDGET_MS + 20_000);
  const answers = new Map<string, string>();
  const replaced: string[] = [];
  const unreachable: string[] = [];
  const silent: string[] = [];
  let hovered = 0;

  for (const route of ROUTES) {
    await page.goto(`/visual/?${route}&theme=light`);
    await page.waitForSelector("html[data-visual-ready]");
    await page.waitForTimeout(250);

    for (let attempt = 0; attempt < 20; attempt += 1) {
      const count = await page.locator(CONTROL).count();
      await page.waitForTimeout(150);
      if (count > 0 && count === (await page.locator(CONTROL).count())) break;
    }

    const neutralWash = await page.evaluate(() => {
      const probe = document.createElement("div");
      probe.style.backgroundColor = "var(--wash-hover)";
      document.body.append(probe);
      const value = getComputedStyle(probe).backgroundColor;
      probe.remove();
      return value;
    });

    const total = await page.locator(CONTROL).evaluateAll((nodes) => {
      nodes.forEach((node, index) => node.setAttribute("data-hover-probe", String(index)));
      return nodes.length;
    });

    for (let index = 0; index < total; index += 1) {
      const control = page.locator(`[data-hover-probe="${index}"]`);
      await page.mouse.move(1119, 719);
      await page.waitForTimeout(50);
      const before = await control.evaluate((node, args) => {
        const read = new Function("return " + args.visible)();
        const readBox = new Function("return " + args.box)();
        const style = getComputedStyle(node);
        let box = node.getBoundingClientRect();
        let visible = { top: box.top, bottom: box.bottom, left: box.left, right: box.right };
        for (let parent = node.parentElement; parent; parent = parent.parentElement) {
          const parentStyle = getComputedStyle(parent);
          if (parentStyle.overflowX === "visible" && parentStyle.overflowY === "visible") continue;
          const clip = parent.getBoundingClientRect();
          visible = {
            top: Math.max(visible.top, clip.top),
            bottom: Math.min(visible.bottom, clip.bottom),
            left: Math.max(visible.left, clip.left),
            right: Math.min(visible.right, clip.right),
          };
        }
        const chain: { fill: string; what: string }[] = [];
        for (
          let parent = node.parentElement;
          parent && chain.length < 6;
          parent = parent.parentElement
        ) {
          const parentStyle = getComputedStyle(parent);
          chain.push({
            fill: parentStyle.backgroundColor,
            what: `${parent.tagName.toLowerCase()}${parent.hasAttribute("data-active") ? "[data-active]" : ""}`,
          });
          if (parentStyle.overflowY === "auto" || parentStyle.overflowY === "scroll") break;
        }
        return {
          rest: style.backgroundColor,
          state: readBox(node, read) as string,
          mayBeSilent: node.matches(args.allowed),
          chain,
          x: (visible.left + visible.right) / 2,
          y: (visible.top + visible.bottom) / 2,
          width: visible.right - visible.left,
          height: visible.bottom - visible.top,
          skip:
            node.matches(':disabled, [aria-disabled="true"]') ||
            node.matches('input[type="range"]') ||
            style.pointerEvents === "none" ||
            style.opacity === "0",
          label: (node.getAttribute("aria-label") ?? node.textContent ?? "")
            .trim()
            .replace(/\s+/g, " ")
            .slice(0, 30),
          tag: node.tagName.toLowerCase(),
        };
      }, ARGS);
      if (before.width < 2 || before.height < 2 || before.skip) continue;
      if (before.x < 0 || before.y < 0 || before.x > 1120 || before.y > 720) continue;

      await page.mouse.move(before.x, before.y);
      await page.mouse.move(before.x + 1, before.y);
      await page.waitForTimeout(70);
      const after = await control.evaluate((node, args) => {
        const read = new Function("return " + args.visible)();
        const readBox = new Function("return " + args.box)();
        const style = getComputedStyle(node);
        const chain: string[] = [];
        for (
          let parent = node.parentElement;
          parent && chain.length < 6;
          parent = parent.parentElement
        ) {
          const parentStyle = getComputedStyle(parent);
          chain.push(parentStyle.backgroundColor);
          if (parentStyle.overflowY === "auto" || parentStyle.overflowY === "scroll") break;
        }
        return {
          fill: style.backgroundColor,
          state: readBox(node, read) as string,
          chain,
          reached: node.matches(":hover"),
          shown: style.visibility !== "hidden" && style.opacity !== "0",
        };
      }, ARGS);
      if (!after.reached) {
        if (after.shown) unreachable.push(`${route} <${before.tag}> "${before.label}"`);
        continue;
      }
      hovered += 1;

      before.chain.forEach((link, depth) => {
        const now = after.chain[depth];
        if (now === undefined || now === link.fill) return;
        if (link.fill === "rgba(0, 0, 0, 0)" || now !== neutralWash) return;
        replaced.push(`${route} <${link.what}> above "${before.label}"  ${link.fill} -> ${now}`);
      });

      if (after.state === before.state && !before.mayBeSilent) {
        silent.push(`${route} <${before.tag}> "${before.label}"`);
      }

      if (after.fill === before.rest) continue;

      answers.set(`${before.rest} -> ${after.fill}`, `${route} <${before.tag}> "${before.label}"`);
      if (before.rest !== "rgba(0, 0, 0, 0)" && after.fill === neutralWash) {
        replaced.push(
          `${route} <${before.tag}> "${before.label}"  ${before.rest} -> ${after.fill}`,
        );
      }
    }

    await page.mouse.move(1119, 719);
  }

  console.log(
    `hovered ${hovered}, pointer never arrived on ${unreachable.length}` +
      (unreachable.length > 0 ? `\n  ${unreachable.join("\n  ")}` : ""),
  );
  expect(hovered, "the sweep has to reach real controls").toBeGreaterThan(100);
  expect(
    [...new Set(unreachable)],
    "the pointer never landed on these — the aim is wrong, or something covers them",
  ).toEqual([]);
  expect(
    [...new Set(silent)],
    `controls that answer the pointer with nothing — allowed only for ${MAY_ANSWER_NOTHING}`,
  ).toEqual([]);
  expect(
    [...new Set(replaced)],
    "hover replaced a resting fill with the neutral wash — lay the ink over that fill instead",
  ).toEqual([]);
  const distinct = [...answers].map(([answer, where]) => `${answer}  first at ${where}`).sort();
  expect(
    distinct.length,
    `one gesture, a closed set of answers:\n${distinct.join("\n")}`,
  ).toBeLessThanOrEqual(MAX_DISTINCT_ANSWERS);
});
