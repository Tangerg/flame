import { expect, test, type Page } from "./test";

// A model writes hashes, base64, minified payloads and paths with no space in them. A run with no
// break opportunity is the one input that can widen a block past the column that holds it, and no
// fixture contains one — snapshots are prose, so a renderer that forgot to allow the break paints
// correctly in every golden and only spills in front of a user.
//
// The blob is injected instead of fixtured because the defect belongs to the RENDERER, not to any
// one message: `CONTENT_RENDERING.md` keeps adding renderers, and each new one has to answer this
// on its own. Injecting reaches every element a renderer produced, including the ones no snapshot
// happens to exercise.
//
// `scrollWidth > clientWidth` alone is not the defect — that is what `truncate` is FOR, and the
// session title, the activity label and every `truncate-fade` legitimately report it. The defect
// is that overflow being VISIBLE: text painting outside the box that was supposed to bound it.

const UNBREAKABLE = "Q7fJ2xL9pN4vR8mK3sT6wY1zB5cD0eG".repeat(7);

const STATES = ["narrative", "long-content", "tool-shells", "terminal", "question", "error"];

interface Spill {
  tag: string;
  over: number;
  wrap: string;
  wordBreak: string;
  sample: string;
}

async function spills(page: Page): Promise<Spill[]> {
  return page.evaluate((blob) => {
    const out: Spill[] = [];
    // Whatever tag a renderer reached for: the element that HOLDS the text, not one that
    // contains an element that holds it. A tag list would have missed the `div` and `span`
    // most of the tool previews are written with.
    const holdsText = (el: Element) =>
      [...el.childNodes].some(
        (node) => node.nodeType === Node.TEXT_NODE && (node.textContent ?? "").trim().length > 12,
      );
    // The panel only. The agent fixture fills the drawer with its own caption, and scaffolding
    // failing a product rule teaches the next reader to add breaking where no model writes.
    const panel = document.querySelector("main");
    if (!panel) throw new Error("no panel to audit");
    const targets = [...panel.querySelectorAll("*")].filter(
      (el) => holdsText(el) && !el.closest("[data-slot='composer-root']") && el.clientWidth > 40,
    );
    for (const el of targets) {
      const restore = el.textContent;
      el.textContent = blob;
      const style = getComputedStyle(el);
      if (el.scrollWidth - el.clientWidth > 1 && style.overflowX === "visible") {
        out.push({
          tag: el.tagName.toLowerCase(),
          over: el.scrollWidth - el.clientWidth,
          wrap: style.overflowWrap,
          wordBreak: style.wordBreak,
          sample: (el.getAttribute("class") ?? "").slice(0, 60),
        });
      }
      el.textContent = restore;
    }
    return out;
  }, UNBREAKABLE);
}

test("rendered content keeps an unbreakable run inside its column", async ({ page }) => {
  const found: { state: string; spill: Spill }[] = [];
  for (const state of STATES) {
    await page.goto(`/visual/?fixture=agent&state=${state}&theme=light`);
    await page.waitForSelector("html[data-visual-ready]");
    for (const spill of await spills(page)) found.push({ state, spill });
  }

  expect(
    found,
    found
      .map(
        ({ state, spill }) =>
          `\n  ${state}: <${spill.tag}> paints ${spill.over}px outside its box` +
          `\n     overflow-wrap: ${spill.wrap}, word-break: ${spill.wordBreak}  on "${spill.sample}"`,
      )
      .join(""),
  ).toEqual([]);
});

test("a renderer that forbids the break is caught", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&state=narrative&theme=light");
  await page.waitForSelector("html[data-visual-ready]");

  await page.evaluate(() => {
    const sheet = document.createElement("style");
    sheet.textContent = `* { overflow-wrap: normal !important; word-break: normal !important; }`;
    document.head.append(sheet);
  });

  expect((await spills(page)).length).toBeGreaterThan(0);
});
