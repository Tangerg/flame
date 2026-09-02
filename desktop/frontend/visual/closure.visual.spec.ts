import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Browser, type Page } from "./test";
import { DEFAULT_MOTION } from "@/lib/appearance";
import { VISUAL_AGENT_STATES } from "./agentSessionSnapshots";
import { VISUAL_WORK_INDEX_STATES } from "./shellFixtureStates";
import { VISUAL_WORKSPACE_STATES, VISUAL_WORKSPACE_VIEWPORT } from "./workspaceFixtureStates";
import { en } from "@/lib/i18n/locales/en";

// Named from the catalogue, not copied out of it. This string had seven literal copies
// across three spec files, so changing one character of the copy broke five tests that
// have nothing to do with the copy.
const SETTINGS_SEARCH = { name: en["settings.searchPlaceholder"]! };

const VISUAL_URL = "http://127.0.0.1:4174/visual/";
const WCAG_TAGS = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"] as const;

interface FixtureRoute {
  fixture: "agent" | "shell" | "workspace";
  state: string;
  theme?: "light" | "dark";
  motion?: "full";
  fontSize?: number;
}

async function openFixture(page: Page, route: FixtureRoute): Promise<void> {
  const viewport = page.viewportSize();
  if (
    route.fixture === "workspace" &&
    route.state !== "settings" &&
    viewport !== null &&
    viewport.width < VISUAL_WORKSPACE_VIEWPORT.width
  ) {
    await page.setViewportSize({ width: VISUAL_WORKSPACE_VIEWPORT.width, height: viewport.height });
  }

  const query = new URLSearchParams({
    fixture: route.fixture,
    state: route.state,
    theme: route.theme ?? "light",
  });
  if (route.motion) query.set("motion", route.motion);
  if (route.fontSize !== undefined) query.set("font-size", String(route.fontSize));

  await page.goto(`${VISUAL_URL}?${query}`);
  await page.locator("html[data-visual-ready]").waitFor();

  if (route.fixture === "agent" && route.state === "long-content") {
    await expect(page.locator(".shiki-block .shiki")).toHaveCount(3);
    await expect(page.getByRole("img", { name: "Diagram" })).toBeVisible();
    const transcript = page.locator(".msg-scroll > .panel-scroll");
    // Shiki grows after the initial instant scroll. Production's smooth resize
    // reaches the tail, but Chromium may settle one physical pixel short after
    // a warmed full-suite run. Prove it followed correctly, then canonicalize
    // the screenshot boundary to the exact tail so a rounding residue cannot
    // shift the entire transcript raster by one pixel.
    await expect
      .poll(() =>
        transcript.evaluate(
          (element) => element.scrollHeight - element.clientHeight - element.scrollTop,
        ),
      )
      .toBeLessThanOrEqual(1);
    await transcript.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
    await expect
      .poll(() =>
        transcript.evaluate(
          (element) => element.scrollHeight - element.clientHeight - element.scrollTop,
        ),
      )
      .toBe(0);
  }
  if (route.fixture === "shell" && route.state === "populated") {
    await expect(
      page.getByRole("complementary", { name: "Work index" }).getByRole("button", {
        name: "scope 6",
      }),
    ).toBeVisible();
  }
  if (route.fixture === "workspace" && route.state === "dock-review") {
    await expect(page.locator("[data-diff-file]")).toHaveCount(2);
    await page.locator('[data-diff-file] span[style*="color"]').first().waitFor();
  }
  if (route.fixture === "workspace" && route.state === "settings") {
    await expect(page.getByRole("heading", { name: "Appearance" })).toBeVisible();
    // The heading belongs to the host and precedes the lazy pane body. Waiting
    // for a control owned by Appearance proves the chunk has resolved before
    // accessibility checks or screenshots inspect the page.
    await expect(page.getByRole("button", { name: en["settings.theme"]! })).toBeVisible();
  }
}

function pageHorizontalOverflow(page: Page): Promise<number> {
  return page.locator("html").evaluate((element) => element.scrollWidth - element.clientWidth);
}

// Every declared state, in both schemes, derived from the lists the fixtures
// themselves export — not a hand-picked sample. The sample this replaced named nine
// routes and left eighteen states unvisited, and the states it skipped were the ones
// holding the newest surfaces: every progress bar in the app was an unnamed
// `progressbar` (a serious WCAG failure) and the only state that renders one was not
// on the list. A sample cannot be kept honest by hand, because the thing it has to
// track is which component appears where, and nothing tells you when that changes.
const ACCESSIBILITY_ROUTES: readonly FixtureRoute[] = [
  ...VISUAL_AGENT_STATES.map((state) => ({ fixture: "agent" as const, state })),
  ...VISUAL_WORK_INDEX_STATES.map((state) => ({ fixture: "shell" as const, state })),
  ...VISUAL_WORKSPACE_STATES.map((state) => ({ fixture: "workspace" as const, state })),
].flatMap((route) => [
  { ...route, theme: "light" as const },
  { ...route, theme: "dark" as const },
]);

