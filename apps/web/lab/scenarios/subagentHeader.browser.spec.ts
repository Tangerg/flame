import { expect, test } from "./test";
import { expectLabReady } from "../readiness";
import { en } from "../../src/lib/i18n/locales/en";
import zh from "../../../../plugins/locale-zh/package/io.github.tangerg.flame/messages.json" with { type: "json" };
const label =
  "Audit application-owned cancellation, transaction boundaries and dependency graph verification across the complete Agent Framework";

for (const [fontSize, theme] of [
  [14, "light"],
  [18, "dark"],
] as const) {
  test(`a narrow subagent header keeps the selected identity and commands together ${fontSize}`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width: 1472, height: 900 });
    await page.goto(
      `/lab/fixture.html?fixture=workspace&state=dock-subagents&theme=${theme}&font-size=${fontSize}&locale=en`,
    );
    await expectLabReady(page);
    const panel = page.getByRole("region", { name: "Subagents", exact: true });
    await expect(panel.getByText("6 subagents · 1 running", { exact: true })).toBeVisible();
    await panel
      .getByRole("button", { name: "Audit Agent Framework ownership Needs input" })
      .click();
    const dock = page.getByRole("complementary", { name: "Subagents", exact: true });
    const resizer = page.locator(".agent-pane-resizer");
    await resizer.focus();
    await resizer.press("Home");
    await expect(resizer).toHaveAttribute("aria-valuenow", "320");
    const frame = (await dock.boundingBox())!;
    expect(frame.width).toBeCloseTo(320, 0);
    const content = (await panel.boundingBox())!;
    expect(content.x).toBeGreaterThanOrEqual(frame.x);
    expect(content.x + content.width).toBeLessThanOrEqual(frame.x + frame.width);

    await page.evaluate(async (label) => {
      const path = "/lab/scenarios/fixtureRuntime.tsx";
      const { visualClient } = (await import(path)) as typeof import("./fixtureRuntime");
      const store = visualClient.agent.projection.store.getState();
      const view = store.sessions.ses_visual!.view;
      const refresh = store.beginViewRefresh("ses_visual", true);
      if (!refresh) throw new Error("Expected a refresh token");
      store.commitViewRefresh("ses_visual", refresh, {
        ...view,
        toolCalls: {
          ...view.toolCalls,
          item_delegate: { ...view.toolCalls.item_delegate!, fn: label },
        },
      });
    }, label);

    const header = panel.locator("header");
    const identity = header.getByText(label, { exact: true });
    await expect(identity).toHaveText(label);
    await expect(resizer).toBeFocused();
    await expect(panel.getByRole("region", { name: label, exact: true })).toBeAttached();
    await expect(header.getByText("Needs input", { exact: true })).toBeInViewport({ ratio: 1 });
    await expect(panel.getByText("6 subagents · 1 running", { exact: true })).toHaveCount(0);
    // The header wraps the complete task label instead of clipping it.
    expect(
      await identity.evaluate(
        (node) => node.scrollHeight <= node.clientHeight && node.scrollWidth <= node.clientWidth,
      ),
    ).toBe(true);
    await expect(identity).toBeInViewport({ ratio: 1 });
    for (const button of await header.getByRole("button").all()) {
      await expect(button).toBeInViewport({ ratio: 1 });
      expect(
        await button.evaluate((node) => {
          const box = node.getBoundingClientRect();
          const host = node.closest("header")!.getBoundingClientRect();
          return (
            box.left >= host.left &&
            box.right <= host.right &&
            node.contains(document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2))
          );
        }),
      ).toBe(true);
    }
    expect(await panel.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
    await testInfo.attach("narrow-selected-task.png", {
      body: await page.screenshot(),
      contentType: "image/png",
    });
    const back = header.getByRole("button", { name: "All subagents", exact: true });
    await back.focus();
    await back.press("Enter");
    const task = panel.getByRole("button", { name: `${label} Needs input`, exact: true });
    await expect(task.getByText(label, { exact: true })).toBeInViewport({ ratio: 1 });
    await expect(task).toBeFocused();
    await task.press("Enter");
    await expect(back).toBeFocused();
    await expect(identity).toBeInViewport({ ratio: 1 });
    await header.getByRole("button", { name: "Cancel this run", exact: true }).click();
    await expect(page.locator("html")).toHaveAttribute("data-visual-canceled-run", "run_child");
    await header.getByRole("button", { name: "All subagents", exact: true }).click();
    await expect(header.getByText("6 subagents · 1 running", { exact: true })).toBeVisible();
    await expect(header.getByRole("button", { name: "All subagents", exact: true })).toHaveCount(0);
  });
}

