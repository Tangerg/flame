import { expect, test } from "./test";

// The product draws two corners, and which one a box gets is a statement about what it IS.
//
// `globals.css` puts `corner-shape: superellipse(1.5)` on everything, which is right for
// chrome: a curve that holds more material near the corner reads as machined, and every
// control, card, panel and row in the app wants that. It is wrong for speech. A block someone
// typed, drawn with a squircle, reads as another panel in the toolbar — so the transcript's
// planes opt back out, the same way the pill step does and for the same reason.
//
// Measured against the reference desktop agent client, which arrived at the identical split
// from the other direction: it applies its squircle per-component rather than universally, and
// its two message-bubble components BOTH override it back — one spelled `round`, the other
// `superellipse(1)`. Two independent opt-outs in one codebase is a decision, not an oversight.
//
// The line is drawn at SPEECH, not at the transcript. An approval card and a question card sit
// in the same column and take the same radius, but they carry controls and are answered rather
// than read — chrome that arrived in the conversation, and they keep the squircle. The
// reference client agrees: the cards in its thread take its squircle token, and only its two
// message-bubble components override it.
//
// That distinction is the reason this audit samples all three groups. The first version asserted
// only "speech is round, controls are squircles", and a change that over-applied the round
// corner to both cards passed it — the regression was caught by four golden screenshots
// instead, which report a pixel count rather than a reason.
//
// The universal rule is `:where()`, so it carries no specificity and any call site that has
// decided on a shape keeps it. That is also why this can regress silently: a new transcript
// plane that takes `radius.bubble` instead of `corner.bubble` gets the squircle without a word.
const ROUTES = [
  "/visual/?fixture=agent&state=narrative&theme=light",
  "/visual/?fixture=agent&state=question&theme=light",
] as const;

/** What the engine reports for a box that opted out, and for one that did not. */
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

    // The guard cannot be allowed to pass vacuously. In an engine with no `corner-shape` every
    // box reports the same empty string, the groups below agree trivially, and the audit
    // reports green about a distinction it never measured. WebKit is exactly that engine as of
    // Safari 26.5 — which is what the desktop app ships in — so this is not hypothetical.
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
        // The two planes that share the bubble's RADIUS and must not share its shape. Named by
        // their slots rather than found by radius, so this asks about the cards themselves.
        cards: [
          ...document.querySelectorAll(
            '[data-slot="approval-surface"],[data-slot="question-request-surface"]',
          ),
        ].map((node) => named(node, `card ${node.getAttribute("data-slot")}`)),
        // Controls the same screen is already showing — minus the ones on the PILL step, which
        // are round on purpose and by the same argument: a superellipse at pill radius is a
        // rounded square rather than a circle. Measured: without this filter the route's two
        // icon buttons read `round` and the audit called them defects. A pill is recognised by
        // its geometry rather than by a class, because that is what "pill" means — the radius
        // has reached half the shorter side and stopped.
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

  // Floors, not targets: a sweep that reached neither group agrees with any product. The card
  // floor is separate because the cards are the half this audit exists to hold — a route that
  // stopped rendering them would otherwise quietly narrow it back to its first version.
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