for (const route of ACCESSIBILITY_ROUTES) {
  test(`WCAG audit ${route.fixture} ${route.state} ${route.theme}`, async ({ page }) => {
    await openFixture(page, route);

    // No exclusions. Platform window controls are outside the document, while every
    // application-owned target remains inside this audit.
    const results = await new AxeBuilder({ page }).withTags([...WCAG_TAGS]).analyze();
    expect(
      results.violations,
      results.violations
        .map(
          (violation) =>
            `${violation.id}: ${violation.help}\n${violation.nodes
              .map((node) => `  ${node.target.join(" ")}: ${node.failureSummary ?? ""}`)
              .join("\n")}`,
        )
        .join("\n\n"),
    ).toEqual([]);
  });
}

test("structural panels share one spring, containment, and reduced-motion authority", async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await openFixture(page, {
    fixture: "shell",
    state: "populated",
    theme: "light",
    motion: "full",
  });

  // The drawer PANEL — `left` and `width` travel together, `visibility` is the
  // discrete third entry that waits for them.
  const drawer = page.locator(".agent-drawer");
  // Read from the style rather than copied out of it: what this asserts is that the
  // shipped duration is the one that reaches CSS, and a literal here would only assert
  // that someone updated two places at once. It did not survive the first time the
  // design value moved.
  const declared = `${DEFAULT_MOTION.drawerMs / 1000}s`;
  await expect(drawer).toHaveCSS("transition-duration", `${declared}, ${declared}, 0s`);
  await expect(drawer.locator(".agent-drawer-surface")).toHaveCSS("contain", "layout paint");
  expect(
    await drawer.evaluate((element) => getComputedStyle(element).transitionTimingFunction),
  ).toContain("linear(");

  await page.emulateMedia({ reducedMotion: "reduce" });
  await expect(drawer).toHaveCSS("transition-duration", "0.001s");

  await page.emulateMedia({ reducedMotion: "no-preference" });
  await openFixture(page, { fixture: "shell", state: "populated", theme: "light" });
  await expect(page.locator("html")).toHaveAttribute("data-motion", "off");
  await expect(page.locator(".agent-drawer")).toHaveCSS("transition-duration", "0.001s");

  // The trailing flank consumes the same token and isolates the same fixed-width
  // descendant tree. A different curve here would put the two sides of one workspace
  // back on visibly different clocks.
  await openFixture(page, {
    fixture: "workspace",
    state: "dock-light",
    theme: "light",
    motion: "full",
  });
  const dock = page.locator(".agent-context-dock");
  await expect(dock).toHaveCSS("transition-duration", `${declared}, 0s`);
  await expect(dock).toHaveCSS("contain", "layout paint");
  expect(
    await dock.evaluate((element) => getComputedStyle(element).transitionTimingFunction),
  ).toContain("linear(");
});

test("coarse pointers receive real 44px controls without overlapping hit targets", async ({
  browser,
}) => {
  const { context, page } = await closurePage(browser, {
    hasTouch: true,
    viewport: { width: 1120, height: 720 },
  });
  try {
    await openFixture(page, { fixture: "workspace", state: "dock-light" });
    expect(await page.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);

    for (const control of [
      page.getByRole("tab", { name: "Plan" }),
      page.getByRole("button", { name: "Collapse right workspace" }),
      page.getByRole("button", { name: "Attach image" }),
    ]) {
      const box = await control.boundingBox();
      if (!box) throw new Error("Coarse-pointer control has no layout box");
      expect(box.width).toBeGreaterThanOrEqual(44);
      expect(box.height).toBeGreaterThanOrEqual(44);
    }

    await openFixture(page, { fixture: "workspace", state: "settings" });
    const search = page.getByRole("searchbox", SETTINGS_SEARCH);
    const searchBox = await search.boundingBox();
    if (!searchBox) throw new Error("Settings search has no layout box");
    expect(searchBox.height).toBeGreaterThanOrEqual(44);
    expect(await pageHorizontalOverflow(page)).toBeLessThanOrEqual(0);
  } finally {
    await context.close();
  }
});

// A control that only appears when the pointer arrives has no way to appear where there is
// no pointer. Every one of these was hidden at rest and revealed on `:hover` alone, so on a
// touch screen the dock tab's close, a code block's copy and a message's actions were
// reachable by nothing at all — the markdown table had been given the exception on its own,
// which is how the rule was known and applied once.
test("a pointer-only affordance is permanently shown where there is no pointer", async ({
  browser,
}) => {
  const { context, page } = await closurePage(browser, {
    hasTouch: true,
    viewport: { width: 1120, height: 720 },
  });
  try {
    expect(await page.evaluate(() => matchMedia("(hover: none)").matches)).toBe(true);

    await openFixture(page, { fixture: "workspace", state: "dock-light" });
    const close = page.getByRole("button", { name: "Close Plan" });
    await expect.poll(() => close.evaluate((n) => getComputedStyle(n).opacity)).toBe("1");
    await expect.poll(() => close.evaluate((n) => getComputedStyle(n).visibility)).toBe("visible");

    await openFixture(page, { fixture: "agent", state: "narrative" });
    const hidden = await page.evaluate(() =>
      [...document.querySelectorAll<HTMLElement>('[data-reveal="hover"]')]
        .filter((node) => node.getClientRects().length > 0)
        .filter((node) => getComputedStyle(node).opacity !== "1")
        .map((node) => node.className),
    );
    expect(hidden).toEqual([]);
  } finally {
    await context.close();
  }
});

