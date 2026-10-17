import { expect, test } from "./test";

interface Conflict {
  property: string;
  stylexValue: string;
  sheetValue: string;
  overruled: string;
  rendered: string;
  sample: string;
}

const ROUTES = [
  "fixture=agent&state=waiting",
  "fixture=agent&state=tool-shells",
  "fixture=agent&state=narrative",
  "fixture=shell&state=populated",
  "fixture=workspace&state=dock-light",
  "fixture=workspace&state=settings",
];

test("no declaration is decided by StyleX out-ranking a stylesheet rule", async ({ page }) => {
  test.setTimeout(ROUTES.length * 20_000 + 30_000);
  const conflicts: Conflict[] = [];
  let examined = 0;

  for (const route of ROUTES) {
    await page.goto(`/visual/?${route}&theme=light`);
    await page.waitForSelector("html[data-visual-ready]");
    await page.waitForTimeout(200);

    const found = await page.evaluate(() => {
      interface Rule {
        selector: string;
        fromStylex: boolean;
        sheet: string;
        properties: Record<string, string>;
      }
      const rules: Rule[] = [];

      const walk = (list: CSSRuleList, layer: string | null, sheet: string) => {
        for (const rule of list) {
          const asLayer = rule as CSSRule & { name?: string };
          const asGroup = rule as CSSRule & {
            cssRules?: CSSRuleList;
            conditionText?: string;
            media?: MediaList;
          };
          const asStyle = rule as CSSStyleRule;
          if (rule.constructor.name === "CSSLayerBlockRule") {
            walk(asGroup.cssRules!, asLayer.name || layer, sheet);
          } else if (asGroup.cssRules && !asStyle.selectorText) {
            const condition = asGroup.conditionText ?? asGroup.media?.mediaText;
            if (condition && !window.matchMedia(condition).matches) continue;
            walk(asGroup.cssRules, layer, sheet);
          } else if (asStyle.selectorText && asStyle.style) {
            if (layer === "base") continue;
            const properties: Record<string, string> = {};
            for (const property of asStyle.style) {
              if (asStyle.style.getPropertyPriority(property) === "important") continue;
              if (property.startsWith("--")) continue;
              properties[property] = asStyle.style.getPropertyValue(property);
            }
            if (Object.keys(properties).length > 0) {
              rules.push({
                selector: asStyle.selectorText,
                fromStylex: asStyle.selectorText.includes(":not(#\\#)"),
                sheet,
                properties,
              });
            }
          }
        }
      };
      for (const sheet of document.styleSheets) {
        const node = sheet.ownerNode as HTMLElement | null;
        const name =
          sheet.href?.split("/").pop() ?? node?.dataset?.viteDevId?.split("/").pop() ?? "inline";
        try {
          walk(sheet.cssRules, null, name);
        } catch {
          continue;
        }
      }

      const physical = (property: string) =>
        property
          .replace("-inline-start", "-left")
          .replace("-inline-end", "-right")
          .replace("-block-start", "-top")
          .replace("-block-end", "-bottom");
      const normalise = (properties: Record<string, string>) =>
        Object.fromEntries(
          Object.entries(properties).map(([key, value]) => [physical(key), value]),
        );

      const fromStylex = rules.filter((rule) => rule.fromStylex);
      const fromSheet = rules.filter((rule) => !rule.fromStylex);
      const matches = (element: Element, selector: string) => {
        try {
          return element.matches(selector);
        } catch {
          return false;
        }
      };

      const out: Conflict[] = [];
      for (const element of document.querySelectorAll("*")) {
        if (element.closest("[data-fixture-chrome]")) continue;
        for (const atomic of fromStylex) {
          if (!matches(element, atomic.selector)) continue;
          const wanted = normalise(atomic.properties);
          for (const sheetRule of fromSheet) {
            if (!matches(element, sheetRule.selector)) continue;
            const declared = normalise(sheetRule.properties);
            for (const [property, stylexValue] of Object.entries(wanted)) {
              const sheetValue = declared[property];
              if (sheetValue === undefined || sheetValue === stylexValue) continue;
              out.push({
                property,
                stylexValue,
                sheetValue,
                overruled: `${sheetRule.sheet} \`${sheetRule.selector}\``,
                rendered: getComputedStyle(element).getPropertyValue(property),
                sample: `<${element.tagName.toLowerCase()} class="${(element.getAttribute("class") ?? "").slice(0, 60)}">`,
              });
            }
          }
        }
      }
      return { out, stylex: fromStylex.length, sheet: fromSheet.length };
    });

    examined = Math.min(
      examined === 0 ? Number.MAX_SAFE_INTEGER : examined,
      found.stylex,
      found.sheet,
    );
    conflicts.push(...found.out);
  }

  expect(examined, "both sides of the comparison have to reach real rules").toBeGreaterThan(50);

  const unique = [
    ...new Map(
      conflicts.map((one) => [`${one.property}|${one.overruled}|${one.stylexValue}`, one]),
    ).values(),
  ];
  expect(
    unique,
    unique
      .map(
        (one) =>
          `\n  ${one.property}: StyleX asks ${one.stylexValue || "(shorthand)"}, ` +
          `${one.overruled} asks ${one.sheetValue}, rendered ${one.rendered}\n     on ${one.sample}`,
      )
      .join(""),
  ).toEqual([]);
});
