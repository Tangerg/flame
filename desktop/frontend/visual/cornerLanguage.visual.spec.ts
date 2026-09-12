import { expect, test } from "./test";

const ROUTES = [
  "/visual/?fixture=agent&state=narrative&theme=light",
  "/visual/?fixture=agent&state=question&theme=light",
] as const;

const ROUND = "round";
const SQUIRCLE = "superellipse(1.5)";

test("speech is round; everything else, including a card in the transcript, is a squircle", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1472, height: 900 });

  const speech: { what: string; shape: string }[] = [];
  const chrome: { what: string; shape: string }[] = [];

  for (const route of ROUTES) {
    await page.goto(route);
    await page.locator("html[data-visual-ready]").waitFor();
    await page.waitForTimeout(300);

    const supported = await page.evaluate(() =>
      typeof CSS !== "undefined" && typeof CSS.supports === "function"
        ? CSS.supports("corner-shape", "superellipse(1.5)")
        : false,
    );
    expect(supported, "this audit is meaningless in an engine without `corner-shape`").toBe(true);

    const measured = await page.evaluate(() => {
      const shape = (element: Element) =>
        getComputedStyle(element).getPropertyValue("corner-shape");
      const named = (node: Element, what: string) => ({ what, shape: shape(node) });

      return {
        speech: [...document.querySelectorAll("[data-user-message-bubble]")].map((node) =>
          named(node, "user bubble"),
        ),
        cards: [
          ...document.querySelectorAll(
            '[data-slot="approval-surface"],[data-slot="question-request-surface"]',
          ),
        ].map((node) => named(node, `card ${node.getAttribute("data-slot")}`)),
        buttons: [...document.querySelectorAll('[data-slot="button"]')]
          .filter((node) => !node.closest("[data-fixture-chrome]"))
          .filter((node) => {
            const box = node.getBoundingClientRect();
            const radius = Number.parseFloat(getComputedStyle(node).borderTopLeftRadius);
            return Number.isFinite(radius) && radius < Math.min(box.width, box.height) / 2 - 0.5;
          })
          .slice(0, 6)
          .map((node) => named(node, `button "${(node.textContent ?? "").trim().slice(0, 16)}"`)),
      };
    });

    speech.push(...measured.speech);
    chrome.push(...measured.cards, ...measured.buttons);
  }

  expect(speech.length, "the routes have to render a user message").toBeGreaterThan(0);
  expect(
    chrome.filter((one) => one.what.startsWith("card ")).length,
    "the routes have to render an approval or question card",
  ).toBeGreaterThan(0);
  expect(chrome.length, "the routes have to render controls").toBeGreaterThan(4);

  expect(
    speech.filter((one) => one.shape !== ROUND),
    "speech drawn as chrome",
  ).toEqual([]);
  expect(
    chrome.filter((one) => one.shape !== SQUIRCLE),
    "chrome that fell off the corner language",
  ).toEqual([]);
});