test("keyboard-only traversal reaches recovery, HITL, and settings actions", async ({ page }) => {
  await openFixture(page, { fixture: "shell", state: "error", theme: "light" });
  const settings = page.getByRole("button", { name: "Settings" });
  await tabTo(page, settings);
  await assertVisibleKeyboardFocus(settings);

  await openFixture(page, { fixture: "agent", state: "waiting", theme: "dark" });
  const approve = page.getByRole("button", { name: /Allow once/ });
  await tabTo(page, approve);
  await assertVisibleKeyboardFocus(approve);
  await page.keyboard.press("Enter");
  await expect(page.getByText("Approved", { exact: true })).toBeVisible();

  await openFixture(page, { fixture: "workspace", state: "settings", theme: "light" });
  const search = page.getByRole("searchbox", SETTINGS_SEARCH);
  await tabTo(page, search);
  await assertVisibleKeyboardFocus(search);
  await page.keyboard.type("Providers");
  await expect(page.getByRole("heading", { name: "Providers" })).toBeVisible();
});

// Codex keeps an activity summary inline and returns its disclosed material to the
// reading edge. A body may declare its own margin — reasoning uses one for its aside
// rule — but it must not inherit an invisible legacy gutter from the summary mark.
for (const state of ["waves", "tool-shells", "delegated", "narrative"] as const) {
  test(`a disclosed body honors its own reading-edge inset — ${state}`, async ({ page }) => {
    await openFixture(page, { fixture: "agent", state });
    for (let i = 0; i < 8; i++) {
      const shut = page.locator(
        "[data-slot='agent-activity-disclosure'] button[aria-expanded='false']",
      );
      if ((await shut.count()) === 0) break;
      await shut
        .first()
        .click({ timeout: 2000 })
        .catch(() => {});
    }

    const drift = await page.evaluate(() => {
      const out: string[] = [];
      for (const d of document.querySelectorAll<HTMLElement>(
        "[data-slot='agent-activity-disclosure']",
      )) {
        // A card groups with its fill, so its body answers to the card's padding.
        if (d.dataset.shell !== "line") continue;
        const trigger = d.querySelector("button[aria-expanded]");
        if (trigger?.getAttribute("aria-expanded") !== "true") continue;
        const label = d.querySelector<HTMLElement>("[data-slot='agent-activity-label']");
        const body = d.querySelector<HTMLElement>("[role='region']");
        if (!label || !body) continue;
        const declaredMargin = Number.parseFloat(getComputedStyle(body).marginLeft) || 0;
        const delta = Math.round(
          body.getBoundingClientRect().left - d.getBoundingClientRect().left - declaredMargin,
        );
        if (Math.abs(delta) > 1) out.push(`${label.textContent?.trim().slice(0, 24)}: ${delta}px`);
      }
      return out;
    });

    expect(drift).toEqual([]);
  });

  // …and nothing in the gutter reaches into it. The slot is one width for every row,
  // so a mark too wide for it no longer moves the label — it runs underneath it, which
  // is what a four-glyph strip did to the word beside it.
  test(`a mark stays inside its gutter — ${state}`, async ({ page }) => {
    await openFixture(page, { fixture: "agent", state });

    const collisions = await page.evaluate(() => {
      const out: string[] = [];
      for (const d of document.querySelectorAll<HTMLElement>(
        "[data-slot='agent-activity-disclosure']",
      )) {
        const trigger = d.querySelector("button[aria-expanded]");
        const children = [...(trigger?.children ?? [])];
        const mark = children.find(
          (c) => c.getAttribute("aria-hidden") !== null && c.tagName === "SPAN",
        );
        const label = children.find(
          (c) => c.getAttribute("aria-hidden") === null && c.tagName === "SPAN",
        );
        if (!mark || !label) continue;
        // The slot's own box does not grow, so an oversized mark spills out of it
        // rather than pushing anything: measure the CONTENT against the slot, and the
        // furthest thing it draws against the label.
        const spill = mark.scrollWidth - mark.clientWidth;
        const reach = Math.max(
          ...[...mark.querySelectorAll("*"), mark].map((n) => n.getBoundingClientRect().right),
        );
        const overlap = Math.round(reach - label.getBoundingClientRect().left);
        if (spill > 1 || overlap > 0) {
          out.push(`${label.textContent?.trim().slice(0, 20)}: spill=${spill} overlap=${overlap}`);
        }
      }
      return out;
    });

    expect(collisions).toEqual([]);
  });
}

