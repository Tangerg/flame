import { expect, test } from "./test";
import { CONTROL } from "./controls";

// The UI font is the one free input that changes SIZE rather than colour: the picker lists the
// families the machine has, and every control in the product states a fixed height from the
// `--control-height-*` ladder. Whether those heights survive a family with different metrics
// was not something anything asked.
//
// They do. Nine faces a person might actually choose — grotesque, humanist, geometric, serif,
// monospace — leave the count exactly where the bundled face leaves it. What this pins is that
// they go on doing so, because the failure mode is a label painting outside its own control and
// the only thing standing between the design and it is that every control sets a line-height
// from the ladder rather than leaving it to the font.
//
// Zapfino is deliberately not here. It is a script display face whose glyphs exceed any line
// box it is given — measured, it puts 31 controls over their bounds by up to 11px — and no
// arrangement of fixed heights survives it. Excluding it is a judgement about what the picker
// is for, and it is written down rather than left as a gap in the list.
const FONTS = [
  "",
  "Helvetica",
  "Verdana",
  "Georgia",
  "Times New Roman",
  "Menlo",
  "Optima",
  "Futura",
  "Avenir Next",
] as const;

const ROUTES = [
  "fixture=shell&state=populated",
  "fixture=workspace&state=dock-light",
  "fixture=workspace&state=settings",
  "fixture=agent&state=narrative",
] as const;

const ROUTE_BUDGET_MS = 30_000;

test("a control holds its own content whatever font it is given", async ({ page }) => {
  test.setTimeout(FONTS.length * ROUTES.length * ROUTE_BUDGET_MS + 20_000);
  const spilling: string[] = [];
  let examined = 0;

  for (const font of FONTS) {
    for (const route of ROUTES) {
      await page.setViewportSize({ width: 1472, height: 900 });
      const query = font ? `&ui-font=${encodeURIComponent(font)}` : "";
      await page.goto(`/visual/?${route}&theme=light${query}`);
      await page.locator("html[data-visual-ready]").waitFor();
      await page.waitForTimeout(400);

      // A family the machine does not have resolves to the fallback, and then this measures the
      // bundled face nine times over while reporting nine fonts.
      //
      // Not `document.fonts.check()`: it answers true for a family that does not exist, so the
      // first version of this line let `NoSuchFamilyInstalled` through without a word. Measuring
      // a string against a sentinel stack is the only answer that cannot be faked — if the
      // family is missing, both render in the sentinel and come out the same width.
      if (font) {
        const applied = await page.evaluate((family) => {
          const measure = (stack: string) => {
            const probe = document.createElement("span");
            probe.style.cssText = `position:absolute;visibility:hidden;white-space:nowrap;font-size:64px;font-family:${stack}`;
            probe.textContent = "mmmiiiWWW@#%";
            document.body.append(probe);
            const width = probe.getBoundingClientRect().width;
            probe.remove();
            return width;
          };
          return measure(`"${family}", monospace`) !== measure("monospace");
        }, font);
        expect(applied, `${font} is not installed, so this route proves nothing`).toBe(true);
      }

      const found = await page.locator(CONTROL).evaluateAll((nodes) => {
        const out: string[] = [];
        let seen = 0;
        for (const node of nodes) {
          const box = node.getBoundingClientRect();
          if (box.width < 2 || box.height < 2) continue;
          // A range input is a slider's accessibility surface, not a box anything is drawn in:
          // its native shadow parts exceed it by design and it clips them itself.
          if (node.matches('input[type="range"]')) continue;
          seen += 1;
          const down = node.scrollHeight - node.clientHeight;
          const across = node.scrollWidth - node.clientWidth;
          if (Math.max(down, across) > 1) {
            out.push(
              `${node.tagName.toLowerCase()} ${Math.round(box.width)}x${Math.round(box.height)} over by ${Math.round(Math.max(down, across))}px "${(node.textContent ?? "").trim().slice(0, 20)}"`,
            );
          }
        }
        return { out, seen };
      });

      examined += found.seen;
      spilling.push(...found.out.map((one) => `${font || "bundled"}: ${one}`));
    }
  }

  // A sweep that found no controls agrees with every font.
  expect(examined, "the sweep has to reach real controls").toBeGreaterThan(600);
  expect([...new Set(spilling)], "content overflowing the control that states its height").toEqual(
    [],
  );
});
