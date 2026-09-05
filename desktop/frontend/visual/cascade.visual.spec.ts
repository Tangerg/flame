import { expect, test } from "./test";

// A class a call site writes and the cascade discards is worse than a missing one: it reads as
// an instruction while deciding nothing, and editing it changes nothing. Three ways it happens
// here, all found by this check before it existed — `truncate` on the session title losing to an
// unlayered `text-wrap`, and `tabular-nums`, `gap-1` and `flex-1` restating a value a stylesheet
// rule had already fixed.
//
// The mechanism is the layer order: Tailwind's utilities sit in `@layer utilities`, and an
// UNLAYERED rule beats every layer whatever its specificity — `:where(...)` at zero specificity
// included. So a property set by both an unlayered rule and a utility on the same element is a
// call site being overruled, and only a DISAGREEMENT between the two values is a real one.
//
// Conditions are evaluated, not assumed: the touch hit-area floor lives under
// `pointer: coarse` and never meets the pointer these tests run with.

interface Conflict {
  property: string;
  wanted: string;
  rendered: string;
  utility: string;
  overruledBy: string;
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

test("no call site writes a utility the cascade then discards", async ({ page }) => {
  const conflicts: Conflict[] = [];

  for (const route of ROUTES) {
    await page.goto(`/visual/?${route}&theme=light`);
    await page.waitForSelector("html[data-visual-ready]");
    await page.waitForTimeout(200);

    const found = await page.evaluate(() => {
      const rules: { sel: string; layer: string | null; props: Record<string, string> }[] = [];
      const walk = (list: CSSRuleList, layer: string | null) => {
        for (const rule of list) {
          const asLayer = rule as CSSRule & { name?: string };
          const asGroup = rule as CSSRule & {
            cssRules?: CSSRuleList;
            conditionText?: string;
            media?: MediaList;
          };
          const asStyle = rule as CSSStyleRule;
          if (rule.constructor.name === "CSSLayerBlockRule") {
            walk(asGroup.cssRules!, asLayer.name || layer || "anonymous");
          } else if (asGroup.cssRules && !asStyle.selectorText) {
            const condition = asGroup.conditionText ?? asGroup.media?.mediaText;
            if (condition && !window.matchMedia(condition).matches) continue;
            walk(asGroup.cssRules, layer);
          } else if (asStyle.selectorText && asStyle.style) {
            const props: Record<string, string> = {};
            for (const property of asStyle.style) {
              if (asStyle.style.getPropertyPriority(property) === "important") continue;
              if (property.startsWith("--")) continue;
              props[property] = asStyle.style.getPropertyValue(property);
            }
            if (Object.keys(props).length > 0) {
              rules.push({ sel: asStyle.selectorText, layer, props });
            }
          }
        }
      };
      for (const sheet of document.styleSheets) {
        try {
          walk(sheet.cssRules, null);
        } catch {
          continue;
        }
      }

      // A rule may spell the same edge logically and a utility physically, and the two collide
      // in the cascade under different names. The app is LTR everywhere — the one `dir` in the
      // tree is a code block declaring the same — so start is left and end is right.
      const physical = (property: string) =>
        property
          .replace("-inline-start", "-left")
          .replace("-inline-end", "-right")
          .replace("-block-start", "-top")
          .replace("-block-end", "-bottom");
      const normalise = (props: Record<string, string>) =>
        Object.fromEntries(Object.entries(props).map(([key, value]) => [physical(key), value]));

      const unlayered = rules.filter((rule) => rule.layer === null);
      const utilities = rules.filter((rule) => rule.layer === "utilities");
      const out: Conflict[] = [];
      const matches = (element: Element, selector: string) => {
        try {
          return element.matches(selector);
        } catch {
          return false;
        }
      };

      for (const element of document.querySelectorAll("*")) {
        if (!element.getAttribute("class")) continue;
        for (const utility of utilities) {
          if (!matches(element, utility.sel)) continue;
          for (const global of unlayered) {
            if (!matches(element, global.sel)) continue;
            const rendered_ = normalise(global.props);
            for (const [property, wanted] of Object.entries(normalise(utility.props))) {
              const rendered = rendered_[property];
              if (rendered === undefined || rendered === wanted) continue;
              out.push({
                property,
                wanted,
                rendered,
                utility: utility.sel,
                overruledBy: global.sel,
                sample: `<${element.tagName.toLowerCase()} class="${element.getAttribute("class")!.slice(0, 70)}">`,
              });
            }
          }
        }
      }
      return out;
    });
    conflicts.push(...found);
  }

  const unique = [
    ...new Map(conflicts.map((c) => [`${c.property}|${c.utility}|${c.overruledBy}`, c])).values(),
  ];
  expect(
    unique,
    unique
      .map(
        (c) =>
          `\n  ${c.property}: \`${c.utility}\` asks ${c.wanted || "(shorthand)"}, ` +
          `\`${c.overruledBy}\` renders ${c.rendered}\n     on ${c.sample}`,
      )
      .join(""),
  ).toEqual([]);
});