// The vertical half of the same question. `truncate` clips both axes, so text set at a
// line box the height of its own font size has the glyph box's descender outside it —
// the sidebar's section labels were shaving the tail off the "j" in "Projects". A
// `line-clamp` is exempt: it cuts on purpose and says so with an ellipsis.
for (const route of ACCESSIBILITY_ROUTES.filter((r) => r.theme === "light")) {
  test(`no text is cut off vertically — ${route.fixture} ${route.state}`, async ({ page }) => {
    await page.setViewportSize({ width: 1120, height: 720 });
    await openFixture(page, { ...route, fontSize: 18 });

    const cut = await page.evaluate(() => {
      const out: string[] = [];
      for (const el of document.querySelectorAll<HTMLElement>("*")) {
        if (el.clientHeight <= 2 || el.clientWidth <= 2) continue;
        if (el.scrollHeight <= el.clientHeight + 1) continue;
        const style = getComputedStyle(el);
        if (!(style.overflowY === "hidden" || style.overflowY === "clip")) continue;
        if (style.webkitLineClamp !== "none") continue;
        if (el.closest("[inert]")) continue;
        if (!el.textContent?.trim()) continue;
        out.push(
          `${el.tagName}.${String(el.className).slice(0, 44)} ${el.clientHeight}<${el.scrollHeight}`,
        );
      }
      return out;
    });

    expect(cut).toEqual([]);
  });
}

async function horizontallyClippedText(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const out: string[] = [];
    // Something the user can SEE reaches past the edge that clips it. `scrollWidth`
    // alone cannot say that: it counts every box beyond the edge, including ones
    // deliberately parked there. The context dock rests one full measure past the
    // reading plane while hidden, so that returning is a slide rather than an
    // appearance — and that made the plane report 336px of overflow that cuts no text.
    //
    // Asking whether a visible CHILD BOX sticks out is not enough, and getting that
    // wrong would have quietly retired this whole check: the defect it was written for
    // is a `pre` whose box fits its column exactly while its text runs past the end of
    // it, so the boxes all agree and only the glyphs are gone. Measure the text.
    const visibleContentPast = (el: HTMLElement, edge: number) => {
      const boundary = el.getBoundingClientRect();
      // A nested surface can make its own long line readable, but only when it
      // owns a real horizontal scroll range and its viewport itself fits inside
      // this clipping edge. Merely spelling `overflow-x:auto` is not enough —
      // the historical review-diff bug had that declaration on a box whose
      // scrollWidth never grew, so there was still nowhere to scroll.
      const readableByNestedScroller = (subject: Element) => {
        for (
          let owner: Element | null = subject;
          owner instanceof HTMLElement && owner !== el;
          owner = owner.parentElement
        ) {
          const ownerStyle = getComputedStyle(owner);
          if (
            (ownerStyle.overflowX === "auto" || ownerStyle.overflowX === "scroll") &&
            owner.scrollWidth > owner.clientWidth + 1
          ) {
            const ownerBox = owner.getBoundingClientRect();
            if (ownerBox.left >= boundary.left - 1 && ownerBox.right <= edge + 1) return true;
          }
        }
        return false;
      };
      const crossesEdge = (box: DOMRect) => box.left < edge - 1 && box.right > edge + 1;

      for (const child of el.querySelectorAll<HTMLElement>("*")) {
        if (getComputedStyle(child).visibility === "hidden") continue;
        if (crossesEdge(child.getBoundingClientRect()) && !readableByNestedScroller(child)) {
          return true;
        }
      }
      const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
      const range = document.createRange();
      for (let node = walker.nextNode(); node; node = walker.nextNode()) {
        if (!node.nodeValue?.trim()) continue;
        const owner = node.parentElement;
        if (!owner || getComputedStyle(owner).visibility === "hidden") continue;
        range.selectNodeContents(node);
        if (crossesEdge(range.getBoundingClientRect()) && !readableByNestedScroller(owner)) {
          return true;
        }
      }
      return false;
    };
    for (const el of document.querySelectorAll<HTMLElement>("*")) {
      // A 1-2px box holds no readable text by construction — that is how a
      // screen-reader-only node is built, not a layout that ran out of room.
      if (el.clientWidth <= 2 || el.clientHeight <= 2) continue;
      if (el.scrollWidth <= el.clientWidth + 1) continue;
      const style = getComputedStyle(el);
      if (!(style.overflowX === "hidden" || style.overflowX === "clip")) continue;
      if (style.textOverflow === "ellipsis") continue;
      // The other way an edge admits it is an edge. An ellipsis does not let
      // anyone READ the missing characters either — what it does is say the
      // string did not end there — and a gradient that dissolves the text into
      // the clip says the same thing without spending three of the characters
      // it had left to say it. Matched on direction, not merely on "has a
      // mask": a mask fading some other edge, or shaping the box, is not a
      // statement about THIS overflow and must not buy an exemption from it.
      const fade = style.maskImage === "none" ? style.webkitMaskImage : style.maskImage;
      if (fade?.startsWith("linear-gradient(to right")) continue;
      if (!el.textContent?.trim()) continue;
      const box = el.getBoundingClientRect();
      const clipEdge = box.left + Number.parseFloat(style.borderLeftWidth) + el.clientWidth;
      if (!visibleContentPast(el, clipEdge)) continue;
      // No ancestor can rescue this: an ancestor scroller only ever sees this
      // element's box, and the box is where the content was cut. A descendant
      // scroller was handled above only when its own range genuinely grew.
      out.push(
        `${el.tagName}.${String(el.className).slice(0, 40)} ${el.clientWidth}<${el.scrollWidth}`,
      );
    }
    return out;
  });
}

