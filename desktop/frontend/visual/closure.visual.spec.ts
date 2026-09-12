import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Browser, type Page } from "./test";
import { DEFAULT_MOTION } from "@/lib/appearance";
import { VISUAL_AGENT_STATES } from "./agentSessionSnapshots";
import {
  VISUAL_SHELL_OVERLAYS,
  VISUAL_WORK_INDEX_STATES,
  type VisualShellOverlay,
} from "./shellFixtureStates";
import {
  VISUAL_SETTINGS_PANES,
  VISUAL_WORKSPACE_STATES,
  VISUAL_WORKSPACE_VIEWPORT,
  type VisualSettingsPane,
} from "./workspaceFixtureStates";
import { en } from "@/lib/i18n/locales/en";

const SETTINGS_SEARCH = { name: en["settings.searchPlaceholder"]! };

const VISUAL_URL = "http://127.0.0.1:4174/visual/";
const WCAG_TAGS = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"] as const;

interface WcagException {
  readonly rule: string;
  readonly within: string;
  readonly because: string;
}

async function expectNoWcagViolations(
  page: Page,
  exceptions: readonly WcagException[] = [],
): Promise<void> {
  await settleByCount(page, 'button, input, textarea, [role="tab"]');
  const raw = await new AxeBuilder({ page }).withTags([...WCAG_TAGS]).analyze();
  const excused = await Promise.all(
    raw.violations.map(async (violation) => {
      const match = exceptions.find((exception) => exception.rule === violation.id);
      if (!match) return violation;
      const outside = [];
      for (const node of violation.nodes) {
        const contained = await page.evaluate(
          ([selector, within]) => document.querySelector(selector)?.closest(within) !== null,
          [node.target.join(" "), match.within] as const,
        );
        if (!contained) outside.push(node);
      }
      return outside.length === 0 ? null : { ...violation, nodes: outside };
    }),
  );
  const violations = excused.filter((violation) => violation !== null);
  expect(
    violations,
    violations
      .map(
        (violation) =>
          `${violation.id}: ${violation.help}\n${violation.nodes
            .map((node) => `  ${node.target.join(" ")}: ${node.failureSummary ?? ""}`)
            .join("\n")}`,
      )
      .join("\n\n"),
  ).toEqual([]);
}

interface FixtureRoute {
  fixture: "agent" | "shell" | "workspace";
  state: string;
  pane?: VisualSettingsPane;
  theme?: "light" | "dark";
  motion?: "full";
  fontSize?: number;
  density?: "compact" | "comfortable" | "spacious";
  overlay?: VisualShellOverlay;
  locale?: string;
  contrast?: number;
  accent?: string;
  custom?: { readonly bg: string; readonly fg: string };
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
  if (route.overlay) query.set("overlay", route.overlay);
  if (route.pane) query.set("pane", route.pane);
  if (route.locale) query.set("locale", route.locale);
  if (route.density) query.set("density", route.density);
  if (route.contrast !== undefined) query.set("contrast", String(route.contrast));
  if (route.accent) query.set("accent", route.accent);
  if (route.custom) {
    query.set("custom-bg", route.custom.bg);
    query.set("custom-fg", route.custom.fg);
  }

  await page.goto(`${VISUAL_URL}?${query}`);
  await page.locator("html[data-visual-ready]").waitFor();

  if (route.fixture === "agent" && route.state === "long-content") {
    await expect(page.locator(".shiki-block .shiki")).toHaveCount(3);
    await expect(page.getByRole("img", { name: "Diagram" })).toBeVisible();
    const transcript = page.locator(".msg-scroll-viewport");
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
      page.getByRole("complementary").getByRole("button", { name: /\b6\b/ }).first(),
    ).toBeVisible();
  }
  if (route.fixture === "workspace" && route.state === "dock-review") {
    await expect(page.locator("[data-diff-file]")).toHaveCount(2);
    await page.locator('[data-diff-file] span[style*="color"]').first().waitFor();
  }
  if (route.fixture === "workspace" && route.state === "settings") {
    await expect(page.getByRole("heading").first()).toBeVisible();
    await expect(page.locator('main section [aria-busy="true"]')).toHaveCount(0);
  }
}

async function settleByCount(page: Page, selector: string): Promise<void> {
  const STABLE_READINGS = 4;
  let previous = -1;
  let stable = 0;
  for (let attempt = 0; attempt < 40; attempt += 1) {
    const count = await page.locator(selector).count();
    stable = count === previous ? stable + 1 : 0;
    previous = count;
    if (count > 0 && stable >= STABLE_READINGS) return;
    await page.waitForTimeout(120);
  }
}

function pageHorizontalOverflow(page: Page): Promise<number> {
  return page.locator("html").evaluate((element) => element.scrollWidth - element.clientWidth);
}

