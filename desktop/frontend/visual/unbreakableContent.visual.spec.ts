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

async function spills(page: Page, root: string): Promise<Spill[]> {
  return page.evaluate(
    ({ blob, root }) => {
      const out: Spill[] = [];
      // Whatever tag a renderer reached for: the element that HOLDS the text, not one that
      // contains an element that holds it. A tag list would have missed the `div` and `span`
      // most of the tool previews are written with.
      const holdsText = (el: Element) =>
        [...el.childNodes].some(
          (node) => node.nodeType === Node.TEXT_NODE && (node.textContent ?? "").trim().length > 12,
        );
      // A named root, because the two surfaces this runs on hold different things. On the agent
      // fixture it is the panel: that fixture fills the drawer with its own caption, and
      // scaffolding failing a product rule teaches the next reader to add breaking where no
      // model writes. On the workspace fixture the dock IS the product.
      const panel = document.querySelector(root);
      if (!panel) throw new Error(`no ${root} to audit`);
      const targets = [...panel.querySelectorAll("*")].filter(
        (el) =>
          holdsText(el) &&
          !el.closest("[data-fixture-chrome]") &&
          !el.closest("[data-slot='composer-root']") &&
          el.clientWidth > 40,
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
            sample: (restore ?? "").trim().slice(0, 44),
          });
        }
        el.textContent = restore;
      }
      return out;
    },
    { blob: UNBREAKABLE, root },
  );
}

test("rendered content keeps an unbreakable run inside its column", async ({ page }) => {
  const found: { state: string; spill: Spill }[] = [];
  for (const state of STATES) {
    await page.goto(`/visual/?fixture=agent&state=${state}&theme=light`);
    await page.waitForSelector("html[data-visual-ready]");
    for (const spill of await spills(page, "main")) found.push({ state, spill });
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

  expect((await spills(page, "main")).length).toBeGreaterThan(0);
});

// The same defect, on the other surface a model writes into.
//
// The audit above stops at the transcript, and for one round that looked like the whole of it.
// It is not: a dock view renders plan steps the agent wrote, memory entries it wrote, skill
// descriptions a third party's file wrote, and hunk headers `git` wrote — all unbounded, none
// of them prose. Measured before this existed: the plan pane spilled 1368px, a memory entry
// 1636px, a skill description 1430px and a diff hunk header 1310px.
//
// The dock element itself is the root, not the window. Settings sits on the same fixture and is
// the app's OWN copy — "Global corner radius." never grows a hash — so auditing it would teach
// the next reader to add breaking where nothing unbreakable is ever written.
const DOCK_STATES = [
  "dock-light",
  "dock-review",
  "dock-files",
  "dock-inbox",
  "dock-agent-memory",
  "dock-skill-library",
  "dock-skill-proposals",
  "dock-recipes",
  "dock-knowledge",
  "dock-timeline",
  "dock-run-summary",
  "dock-notifications",
];

test("a dock view keeps an unbreakable run inside its pane", async ({ page }) => {
  test.setTimeout(DOCK_STATES.length * 20_000 + 30_000);
  await page.setViewportSize({ width: 1120, height: 720 });

  const found: { state: string; spill: Spill }[] = [];
  for (const state of DOCK_STATES) {
    await page.goto(`/visual/?fixture=workspace&state=${state}&theme=light`);
    await page.waitForSelector("html[data-visual-ready]");
    await page.waitForSelector(".agent-context-dock");
    await page.waitForTimeout(300);
    for (const spill of await spills(page, ".agent-context-dock")) found.push({ state, spill });
  }

  expect(
    found,
    found
      .map(
        ({ state, spill }) =>
          `\n  ${state}: <${spill.tag}> paints ${spill.over}px outside the dock` +
          `\n     overflow-wrap: ${spill.wrap}, word-break: ${spill.wordBreak}  on "${spill.sample}"`,
      )
      .join(""),
  ).toEqual([]);
});

// The third region was tried and does not need one.
//
// Neither audit above reaches the work index: one is rooted at `main`, the other at the dock,
// and the drawer sits outside both. What lands there is a session TITLE, which the agent writes
// — so it looked like the same gap, and a third test was written for it.
//
// It cannot fail. Measured on the shell fixture with the blob in all fifteen of the drawer's
// text holders AND `overflow-wrap: normal !important` forced on everything: the drawer reads
// `clientWidth === scrollWidth === 275` in every combination. Its width is the sidebar's, fixed,
// and every title inside is clipped by an ancestor rather than by the element holding the text —
// so the premise this audit rests on, that a text holder must permit the break, does not apply
// there at all.
//
// Recorded rather than committed, because a guard that cannot go red is worse than no guard: it
// reports coverage of a region it never examined.
