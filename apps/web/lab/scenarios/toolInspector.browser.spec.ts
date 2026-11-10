import { expect, test } from "./test";
import { expectLabReady } from "../readiness";
import type { Locator } from "@playwright/test";
import { en } from "../../src/lib/i18n/locales/en";
import zh from "../../../../plugins/locale-zh/package/io.github.tangerg.flame/messages.json" with { type: "json" };
for (const [theme, fontSize, width] of [
  ["light", 14, 1120],
  ["dark", 18, 800],
] as const) {
  test(`generic tool results stay primary and scrollable before and after highlighting in ${theme}`, async ({
    page,
  }, testInfo) => {
    let releaseHighlight!: () => void;
    const highlight = new Promise<void>((resolve) => (releaseHighlight = resolve));
    let requested = false;
    await page.route("**/node_modules/.vite/deps/shiki_core.js*", async (route) => {
      requested = true;
      await highlight;
      await route.continue();
    });
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      `/lab/fixture.html?fixture=agent&state=tool-agentic&theme=${theme}&font-size=${fontSize}`,
    );
    await expectLabReady(page);
    await page.evaluate(() => document.fonts.ready);
    await page.getByRole("button", { name: "4 search · 2 run 6 steps", exact: true }).click();
    const card = page.locator('[data-tool="inspect_workspace"]');
    const header = card.locator('[data-slot="agent-activity-header"]');
    const trigger = header.getByRole("button", { name: "inspect_workspace", exact: true });
    await trigger.click();
    const result = page.getByRole("region", { name: "Result", exact: true });
    const argumentsButton = page.getByRole("button", { name: "Arguments", exact: true });
    const source = () => result;
    const assertResult = async () => {
      const value = JSON.parse(await source().innerText());
      expect(value).toHaveProperty("summary", { status: "reviewed", files: 24 });
      expect(value).toHaveProperty("files.length", 24);
      expect(value).toHaveProperty("files.23", {
        path: "runtime/internal/session/state_24.go",
        status: "reviewed",
      });
      const dimensions = await result.evaluate((node) => ({
        height: node.clientHeight,
        maximum: Number.parseFloat(getComputedStyle(node).maxHeight),
        content: node.scrollHeight,
      }));
      expect(dimensions.height).toBeLessThanOrEqual(dimensions.maximum);
      expect(dimensions.content).toBeGreaterThan(dimensions.height);
      expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
    };
    try {
      await expect.poll(() => requested).toBe(true);
      await expect(argumentsButton).toHaveAttribute("aria-expanded", "false");
      await expect(source().locator("code .line")).toHaveCount(0);
      await assertResult();
      const before = (await result.boundingBox())!;
      await result.focus();
      await expect(result).toBeFocused();
      await testInfo.attach(`tool-result-plain-${theme}`, {
        body: await page.screenshot({ animations: "disabled" }),
        contentType: "image/png",
      });
      releaseHighlight();
      await result.locator("code .line").first().waitFor();
      await assertResult();
      const after = (await result.boundingBox())!;
      expect({ width: after.width, height: after.height }).toEqual({
        width: before.width,
        height: before.height,
      });
      await expect(result).toBeFocused();
      await testInfo.attach(`tool-result-highlighted-${theme}`, {
        body: await page.screenshot({ animations: "disabled" }),
        contentType: "image/png",
      });
      await page.keyboard.press("Tab");
      await expect(argumentsButton).toBeFocused();
      await result.focus();
      await result.press("PageDown");
      await expect.poll(() => result.evaluate((node) => node.scrollTop)).toBeGreaterThan(0);
      await page.evaluate(() => {
        Object.defineProperty(navigator, "clipboard", {
          configurable: true,
          value: {
            writeText: async (value: string) => {
              (window as unknown as Record<string, unknown>).inspectorCopied = value;
            },
          },
        });
      });
      await result.locator("..").getByRole("button", { name: "Copy code", exact: true }).click();
      await expect
        .poll(() =>
          page.evaluate(() => (window as unknown as Record<string, unknown>).inspectorCopied),
        )
        .toBe(await source().innerText());

      await argumentsButton.click();
      const args = page.getByRole("region", { name: "Arguments", exact: true });
      await args.locator("code .line").first().waitFor();
      const value = JSON.parse(await args.innerText());
      expect(value).toEqual({
        workspace: {
          root: `/Users/visual/Flame workspaces/${"application-boundary-".repeat(12)}review`,
          include: ["runtime/internal/session/**", "apps/web/src/**"],
        },
        checks: ["ownership", "dependency-direction"],
        notes: "Keep <component> tags and source paths as literal text.",
      });
      await expect(args.locator("component")).toHaveCount(0);
      const frame = args.locator("..");
      await expect(frame.getByRole("button", { name: "Copy code", exact: true })).toHaveCount(0);
      const copyArguments = header.getByRole("button", { name: "Copy arguments", exact: true });
      await expect(copyArguments).toHaveCount(1);
      await trigger.hover();
      await expect(copyArguments).toHaveCSS("opacity", "1");
      await expect(copyArguments).toHaveCSS("pointer-events", "auto");
      await copyArguments.click();
      expect(
        JSON.parse(
          (await page.evaluate(
            () => (window as unknown as Record<string, unknown>).inspectorCopied,
          )) as string,
        ),
      ).toEqual(value);
      const wrap = frame.getByRole("button", { name: "Enable word wrap", exact: true });
      await wrap.click();
      await expect
        .poll(() => args.evaluate((node) => node.scrollWidth - node.clientWidth))
        .toBeLessThanOrEqual(1);
      await expect(
        page.getByRole("button", { name: "Disable word wrap", exact: true }),
      ).toHaveCount(2);
      expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
    } finally {
      releaseHighlight();
    }
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
    draft: "Summarize the inspected boundary",
    continuation: " before release.",
  },
  {
    locale: "zh",
    theme: "dark",
    fontSize: 18,
    motion: "off",
    messages: zh as Record<string, string>,
    draft: "总结已检查的边界",
    continuation: "，然后再发布。",
  },
] as const) {
  const { locale, theme, fontSize, motion, messages, draft, continuation } = profile;
  webkit(
    `WebKit keyboard tool reading keeps reader and draft through reflow in ${locale}/${theme}/${fontSize}`,
    async ({ page }, testInfo) => {
      const t = (key: string, count?: number) => messages[key]!.replace("{{count}}", String(count));
      const name = (key: string) => ({ name: t(key), exact: true });
      const capture = async (label: string) =>
        testInfo.attach(`tool-reading-${label}-${locale}`, {
          body: await page.screenshot({ animations: "disabled" }),
          contentType: "image/png",
        });
      const atTail = async (reader: Locator) => {
        await expect
          .poll(() =>
            reader.evaluate((node) => node.scrollHeight - node.clientHeight - node.scrollTop),
          )
          .toBeLessThanOrEqual(1);
        const frame = (await reader.boundingBox())!;
        const line = (await reader.locator("code .line").last().boundingBox())!;
        expect(line.y).toBeGreaterThanOrEqual(frame.y - 1);
        expect(line.y + line.height).toBeLessThanOrEqual(frame.y + frame.height + 1);
      };

      await page.setViewportSize({ width: 1120, height: 720 });
      await page.goto(
        `/lab/fixture.html?fixture=agent&state=tool-agentic&sidebar=collapsed&locale=${locale}&theme=${theme}&font-size=${fontSize}&motion=${motion}`,
      );
      await expectLabReady(page);
      await page.evaluate(() => document.fonts.ready);
      const composer = page.getByRole("textbox", name("composer.input.label"));
      await composer.click();
      await page.keyboard.type(draft);
      await expect(composer).toHaveValue(draft);
      const input = (await composer.elementHandle())!;

      await page
        .getByRole("button", {
          name: `${t("tool.group.search", 4)} · ${t("tool.group.run", 2)} ${t("agent.steps_other", 6)}`,
          exact: true,
        })
        .click();
      const header = page.locator(
        '[data-tool="inspect_workspace"] [data-slot="agent-activity-header"]',
      );
      const trigger = header.getByRole("button", { name: "inspect_workspace", exact: true });
      const copyArguments = header.getByRole("button", name("toolAction.copyArguments"));
      await trigger.focus();
      await page.keyboard.press("Enter");
      await expect(trigger).toBeFocused();
      const result = page.getByRole("region", name("toolInspector.result"));
      const resultFrame = result.locator("..");
      const argumentsButton = page.getByRole("button", name("toolInspector.arguments"));
      await expect(result).toBeVisible();
      await expect(argumentsButton).toHaveAttribute("aria-expanded", "false");
      await result.locator("code .line").first().waitFor();
      const resultNode = (await result.elementHandle())!;

      for (const stop of [
        copyArguments,
        resultFrame.getByRole("button", name("message.code.wrap.enable")),
        resultFrame.getByRole("button", name("message.code.copy")),
        result,
      ]) {
        await page.keyboard.press("Tab");
        await expect(stop).toBeFocused();
      }
      await page.keyboard.press("End");
      await atTail(result);
      expect(JSON.parse(await result.innerText())).toEqual({
        summary: { status: "reviewed", files: 24 },
        files: Array.from({ length: 24 }, (_, index) => ({
          path: `runtime/internal/session/state_${index + 1}.go`,
          status: "reviewed",
        })),
      });
      await capture("result-tail");

      await page.keyboard.press("Tab");
      await expect(argumentsButton).toBeFocused();
      await page.keyboard.press("Enter");
      await expect(argumentsButton).toHaveAttribute("aria-expanded", "true");
      const args = page.getByRole("region", name("toolInspector.arguments"));
      const argsFrame = args.locator("..");
      await args.locator("code .line").first().waitFor();
      await page.keyboard.press("Tab");
      await expect(argsFrame.getByRole("button", name("message.code.wrap.enable"))).toBeFocused();
      await page.keyboard.press("Enter");
      const argsWrap = argsFrame.getByRole("button", name("message.code.wrap.disable"));
      await expect(argsWrap).toBeFocused();
      await expect(page.getByRole("button", name("message.code.wrap.disable"))).toHaveCount(2);
      await page.keyboard.press("Tab");
      await expect(args).toBeFocused();
      await page.keyboard.press("End");
      await atTail(args);
      const source = await args.innerText();
      expect(JSON.parse(source)).toEqual({
        workspace: {
          root: `/Users/visual/Flame workspaces/${"application-boundary-".repeat(12)}review`,
          include: ["runtime/internal/session/**", "apps/web/src/**"],
        },
        checks: ["ownership", "dependency-direction"],
        notes: "Keep <component> tags and source paths as literal text.",
      });
      await expect(args.locator("component")).toHaveCount(0);
      await expect
        .poll(() => args.evaluate((node) => node.scrollWidth - node.clientWidth))
        .toBeLessThanOrEqual(1);

      const reader = (await args.elementHandle())!;
      const retained = async () => {
        expect(
          await reader.evaluate((node) => node.isConnected && node === document.activeElement),
        ).toBe(true);
        expect(await args.evaluate((node, held) => node === held, reader)).toBe(true);
        expect(await composer.evaluate((node, held) => node === held, input)).toBe(true);
        expect(await args.innerText()).toBe(source);
        await expect(composer).toHaveValue(draft);
        expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(
          0,
        );
      };
      await page.setViewportSize({ width: 640, height: 400 });
      await retained();
      await capture("arguments-narrow");
      const reflow = [];
      for (let zoom = 200; zoom <= 300; zoom += 5) {
        await page.setViewportSize({
          width: Math.floor(112000 / zoom),
          height: Math.floor(75500 / zoom),
        });
        reflow.push(
          await args.evaluate(async (node) => {
            for (let frame = 0; frame < 8; frame += 1) await new Promise(requestAnimationFrame);
            const viewport = node.closest<HTMLElement>(".msg-scroll-viewport")!;
            const rect = viewport.getBoundingClientRect();
            const style = getComputedStyle(viewport);
            return {
              width: innerWidth,
              height: innerHeight,
              bounds: node.getBoundingClientRect().toJSON(),
              top: rect.top + parseFloat(style.scrollPaddingTop),
              bottom: rect.bottom - parseFloat(style.scrollPaddingBottom),
              scroll: viewport.scrollTop,
            };
          }),
        );
      }
      await testInfo.attach(`continuous-tool-reflow-${locale}`, {
        body: JSON.stringify(reflow),
        contentType: "application/json",
      });
      for (const frame of reflow) {
        expect(frame.bounds.top).toBeGreaterThanOrEqual(frame.top - 1);
        expect(frame.bounds.bottom).toBeLessThanOrEqual(frame.bottom + 1);
      }
      await page.setViewportSize({ width: 373, height: 251 });
      await retained();
      await args.evaluate(async () => {
        for (let frame = 0; frame < 8; frame += 1) await new Promise(requestAnimationFrame);
      });
      await page.keyboard.press("End");
      await atTail(args);
      const observeReading = () =>
        args.evaluate(async (node, jumpLabel) => {
          const viewport = node.closest<HTMLElement>(".msg-scroll-viewport")!;
          const jump = [...document.querySelectorAll<HTMLButtonElement>("button")].find(
            (button) => button.getAttribute("aria-label") === jumpLabel,
          );
          const frames = [];
          for (let frame = 0; frame < 8; frame += 1) {
            const bounds = node.getBoundingClientRect();
            const outside = viewport.getBoundingClientRect();
            const style = getComputedStyle(viewport);
            const top = outside.top + parseFloat(style.scrollPaddingTop);
            const bottom = outside.bottom - parseFloat(style.scrollPaddingBottom);
            const action = jump?.getBoundingClientRect();
            const overlap = action
              ? Math.max(
                  0,
                  Math.min(bounds.right, action.right) - Math.max(bounds.left, action.left),
                ) *
                Math.max(
                  0,
                  Math.min(bounds.bottom, action.bottom) - Math.max(bounds.top, action.top),
                )
              : 0;
            const padding = getComputedStyle(node);
            const left = bounds.left + parseFloat(padding.paddingLeft);
            const right = bounds.right - parseFloat(padding.paddingRight);
            const start = bounds.top + parseFloat(padding.paddingTop);
            const end = bounds.bottom - parseFloat(padding.paddingBottom);
            // Native overlay scrollbars can occupy the padding outside the source area.
            const points = [
              [left, start],
              [right, start],
              [bounds.left + bounds.width / 2, bounds.top + bounds.height / 2],
              [left, end],
              [right, end],
            ];
            const hits = points.map(([x, y]) => {
              const hit = document.elementFromPoint(x!, y!);
              return {
                point: [x, y],
                tag: hit?.localName,
                label: hit?.getAttribute("aria-label"),
                slot: hit?.getAttribute("data-slot"),
                contained: hit !== null && node.contains(hit),
              };
            });
            frames.push({
              bounds: bounds.toJSON(),
              client: [node.clientWidth, node.clientHeight],
              top,
              bottom,
              overlap,
              hits,
              readable: hits.every((hit) => hit.contained),
            });
            await new Promise(requestAnimationFrame);
          }
          return frames;
        }, t("chat.jumpToBottom"));
      const expectReading = async (label: string) => {
        const reading = await observeReading();
        await testInfo.attach(`tool-reading-${label}-${locale}`, {
          body: JSON.stringify(reading),
          contentType: "application/json",
        });
        for (const frame of reading) {
          expect(frame.bounds.top).toBeGreaterThanOrEqual(frame.top - 1);
          expect(frame.bounds.bottom).toBeLessThanOrEqual(frame.bottom + 1);
          expect(frame.overlap, "floating actions must remain outside the focused reader").toBe(0);
          expect(frame.readable, "the focused code reader must admit pointer reading").toBe(true);
        }
      };
      await expectReading("short");
      await capture("arguments-short");
      await atTail(args);
      await args.hover();
      const beforeWheel = await args.evaluate((node) => node.scrollTop);
      await page.mouse.wheel(0, -80);
      await expect.poll(() => args.evaluate((node) => node.scrollTop)).toBeLessThan(beforeWheel);
      for (const size of [
        { width: 504, height: 316 },
        { width: 373, height: 251 },
      ]) {
        await page.setViewportSize(size);
        await args.evaluate(async () => {
          for (let frame = 0; frame < 8; frame += 1) await new Promise(requestAnimationFrame);
        });
        await retained();
        await expectReading(`pointer-${size.width}`);
      }
      await capture("arguments-pointer-restored");
      await page.setViewportSize({ width: 1120, height: 720 });
      await retained();
      await page.keyboard.press("End");
      await atTail(args);
      await capture("arguments-wrapped-tail");

      await page.keyboard.press("Shift+Tab");
      await expect(argsWrap).toBeFocused();
      await page.keyboard.press("Shift+Tab");
      await expect(argumentsButton).toBeFocused();
      await page.keyboard.press("Enter");
      await expect(argumentsButton).toHaveAttribute("aria-expanded", "false");
      for (const stop of [
        result,
        resultFrame.getByRole("button", name("message.code.copy")),
        resultFrame.getByRole("button", name("message.code.wrap.disable")),
        copyArguments,
        trigger,
      ]) {
        await page.keyboard.press("Shift+Tab");
        await expect(stop).toBeFocused();
      }
      await page.keyboard.press("Enter");
      await expect(trigger).toHaveAttribute("aria-expanded", "false");
      await expect
        .poll(() =>
          resultNode.evaluate((node) => {
            const closed = node.closest<HTMLElement>("[inert]");
            return closed
              ? {
                  height: closed.getBoundingClientRect().height,
                  opacity: getComputedStyle(closed).opacity,
                }
              : null;
          }),
        )
        .toEqual({ height: 0, opacity: "0" });
      await expect(trigger).toBeFocused();

      for (const stop of [
        copyArguments,
        page.getByRole("button", name("msgActions.copy")).last(),
        page.getByRole("button", name("msgActions.regenerate")),
        composer,
      ]) {
        await page.keyboard.press("Tab");
        await expect(stop).toBeFocused();
      }
      await expect(composer).toHaveValue(draft);
      await page.keyboard.press("ControlOrMeta+A");
      await page.keyboard.press("ArrowRight");
      expect(
        await composer.evaluate((node: HTMLTextAreaElement) => [
          node.selectionStart,
          node.selectionEnd,
        ]),
      ).toEqual([draft.length, draft.length]);
      await page.keyboard.type(continuation);
      await expect(composer).toHaveValue(draft + continuation);
      await capture("continued-draft");
    },
  );
}