const ACCESSIBILITY_ROUTES: readonly FixtureRoute[] = [
  ...VISUAL_AGENT_STATES.map((state) => ({ fixture: "agent" as const, state })),
  ...VISUAL_WORK_INDEX_STATES.map((state) => ({ fixture: "shell" as const, state })),
  ...VISUAL_WORKSPACE_STATES.map((state) => ({ fixture: "workspace" as const, state })),
  ...VISUAL_SHELL_OVERLAYS.map((overlay) => ({
    fixture: "shell" as const,
    state: "populated",
    overlay,
  })),
].flatMap((route) => [
  { ...route, theme: "light" as const },
  { ...route, theme: "dark" as const },
]);

for (const route of ACCESSIBILITY_ROUTES) {
  test(`WCAG audit ${route.fixture} ${route.overlay ?? route.state} ${route.theme}`, async ({
    page,
  }) => {
    await openFixture(page, route);

    await expectNoWcagViolations(page);
  });
}

for (const pane of VISUAL_SETTINGS_PANES) {
  for (const theme of ["light", "dark"] as const) {
    test(`WCAG audit settings pane ${pane} ${theme}`, async ({ page }) => {
      await openFixture(page, { fixture: "workspace", state: "settings", theme, pane });

      await expectNoWcagViolations(page);
    });
  }
}

const CUSTOM_PALETTES = [
  { bg: "#ffffff", fg: "#000000" },
  { bg: "#1d1f23", fg: "#e3e5e9" },
  { bg: "#f5f0e8", fg: "#2b2a26" },
  { bg: "#101820", fg: "#c8d0d8" },
] as const;

for (const custom of CUSTOM_PALETTES) {
  test(`a custom palette's ink reads on its own surface ${custom.bg}`, async ({ page }) => {
    await openFixture(page, { fixture: "agent", state: "narrative", custom });

    const rungs = await page.evaluate(() => {
      const luminance = (css: string) => {
        const canvas = document.createElement("canvas");
        canvas.width = canvas.height = 1;
        const context = canvas.getContext("2d")!;
        context.fillStyle = css;
        context.fillRect(0, 0, 1, 1);
        const [r, g, b] = context.getImageData(0, 0, 1, 1).data;
        return [r, g, b]
          .map((value) => {
            const scaled = (value ?? 0) / 255;
            return scaled <= 0.03928 ? scaled / 12.92 : ((scaled + 0.055) / 1.055) ** 2.4;
          })
          .reduce((total, channel, index) => total + [0.2126, 0.7152, 0.0722][index]! * channel, 0);
      };
      const style = getComputedStyle(document.documentElement);
      const authored = (name: string) => style.getPropertyValue(name).trim();
      const ratio = (one: string, two: string) => {
        const [lighter, darker] = [luminance(one), luminance(two)].sort((a, b) => b - a);
        return (lighter! + 0.05) / (darker! + 0.05);
      };
      const surface = authored("--color-surface");
      return {
        soft: ratio(surface, authored("--color-text-soft")),
        muted: ratio(surface, authored("--color-text-muted")),
        faint: ratio(surface, authored("--color-text-faint")),
      };
    });

    for (const [rung, measured] of Object.entries(rungs)) {
      expect(measured, `${rung} ink on a custom surface`).toBeGreaterThanOrEqual(4.5);
    }
  });
}

const ACCENTS = ["#ffe066", "#a8e6a3", "#00b3ff", "#111111", "#f5c2e7"] as const;

for (const accent of ACCENTS) {
  for (const theme of ["light", "dark"] as const) {
    test(`ink stays legible on the accent ${accent} ${theme}`, async ({ page }) => {
      await openFixture(page, { fixture: "agent", state: "narrative", theme, accent });

      const pairs = await page.evaluate(() => {
        const style = getComputedStyle(document.documentElement);
        const value = (name: string) => style.getPropertyValue(name).trim();
        const channels = (css: string) => {
          const context = document.createElement("canvas").getContext("2d")!;
          context.fillStyle = css;
          const packed = Number.parseInt((context.fillStyle as string).slice(1), 16);
          return [(packed >> 16) & 255, (packed >> 8) & 255, packed & 255];
        };
        const luminance = (css: string) =>
          channels(css)
            .map((channel) => {
              const scaled = channel / 255;
              return scaled <= 0.03928 ? scaled / 12.92 : ((scaled + 0.055) / 1.055) ** 2.4;
            })
            .reduce(
              (total, channel, index) => total + [0.2126, 0.7152, 0.0722][index]! * channel,
              0,
            );
        const ratio = (one: string, two: string) => {
          const [lighter, darker] = [luminance(one), luminance(two)].sort((a, b) => b - a);
          return (lighter! + 0.05) / (darker! + 0.05);
        };
        return {
          mark: ratio(value("--color-accent"), value("--color-text-on-accent")),
          label: ratio(value("--color-cta"), value("--color-cta-text")),
        };
      });

      expect(pairs.mark, "a mark on the accent").toBeGreaterThanOrEqual(3);
      expect(pairs.label, "a button's label on the CTA fill").toBeGreaterThanOrEqual(4.5);
    });
  }
}