// Text that is simply gone: clipped by its own box, with no ellipsis to say so and
// nothing in the ancestry that scrolls. Every code surface but one was like this —
// the review diff, the file view and the transcript's inline diff all set `pre`
// inside a clipped box, so any line longer than the column lost its tail silently.
// The goldens could not see it: a cut line and a short line look identical.
//
// Run at the smallest window the shell allows and the largest UI type a user can
// pick. Dock routes move to the canonical two-column viewport in `openFixture`:
// below that width the product intentionally folds the material, and the narrow
// presentation boundary has its own workspace test.
for (const route of ACCESSIBILITY_ROUTES.filter((r) => r.theme === "light")) {
  test(`no text is cut off with no way to read it — ${route.fixture} ${route.state}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1120, height: 720 });
    await openFixture(page, { ...route, fontSize: 18 });

    // Unfold what the page hides. Collapsed content is where the cutting was: the
    // transcript's inline diff only exists inside an expanded tool row, so a check
    // that measures the resting page measures none of it.
    for (let i = 0; i < 6; i++) {
      const shut = page.locator(
        "[data-slot='agent-activity-disclosure'] button[aria-expanded='false']",
      );
      const n = await shut.count();
      if (n === 0) break;
      await shut
        .first()
        .click({ timeout: 2000 })
        .catch(() => {});
    }

    const cut = await horizontallyClippedText(page);

    expect(cut).toEqual([]);
  });
}

test("maximum UI text keeps long code readable through its own horizontal scroller", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1120, height: 720 });
  await openFixture(page, {
    fixture: "agent",
    state: "long-content",
    theme: "light",
    fontSize: 18,
  });

  const code = page.locator(".shiki-block .shiki").filter({ hasText: "parent.postMessage" });
  await expect(code).toHaveCSS("overflow-x", "auto");
  expect(
    await code.evaluate((element) => element.scrollWidth - element.clientWidth),
  ).toBeGreaterThan(0);
  expect(await horizontallyClippedText(page)).toEqual([]);
});

// The mirror of the test above, and the half that was missing: every assertion here
// had been "the keyboard can reach X", never "the keyboard cannot reach what is not on
// screen". Two ways of hiding a control stop the pointer and neither stops Tab —
// transparency, and a box clipped to nothing — so a folded tool card kept its buttons
// in the tab order and a streaming run put two per message there. Nothing painted
// differently in either case, so no golden could see it.
for (const state of ["tool-shells", "running"] as const) {
  test(`keyboard traversal skips what is hidden (${state})`, async ({ page }) => {
    await openFixture(page, { fixture: "agent", state, theme: "light" });

    // Fold a disclosure that has been open, which is the only way its body mounts and
    // then goes away: content that was never revealed was never in the tab order.
    const trigger = page
      .locator("[data-slot='agent-activity-disclosure'] button[aria-expanded]")
      .first();
    if (await trigger.count()) {
      const wasOpen = (await trigger.getAttribute("aria-expanded")) === "true";
      if (!wasOpen) await trigger.click();
      await expect(trigger).toHaveAttribute("aria-expanded", "true");
      await trigger.click();
      await expect(trigger).toHaveAttribute("aria-expanded", "false");
    }

    const dead = await page.evaluate(() => {
      // Settled styles, not in-flight ones: a reveal is a transition, and computed
      // opacity one tick after focus is still the value it is animating away from.
      const freeze = document.createElement("style");
      freeze.textContent = "* { transition: none !important; }";
      document.head.append(freeze);

      const focusable =
        "a[href], button:not([disabled]), input:not([disabled]), textarea, select, [tabindex]";

      const transparent = (element: Element) => {
        for (let node: Element | null = element; node; node = node.parentElement) {
          const style = getComputedStyle(node);
          if (style.visibility === "hidden" || Number.parseFloat(style.opacity) === 0) return true;
        }
        return false;
      };

      // Clipped away rather than transparent: a collapsed disclosure keeps its body at
      // full opacity in a box with no height, so nothing about the element's own style
      // says it cannot be seen. What says so is that the pixel at its centre belongs to
      // something else.
      const clippedAway = (element: Element) => {
        const box = element.getBoundingClientRect();
        const x = box.left + box.width / 2;
        const y = box.top + box.height / 2;
        if (x < 0 || y < 0 || x > innerWidth || y > innerHeight) return false; // scrolled off
        const hit = document.elementFromPoint(x, y);
        return hit === null || !(element.contains(hit) || hit.contains(element));
      };

      const out: string[] = [];
      for (const element of document.querySelectorAll<HTMLElement>(focusable)) {
        // A negative tabIndex is programmatically focusable but not a tab stop, which
        // is how a control that hides itself is supposed to withdraw.
        if (element.tabIndex < 0) continue;
        const box = element.getBoundingClientRect();
        if (box.width === 0 && box.height === 0) continue;
        // Ask the browser about the rest: focus is refused inside `inert`,
        // `visibility: hidden` and `display: none`, which is exactly the difference
        // between withdrawing a control and merely making it invisible. A
        // hover-revealed control passes, because tabbing to it is what reveals it.
        element.focus();
        if (document.activeElement === element && (transparent(element) || clippedAway(element))) {
          out.push(element.getAttribute("aria-label") ?? element.textContent?.trim() ?? "?");
        }
        element.blur();
      }
      freeze.remove();
      return out;
    });

    expect(dead).toEqual([]);
  });
}

// A sticky header positions against the nearest ancestor that is a scroll container,
// and `overflow: hidden` makes a box one even when it can never scroll. A tool group
// folded inside a wave had landed in exactly that box, so its header stuck to a port
// with nowhere to travel — visible, correct, and doing nothing.
test("a sticky header has a scrollport that can scroll", async ({ page }) => {
  await openFixture(page, { fixture: "agent", state: "waves", theme: "light" });

  for (let i = 0; i < 4; i++) {
    const shut = page.locator(
      "[data-slot='agent-activity-disclosure'] button[aria-expanded='false']",
    );
    if ((await shut.count()) === 0) break;
    await shut.first().click();
  }

  const stranded = await page.evaluate(() =>
    [...document.querySelectorAll<HTMLElement>("*")]
      .filter((element) => getComputedStyle(element).position === "sticky")
      .filter((element) => {
        for (let node = element.parentElement; node; node = node.parentElement) {
          if (!/(auto|scroll|hidden)/.test(getComputedStyle(node).overflowY)) continue;
          return node.scrollHeight <= node.clientHeight + 1;
        }
        return false;
      })
      .map((element) => element.className),
  );

  expect(stranded).toEqual([]);
});

// A hidden-until-hover affordance revealed by `:focus-within` never goes away again:
// clicking a row focuses it, and DOM focus outlives the pointer. One row in a column of
// identical rows then stays lit with nothing on screen saying why. Codex reveals these on
// `:focus-visible`, so a mouse click leaves no residue while Tab still reaches them.
test("a hover affordance does not stay lit after the pointer leaves", async ({ page }) => {
  await openFixture(page, { fixture: "agent", state: "waves", theme: "light" });

  const header = page
    .locator("[data-slot='agent-activity-disclosure'] button[aria-expanded]")
    .first();
  await header.click();
  await header.click();
  await expect(header).toHaveAttribute("aria-expanded", "false");
  // Park the pointer somewhere with no row under it, then let the transition finish.
  await page.mouse.move(4, 4);

  const chevron = header.locator("[data-slot='agent-activity-chevron']");
  await expect.poll(() => chevron.evaluate((node) => getComputedStyle(node).opacity)).toBe("0");

  // …and Tab must still bring it back, or the fix has traded a stuck affordance for a
  // keyboard-invisible one. Walked with real key presses: `focus()` inherits whatever
  // modality came last, so it cannot tell the two apart.
  // Drop the focus the click left behind first: without this the row is ALREADY the active
  // element, the loop presses nothing, and the assertion passes on mouse focus.
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
  const focused = () => header.evaluate((node) => node === document.activeElement);
  for (let step = 0; step < 60 && !(await focused()); step += 1) {
    await page.keyboard.press("Tab");
  }
  expect(await focused()).toBe(true);
  await expect.poll(() => chevron.evaluate((node) => getComputedStyle(node).opacity)).toBe("1");
});

test("IME composition keeps Enter inside the composer until text is committed", async ({
  page,
}) => {
  await openFixture(page, { fixture: "agent", state: "steer", theme: "light" });
  const composer = page.getByRole("textbox", { name: "Message composer" });
  await composer.focus();

  await composer.evaluate((element) => {
    const textarea = element as HTMLTextAreaElement;
    textarea.dispatchEvent(new CompositionEvent("compositionstart", { bubbles: true, data: "ni" }));
    const setValue = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set;
    setValue?.call(textarea, "你");
    textarea.dispatchEvent(
      new InputEvent("input", {
        bubbles: true,
        data: "你",
        inputType: "insertCompositionText",
        isComposing: true,
      }),
    );
    textarea.dispatchEvent(
      new KeyboardEvent("keydown", {
        bubbles: true,
        cancelable: true,
        isComposing: true,
        key: "Enter",
      }),
    );
  });

  await expect(composer).toHaveValue("你");
  await expect(page.locator("html")).not.toHaveAttribute("data-visual-sent-input");
  await composer.evaluate((element) => {
    element.dispatchEvent(new CompositionEvent("compositionend", { bubbles: true, data: "你" }));
  });
  await expect(composer).toHaveValue("你");
});

test("Chinese IME Latin commit does not turn its plain Enter into send", async ({ page }) => {
  await openFixture(page, { fixture: "agent", state: "steer", theme: "light" });
  const composer = page.getByRole("textbox", { name: "Message composer" });
  await composer.focus();

  await composer.evaluate((element) => {
    const textarea = element as HTMLTextAreaElement;
    textarea.dispatchEvent(
      new CompositionEvent("compositionstart", { bubbles: true, data: "english" }),
    );
    const setValue = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set;
    setValue?.call(textarea, "中文 english");
    textarea.dispatchEvent(
      new InputEvent("input", {
        bubbles: true,
        data: "english",
        inputType: "insertCompositionText",
        isComposing: true,
      }),
    );
    textarea.dispatchEvent(
      new CompositionEvent("compositionend", { bubbles: true, data: "english" }),
    );
  });
  await expect(composer).toHaveValue("中文 english");

  await composer.evaluate((element) => {
    const commitEnter = new KeyboardEvent("keydown", {
      bubbles: true,
      cancelable: true,
      isComposing: false,
      key: "Enter",
    });
    Object.defineProperty(commitEnter, "keyCode", { value: 13 });
    element.dispatchEvent(commitEnter);
    element.dispatchEvent(
      new KeyboardEvent("keyup", {
        bubbles: true,
        isComposing: false,
        key: "Enter",
      }),
    );
  });

  await expect(composer).toHaveValue("中文 english");
  await expect(page.locator("html")).not.toHaveAttribute("data-visual-sent-input");

  await composer.press("Enter");
  await expect(page.locator("html")).toHaveAttribute("data-visual-sent-input", /中文 english/);
});

test("message copy writes through the production clipboard path", async ({ context, page }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"], {
    origin: "http://127.0.0.1:4174",
  });
  await openFixture(page, {
    fixture: "agent",
    state: "long-content",
    theme: "light",
  });

  const response = page.locator(".msg-content").filter({
    hasText: "The consumer owns persistence policy and transaction scope.",
  });
  await response.click({ button: "right" });
  await page.getByRole("menuitem", { name: "Copy markdown" }).click();

  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toContain("The consumer owns persistence policy and transaction scope.");
});

for (const theme of ["light", "dark"] as const) {
  test(`maximum UI text remains readable without horizontal clipping ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await openFixture(page, {
      fixture: "agent",
      state: "long-content",
      theme,
      fontSize: 18,
    });

    await expect(page.locator("body")).toHaveCSS("font-size", "18px");
    expect(await pageHorizontalOverflow(page)).toBeLessThanOrEqual(0);
    await expect(page.getByRole("textbox", { name: "Message composer" })).toBeVisible();
    await expect(page).toHaveScreenshot(`closure-${theme}-agent-long-font18-1280x800.png`);

    await openFixture(page, { fixture: "workspace", state: "settings", theme, fontSize: 18 });
    await expect(page.locator("body")).toHaveCSS("font-size", "18px");
    expect(await pageHorizontalOverflow(page)).toBeLessThanOrEqual(0);
    await expect(page.getByRole("searchbox", SETTINGS_SEARCH)).toBeVisible();
    await expect(page).toHaveScreenshot(`closure-${theme}-settings-font18-1280x800.png`);
  });
}

