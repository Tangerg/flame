import { expect, test } from "./test";
import { CONTROL } from "./controls";

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

  expect(examined, "the sweep has to reach real controls").toBeGreaterThan(600);
  expect([...new Set(spilling)], "content overflowing the control that states its height").toEqual(
    [],
  );
});
