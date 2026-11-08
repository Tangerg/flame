import { expect, test } from "./test";
import { eachTabStop } from "./tabWalk";
import { expectLabReady } from "../readiness";
const ROUTES = [
  "fixture=agent&state=question",
  "fixture=shell&state=populated",
  "fixture=workspace&state=dock-light",
  "fixture=workspace&state=settings",
];

const PROFILES = [
  { locale: "en", theme: "light", fontSize: 14 },
  { locale: "zh", theme: "dark", fontSize: 18 },
] as const;

const SETTLE_MS = 500;

const ROUTE_BUDGET_MS = 120_000;

// These fields paint focus on an enclosing surface, outside their own box.
const FOCUS_SURFACE = '[data-focus-proxy], [data-slot="composer-root"]';

for (const profile of PROFILES) {
  const appearance = `locale=${profile.locale}&theme=${profile.theme}&font-size=${profile.fontSize}`;
  const title = `a control that turns off the ring shows focus some other way (${appearance})`;

  test(title, async ({ page }, testInfo) => {
    test.setTimeout(ROUTES.length * ROUTE_BUDGET_MS + 20_000);
    const silent: string[] = [];
    let optOuts = 0;
    const delegated = new Set<string>();

    for (const route of ROUTES) {
      await page.goto(`/lab/fixture.html?${route}&${appearance}`);
      await expectLabReady(page);

      await eachTabStop(page, async () => {
        const meta = await page.evaluate((selector) => {
          const active = document.activeElement as HTMLElement;
          if (!active.hasAttribute("data-chrome-focus")) return null;
          if (!active.matches(":focus-visible")) return null;
          const viewport = { width: window.innerWidth, height: window.innerHeight };
          const box = active.getBoundingClientRect();
          if (box.width === 0 || box.y < 0 || box.bottom > viewport.height) return null;
          if (box.right > viewport.width) return null;
          const surface = active.closest<HTMLElement>(selector) ?? active;
          const style = getComputedStyle(surface);
          const paint = surface.getBoundingClientRect();
          const left = Math.max(0, paint.x - 3);
          const top = Math.max(0, paint.y - 3);
          const rect = ({ x, y, width, height }: DOMRect) => ({ x, y, width, height });
          active.dataset.chromeFocusProbe = "";
          return {
            label: (
              active.getAttribute("aria-label") ??
              active.getAttribute("title") ??
              active.textContent ??
              ""
            )
              .trim()
              .replace(/\s+/g, " ")
              .slice(0, 30),
            tag: active.tagName.toLowerCase(),
            surface:
              surface === active
                ? null
                : surface.matches("[data-focus-proxy]")
                  ? "focus-proxy"
                  : "composer-root",
            active: rect(box),
            painted: rect(paint),
            paint: {
              outline: style.outline,
              shadow: style.boxShadow,
              border: style.borderColor,
              background: style.backgroundColor,
            },
            viewport,
            clip: {
              x: left,
              y: top,
              width: Math.min(viewport.width, paint.right + 3) - left,
              height: Math.min(viewport.height, paint.bottom + 3) - top,
            },
          };
        }, FOCUS_SURFACE);
        if (!meta) return;
        optOuts += 1;

        const focused = await page.screenshot({
          clip: meta.clip,
          caret: "hide",
          animations: "disabled",
        });
        await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
        await page.waitForTimeout(SETTLE_MS);
        const blurred = await page.screenshot({
          clip: meta.clip,
          caret: "hide",
          animations: "disabled",
        });
        if (meta.surface) {
          const blurredPaint = await page.evaluate((selector) => {
            const probe = document.querySelector<HTMLElement>("[data-chrome-focus-probe]");
            if (!probe) throw new Error("The original focus probe retired during capture");
            const style = getComputedStyle(probe.closest(selector) ?? probe);
            return {
              outline: style.outline,
              shadow: style.boxShadow,
              border: style.borderColor,
              background: style.backgroundColor,
            };
          }, FOCUS_SURFACE);
          delegated.add(meta.surface);
          const name = `${route} ${meta.label}`;
          await testInfo.attach(`${name} geometry`, {
            body: JSON.stringify({ ...meta, blurredPaint }, null, 2),
            contentType: "application/json",
          });
          await testInfo.attach(`${name} focused`, { body: focused, contentType: "image/png" });
          await testInfo.attach(`${name} blurred`, { body: blurred, contentType: "image/png" });
          expect(blurredPaint, "the containing field surface owns a focus change").not.toEqual(
            meta.paint,
          );
        }
        if (Buffer.compare(focused, blurred) === 0) {
          silent.push(`${route}  <${meta.tag}> "${meta.label}"`);
        }

        const restored = await page.evaluate(() => {
          const probe = document.querySelector<HTMLElement>("[data-chrome-focus-probe]");
          if (!probe) throw new Error("The original focus probe retired during capture");
          probe.focus();
          probe.removeAttribute("data-chrome-focus-probe");
          return document.activeElement === probe;
        });
        expect(restored, "continue the keyboard walk from its original control").toBe(true);
      });
    }

    expect([...delegated]).toEqual(expect.arrayContaining(["composer-root", "focus-proxy"]));
    expect(optOuts, "the walk has to reach controls that opted out").toBeGreaterThan(8);
    expect(
      [...new Set(silent)],
      "controls with `data-chrome-focus` that show nothing when a keyboard reaches them",
    ).toEqual([]);
  });
}