for (const theme of ["light", "dark"] as const) {
  test(`Retina closure ${theme}`, async ({ browser }) => {
    const { context, page } = await closurePage(browser, {
      deviceScaleFactor: 2,
      viewport: VISUAL_WORKSPACE_VIEWPORT,
    });
    try {
      await openFixture(page, { fixture: "agent", state: "waiting", theme });
      expect(await page.evaluate(() => devicePixelRatio)).toBe(2);
      await expect(page).toHaveScreenshot(`closure-${theme}-agent-waiting-retina.png`);

      await openFixture(page, { fixture: "workspace", state: "dock-review", theme });
      await expect(page).toHaveScreenshot(`closure-${theme}-workspace-review-retina.png`);
    } finally {
      await context.close();
    }
  });
}

async function closurePage(
  browser: Browser,
  overrides: {
    deviceScaleFactor?: number;
    hasTouch?: boolean;
    viewport: { width: number; height: number };
  },
) {
  const context = await browser.newContext({
    colorScheme: "light",
    deviceScaleFactor: overrides.deviceScaleFactor ?? 1,
    hasTouch: overrides.hasTouch,
    locale: "en-US",
    reducedMotion: "reduce",
    timezoneId: "UTC",
    viewport: overrides.viewport,
  });
  return { context, page: await context.newPage() };
}

