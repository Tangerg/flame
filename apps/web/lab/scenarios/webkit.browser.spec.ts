import { ACTIVE_DIFF_FILE, REVIEW_DIFF } from "./diffSnapshot";
import { expect, test, type Page } from "./test";
import { DOCK_MIN_WIDTH_PX } from "@/lib/shellGeometry";
import { en } from "@/lib/i18n/locales/en";
import { expectLabReady } from "../readiness";
const SETTINGS_SEARCH = { name: en["settings.searchPlaceholder"]! };

test.use({ browserName: "webkit" });

interface FixtureRoute {
  fixture: "agent" | "shell" | "workspace";
  state: string;
  theme?: "light" | "dark";
  motion?: "full";
  fontSize?: number;
}

async function openFixture(page: Page, route: FixtureRoute): Promise<void> {
  const query = new URLSearchParams({
    fixture: route.fixture,
    state: route.state,
    theme: route.theme ?? "light",
  });
  if (route.motion) query.set("motion", route.motion);
  if (route.fontSize !== undefined) query.set("font-size", String(route.fontSize));
  if (route.motion === "full") await page.emulateMedia({ reducedMotion: "no-preference" });

  await page.goto(`/lab/fixture.html?${query}`);
  await expectLabReady(page);
}

async function expectNoPageOverflow(page: Page): Promise<void> {
  await expect
    .poll(() =>
      page.locator("html").evaluate((element) => element.scrollWidth - element.clientWidth),
    )
    .toBeLessThanOrEqual(0);
}

test("WebKit shell preserves minimum geometry and drawer focus handoff", async ({ page }) => {
  await openFixture(page, { fixture: "shell", state: "populated", motion: "full" });

  await expect(page.getByRole("complementary", { name: "Work index" })).toBeVisible();
  await expect(page.getByRole("button", { name: "scope 6" })).toBeVisible();
  const drawer = page.locator(".agent-drawer");
  await expect(drawer).toHaveCSS("transition-duration", "0.5s, 0.5s, 0s");
  expect(
    await drawer.evaluate((element) => getComputedStyle(element).transitionTimingFunction),
  ).toContain("linear(");
  await expectNoPageOverflow(page);

  await page.getByRole("button", { name: "Hide sidebar" }).click();
  await expect(page.getByRole("button", { name: "Expand sidebar" })).toBeFocused();
  await page.getByRole("button", { name: "Expand sidebar" }).click();
  await expect(page.getByRole("button", { name: "Hide sidebar" })).toBeFocused();
});

test("WebKit agent HITL remains keyboard-operable", async ({ page }) => {
  await openFixture(page, { fixture: "agent", state: "waiting", theme: "dark" });

  const approve = page.getByRole("button", { name: /Allow once/ });
  await approve.focus();
  await page.keyboard.press("Enter");

  await expect(page.getByText("Approved", { exact: true })).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("data-visual-resumed-run", "run_root");
  await expect(page.locator("html")).toHaveAttribute("data-visual-resumed-item", "item_approval");
  await expect(page.locator('[data-slot="approval-surface"]')).toHaveCount(0);
  await expect(page.getByRole("textbox", { name: "Message composer" })).toBeVisible();
  await expectNoPageOverflow(page);
});