for (const contrast of [0, 100]) {
  for (const theme of ["light", "dark"] as const) {
    test(`WCAG audit the workspace at contrast ${contrast} ${theme}`, async ({ page }) => {
      await openFixture(page, { fixture: "workspace", state: "dock-light", theme, contrast });

      await expectNoWcagViolations(page, [RAIL_TARGET_SIZE]);
    });

    test(`WCAG audit the work index at contrast ${contrast} ${theme}`, async ({ page }) => {
      await openFixture(page, { fixture: "shell", state: "populated", theme, contrast });

      await expectNoWcagViolations(page, [RAIL_TARGET_SIZE]);
    });
  }
}

const RAIL_TARGET_SIZE: WcagException = {
  rule: "target-size",
  within: "nav[aria-label]",
  because:
    "a minimap tick is a pointer shortcut to a turn whose content is reachable on its own; see the test that proves each turn is",
};

const WIDE_VIEWPORT = { width: 1472, height: 900 } as const;

for (const state of VISUAL_AGENT_STATES) {
  test(`WCAG audit agent ${state} at a width that mounts the turn rail`, async ({ page }) => {
    await page.setViewportSize({ ...WIDE_VIEWPORT });
    await openFixture(page, { fixture: "agent", state });

    await expectNoWcagViolations(page, [RAIL_TARGET_SIZE]);
  });
}

test("every turn the rail points at is reachable without the rail", async ({ page }) => {
  await page.setViewportSize({ ...WIDE_VIEWPORT });
  await openFixture(page, { fixture: "agent", state: "narrative" });

  const counted = await page.evaluate(() => {
    const rail = document.querySelector("nav[aria-label]");
    const ticks = rail?.querySelectorAll("button").length ?? 0;
    const turnsWithOwnControls = [...document.querySelectorAll("[data-turn-id]")].filter((turn) =>
      turn.querySelector('button, [tabindex]:not([tabindex="-1"])'),
    ).length;
    return {
      ticks,
      turnsWithOwnControls,
      railMounted: (rail as HTMLElement | null)?.offsetParent !== null,
    };
  });

  expect(
    counted.railMounted,
    "this width has to mount the rail, or the exception is untested",
  ).toBe(true);
  expect(counted.ticks, "the rail has to be showing turns").toBeGreaterThan(0);
  expect(
    counted.turnsWithOwnControls,
    "a turn with no control of its own would leave the rail as its only route",
  ).toBeGreaterThanOrEqual(counted.ticks);
});

const INTERACTION_SURFACES: readonly {
  readonly name: string;
  readonly route: FixtureRoute;
  readonly open: (page: Page) => Promise<void>;
}[] = [
  {
    name: "the context menu on a user message",
    route: { fixture: "agent", state: "idle" },
    open: async (page) => {
      await page.locator("[data-user-message-bubble]").first().click({ button: "right" });
      await expect(page.locator('[role="menu"]')).toBeVisible();
    },
  },
  ...(["Switch model", "Approval mode", "Switch reasoning effort"] as const).map((control) => ({
    name: `the composer's ${control} popup`,
    route: { fixture: "workspace" as const, state: "dock-light" },
    open: async (page: Page) => {
      await page.getByRole("button", { name: control }).first().click();
      await expect(
        page.locator('[role="menu"], [role="listbox"], [role="dialog"]').first(),
      ).toBeVisible();
    },
  })),
  {
    name: "the dock catalogue",
    route: { fixture: "workspace", state: "dock-light" },
    open: async (page) => {
      await page
        .getByRole("button", { name: /Browse panels/ })
        .first()
        .click();
      await expect(
        page.locator('[role="menu"], [role="listbox"], [role="dialog"]').first(),
      ).toBeVisible();
    },
  },
  {
    name: "the goal editor",
    route: { fixture: "workspace", state: "dock-light" },
    open: async (page) => {
      await page
        .getByRole("button", { name: /Pursuing goal/ })
        .first()
        .click();
      await expect(page.getByRole("textbox")).toBeVisible();
    },
  },
];

for (const surface of INTERACTION_SURFACES) {
  test(`WCAG audit ${surface.name}, which a route audit never opens`, async ({ page }) => {
    await openFixture(page, surface.route);
    await surface.open(page);

    await expectNoWcagViolations(page);
  });
}

test("WCAG audit the schedules form, which a pane audit never opens", async ({ page }) => {
  for (const theme of ["light", "dark"] as const) {
    await openFixture(page, { fixture: "workspace", state: "settings", theme, pane: "schedules" });
    await page.getByRole("button", { name: /New schedule/ }).click();
    await expect(page.getByRole("textbox", { name: "Cron expression" })).toBeVisible();

    await expectNoWcagViolations(page);
  }
});

const SHIPPED_LOCALES = ["zh", "zh-TW", "ja", "ko", "es", "fr", "de"] as const;

const LOCALE_ROUTES: FixtureRoute[] = [
  { fixture: "agent", state: "waiting" },
  { fixture: "agent", state: "tool-shells" },
  { fixture: "agent", state: "question" },
  { fixture: "agent", state: "tool-search" },
  { fixture: "agent", state: "tool-remote" },
  { fixture: "agent", state: "tool-tail" },
  { fixture: "shell", state: "populated" },
  { fixture: "workspace", state: "dock-stats" },
  { fixture: "workspace", state: "settings", pane: "schedules" },
  { fixture: "workspace", state: "settings", pane: "providers" },
];