async function tabTo(page: Page, target: ReturnType<Page["locator"]>, limit = 80): Promise<void> {
  for (let index = 0; index < limit; index += 1) {
    if (await target.evaluate((element) => element === document.activeElement)) return;
    await page.keyboard.press("Tab");
  }
  throw new Error(`Keyboard traversal did not reach ${await target.getAttribute("aria-label")}`);
}

async function assertVisibleKeyboardFocus(target: ReturnType<Page["locator"]>): Promise<void> {
  const style = await target.evaluate((element) => {
    const computed = getComputedStyle(element);
    return {
      backgroundColor: computed.backgroundColor,
      outlineStyle: computed.outlineStyle,
      outlineWidth: computed.outlineWidth,
    };
  });
  expect(
    style.outlineStyle !== "none" ||
      style.outlineWidth !== "0px" ||
      style.backgroundColor !== "rgba(0, 0, 0, 0)",
  ).toBe(true);
}

// The audit above visits every declared state, but only as first rendered. A menu, a picker
// or a dialog is not in the document until someone opens it, so its subtree was never
// audited — and both defects this found were in one: 12px metadata at 3.7:1 because an
// opacity was stacked on a token that already carries the faint step, and a 16px-tall target
// that passed the size rule only while nothing was near enough to fail the spacing exception.
const OVERLAYS: ReadonlyArray<{
  readonly label: string;
  readonly route: FixtureRoute;
  readonly open: string;
}> = [
  {
    label: "approval mode menu",
    route: { fixture: "agent", state: "idle" },
    open: "Approval mode",
  },
  { label: "model picker", route: { fixture: "agent", state: "idle" }, open: "Switch model" },
  {
    label: "reasoning effort menu",
    route: { fixture: "agent", state: "idle" },
    open: "Switch reasoning effort",
  },
];