for (const theme of ["light", "dark"] as const) {
  test(`WebKit tool arrivals animate while staying opaque in ${theme} mode`, async ({ page }) => {
    await openFixture(page, { fixture: "agent", state: "tool-shells", theme, motion: "full" });

    const frames = await page.evaluate(async () => {
      const storePath = "/lab/scenarios/fixtureRuntime.tsx";
      const fixturePath = "/lab/scenarios/agentSessionSnapshots.ts";
      const projectionPath = "/src/features/agent/application/session/sessionSnapshot.ts";
      const { visualClient } = (await import(storePath)) as typeof import("./fixtureRuntime");
      const { RUNTIME_AGENT_SESSION_SNAPSHOTS, projectRuntimeAgentSnapshot, VISUAL_SESSION_ID } =
        (await import(fixturePath)) as typeof import("./agentSessionSnapshots");
      const { projectAgentSessionSnapshot } = (await import(
        projectionPath
      )) as typeof import("@/features/agent/application/session/sessionSnapshot");
      const snapshot = projectRuntimeAgentSnapshot(RUNTIME_AGENT_SESSION_SNAPSHOTS["tool-shells"]);
      const store = visualClient.agent.projection.store.getState();
      const frames: { count: number; opacity: string; transform: string; top: number }[][] = [];
      for (let count = 1; count <= snapshot.items.length; count += 1) {
        const token = store.beginViewRefresh(VISUAL_SESSION_ID, true);
        if (!token) throw new Error("Expected a refresh token");
        const view = projectAgentSessionSnapshot({
          ...snapshot,
          items: snapshot.items.slice(0, count),
        });
        if (!store.commitViewRefresh(VISUAL_SESSION_ID, token, view)) {
          throw new Error("Expected the tool arrival to be published");
        }
        await new Promise(requestAnimationFrame);
        await new Promise(requestAnimationFrame);
        frames.push(
          [...document.querySelectorAll("[data-turn-id], [data-block-anchor]")].map((node) => {
            const style = getComputedStyle(node);
            return {
              count,
              opacity: style.opacity,
              transform: style.transform,
              top: Number.parseFloat(style.top) || 0,
            };
          }),
        );
      }
      return frames;
    });

    expect(frames.length).toBeGreaterThan(1);
    expect(frames.flat().some((surface) => surface.top > 0)).toBe(true);
    for (const frame of frames) {
      expect(frame.length).toBeGreaterThan(0);
      for (const surface of frame) {
        expect(surface.opacity, `item ${surface.count}`).toBe("1");
        expect(surface.transform, `item ${surface.count}`).toBe("none");
      }
    }
    await expect
      .poll(() =>
        page.locator("[data-turn-id], [data-block-anchor]").evaluateAll((nodes) =>
          nodes.every((node) => {
            const style = getComputedStyle(node);
            return style.top === "0px" && style.opacity === "1" && style.transform === "none";
          }),
        ),
      )
      .toBe(true);
    await expectNoPageOverflow(page);
  });
}

test("WebKit question radio navigation stays editable until explicit confirmation", async ({
  page,
}) => {
  await openFixture(page, { fixture: "agent", state: "question", theme: "dark" });

  const first = page.getByRole("radio", { name: /Race detector/ });
  await first.focus();
  await page.keyboard.press("ArrowDown");

  const selected = page.getByRole("radio", { name: /Visual suite/ });
  await expect(selected).toBeFocused();
  await expect(selected).toHaveAttribute("aria-checked", "true");
  await expect(page.getByText("1 of 2", { exact: true })).toBeVisible();
  await expect(page.getByRole("textbox", { name: "What should this gate protect?" })).toHaveCount(
    0,
  );

  await page.getByRole("button", { name: "Next", exact: true }).click();
  await expect(page.getByRole("textbox", { name: "What should this gate protect?" })).toBeVisible();
  await expect(page.getByRole("textbox", { name: "What should this gate protect?" })).toBeFocused();
  await expect(page.getByText("2 of 2", { exact: true })).toBeVisible();
  await expectNoPageOverflow(page);
});

test("WebKit renders long CJK and highlighted content at maximum UI text size", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 800 });
  await openFixture(page, {
    fixture: "agent",
    state: "long-content",
    fontSize: 18,
  });

  await expect(page.locator("body")).toHaveCSS("font-size", "18px");
  await expect(page.locator(".shiki-block [data-slot=code-body] > code:has(.line)")).toHaveCount(3);
  await expect(page.getByRole("img", { name: "Diagram" })).toBeVisible();
  await expect(page.getByText(/中文混排/)).toBeVisible();
  await expect(page.getByRole("textbox", { name: "Message composer" })).toBeVisible();
  await expectNoPageOverflow(page);
});