for (const locale of SHIPPED_LOCALES) {
  test(`text survives its own language — ${locale}`, async ({ page }) => {
    const clipped: string[] = [];
    for (const route of LOCALE_ROUTES) {
      for (const fontSize of [undefined, 18]) {
        await page.setViewportSize({ width: 1120, height: 720 });
        await openFixture(page, { ...route, locale, ...(fontSize ? { fontSize } : {}) });
        const where = `${route.fixture}/${route.pane ?? route.state}${fontSize ? "@18" : ""}`;
        clipped.push(
          ...(await horizontallyClippedText(page)).map((hit) => `${where} →: ${hit}`),
          ...(await verticallyClippedText(page)).map((hit) => `${where} ↓: ${hit}`),
        );
      }
    }

    expect(clipped).toEqual([]);
  });
}

for (const fixture of ["agent", "workspace"] as const) {
  const states: readonly string[] =
    fixture === "agent" ? VISUAL_AGENT_STATES : VISUAL_WORKSPACE_STATES;
  test(`no ${fixture} element carries two StyleX rules for one property`, async ({ page }) => {
    test.setTimeout(states.length * 4_000 + 20_000);
    const collisions = new Map<string, string>();
    for (const state of states) {
      await openFixture(page, { fixture, state });
      for (const found of await stylexCollisions(page)) collisions.set(found, state);
    }
    expect(
      [...collisions].map(([clash, where]) => `[${where}] ${clash}`),
      "a property declared twice on one element is decided by bundler order",
    ).toEqual([]);
  });
}

async function stylexCollisions(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const decls = new Map<string, { prop: string; value: string }[]>();
    const visit = (list: CSSRuleList) => {
      for (const rule of list) {
        if (rule instanceof CSSGroupingRule) visit(rule.cssRules);
        if (!(rule instanceof CSSStyleRule)) continue;
        const named = /^\.([A-Za-z0-9_-]+)(?::not\(#\\#\))+$/.exec(rule.selectorText);
        if (!named) continue;
        const authored = [
          ...(/\{([^}]*)\}/.exec(rule.cssText)?.[1] ?? "").matchAll(/([-a-z]+)\s*:\s*([^;]+)/g),
        ].map((decl) => ({ prop: decl[1]!, value: decl[2]!.trim() }));
        const expanded = [...rule.style]
          .map((prop) => ({ prop, value: rule.style.getPropertyValue(prop) }))
          .filter((decl) => decl.value !== "");
        const written = expanded.length > 0 ? expanded : authored;
        if (written.length > 0) decls.set(named[1]!, written);
      }
    };
    for (const sheet of document.styleSheets) {
      try {
        visit(sheet.cssRules);
      } catch {}
    }
    const out: string[] = [];
    for (const node of document.querySelectorAll<HTMLElement>("*")) {
      const seen = new Map<string, string>();
      for (const cls of node.classList)
        for (const decl of decls.get(cls) ?? []) {
          const prior = seen.get(decl.prop);
          if (prior !== undefined && prior !== decl.value) {
            const slot = node.dataset.slot ? `[${node.dataset.slot}]` : "";
            out.push(
              `${node.tagName.toLowerCase()}${slot} ${decl.prop}: ${prior} vs ${decl.value}`,
            );
          }
          seen.set(decl.prop, decl.value);
        }
    }
    return [...new Set(out)];
  });
}

test("every navigation rail answers the density setting", async ({ page }) => {
  const measure = async (density: "compact" | "spacious") => {
    await openFixture(page, { fixture: "workspace", state: "settings", density });
    return page.evaluate(() => {
      const token = getComputedStyle(document.documentElement).getPropertyValue(
        "--density-row-height",
      );
      const rows = [
        ...document.querySelectorAll<HTMLElement>(
          '[role="tablist"][aria-orientation="vertical"] [role="tab"], [data-slot="button"].agent-row',
        ),
      ].filter((node) => node.getClientRects().length > 0);
      return {
        token: token.trim(),
        rows: rows.length,
        heights: [...new Set(rows.map((node) => `${node.getBoundingClientRect().height}px`))],
      };
    });
  };

  const compact = await measure("compact");
  const spacious = await measure("spacious");

  expect(compact.rows).toBeGreaterThan(0);
  expect(compact.token).not.toBe(spacious.token);
  expect(compact.heights).toEqual([compact.token]);
  expect(spacious.heights).toEqual([spacious.token]);
});

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

  const drawer = page.locator(".agent-drawer");
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