for (const overlay of OVERLAYS) {
  for (const theme of ["light", "dark"] as const) {
    test(`WCAG audit ${overlay.label} ${theme}`, async ({ page }) => {
      await openFixture(page, { ...overlay.route, theme });
      const trigger = page.getByRole("button", { name: overlay.open }).first();
      await trigger.click();
      // The popup is portalled, so wait for it rather than for the trigger's own state.
      const popup = page.locator('[role="menu"], [role="dialog"], [role="listbox"]').first();
      await expect(popup).toBeVisible();
      // …and then for it to finish arriving. A floating surface fades in from opacity 0, and
      // `toBeVisible` is satisfied the moment it has layout — Axe would sample a translucent
      // element against whatever is behind it and report a contrast the design never had.
      await expect.poll(() => popup.evaluate((node) => getComputedStyle(node).opacity)).toBe("1");

      const results = await new AxeBuilder({ page }).withTags([...WCAG_TAGS]).analyze();
      expect(
        results.violations,
        results.violations
          .map(
            (violation) =>
              `${violation.id}: ${violation.help}\n${violation.nodes
                .map((node) => `  ${node.target.join(" ")}: ${node.failureSummary ?? ""}`)
                .join("\n")}`,
          )
          .join("\n\n"),
      ).toEqual([]);
    });
  }
}

// WCAG 2.2 target size, asserted on the geometry rather than through a popup that happens to
// land nearby: the spacing exception made a 16px-tall control pass until something moved next
// to it. A control carrying text is the one that must meet the floor on its own — an
// icon-only button sits in a row that spaces it.
test("a text-bearing control meets the minimum target size", async ({ page }) => {
  await openFixture(page, { fixture: "workspace", state: "dock-light" });
  const summary = page.locator('[data-goal="summary"]');
  await expect(summary).toBeVisible();
  expect(
    await summary.evaluate((el) => Math.round(el.getBoundingClientRect().height)),
  ).toBeGreaterThanOrEqual(24);
});

// The SMALLEST UI size is where a control whose box is only its text line falls under the
// minimum — it keeps shrinking with the type, and half a pixel short still fails.
for (const route of ACCESSIBILITY_ROUTES.filter((candidate) => candidate.theme === "light")) {
  test(`WCAG audit ${route.fixture} ${route.state} at the smallest UI size`, async ({ page }) => {
    await openFixture(page, { ...route, fontSize: 11 });
    const results = await new AxeBuilder({ page }).withTags([...WCAG_TAGS]).analyze();
    expect(
      results.violations,
      results.violations
        .map(
          (violation) =>
            `${violation.id}: ${violation.help}\n${violation.nodes
              .map((node) => `  ${node.target.join(" ")}: ${node.failureSummary ?? ""}`)
              .join("\n")}`,
        )
        .join("\n\n"),
    ).toEqual([]);
  });
}

// The transcript scrolls BEHIND a translucent composer, so whatever the composer covers has
// to be masked out — otherwise a card's buttons read straight through the input surface. The
// mask is anchored to `--composer-overlay`, so the invariant is that the measured overlay
// covers the whole overlap; a mask anchored to the viewport's own edge does not.
for (const state of ["narrative", "long-content", "tool-shells"] as const) {
  test(`nothing shows through the composer in ${state}`, async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await openFixture(page, { fixture: "agent", state });

    const geometry = await page.evaluate(() => {
      const viewport = document.querySelector(".msg-scroll-viewport");
      const composer = document.querySelector(".agent-composer-glass");
      if (!viewport || !composer) return null;
      const style = getComputedStyle(viewport);
      // The LAST stop is where the transcript has finished fading. Read it rather than the
      // custom property: the property being right proves nothing if the mask ignores it.
      const stops = [...style.maskImage.matchAll(/calc\(100% - ([\d.]+)px\)|\b(100)%\)/g)];
      const last = stops.at(-1);
      return {
        overlap: viewport.getBoundingClientRect().bottom - composer.getBoundingClientRect().top,
        overlay: Number.parseFloat(style.getPropertyValue("--composer-overlay")),
        fadesOutAt: last?.[1] === undefined ? 0 : Number.parseFloat(last[1]),
      };
    });

    expect(geometry).not.toBeNull();
    // Non-vacuous: the composer really does cover part of the transcript here.
    expect(geometry!.overlap).toBeGreaterThan(24);
    expect(geometry!.overlay).toBeGreaterThanOrEqual(geometry!.overlap);
    expect(geometry!.fadesOutAt).toBeCloseTo(geometry!.overlay, 0);
  });
}
