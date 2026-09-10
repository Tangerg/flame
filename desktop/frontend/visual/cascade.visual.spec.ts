import { expect, test } from "./test";

// A declaration a call site writes and the cascade discards is worse than a missing one: it
// reads as an instruction while deciding nothing, and editing it changes nothing.
//
// This audit used to look for that between Tailwind's `@layer utilities` and the unlayered
// rules in globals.css, keyed on `rule.layer === "utilities"`. Tailwind is gone, so that layer
// does not exist, so the list was empty, so the loop never ran and the test passed while
// examining NOTHING. Measured at the rewrite: 1227 rules on the shell route — 1201 unlayered,
// 26 in `base`, zero in `utilities`.
//
// That is the third time this exact thing happened here. `check-interactive-chrome` had eight
// of nine rules go blind in the same commit, and `check-authored-classes` went dead in it too;
// both say so in their own headers. A guard keyed on the mechanism it was written against dies
// silently when that mechanism leaves, and a passing test is indistinguishable from a blind
// one. Hence the floor at the bottom of this file, which the old version had no equivalent of.
//
// The conflict the product can actually have now is the same shape with both sides renamed.
// StyleX emits UNLAYERED atomic rules, each carrying at least one `:not(#\#)` — an ID-level
// unit, since `#\#` is an ID selector, so one is already more than any rule in globals.css has —
// and
// globals.css holds descendant rules that no atomic class can express. When both set one
// property on one element, StyleX wins on specificity and the stylesheet's declaration is dead
// text. That inversion is not hypothetical: it is exactly how twenty call sites' `outline:
// none` took the global focus ring down with the browser default, and how the glyph-step rule
// lost the one icon this rewrite found.
//
// `@layer base` is excluded because an unlayered rule beats every layer whatever its
// specificity, so a base rule can never be the loser in a way anyone chose — it is losing by
// construction, which is what layering it says.

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
            // Conditions are evaluated, not assumed: the touch hit-area floor lives under
            // `pointer: coarse` and never meets the pointer these tests run with.
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
                // The specificity hack StyleX stamps on every declaration. It is also what
                // makes StyleX the winner, so identifying a rule by it and explaining the
                // conflict by it are the same fact.
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

      // A stylesheet may spell an edge logically and a call site physically, and the two collide
      // under different names. The app is LTR everywhere — the one `dir` in the tree is a code
      // block declaring the same — so start is left and end is right.
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
                // Asked rather than inferred: the browser is the only authority on who won,
                // and a rewrite of this audit that predicted the winner would be asserting
                // its own specificity arithmetic instead of the product's behaviour.
                rendered: getComputedStyle(element).getPropertyValue(property),
                sample: `<${element.tagName.toLowerCase()} class="${(element.getAttribute("class") ?? "").slice(0, 60)}">`,
              });
            }
          }
        }
      }
      return { out, stylex: fromStylex.length, sheet: fromSheet.length };
    });

    // The floor goes on BOTH sides of the comparison, because either one going empty is a
    // silent pass. A count of all rules would not do it — that is the mistake the first draft
    // of this rewrite repeated: the old audit did not die because the stylesheet emptied, it
    // died because ONE FILTER stopped matching while everything around it stayed healthy.
    examined = Math.min(
      examined === 0 ? Number.MAX_SAFE_INTEGER : examined,
      found.stylex,
      found.sheet,
    );
    conflicts.push(...found.out);
  }

  // Floors, not targets. Measured at the rewrite: 872 StyleX rules and 243 stylesheet rules on
  // the lightest route, so these sit an order of magnitude below the real numbers and still
  // catch a filter that has stopped matching.
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