for (const route of ACCESSIBILITY_ROUTES.filter((r) => r.theme === "light")) {
  test(`no text is cut off vertically — ${route.fixture} ${route.overlay ?? route.state}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1120, height: 720 });
    await openFixture(page, { ...route, fontSize: 18 });

    expect(await verticallyClippedText(page)).toEqual([]);
  });
}

async function verticallyClippedText(page: Page): Promise<string[]> {
  return page.evaluate(() => {
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
}

async function horizontallyClippedText(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const out: string[] = [];
    const visibleContentPast = (el: HTMLElement, edge: number) => {
      const boundary = el.getBoundingClientRect();
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
      const saysItStopped = (subject: Element) => {
        for (
          let owner: Element | null = subject;
          owner instanceof HTMLElement && owner !== el;
          owner = owner.parentElement
        ) {
          const s = getComputedStyle(owner);
          if (!(s.overflowX === "hidden" || s.overflowX === "clip")) continue;
          if (owner.getBoundingClientRect().right > edge + 1) continue;
          const mask = s.maskImage === "none" ? s.webkitMaskImage : s.maskImage;
          if (s.textOverflow === "ellipsis" || mask?.startsWith("linear-gradient(to right")) {
            return true;
          }
        }
        return false;
      };

      for (const child of el.querySelectorAll<HTMLElement>("*")) {
        if (getComputedStyle(child).visibility === "hidden") continue;
        if (crossesEdge(child.getBoundingClientRect()) && !readableByNestedScroller(child)) {
          return true;
        }
      }
      const screenReaderOnly = (subject: Element) => {
        for (
          let owner: Element | null = subject;
          owner instanceof HTMLElement && owner !== el;
          owner = owner.parentElement
        ) {
          if (owner.clientWidth <= 2 || owner.clientHeight <= 2) return true;
        }
        return false;
      };
      const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
      const range = document.createRange();
      for (let node = walker.nextNode(); node; node = walker.nextNode()) {
        if (!node.nodeValue?.trim()) continue;
        const owner = node.parentElement;
        if (!owner || getComputedStyle(owner).visibility === "hidden") continue;
        if (screenReaderOnly(owner)) continue;
        range.selectNodeContents(node);
        if (
          crossesEdge(range.getBoundingClientRect()) &&
          !readableByNestedScroller(owner) &&
          !saysItStopped(owner)
        ) {
          return true;
        }
      }
      return false;
    };
    for (const el of document.querySelectorAll<HTMLElement>("*")) {
      if (el.clientWidth <= 2 || el.clientHeight <= 2) continue;
      if (el.scrollWidth <= el.clientWidth + 1) continue;
      const style = getComputedStyle(el);
      if (!(style.overflowX === "hidden" || style.overflowX === "clip")) continue;
      if (style.textOverflow === "ellipsis") continue;
      const fade = style.maskImage === "none" ? style.webkitMaskImage : style.maskImage;
      if (fade?.startsWith("linear-gradient(to right")) continue;
      if (!el.textContent?.trim()) continue;
      const box = el.getBoundingClientRect();
      const clipEdge = box.left + Number.parseFloat(style.borderLeftWidth) + el.clientWidth;
      if (!visibleContentPast(el, clipEdge)) continue;
      out.push(
        `${el.tagName}.${String(el.className).slice(0, 40)} ${el.clientWidth}<${el.scrollWidth}`,
      );
    }
    return out;
  });
}

for (const route of ACCESSIBILITY_ROUTES.filter((r) => r.theme === "light")) {
  test(`no text is cut off with no way to read it — ${route.fixture} ${route.overlay ?? route.state}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1120, height: 720 });
    await openFixture(page, { ...route, fontSize: 18 });

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

