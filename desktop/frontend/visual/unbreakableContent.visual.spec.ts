import { expect, test, type Page } from "./test";

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
      const holdsText = (el: Element) =>
        [...el.childNodes].some(
          (node) => node.nodeType === Node.TEXT_NODE && (node.textContent ?? "").trim().length > 12,
        );
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

const DOCK_STATES = [
  "dock-light",
  "dock-review",
  "dock-inbox",
  "dock-agent-memory",
  "dock-knowledge",
  "dock-timeline",
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