test("WebKit workspace keeps review geometry and separator semantics", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await openFixture(page, { fixture: "workspace", state: "dock-review", theme: "dark" });

  await expect(page.locator("[data-diff-file]")).toHaveCount(1);
  await expect(
    page.getByRole("region", { name: `Changes in ${ACTIVE_DIFF_FILE}`, exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Show changed files", exact: true }).click();
  await expect(page.locator("[data-diff-file]")).toHaveCount(REVIEW_DIFF.files!.length);
  const separator = page.getByRole("separator", { name: "Resize right workspace" });
  await expect(separator).toHaveAttribute("aria-valuemin", String(DOCK_MIN_WIDTH_PX));
  await expect
    .poll(async () => Number(await separator.getAttribute("aria-valuenow")))
    .toBeGreaterThan(DOCK_MIN_WIDTH_PX);
  await expectNoPageOverflow(page);
});

const SHORT_DRAFT_PROFILES = [
  {
    locale: "en",
    theme: "light",
    composer: en["composer.input.label"]!,
    draft: "Keep every word of this short draft visible while the narrow composer wraps it.",
    suffix: " Then continue it.",
  },
  {
    locale: "zh",
    theme: "dark",
    composer: "消息输入框",
    draft: "窄窗口下输入框应完整显示这段简短草稿，自动换行时不能裁切文字。",
    suffix: "继续补充一句。",
  },
] as const;

for (const profile of SHORT_DRAFT_PROFILES) {
  test(`WebKit keeps a short Composer draft fully visible across viewport and sidebar changes in ${profile.locale} ${profile.theme}`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width: 1120, height: 900 });
    const query = new URLSearchParams({
      fixture: "agent",
      state: "idle",
      sidebar: "collapsed",
      motion: "off",
      locale: profile.locale,
      theme: profile.theme,
      "font-size": "18",
    });
    await page.goto(`/lab/fixture.html?${query}`);
    await expectLabReady(page);
    await page.evaluate(() => document.fonts.ready);

    const composer = page.getByRole("textbox", { name: profile.composer, exact: true });
    const capture = async (name: string) =>
      testInfo.attach(name, {
        body: await page.screenshot({ animations: "disabled" }),
        contentType: "image/png",
      });

    await composer.click();
    await page.keyboard.type(profile.draft);
    await expect(composer).toHaveValue(profile.draft);
    const composerNode = await composer.elementHandle();

    const expectCompleteDraft = async (value: string) => {
      await expect(composer).toHaveValue(value);
      expect(await composer.evaluate((node, original) => node === original, composerNode)).toBe(
        true,
      );
      await expect
        .poll(() =>
          composer.evaluate((node) => {
            const box = node.getBoundingClientRect();
            const frame = node.closest("[data-slot=composer-root]")!.getBoundingClientRect();
            return {
              fits:
                node.scrollHeight - node.clientHeight <= 1 &&
                node.scrollWidth - node.clientWidth <= 1,
              atStart: node.scrollTop === 0 && node.scrollLeft === 0,
              contained:
                box.top >= frame.top - 1 &&
                box.bottom <= frame.bottom + 1 &&
                box.left >= frame.left - 1 &&
                box.right <= frame.right + 1,
            };
          }),
        )
        .toEqual({ fits: true, atStart: true, contained: true });
      await expect(composer).toBeInViewport({ ratio: 1 });
      await expectNoPageOverflow(page);
    };

    await expectCompleteDraft(profile.draft);
    await capture("initial-short-draft");

    await page.setViewportSize({ width: 560, height: 900 });
    await expectCompleteDraft(profile.draft);
    await capture("wrapped-short-draft-560px");

    await page.setViewportSize({ width: 390, height: 900 });
    await expectCompleteDraft(profile.draft);

    const expand = page.getByRole("button", {
      name: "Expand the agent fixture sidebar",
      exact: true,
    });
    await expand.focus();
    await page.keyboard.press("Enter");
    await expect(
      page.getByRole("button", { name: "Collapse the agent fixture sidebar", exact: true }),
    ).toBeFocused();
    await expect(composer).toHaveValue(profile.draft);
    await page.keyboard.press("Enter");
    await expect(expand).toBeFocused();
    await expectCompleteDraft(profile.draft);

    await composer.focus();
    await expect(composer).toBeFocused();
    expect(
      await composer.evaluate((node: HTMLTextAreaElement) => [
        node.selectionStart,
        node.selectionEnd,
      ]),
    ).toEqual([profile.draft.length, profile.draft.length]);
    await page.keyboard.type(profile.suffix);
    await expectCompleteDraft(profile.draft + profile.suffix);
    await capture("continued-short-draft-390px");

    await page.setViewportSize({ width: 1120, height: 900 });
    await expectCompleteDraft(profile.draft + profile.suffix);
    await capture("restored-short-draft-1120px");
  });
}

test("WebKit settings menu dismisses and returns focus", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 800 });
  await openFixture(page, {
    fixture: "workspace",
    state: "settings",
    fontSize: 18,
  });

  const search = page.getByRole("searchbox", SETTINGS_SEARCH);
  await expect(search).toBeVisible();
  const theme = page.getByRole("button", { name: "Theme" });
  await theme.click();
  await expect(page.getByRole("menuitemradio", { name: "Light" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("menuitemradio", { name: "Light" })).toHaveCount(0);
  await expect(theme).toBeFocused();
  await expectNoPageOverflow(page);
});