for (const state of ["tool-shells", "running"] as const) {
  test(`keyboard traversal skips what is hidden (${state})`, async ({ page }) => {
    await openFixture(page, { fixture: "agent", state, theme: "light" });

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

      const clippedAway = (element: Element) => {
        const box = element.getBoundingClientRect();
        const x = box.left + box.width / 2;
        const y = box.top + box.height / 2;
        if (x < 0 || y < 0 || x > innerWidth || y > innerHeight) return false;
        const hit = document.elementFromPoint(x, y);
        return hit === null || !(element.contains(hit) || hit.contains(element));
      };

      const out: string[] = [];
      for (const element of document.querySelectorAll<HTMLElement>(focusable)) {
        if (element.tabIndex < 0) continue;
        const box = element.getBoundingClientRect();
        if (box.width === 0 && box.height === 0) continue;
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

test("a hover affordance does not stay lit after the pointer leaves", async ({ page }) => {
  await openFixture(page, { fixture: "agent", state: "waves", theme: "light" });

  const header = page
    .locator("[data-slot='agent-activity-disclosure'] button[aria-expanded]")
    .first();
  await header.click();
  await header.click();
  await expect(header).toHaveAttribute("aria-expanded", "false");
  await page.mouse.move(4, 4);

  const chevron = header.locator("[data-slot='agent-activity-chevron']");
  await expect.poll(() => chevron.evaluate((node) => getComputedStyle(node).opacity)).toBe("0");

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

const OVERLAYS: ReadonlyArray<{
  readonly label: string;
  readonly route: FixtureRoute;
  readonly open: string | RegExp;
  readonly by?: "hover";
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
  {
    label: "plan step list",
    route: { fixture: "agent", state: "running" },
    open: /^Step \d+ \/ \d+$/,
    by: "hover",
  },
];

for (const overlay of OVERLAYS) {
  for (const theme of ["light", "dark"] as const) {
    test(`WCAG audit ${overlay.label} ${theme}`, async ({ page }) => {
      await openFixture(page, { ...overlay.route, theme });
      const trigger = page.getByRole("button", { name: overlay.open }).first();
      if (overlay.by === "hover") await trigger.hover();
      else await trigger.click();
      const popup = page
        .locator('[role="menu"], [role="dialog"], [role="listbox"], [role="tooltip"]')
        .first();
      await expect(popup).toBeVisible();
      await expect.poll(() => popup.evaluate((node) => getComputedStyle(node).opacity)).toBe("1");

      await expectNoWcagViolations(page);
    });
  }
}

test("a chosen typeface reaches the controls, not only the text that inherits", async ({
  page,
}) => {
  await openFixture(page, { fixture: "agent", state: "long-content", theme: "light" });

  const faces = () =>
    page.evaluate(() => {
      const family = (selector: string) => {
        const element = document.querySelector(selector);
        return element
          ? getComputedStyle(element).fontFamily.split(",")[0]!.replace(/"/g, "")
          : null;
      };
      return { body: family("body"), control: family("button"), code: family(".shiki-block") };
    });

  expect(await faces()).toEqual({ body: "Geist", control: "Geist", code: "JetBrains Mono" });

  await page.evaluate(() => {
    document.documentElement.style.setProperty("--font-sans", '"Times New Roman", serif');
    document.documentElement.style.setProperty("--font-mono", '"Courier New", monospace');
  });

  expect(await faces()).toEqual({
    body: "Times New Roman",
    control: "Times New Roman",
    code: "Courier New",
  });
});

test("a mouse-opened menu shows its highlighted item, not a ring around itself", async ({
  page,
}) => {
  await openFixture(page, { fixture: "agent", state: "idle", theme: "light" });

  await page.locator("[data-user-message-bubble]").first().click({ button: "right" });
  const menu = page.locator('[role="menu"]');
  await expect(menu).toBeVisible();
  await expect(menu).toHaveCSS("outline-style", "none");

  await page.keyboard.press("ArrowDown");
  const highlighted = page.locator('[role="menuitem"][data-highlighted]');
  await expect(highlighted).toHaveCount(1);
  await expect(highlighted).not.toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
});

test("a menu owns both its own bounds, not its call sites", async ({ page }) => {
  await page.setViewportSize({ width: 1120, height: 300 });
  await openFixture(page, { fixture: "agent", state: "idle" });

  await page.locator("[data-user-message-bubble]").first().click({ button: "right" });
  const popup = page.locator('[role="menu"]');
  await expect(popup).toBeVisible();
  await expect.poll(() => popup.evaluate((node) => getComputedStyle(node).opacity)).toBe("1");

  const box = await popup.evaluate((node) => {
    const rect = node.getBoundingClientRect();
    return {
      top: rect.top,
      bottom: rect.bottom,
      minWidth: getComputedStyle(node).minWidth,
      maxHeight: getComputedStyle(node).maxHeight,
      measured: getComputedStyle(node.parentElement as HTMLElement).getPropertyValue(
        "--available-height",
      ),
    };
  });

  expect(box.minWidth).toBe("192px");
  expect(Number.parseFloat(box.measured)).toBeLessThan(380);
  expect(box.maxHeight).toBe(box.measured);
  expect(box.top).toBeGreaterThanOrEqual(0);
  expect(box.bottom).toBeLessThanOrEqual(300);
});

test("the catalogue holds its measure whatever its group contains", async ({ page }) => {
  await openFixture(page, { fixture: "agent", state: "idle" });

  await page.getByRole("button", { name: "Switch model" }).first().click();
  const body = page.locator('[data-slot="catalog-body"]');
  await expect(body).toBeVisible();

  const rows = await body.locator('[role="option"]').count();
  expect(rows).toBeLessThan(4);
  expect(await body.evaluate((node) => getComputedStyle(node).height)).toBe("240px");
});

test("the project tray stays inside the composer's edges", async ({ page }) => {
  await openFixture(page, { fixture: "agent", state: "empty" });

  const tray = page.locator('[data-tray="attached"]');
  await expect(tray).toBeVisible();
  const box = await page.evaluate(() => {
    const t = document.querySelector('[data-tray="attached"]')!.getBoundingClientRect();
    const composer = document.querySelector("[data-slot=composer-root]")!.getBoundingClientRect();
    return {
      tray: t.width,
      trayLeft: t.left,
      composer: composer.width,
      composerLeft: composer.left,
    };
  });
  expect(box.tray).toBeLessThan(box.composer);
  expect(box.trayLeft).toBeGreaterThan(box.composerLeft);
  const right = box.composerLeft + box.composer - (box.trayLeft + box.tray);
  expect(Math.abs(right - (box.trayLeft - box.composerLeft))).toBeLessThanOrEqual(1);
});

test("the plan strip holds one height whatever the plan says", async ({ page }) => {
  await openFixture(page, { fixture: "agent", state: "running" });

  const strip = page.locator('[data-slot="active-plan-surface"]');
  await expect(strip).toBeVisible();
  expect(await strip.evaluate((node) => getComputedStyle(node).height)).toBe("32px");

  await page.locator('[data-slot="active-plan-pill"]').hover();
  await expect(page.locator('[role="tooltip"] li')).not.toHaveCount(0);
  expect(await strip.evaluate((node) => getComputedStyle(node).height)).toBe("32px");
});

test("a text-bearing control meets the minimum target size", async ({ page }) => {
  await openFixture(page, { fixture: "workspace", state: "dock-light" });
  const summary = page.locator('[data-goal="summary"]');
  await expect(summary).toBeVisible();
  expect(
    await summary.evaluate((el) => Math.round(el.getBoundingClientRect().height)),
  ).toBeGreaterThanOrEqual(24);
});

for (const route of ACCESSIBILITY_ROUTES.filter((candidate) => candidate.theme === "light")) {
  test(`WCAG audit ${route.fixture} ${route.overlay ?? route.state} at the smallest UI size`, async ({
    page,
  }) => {
    await openFixture(page, { ...route, fontSize: 11 });
    await expectNoWcagViolations(page);
  });
}

for (const state of ["narrative", "long-content", "tool-shells"] as const) {
  test(`nothing shows through the composer in ${state}`, async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await openFixture(page, { fixture: "agent", state });

    const geometry = await page.evaluate(() => {
      const viewport = document.querySelector(".msg-scroll-viewport");
      const composer = document.querySelector(".agent-composer-glass");
      if (!viewport || !composer) return null;
      const style = getComputedStyle(viewport);
      const stops = [...style.maskImage.matchAll(/calc\(100% - ([\d.]+)px\)|\b(100)%\)/g)];
      const last = stops.at(-1);
      return {
        overlap: viewport.getBoundingClientRect().bottom - composer.getBoundingClientRect().top,
        overlay: Number.parseFloat(style.getPropertyValue("--composer-overlay")),
        fadesOutAt: last?.[1] === undefined ? 0 : Number.parseFloat(last[1]),
      };
    });

    expect(geometry).not.toBeNull();
    expect(geometry!.overlap).toBeGreaterThan(24);
    expect(geometry!.overlay).toBeGreaterThanOrEqual(geometry!.overlap);
    expect(geometry!.fadesOutAt).toBeCloseTo(geometry!.overlay, 0);
  });
}

test("a suggestion panel above the composer is not clipped away by it", async ({ page }) => {
  await openFixture(page, { fixture: "agent", state: "idle" });

  const input = page.getByRole("textbox", { name: en["composer.input.label"]! });
  await input.click();
  await input.fill("/");

  const panel = page.getByRole("dialog").filter({ hasText: en["composer.slash.heading"]! });
  await expect(panel).toBeVisible();

  const painted = await page.evaluate((heading) => {
    const panels = [...document.querySelectorAll('[role="dialog"]')];
    const el = panels.find((p) => p.textContent?.includes(heading));
    if (!el) return { found: false };
    const b = el.getBoundingClientRect();
    const hit = document.elementFromPoint(b.left + b.width / 2, b.top + 8);
    const composer = document.querySelector("[data-slot=composer-root]")!.getBoundingClientRect();
    return {
      found: true,
      paintsItself: el.contains(hit) || el === hit,
      abovecomposer: Math.round(b.bottom) <= Math.round(composer.top),
      withinComposerWidth: b.width <= composer.width,
    };
  }, en["composer.slash.heading"]!);

  expect(painted.found).toBe(true);
  expect(painted.paintsItself).toBe(true);
  expect(painted.abovecomposer).toBe(true);
  expect(painted.withinComposerWidth).toBe(true);
});

test("the file mention picker paints over the transcript, not under the composer", async ({
  page,
}) => {
  await openFixture(page, { fixture: "agent", state: "idle" });

  const input = page.getByRole("textbox", { name: en["composer.input.label"]! });
  await input.click();
  await input.pressSequentially("@store", { delay: 30 });

  const listbox = page.locator("#composer-mention-listbox");
  await expect(listbox).toBeVisible();

  const geometry = await page.evaluate(() => {
    const el = document.getElementById("composer-mention-listbox")!;
    const box = el.getBoundingClientRect();
    const composer = document.querySelector("[data-slot=composer-root]")!;
    const cb = composer.getBoundingClientRect();
    const hit = document.elementFromPoint(box.left + box.width / 2, box.top + 8);
    return {
      rows: el.querySelectorAll('[id^="composer-mention-option-"]').length,
      paintsItself: el.contains(hit) || el === hit,
      aboveComposer: Math.round(box.bottom) <= Math.round(cb.top),
      escapedTheClippingSurface: !composer.contains(el),
      focusStillInInput: document.activeElement?.tagName.toLowerCase() === "textarea",
      selects: el.querySelectorAll('[aria-selected="true"]').length,
    };
  });

  expect(geometry.rows).toBeGreaterThan(0);
  expect(geometry.paintsItself).toBe(true);
  expect(geometry.aboveComposer).toBe(true);
  expect(geometry.escapedTheClippingSurface).toBe(true);
  expect(geometry.focusStillInInput).toBe(true);
  expect(geometry.selects).toBe(1);
});

test("the composer's attachment chips are one component, not two", async ({ page }) => {
  await openFixture(page, { fixture: "agent", state: "idle" });
  const input = page.getByRole("textbox", { name: en["composer.input.label"]! });

  await input.click();
  await input.pressSequentially("@store", { delay: 30 });
  await expect(page.locator("#composer-mention-listbox")).toBeVisible();
  await page.keyboard.press("Tab");

  await page.evaluate(() => {
    const ta = document.querySelector("textarea")!;
    const data = new DataTransfer();
    data.setData("text/plain", "x".repeat(2000));
    ta.dispatchEvent(new ClipboardEvent("paste", { clipboardData: data, bubbles: true }));
  });

  const chips = await page.evaluate(() => {
    const rows = [...document.querySelectorAll("[data-slot=chip]")];
    return rows.map((el) => {
      const cs = getComputedStyle(el);
      return {
        kind: el.getAttribute("data-kind"),
        borderWidth: cs.borderTopWidth,
        radius: cs.borderTopLeftRadius,
        height: cs.height,
        family: cs.fontFamily.split(",")[0],
        fill: cs.backgroundColor,
      };
    });
  });

  expect(chips.length).toBeGreaterThanOrEqual(2);
  const distinct = (key: keyof (typeof chips)[number]) => new Set(chips.map((c) => c[key])).size;
  expect(distinct("borderWidth")).toBe(1);
  expect(distinct("radius")).toBe(1);
  expect(distinct("height")).toBe(1);
  expect(distinct("family")).toBe(1);
  expect(chips.every((c) => parseFloat(c.borderWidth) > 0)).toBe(true);
  expect(distinct("fill")).toBe(2);
});

test("the composer's top tray takes the composer's corner and tucks behind it", async ({
  page,
}) => {
  await openFixture(page, { fixture: "agent", state: "empty" });

  const tray = page.locator('[data-slot="composer-top-tray-surface"]');
  await expect(tray).toBeVisible();

  const seam = await page.evaluate(() => {
    const t = document.querySelector('[data-slot="composer-top-tray-surface"]')!;
    const composer = document.querySelector("[data-slot=composer-root]")!;
    const cs = getComputedStyle(t);
    return {
      overlap: Math.round(t.getBoundingClientRect().bottom - composer.getBoundingClientRect().top),
      topLeftRadius: cs.borderTopLeftRadius,
      topRightRadius: cs.borderTopRightRadius,
      composerRadius: getComputedStyle(composer).borderTopLeftRadius,
      bottomBorder: cs.borderBottomWidth,
      overflow: cs.overflow,
    };
  });

  expect(seam.overlap).toBeGreaterThan(0);
  expect(seam.topLeftRadius).toBe(seam.composerRadius);
  expect(seam.topRightRadius).toBe(seam.composerRadius);
  expect(parseFloat(seam.bottomBorder)).toBe(0);
  expect(seam.overflow).toBe("clip");
});

test("the reasoning window fades the edge it actually clips", async ({ page }) => {
  await openFixture(page, { fixture: "agent", state: "answer-opening" });

  const scroller = page.locator('[data-slot="reasoning-scroller"]');
  await expect(scroller).toBeVisible();

  const readFade = () =>
    page.evaluate(() => {
      const el = document.querySelector<HTMLElement>('[data-slot="reasoning-scroller"]')!;
      const cs = getComputedStyle(el);
      return {
        overflowing: el.scrollHeight > el.clientHeight,
        masked: cs.maskImage !== "none",
        overlays: [...el.children].filter((c) => getComputedStyle(c).position === "absolute")
          .length,
        top: cs.getPropertyValue("--fade-top").trim(),
        bottom: cs.getPropertyValue("--fade-bottom").trim(),
      };
    });

  const atTop = await readFade();
  const aside = await page.evaluate(() => {
    const body = document.querySelector<HTMLElement>('[role="region"]');
    if (!body) return null;
    const cs = getComputedStyle(body);
    return {
      leftBorder: parseFloat(cs.borderLeftWidth),
      leftInset: parseFloat(cs.paddingLeft) + parseFloat(cs.marginLeft),
      hasFill: cs.backgroundColor !== "rgba(0, 0, 0, 0)" && cs.backgroundColor !== "transparent",
    };
  });
  expect(aside?.leftBorder).toBeGreaterThan(0);
  expect(aside?.leftInset).toBeGreaterThan(20);
  expect(aside?.hasFill).toBe(false);

  expect(atTop.overflowing).toBe(true);
  expect(atTop.masked).toBe(true);
  expect(atTop.overlays).toBe(0);
  expect(atTop.top).toBe("0px");
  expect(atTop.bottom).toBe("24px");

  await scroller.evaluate((el) => {
    el.scrollTop = 20;
  });
  await expect.poll(async () => (await readFade()).top).toBe("24px");
  expect((await readFade()).bottom).toBe("24px");
});