const webkit = test.extend({ browserName: "webkit" });
for (const profile of [
  {
    locale: "en",
    theme: "light",
    fontSize: 14,
    motion: "full",
    messages: en,
    draft: "  Keep the subagent reading draft.  ",
    continuation: " Continue after reading the source.",
  },
  {
    locale: "zh",
    theme: "dark",
    fontSize: 18,
    motion: "off",
    messages: zh as Record<string, string>,
    draft: "  保留子代理阅读草稿。  ",
    continuation: " 读完来源后继续。",
  },
] as const) {
  webkit(
    `selected subagent source remains readable through short reflow and return ${profile.locale}`,
    async ({ page }, testInfo) => {
      const { locale, theme, fontSize, motion, messages, draft, continuation } = profile;
      const name = (key: string) => ({ name: messages[key]!, exact: true });
      await page.setViewportSize({ width: 1472, height: 900 });
      await page.goto(
        `/lab/fixture.html?fixture=workspace&state=dock-subagents&locale=${locale}&theme=${theme}&font-size=${fontSize}&motion=${motion}`,
      );
      await expectLabReady(page);
      await page
        .getByRole("button", { name: "Collapse the workspace fixture sidebar", exact: true })
        .click();
      const composer = page.getByRole("textbox", {
        ...name("composer.input.label"),
        includeHidden: true,
      });
      await composer.fill(draft);
      const composerNode = (await composer.elementHandle())!;
      const panel = page
        .getByRole("region", name("subagents.title"))
        .and(page.locator(".agent-workspace-view"));
      const entry = panel
        .locator('[data-slot="delegated-run-link"][data-run-id="run_fan_ok"]')
        .getByRole("button")
        .first();
      await entry.focus();
      await page.keyboard.press("Enter");
      const identity = panel.locator('[data-slot="subagent-identity"]');
      const title = await identity.innerText();
      expect(title).toContain(
        "Audit persistence boundaries across Runtime, Framework and delegated agents",
      );
      await page.getByRole("button", name("dock.action.expand")).click();
      const source = panel.getByRole("button", {
        name: "runtime/internal/session/store.go:4",
        exact: true,
      });
      await source.focus();
      const sourceNode = (await source.elementHandle())!;
      const capture = async (label: string) =>
        testInfo.attach(`subagent-${label}-${locale}`, {
          body: await page.screenshot({ animations: "disabled" }),
          contentType: "image/png",
        });
      const reading = async (label: string) => {
        const frames = await source.evaluate(async (node, jumpLabel) => {
          const viewport = node.closest('[data-slot="subagent-transcript"]')!
            .firstElementChild as HTMLElement;
          const jump = node
            .closest('[data-slot="subagent-transcript"]')!
            .querySelector<HTMLButtonElement>(`button[aria-label="${jumpLabel}"]`);
          const frames = [];
          for (let i = 0; i < 8; i += 1) {
            await new Promise(requestAnimationFrame);
            const rect = node.getBoundingClientRect();
            const view = viewport.getBoundingClientRect();
            const style = getComputedStyle(viewport);
            const action = jump?.getBoundingClientRect();
            const overlap = action
              ? Math.max(0, Math.min(rect.right, action.right) - Math.max(rect.left, action.left)) *
                Math.max(0, Math.min(rect.bottom, action.bottom) - Math.max(rect.top, action.top))
              : 0;
            const range = document.createRange();
            range.selectNodeContents(node);
            const fragments = [...range.getClientRects()].filter(
              (r) => r.width > 0 && r.height > 0,
            );
            frames.push({
              bounds: rect.toJSON(),
              top: view.top + (parseFloat(style.scrollPaddingTop) || 0),
              bottom: view.bottom - (parseFloat(style.scrollPaddingBottom) || 0),
              overlap,
              fragments: fragments.map((r) => r.toJSON()),
              hit: fragments.every((r) =>
                node.contains(
                  document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2),
                ),
              ),
            });
          }
          return frames;
        }, messages["chat.jumpToBottom"]!);
        await testInfo.attach(`subagent-reading-${label}-${locale}`, {
          body: JSON.stringify(frames),
          contentType: "application/json",
        });
        await capture(label);
        for (const frame of frames) {
          expect(frame.bounds.top).toBeGreaterThanOrEqual(frame.top - 1);
          expect(frame.bounds.bottom).toBeLessThanOrEqual(frame.bottom + 1);
          expect(frame.overlap, "Jump must not cover the focused source reference").toBe(0);
          expect(frame.hit, "each source text fragment must remain readable").toBe(true);
        }
        await expect(source).toBeFocused();
        await expect(identity).toHaveText(title);
        expect(await source.evaluate((node, retained) => node === retained, sourceNode)).toBe(true);
        expect(await composer.evaluate((node, retained) => node === retained, composerNode)).toBe(
          true,
        );
        await expect(composer).toHaveValue(draft);
      };
      for (const size of [
        { width: 560, height: 377 },
        { width: 373, height: 251 },
        { width: 504, height: 316 },
        { width: 373, height: 251 },
      ]) {
        await page.setViewportSize(size);
        await page.evaluate(async () => {
          for (let i = 0; i < 8; i += 1) await new Promise(requestAnimationFrame);
        });
        await reading(`${size.width}-${size.height}`);
      }
      await page.keyboard.press("Enter");
      await expect(
        page.getByText("runtime/internal/session/store.go", { exact: true }),
      ).toBeVisible();
      await page.keyboard.press("Escape");
      await reading("source-return");
      const back = panel.getByRole("button", name("subagents.all"));
      const identityReader = panel.locator("header").getByRole("group", {
        name: title,
        exact: true,
      });
      const bodyPosition = await source.evaluate(
        (node) => node.closest('[data-slot="subagent-transcript"]')!.firstElementChild!.scrollTop,
      );
      await identityReader.focus();
      for (const edge of ["Home", "End"] as const) {
        await identityReader.press(edge);
        await expect
          .poll(() =>
            identityReader.evaluate(
              (node, edge) =>
                edge === "Home"
                  ? node.scrollTop
                  : node.scrollHeight - node.clientHeight - node.scrollTop,
              edge,
            ),
          )
          .toBeLessThanOrEqual(1);
        const line = await identity.evaluate((node, edge) => {
          const range = document.createRange();
          range.selectNodeContents(node);
          const fragments = [...range.getClientRects()].filter((rect) => rect.width > 0);
          const rect = edge === "Home" ? fragments[0]! : fragments.at(-1)!;
          const reader = node.parentElement!;
          const bounds = reader.getBoundingClientRect();
          const style = getComputedStyle(reader);
          return {
            top: rect.top,
            bottom: rect.bottom,
            visibleTop: bounds.top + parseFloat(style.borderTopWidth),
            visibleBottom: bounds.bottom - parseFloat(style.borderBottomWidth),
            hit: node.contains(
              document.elementFromPoint(rect.left + rect.width / 2, rect.top + rect.height / 2),
            ),
          };
        }, edge);
        await testInfo.attach(`subagent-identity-${edge}-${locale}`, {
          body: JSON.stringify(line),
          contentType: "application/json",
        });
        await capture(`identity-${edge}`);
        expect(line.top).toBeGreaterThanOrEqual(line.visibleTop - 1);
        expect(line.bottom).toBeLessThanOrEqual(line.visibleBottom + 1);
        expect(line.hit).toBe(true);
        await expect(identityReader).toBeFocused();
        await expect(back).toBeInViewport({ ratio: 1 });
        await expect(
          panel
            .locator("header")
            .getByText(messages["agent.runTree.status.finished"]!, { exact: true }),
        ).toBeInViewport({ ratio: 1 });
        expect(
          await source.evaluate(
            (node) =>
              node.closest('[data-slot="subagent-transcript"]')!.firstElementChild!.scrollTop,
          ),
        ).toBe(bodyPosition);
      }
      await source.click();
      await expect(
        page.getByText("runtime/internal/session/store.go", { exact: true }),
      ).toBeVisible();
      await page.keyboard.press("Escape");
      await reading("pointer-source-return");
      await back.focus();
      await back.press("Enter");
      await expect(entry).toBeFocused();
      await entry.press("Enter");
      await expect(back).toBeFocused();
      const close = page
        .locator('[data-slot="workspace-window-header"]')
        .getByRole("button", name("common.close"));
      await close.focus();
      await close.press("Enter");
      await composer.focus();
      await expect(composer).toHaveValue(draft);
      await composer.press("ControlOrMeta+a");
      await page.keyboard.press("ArrowRight");
      await page.keyboard.insertText(continuation);
      await expect(composer).toHaveValue(draft + continuation);
      await page.setViewportSize({ width: 1120, height: 755 });
      await expect(composer).toBeFocused();
      await capture("continued-draft");
    },
  );
}
