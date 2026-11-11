import { expect, test, type Locator } from "./test";
import { en } from "@/lib/i18n/locales/en";
import { presentationTitle } from "@/lib/i18n/presentationTitle";
import brandDeclaration from "../../../../plugins/brand/package/io.github.tangerg.flame/declaration.json" with { type: "json" };
import providersDeclaration from "../../../../plugins/providers/package/io.github.tangerg.flame/declaration.json" with { type: "json" };
import mcpDeclaration from "../../../../plugins/mcp/package/io.github.tangerg.flame/declaration.json" with { type: "json" };
import hooksDeclaration from "../../../../plugins/hooks/package/io.github.tangerg.flame/declaration.json" with { type: "json" };
import diagnosticsDeclaration from "../../../../plugins/diagnostics/package/io.github.tangerg.flame/declaration.json" with { type: "json" };
import galleryDeclaration from "../../../../plugins/icon-gallery/package/io.github.tangerg.flame/declaration.json" with { type: "json" };
import trajectoryDeclaration from "../../../../plugins/trajectory/package/io.github.tangerg.flame/declaration.json" with { type: "json" };
import diffDeclaration from "../../../../plugins/diff/package/io.github.tangerg.flame/declaration.json" with { type: "json" };
import usageDeclaration from "../../../../plugins/usage/package/io.github.tangerg.flame/declaration.json" with { type: "json" };
import reportsDeclaration from "../../../../plugins/usagereports/package/io.github.tangerg.flame/declaration.json" with { type: "json" };
import memoryDeclaration from "../../../../plugins/memory/package/io.github.tangerg.flame/declaration.json" with { type: "json" };
import schedulesDeclaration from "../../../../plugins/schedules/package/io.github.tangerg.flame/declaration.json" with { type: "json" };
import skillsDeclaration from "../../../../plugins/skills/package/io.github.tangerg.flame/declaration.json" with { type: "json" };

import { expectLabReady } from "../readiness";
import zh from "../../../../plugins/locale-zh/package/io.github.tangerg.flame/messages.json" with { type: "json" };

async function composerReturnFrames(scrollport: Locator) {
  return scrollport.evaluate(async (node) => {
    const frames = [];
    for (let frame = 0; frame < 8; frame++) {
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
      frames.push({
        top: node.scrollTop,
        height: node.clientHeight,
        total: node.scrollHeight,
        rect: node.getBoundingClientRect().toJSON(),
        focus: document.activeElement?.getAttribute("aria-label"),
        documentTop: document.scrollingElement?.scrollTop,
        parts: Array.from(
          node.querySelectorAll<HTMLElement>(
            '[data-slot="active-plan-surface"], [data-slot="composer-root"], textarea, [data-slot="composer-footer"], [data-slot="composer-top-tray-surface"]',
          ),
        ).map((part) => ({
          tag: part.localName,
          slot: part.getAttribute("data-slot"),
          rect: part.getBoundingClientRect().toJSON(),
          height: part.clientHeight,
          total: part.scrollHeight,
        })),
      });
    }
    return frames;
  });
}

for (const browserName of ["chromium", "webkit"] as const) {
  const continuityTest = test.extend({ browserName });
  for (const profile of [
    { locale: "en", theme: "light", fontSize: 14, messages: en },
    { locale: "zh", theme: "dark", fontSize: 18, messages: zh },
  ]) {
    continuityTest(
      `${browserName} composer material continuity in ${profile.locale}`,
      async ({ page }, testInfo) => {
        const messages: Record<string, string> = profile.messages;
        await page.setViewportSize({ width: 1472, height: 1000 });
        await page.goto(
          `/lab/fixture.html?${new URLSearchParams({
            fixture: "workspace",
            state: "dock-schedules",
            "agent-state": "running",
            locale: profile.locale,
            theme: profile.theme,
            "font-size": String(profile.fontSize),
            motion: "off",
          })}`,
        );
        await expectLabReady(page);
        await page
          .getByRole("button", { name: "Collapse the workspace fixture sidebar", exact: true })
          .click();
        await page.getByRole("button", { name: messages["dock.action.hide"], exact: true }).click();
        const draft = page.getByRole("textbox", {
          name: messages["composer.input.label"],
          exact: true,
        });
        const value = "  Keep this draft while reading the material.  ";
        await draft.fill(value);
        await draft.evaluate((node: HTMLTextAreaElement) => node.setSelectionRange(2, 12));
        const input = await draft.elementHandle();
        const conversationNode = await page
          .getByRole("region", {
            name: messages["shell.region.conversation"],
            exact: true,
          })
          .elementHandle();
        const scrollport = page.locator('[data-slot="composer-scrollport"]');
        const originalPort = await scrollport.elementHandle();
        await page.setViewportSize({ width: 373, height: 243 });
        for (const button of await page
          .locator('[data-slot="composer-footer"]')
          .getByRole("button")
          .all()) {
          await page.keyboard.press("Tab");
          await expect(button).toBeFocused();
        }
        await expect(
          page.getByRole("button", { name: messages["composer.action.stop"], exact: true }),
        ).toBeFocused();
        for (let visit = 0; visit < 3; visit++) {
          if (visit > 0) {
            const previousTop = await scrollport.evaluate((node) => node.scrollTop);
            const bounds = (await scrollport.boundingBox())!;
            await page.mouse.move(bounds.x + bounds.width - 1, bounds.y + bounds.height / 2);
            await page.mouse.wheel(0, visit === 1 ? -12 : 2000);
            await expect
              .poll(async () => {
                const positions = (await composerReturnFrames(scrollport)).map(
                  (frame) => frame.top,
                );
                return (
                  Math.max(...positions) - Math.min(...positions) < 1 &&
                  positions.at(-1)! > 0 &&
                  (visit === 1 ? positions.at(-1)! < previousTop : positions.at(-1)! > previousTop)
                );
              })
              .toBe(true);
          }
          const before = await composerReturnFrames(scrollport);
          const top = before.at(-1)!.top;
          expect(top).toBeGreaterThan(0);
          await testInfo.attach(`composer-before-material-${visit}`, {
            body: JSON.stringify(before),
            contentType: "application/json",
          });
          await testInfo.attach(`composer-before-material-${visit}-view`, {
            body: await page.screenshot({ animations: "disabled" }),
            contentType: "image/png",
          });
          await page
            .getByRole("button", { name: messages["dock.action.openFull"], exact: true })
            .click();
          await expect(
            page.getByRole("region", { name: messages["shell.region.conversation"], exact: true }),
          ).toHaveCount(0);
          expect(
            await conversationNode!.evaluate((node) => {
              const focused = node.ownerDocument.activeElement;
              node.focus({ preventScroll: true });
              return node.ownerDocument.activeElement === focused && focused !== node;
            }),
          ).toBe(true);
          const search = page.getByRole("searchbox");
          await search.fill("Nightly");
          await expect(search).toHaveValue("Nightly");
          const hidden = await composerReturnFrames(scrollport);
          await testInfo.attach(`composer-hidden-for-material-${visit}`, {
            body: JSON.stringify(hidden),
            contentType: "application/json",
          });
          for (const frame of hidden) {
            expect(frame.height).toBeGreaterThan(0);
            expect(frame.total).toBe(before.at(-1)!.total);
          }
          await page.getByRole("button", { name: messages["common.close"], exact: true }).click();
          const conversation = page.getByRole("region", {
            name: messages["shell.region.conversation"],
            exact: true,
          });
          await expect(conversation).toBeFocused();
          const returned = await composerReturnFrames(scrollport);
          await testInfo.attach(`composer-after-material-${visit}`, {
            body: JSON.stringify(returned),
            contentType: "application/json",
          });
          await testInfo.attach(`composer-after-material-${visit}-view`, {
            body: await page.screenshot({ animations: "disabled" }),
            contentType: "image/png",
          });
          for (const frame of returned) {
            expect(Math.abs(frame.top - top)).toBeLessThanOrEqual(1);
            expect(frame.focus).toBe(messages["shell.region.conversation"]);
            expect(frame.documentTop).toBe(0);
          }
          await expect(draft).toHaveValue(value);
          expect(await draft.evaluate((node, original) => node === original, input)).toBe(true);
          expect(
            await scrollport.evaluate((node, original) => node === original, originalPort),
          ).toBe(true);
          expect(
            await draft.evaluate((node: HTMLTextAreaElement) => [
              node.selectionStart,
              node.selectionEnd,
            ]),
          ).toEqual([2, 12]);
        }
        await draft.focus();
        await page.keyboard.press("ControlOrMeta+ArrowDown");
        await page.keyboard.insertText(" Continue editing.");
        await expect(draft).toHaveValue(`${value} Continue editing.`);
      },
    );
  }
}

for (const browserName of ["chromium", "webkit"] as const) {
  const headerTest = test.extend({ browserName });
  for (const profile of [
    { locale: "en", theme: "light", messages: en },
    { locale: "zh", theme: "dark", messages: zh },
  ]) {
    headerTest(
      `${browserName} running conversation header preserves action space in ${profile.locale}`,
      async ({ page }, testInfo) => {
        const messages: Record<string, string> = profile.messages;
        await page.setViewportSize({ width: 1120, height: 728 });
        await page.goto(
          `/lab/fixture.html?${new URLSearchParams({
            fixture: "workspace",
            state: "dock-schedules",
            "agent-state": "running",
            locale: profile.locale,
            theme: profile.theme,
            "font-size": "18",
            motion: "off",
          })}`,
        );
        await expectLabReady(page);
        await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
        await page.getByRole("button", { name: messages["dock.action.hide"], exact: true }).click();
        const composer = page.getByRole("textbox", {
          name: messages["composer.input.label"],
          exact: true,
        });
        const draft = "  Keep this draft while reviewing changes. 保留原始草稿。  ";
        await composer.fill(draft);
        const conversation = page.getByRole("region", {
          name: messages["shell.region.conversation"],
          exact: true,
        });
        const header = conversation.locator(".agent-surface-header");
        const changes = header.getByRole("button", {
          name: profile.locale === "zh" ? /^查看工作区差异/ : /^Review working tree changes/,
        });
        for (const width of [373, 320, 480, 1120]) {
          await page.setViewportSize({ width, height: 568 });
          const geometry = await header.evaluate(async (node) => {
            await new Promise(requestAnimationFrame);
            await new Promise(requestAnimationFrame);
            return {
              width: node.clientWidth,
              total: node.scrollWidth,
              children: [...node.children].map((child) => ({
                text: child.textContent,
                rect: child.getBoundingClientRect().toJSON(),
                minWidth: getComputedStyle(child).minWidth,
                shrink: getComputedStyle(child).flexShrink,
              })),
            };
          });
          await testInfo.attach(`running-header-${width}-geometry`, {
            body: JSON.stringify(geometry),
            contentType: "application/json",
          });
          await testInfo.attach(`running-header-${width}-view`, {
            body: await page.screenshot({ animations: "disabled" }),
            contentType: "image/png",
          });
          const toggle = page.getByRole("button", {
            name: messages[width < 760 ? "dock.action.openFull" : "dock.action.show"],
            exact: true,
          });
          const action = (await changes.boundingBox())!;
          const dock = (await toggle.boundingBox())!;
          expect(action.x + action.width).toBeLessThanOrEqual(dock.x);
          expect(geometry.total).toBeLessThanOrEqual(geometry.width);
          await expect(header.getByRole("heading", { level: 1 })).toBeVisible();
          for (const button of await header.getByRole("button").all()) {
            const bounds = (await button.boundingBox())!;
            expect(bounds.width).toBeGreaterThanOrEqual(24);
            expect(bounds.height).toBeGreaterThanOrEqual(24);
            await expectCentreHit(button);
          }
          await expectCentreHit(toggle);
          if (width === 320) {
            const status = header.locator('[data-slot="run-status"]');
            const label = (await status.textContent())!;
            const identityMenu = header.getByRole("button").first();
            await page
              .getByRole("button", { name: "Expand the workspace fixture sidebar", exact: true })
              .focus();
            await page.keyboard.press("Tab");
            await expect(identityMenu).toBeFocused();
            await expectCentreHit(identityMenu);
            await page.keyboard.press("Enter");
            const menu = page.getByRole("menu", { name: "Agent · running", exact: true });
            await expect(menu).toBeVisible();
            await expect(menu).toContainText(label);
            await testInfo.attach("running-header-keyboard-status", {
              body: await page.screenshot({ animations: "disabled" }),
              contentType: "image/png",
            });
            await page.keyboard.press("Escape");
            await expect(menu).toHaveCount(0);
            await expect(identityMenu).toBeFocused();
          }
        }
        await page.setViewportSize({ width: 373, height: 243 });
        const status = header.locator('[data-slot="run-status"]');
        const label = (await status.textContent())!;
        await status.hover();
        await expect(page.getByRole("tooltip")).toHaveText(label);
        await testInfo.attach("running-header-pointer-status", {
          body: await page.screenshot({ animations: "disabled" }),
          contentType: "image/png",
        });
        await changes.focus();
        await expect(changes).toBeFocused();
        await expectCentreHit(changes);
        const opener = await changes.elementHandle();
        await page.keyboard.press("Enter");
        await expect(page.getByTestId("active-dock-view")).toHaveText("package:visual:diff");
        await expect(page.getByRole("tooltip").filter({ hasText: label })).toHaveCount(0);
        expect(await opener!.evaluate((node) => node.isConnected)).toBe(false);
        const back = page.getByRole("button", {
          name: messages["dock.action.returnToConversation"],
          exact: true,
        });
        await back.click();
        await expect(conversation).toBeFocused();
        await changes.click();
        await expect(page.getByTestId("active-dock-view")).toHaveText("package:visual:diff");
        await back.click();
        await expect(conversation).toBeFocused();
        await expect(composer).toHaveValue(draft);
        await composer.focus();
        await page.keyboard.press("ControlOrMeta+ArrowDown");
        await page.keyboard.insertText(" Continue.");
        await expect(composer).toHaveValue(draft + " Continue.");
      },
    );
  }
}

for (const browserName of ["chromium", "webkit"] as const) {
  const materialTest = test.extend({ browserName });
  for (const profile of [
    { locale: "en", theme: "light", messages: en },
    { locale: "zh", theme: "dark", messages: zh },
  ]) {
    materialTest(
      `${browserName} full-width material preserves its name and return path in ${profile.locale}`,
      async ({ page }, testInfo) => {
        const messages: Record<string, string> = profile.messages;
        await page.setViewportSize({ width: 1120, height: 728 });
        await page.goto(
          `/lab/fixture.html?${new URLSearchParams({
            fixture: "workspace",
            state: "dock-schedules",
            "agent-state": "running",
            locale: profile.locale,
            theme: profile.theme,
            "font-size": "18",
            motion: "off",
          })}`,
        );
        await expectLabReady(page);
        await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
        await page.getByRole("button", { name: messages["dock.action.hide"], exact: true }).click();
        const conversation = page.getByRole("region", {
          name: messages["shell.region.conversation"],
          exact: true,
        });
        const composer = conversation.getByRole("textbox", {
          name: messages["composer.input.label"],
          exact: true,
        });
        const draft = "  Keep this draft while identifying the material. 保留原文。  ";
        await composer.fill(draft);
        const changes = conversation.getByRole("button", {
          name: profile.locale === "zh" ? /^查看工作区差异/ : /^Review working tree changes/,
        });
        await changes.focus();
        await page.keyboard.press("Enter");
        await page
          .getByRole("button", { name: messages["dock.action.expand"], exact: true })
          .click();
        const header = page.locator('[data-slot="workspace-window-header"]');
        const title = header.locator(":scope > span[title]");
        const diff = diffDeclaration.components.find((entry) => entry.id === "diff")!;
        await expect(title).toHaveText(
          presentationTitle(diff.title, diff.localizedTitles, profile.locale),
        );
        const frames = [];
        for (const width of [373, 320, 480, 1120]) {
          await page.setViewportSize({ width, height: 568 });
          const geometry = await header.evaluate(async (node) => {
            await new Promise(requestAnimationFrame);
            await new Promise(requestAnimationFrame);
            const title = node.querySelector<HTMLElement>(":scope > span[title]")!;
            return {
              width: node.clientWidth,
              total: node.scrollWidth,
              title: {
                width: title.clientWidth,
                total: title.scrollWidth,
                rect: title.getBoundingClientRect().toJSON(),
              },
              actions: Array.from(node.querySelectorAll("button"), (button) => {
                const rect = button.getBoundingClientRect();
                const hit = node.ownerDocument.elementFromPoint(
                  rect.x + rect.width / 2,
                  rect.y + rect.height / 2,
                );
                return {
                  label: button.getAttribute("aria-label"),
                  rect: rect.toJSON(),
                  reachable: hit !== null && button.contains(hit),
                };
              }),
            };
          });
          frames.push(geometry);
          await testInfo.attach(`material-header-${width}-geometry`, {
            body: JSON.stringify(geometry),
            contentType: "application/json",
          });
          await testInfo.attach(`material-header-${width}-view`, {
            body: await page.screenshot({ animations: "disabled" }),
            contentType: "image/png",
          });
        }
        for (const frame of frames) {
          expect(frame.total, `header at ${frame.width}px`).toBeLessThanOrEqual(frame.width);
          expect(frame.title.width, `title at ${frame.width}px`).toBeGreaterThan(0);
          if (frame.width >= 373)
            expect(frame.title.total, `complete name at ${frame.width}px`).toBeLessThanOrEqual(
              frame.title.width,
            );
          for (const action of frame.actions) {
            expect(action.reachable, action.label ?? "material action").toBe(true);
            expect(action.rect.width).toBeGreaterThanOrEqual(24);
            expect(action.rect.height).toBeGreaterThanOrEqual(24);
          }
        }
        await page.setViewportSize({ width: 373, height: 243 });
        const back = header.getByRole("button", {
          name: messages["dock.action.returnToConversation"],
          exact: true,
        });
        await page
          .getByRole("button", { name: "Expand the workspace fixture sidebar", exact: true })
          .focus();
        await page.keyboard.press("Tab");
        await expect(back).toBeFocused();
        await expectCentreHit(back);
        await expect(page.getByRole("tooltip")).toHaveText(
          messages["dock.action.returnToConversation"]!,
        );
        await testInfo.attach("material-header-keyboard-return", {
          body: await page.screenshot({ animations: "disabled" }),
          contentType: "image/png",
        });
        await page.keyboard.press("Enter");
        await expect(conversation).toBeFocused();
        await expect(composer).toHaveValue(draft);
        await changes.click();
        await expect(header).toBeVisible();
        await back.hover();
        await expect(page.getByRole("tooltip")).toHaveText(
          messages["dock.action.returnToConversation"]!,
        );
        await testInfo.attach("material-header-pointer-return", {
          body: await page.screenshot({ animations: "disabled" }),
          contentType: "image/png",
        });
        await back.click();
        await expect(conversation).toBeFocused();
        await expect(composer).toHaveValue(draft);
        await page.setViewportSize({ width: 1120, height: 728 });
        await composer.focus();
        await page.keyboard.press("ControlOrMeta+ArrowDown");
        await page.keyboard.insertText(" Continue.");
        await expect(composer).toHaveValue(draft + " Continue.");
      },
    );
  }
}

async function expectBelowWindowControl(content: Locator, toggle: Locator) {
  await expect(content).toBeVisible();
  const control = (await toggle.boundingBox())!;
  const bounds = (await content.boundingBox())!;
  expect(bounds.y).toBeGreaterThanOrEqual(control.y + control.height);
}

async function expectCentreHit(control: Locator) {
  expect(
    await control.evaluate((node) => {
      const bounds = node.getBoundingClientRect();
      const target = document.elementFromPoint(
        bounds.x + bounds.width / 2,
        bounds.y + bounds.height / 2,
      );
      return target !== null && node.contains(target);
    }),
  ).toBe(true);
}

async function stateRowFrames(row: Locator) {
  return row.evaluate(async (node) => {
    let viewport = node.parentElement;
    while (viewport && !/(auto|scroll)/.test(getComputedStyle(viewport).overflowY)) {
      viewport = viewport.parentElement;
    }
    if (!viewport) throw new Error("the state row has no scrollport");
    const frames = [];
    await new Promise(requestAnimationFrame);
    for (let frame = 0; frame < 8; frame += 1) {
      await new Promise(requestAnimationFrame);
      const bounds = viewport.getBoundingClientRect();
      const top = bounds.top + viewport.clientTop;
      const left = bounds.left + viewport.clientLeft;
      frames.push({
        row: node.getBoundingClientRect().toJSON(),
        viewport: {
          top,
          bottom: top + viewport.clientHeight,
          left,
          right: left + viewport.clientWidth,
        },
        window: { width: innerWidth, height: innerHeight },
        focused: document.activeElement === node,
        offsetTop: (node as HTMLElement).offsetTop,
        scrollTop: viewport.scrollTop,
        scrollHeight: viewport.scrollHeight,
      });
    }
    return frames;
  });
}

test("Workspace layout observation is opt-in and preserves the material and draft", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1472, height: 1000 });
  await page.goto(
    "/lab/fixture.html?fixture=workspace&state=dock-skills-catalog&locale=en&theme=light&font-size=14&motion=off",
  );
  await expectLabReady(page);
  const observe = page.getByRole("checkbox", { name: "Observe layout", exact: true });
  const output = page.getByTestId("workspace-layout-observation");
  const readingOutput = page.getByTestId("workspace-reading-observation");
  const transcriptOutput = page.getByTestId("transcript-reading-observation");
  await expect(observe).not.toBeChecked();
  await expect(output).toHaveText("");
  await expect(readingOutput).toHaveText("");
  await expect(transcriptOutput).toHaveText("");
  await observe.click();
  await expect(observe).toBeChecked();
  await expect
    .poll(async () => {
      const value = await output.textContent();
      return value ? (JSON.parse(value) as { window: { width: number } }).window.width : null;
    })
    .toBe(1472);
  await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
  const composer = page.getByRole("textbox", { name: "Message composer", exact: true });
  const draft = "Unsent layout-observation draft — 保留原始输入。";
  await composer.fill(draft);
  const composerObservationFrames = () =>
    output.evaluate(async (node) => {
      const surface = node.ownerDocument.querySelector<HTMLElement>('[data-slot="composer-root"]')!;
      const scrollport = node.ownerDocument.querySelector<HTMLElement>(
        '[data-slot="composer-scrollport"]',
      )!;
      const frames = [];
      for (let frame = 0; frame < 8; frame++) {
        await new Promise(requestAnimationFrame);
        const observed = JSON.parse(node.textContent!) as {
          composer: {
            clientWidth: number;
            scrollport: { top: number; height: number; total: number };
          };
        };
        frames.push({
          measured: {
            width: surface.clientWidth,
            top: scrollport.scrollTop,
            height: scrollport.clientHeight,
            total: scrollport.scrollHeight,
          },
          reported: { width: observed.composer.clientWidth, ...observed.composer.scrollport },
        });
      }
      return frames;
    });
  const expectObservedComposer = async (label: string) => {
    let frames = await composerObservationFrames();
    await expect
      .poll(async () => {
        frames = await composerObservationFrames();
        return frames.every(
          ({ measured, reported }) =>
            measured.width === frames[0]!.measured.width &&
            measured.width === reported.width &&
            measured.top === reported.top &&
            measured.height === reported.height &&
            measured.total === reported.total,
        );
      })
      .toBe(true);
    await test
      .info()
      .attach(label, { body: JSON.stringify(frames), contentType: "application/json" });
    return frames.at(-1)!.measured.width;
  };
  const initialComposerWidth = await expectObservedComposer("workspace-composer-initial-frames");
  const messageAction = page.locator(".msg-scroll-viewport").getByRole("button", {
    name: "Regenerate response",
    exact: true,
  });
  await messageAction.focus();
  await expect
    .poll(async () =>
      transcriptOutput.evaluate((node) => {
        const viewport = document.querySelector<HTMLElement>(".msg-scroll-viewport")!;
        const rect = viewport.getBoundingClientRect();
        const style = getComputedStyle(viewport);
        const focus = document.activeElement!;
        const bounds = focus.getBoundingClientRect();
        const reported = JSON.parse(node.textContent!) as {
          frame: number[];
          focus: { bounds: [number, number] } | null;
        };
        return (
          reported.focus?.bounds[0] === bounds.top &&
          reported.focus?.bounds[1] === bounds.bottom &&
          reported.frame[0] === rect.top + parseFloat(style.scrollPaddingTop) &&
          reported.frame[1] === rect.bottom - parseFloat(style.scrollPaddingBottom)
        );
      }),
    )
    .toBe(true);
  const search = page.getByRole("searchbox", { name: "Search this catalog", exact: true });
  const viewport = page.locator('[data-slot="workspace-view-body"]:visible');
  const reportedHeight = async () => {
    const value = await output.textContent();
    return value ? (JSON.parse(value) as { viewport: { total: number } }).viewport.total : null;
  };
  await search.focus();
  const initialHeight = await viewport.evaluate((node) => node.scrollHeight);
  await expect.poll(reportedHeight).toBe(initialHeight);
  await search.fill("runtime");
  await expect(
    page.getByRole("heading", { name: "40 of 80 available Skills", exact: true }),
  ).toBeVisible();
  const filteredFrames = await output.evaluate(async (node) => {
    const frames = [];
    await new Promise(requestAnimationFrame);
    for (let frame = 0; frame < 8; frame += 1) {
      await new Promise(requestAnimationFrame);
      const observed = JSON.parse(node.textContent!) as { viewport: { total: number } };
      const reader = document.querySelector<HTMLElement>('[data-slot="workspace-view-body"]')!;
      frames.push({ reported: observed.viewport.total, measured: reader.scrollHeight });
    }
    return frames;
  });
  await test.info().attach("workspace-layout-filtered-frames", {
    body: JSON.stringify({ initialHeight, filteredFrames }),
    contentType: "application/json",
  });
  for (const frame of filteredFrames) {
    expect(frame.measured).toBeLessThan(initialHeight);
    expect(frame.reported).toBe(frame.measured);
  }
  await search.fill("");
  await expect.poll(reportedHeight).toBe(initialHeight);
  await search.fill("runtime");
  await expect.poll(reportedHeight).toBe(filteredFrames[0]!.measured);
  const reader = await viewport.elementHandle();
  const reading = viewport.getByRole("button").filter({
    has: page.getByText("runtime-review-035", { exact: true }),
  });
  await reading.focus();
  await expect(reading).toBeFocused();
  await expect
    .poll(async () => {
      const value = await output.textContent();
      return value
        ? ((JSON.parse(value) as { reading: { name: string } | null }).reading?.name ?? "")
        : "";
    })
    .toContain("runtime-review-035");
  await expect(output).not.toContainText(draft);
  await expect(readingOutput).toContainText("runtime-review-035");
  await expect(readingOutput).not.toContainText(draft);
  await expect(composer).toHaveValue(draft);
  await expect(transcriptOutput).toContainText('"frame":');
  await expect(transcriptOutput).not.toContainText(draft);
  await expect(search).toHaveValue("runtime");
  expect(await viewport.evaluate((node, before) => node === before, reader)).toBe(true);

  await page.setViewportSize({ width: 1120, height: 728 });
  const resizedComposerWidth = await expectObservedComposer("workspace-composer-resized-frames");
  expect(resizedComposerWidth).not.toBe(initialComposerWidth);
  await expect(output).not.toContainText(draft);
  await expect(readingOutput).not.toContainText(draft);
  await page.getByRole("button", { name: "Expand the workspace fixture sidebar" }).click();
  await observe.click();
  await expect(observe).not.toBeChecked();
  await expect(output).toHaveText("");
  await expect(readingOutput).toHaveText("");
  await expect(transcriptOutput).toHaveText("");
  await search.fill("");
  await page.setViewportSize({ width: 1400, height: 900 });
  const retired = await output.evaluate(async (node) => {
    const frames = [];
    for (let frame = 0; frame < 3; frame += 1) {
      await new Promise(requestAnimationFrame);
      frames.push(node.textContent);
    }
    return frames;
  });
  expect(retired).toEqual(["", "", ""]);
  await expect(readingOutput).toHaveText("");
  await expect(transcriptOutput).toHaveText("");
  await expect(composer).toHaveValue(draft);
  expect(await viewport.evaluate((node, before) => node === before, reader)).toBe(true);
});

for (const profile of [
  { locale: "en", theme: "light", fontSize: 14, composer: "Message composer" },
  { locale: "zh", theme: "dark", fontSize: 18, composer: "消息输入框" },
]) {
  test(`Workspace state reading retains its focused row through native-sized reflow ${profile.locale}`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    const query = new URLSearchParams({
      fixture: "workspace",
      state: "dock-skills-catalog",
      locale: profile.locale,
      theme: profile.theme,
      "font-size": String(profile.fontSize),
      motion: "off",
    });
    await page.goto(`/lab/fixture.html?${query}`);
    await expectLabReady(page);
    const composer = page.getByRole("textbox", { name: profile.composer, exact: true });
    const draft = "Keep the current reading draft.";
    await composer.fill(draft);
    await page
      .getByRole("button", {
        name: profile.locale === "zh" ? "计划任务" : "Schedules",
        exact: true,
      })
      .focus();
    await page.keyboard.press("Tab");
    const row = page.getByRole("button", {
      name: profile.locale === "zh" ? "计划任务 · 模型选择" : "Schedules · model choices",
      exact: true,
    });
    await expect(row).toBeFocused();
    const samples: unknown[] = [];
    const expectReading = async (target: Locator, focused: boolean) => {
      const frames = await stateRowFrames(target);
      samples.push(frames);
      for (const frame of frames) {
        expect(frame.focused).toBe(focused);
        expect(frame.row.top).toBeGreaterThanOrEqual(Math.max(0, frame.viewport.top) - 1);
        expect(frame.row.bottom).toBeLessThanOrEqual(
          Math.min(frame.window.height, frame.viewport.bottom) + 1,
        );
        expect(frame.row.left).toBeGreaterThanOrEqual(Math.max(0, frame.viewport.left) - 1);
        expect(frame.row.right).toBeLessThanOrEqual(
          Math.min(frame.window.width, frame.viewport.right) + 1,
        );
      }
    };
    try {
      await page.setViewportSize({ width: 720, height: 450 });
      await expectReading(row, true);
      await page.setViewportSize({ width: 1440, height: 900 });
      await expectReading(row, true);
      await composer.focus();
      await page.setViewportSize({ width: 720, height: 450 });
      const selected = page.getByRole("button", {
        name: profile.locale === "zh" ? "技能 · 目录" : "Skills · catalog",
        exact: true,
      });
      await expectReading(selected, false);
      await expect(composer).toBeFocused();
      await expect(composer).toHaveValue(draft);
      await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
      await composer.focus();
      await composer.press(process.platform === "darwin" ? "Meta+ArrowDown" : "Control+End");
      await page.keyboard.insertText(" Continue.");
      await expect(composer).toHaveValue(draft + " Continue.");
    } finally {
      await testInfo.attach("workspace-state-reading-frames", {
        body: JSON.stringify(samples),
        contentType: "application/json",
      });
      await testInfo.attach(`workspace-state-reflow-${profile.locale}`, {
        body: await page.screenshot({ animations: "disabled" }),
        contentType: "image/png",
      });
    }
  });
}

for (const theme of ["light", "dark"]) {
  for (const width of [320, 1440]) {
    test(`Workspace state labels remain readable by keyboard and pointer ${theme} ${width}`, async ({
      page,
    }, testInfo) => {
      await page.setViewportSize({ width: 1440, height: 568 });
      const query = new URLSearchParams({
        fixture: "workspace",
        state: "dock-skills-catalog",
        locale: theme === "dark" ? "zh" : "en",
        theme,
        "font-size": "18",
        motion: "off",
      });
      await page.goto(`/lab/fixture.html?${query}`);
      await expectLabReady(page);
      const composer = page.getByRole("textbox", {
        name: theme === "dark" ? "消息输入框" : "Message composer",
        exact: true,
      });
      const draft = "Keep this fixture draft.";
      await composer.fill(draft);
      await page.setViewportSize({ width, height: 568 });
      const label = theme === "dark" ? "计划任务 · 模型选择" : "Schedules · model choices";
      const previous = page.getByRole("button", {
        name: theme === "dark" ? "计划任务" : "Schedules",
        exact: true,
      });
      const row = page.getByRole("button", { name: label, exact: true });
      const tooltip = page.getByRole("tooltip").filter({ hasText: label });
      await previous.focus();
      await page.keyboard.press("Tab");
      await expect(row).toBeFocused();
      const labelGeometry = await row.locator(".truncate-fade").evaluate((node) => {
        const range = document.createRange();
        range.selectNodeContents(node);
        return {
          width: node.clientWidth,
          total: node.scrollWidth,
          box: node.getBoundingClientRect().toJSON(),
          text: range.getBoundingClientRect().toJSON(),
          mask: getComputedStyle(node).maskImage,
        };
      });
      await testInfo.attach("workspace-state-label-measurements", {
        body: JSON.stringify(labelGeometry),
        contentType: "application/json",
      });
      await expect(tooltip).toBeVisible();
      await expect(tooltip).toHaveText(label);
      const geometry = await tooltip.evaluate((node) => {
        const range = document.createRange();
        range.selectNodeContents(node);
        return {
          box: node.getBoundingClientRect().toJSON(),
          lines: Array.from(range.getClientRects(), (box) => box.toJSON()),
          width: innerWidth,
          height: innerHeight,
        };
      });
      expect(geometry.lines.length).toBeGreaterThan(0);
      for (const line of geometry.lines) {
        expect(line.left).toBeGreaterThanOrEqual(Math.max(0, geometry.box.left) - 1);
        expect(line.right).toBeLessThanOrEqual(Math.min(geometry.width, geometry.box.right) + 1);
        expect(line.top).toBeGreaterThanOrEqual(Math.max(0, geometry.box.top) - 1);
        expect(line.bottom).toBeLessThanOrEqual(Math.min(geometry.height, geometry.box.bottom) + 1);
      }
      await testInfo.attach(`workspace-state-label-${theme}-${width}`, {
        body: await page.screenshot({ animations: "disabled" }),
        contentType: "image/png",
      });
      await page.keyboard.press("Escape");
      await expect(tooltip).toBeHidden();
      await expect(row).toBeFocused();
      await previous.focus();
      await row.hover();
      await expect(tooltip).toBeVisible();
      await expect(tooltip).toHaveText(label);
      await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
      await expect(tooltip).toBeHidden();
      await expect(composer).toHaveValue(draft);
      await composer.focus();
      await composer.press(process.platform === "darwin" ? "Meta+ArrowDown" : "Control+End");
      await page.keyboard.insertText(" Continue.");
      await expect(composer).toHaveValue(draft + " Continue.");
    });
  }
}

for (const theme of ["light", "dark"]) {
  for (const width of [320, 1120]) {
    for (const view of ["subagents", "usage", "diagnostics"]) {
      test(`full-width ${view} keeps its content below window controls ${theme} ${width}`, async ({
        page,
      }) => {
        await page.setViewportSize({ width, height: 568 });
        const query = new URLSearchParams({
          fixture: "workspace",
          state: "full-view",
          "full-view": view === "subagents" ? view : `package:visual:${view}`,
          "agent-state": "delegated",
          "font-size": "18",
          theme,
        });
        await page.goto(`/lab/fixture.html?${query}`);
        await expectLabReady(page);
        await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
        const toggle = page.getByRole("button", {
          name: "Expand the workspace fixture sidebar",
        });
        await expect(toggle).toHaveAttribute("aria-expanded", "false");
        const header = page.locator('[data-slot="workspace-window-header"]');
        await expect(header).toBeInViewport({ ratio: 1 });
        for (const action of await header.getByRole("button").all()) await expectCentreHit(action);

        if (view === "subagents") {
          const panel = page.getByRole("region", { name: "Subagents", exact: true });
          await expectBelowWindowControl(
            panel.getByText("6 subagents · 1 running", { exact: true }),
            toggle,
          );
          await panel
            .getByRole("button", { name: "Audit Agent Framework ownership Needs input" })
            .click();
          await expectBelowWindowControl(
            panel.getByRole("button", { name: "All subagents", exact: true }),
            toggle,
          );
          return;
        }

        const panel = page.getByRole("region", {
          name: view === "usage" ? "Usage" : "Client diagnostics",
          exact: true,
        });
        if (view === "usage") {
          for (const details of await panel
            .getByRole("button", { name: "Token and coverage details" })
            .all())
            await details.click();
        } else {
          await page.evaluate(async () => {
            const modulePath = "/src/lib/observability/stores.ts";
            const { useTelemetryStore } = await import(modulePath);
            useTelemetryStore.getState().ingestSpans([
              {
                id: "window-chrome-evidence",
                traceId: "trace_window_chrome",
                name: "Inspect original request evidence",
                kind: "client",
                startMs: 1785484800000,
                durationMillis: 124,
                status: "ok",
                attrs: { path: `/Users/visual/${"retained-evidence/".repeat(30)}request.json` },
              },
            ]);
          });
          await panel.getByRole("button", { name: "Refresh", exact: true }).click();
          await panel
            .getByRole("region", { name: "Local observations", exact: true })
            .getByRole("button", { expanded: false })
            .first()
            .click();
        }
        const viewport = page.locator('[data-slot="workspace-view-body"]').filter({ has: panel });
        expect(await viewport.evaluate((node) => node.scrollHeight > node.clientHeight)).toBe(true);
        await viewport.evaluate((node) => {
          node.scrollTop = node.scrollHeight;
        });
        const controls = panel.locator('[data-slot="panel-controls"]');
        await expectBelowWindowControl(controls, toggle);
        await expect(controls).toBeInViewport({ ratio: 1 });
        await expect(header).toBeInViewport({ ratio: 1 });
        const action =
          view === "usage"
            ? panel.getByRole("searchbox", { name: "Filter sources", exact: true })
            : panel.getByRole("button", { name: "Refresh", exact: true });
        await action.click();
        await expect(action).toBeFocused();
        await expectCentreHit(action);
      });
    }
  }
}

test.describe("coarse pointer window controls", () => {
  test.use({ hasTouch: true, isMobile: true });

  for (const theme of ["light", "dark"]) {
    test(`window actions keep full touch targets beside native controls ${theme}`, async ({
      page,
    }) => {
      await page.setViewportSize({ width: 320, height: 568 });
      await page.goto(
        `/lab/fixture.html?fixture=workspace&state=full-view&full-view=subagents&agent-state=delegated&theme=${theme}&font-size=18`,
      );
      await expectLabReady(page);
      expect(await page.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);
      await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
      const toggle = page.getByRole("button", { name: "Expand the workspace fixture sidebar" });
      const native = (await toggle.boundingBox())!;
      const header = page.locator('[data-slot="workspace-window-header"]');
      const bounds = (await header.boundingBox())!;
      for (const action of await header.getByRole("button").all()) {
        const box = (await action.boundingBox())!;
        expect(box.width).toBeGreaterThanOrEqual(44);
        expect(box.height).toBeGreaterThanOrEqual(44);
        expect(box.x).toBeGreaterThanOrEqual(native.x + native.width);
        expect(box.x + box.width).toBeLessThanOrEqual(320);
        expect(box.y).toBeGreaterThanOrEqual(bounds.y);
        expect(box.y + box.height).toBeLessThanOrEqual(bounds.y + bounds.height);
        await expectCentreHit(action);
      }
    });
  }
});

for (const theme of ["light", "dark"]) {
  for (const width of [320, 1120]) {
    test(`Settings reserves native chrome without a phantom sidebar ${theme} ${width}`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 568 });
      await page.goto(
        `/lab/fixture.html?fixture=workspace&state=settings&theme=${theme}&font-size=18`,
      );
      await expectLabReady(page);
      const header = page.locator('[data-slot="workspace-window-header"]');
      await expect(page.locator(".agent-shell")).toHaveAttribute("data-sidebar", "absent");
      await expect(page.getByRole("button", { name: /workspace fixture sidebar/ })).toHaveCount(0);
      const search = page.getByRole("searchbox", { name: en["settings.searchPlaceholder"]! });
      for (const gutter of [78, 0]) {
        await page.evaluate((value) => {
          document.documentElement.style.setProperty("--window-controls-gutter", `${value}px`);
        }, gutter);
        const padding = await header.evaluate((node) => {
          const style = getComputedStyle(node);
          return {
            start: parseFloat(style.paddingInlineStart),
            end: parseFloat(style.paddingInlineEnd),
          };
        });
        expect(padding.start).toBe(Math.max(gutter, padding.end));
        const window = (await header.boundingBox())!;
        expect((await search.boundingBox())!.y).toBeGreaterThanOrEqual(window.y + window.height);
        const close = header.getByRole("button", { name: "Close", exact: true });
        await expect(close).toBeInViewport({ ratio: 1 });
        await expectCentreHit(close);
      }
      await header.getByRole("button", { name: "Close", exact: true }).click();
      await expect(page.getByTestId("active-main-view")).toHaveText("");
      await expect(
        page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }),
      ).toBeVisible();
      await expect(page.getByRole("textbox", { name: "Message composer" })).toBeVisible();
    });
  }
}

for (const browserName of ["chromium", "webkit"] as const) {
  const capacityTest = test.extend({ browserName });
  for (const profile of [
    { locale: "en", theme: "light", messages: en },
    { locale: "zh", theme: "dark", messages: zh },
  ]) {
    capacityTest(
      browserName +
        " sidebar-only capacity changes preserve the inspected material in " +
        profile.locale,
      async ({ page, browser }, testInfo) => {
        expect(browser.browserType().name()).toBe(browserName);
        const messages: Record<string, string> = profile.messages;
        await page.setViewportSize({ width: 1120, height: 728 });
        await page.goto(
          "/lab/fixture.html?" +
            new URLSearchParams({
              fixture: "workspace",
              state: "dock-schedules",
              "agent-state": "running",
              locale: profile.locale,
              theme: profile.theme,
              "font-size": "18",
              motion: "off",
            }),
        );
        await expectLabReady(page);
        const composer = page.getByRole("textbox", {
          name: messages["composer.input.label"],
          exact: true,
        });
        const draft = "  Keep the inspected material and this draft. 保留原文。  ";
        await composer.fill(draft);
        const originalComposer = await composer.elementHandle();
        const changes = page.getByRole("button", {
          name: profile.locale === "zh" ? /^查看工作区差异/ : /^Review working tree changes/,
        });
        const origin = await changes.elementHandle();
        await changes.click();
        const row = page.locator(".agent-dock-row");
        await expect(row).toHaveAttribute("data-dock", "open");
        const material = row.locator(
          '[data-dock-view-id="package:visual:diff"] [data-slot="workspace-view-body"]',
        );
        await expect(material).toBeVisible();
        const originalMaterial = await material.elementHandle();
        const samples = await row.evaluateHandle((node) => {
          const frames: unknown[] = [];
          let windowResizes = 0;
          const resized = () => {
            windowResizes++;
          };
          window.addEventListener("resize", resized);
          const observer = new ResizeObserver(() => {
            const dock = node.querySelector<HTMLElement>(".agent-context-dock")!;
            const conversation = node.querySelector<HTMLElement>('[aria-label][role="region"]');
            frames.push({
              width: node.clientWidth,
              height: node.clientHeight,
              open: node.getAttribute("data-dock"),
              available: node.hasAttribute("data-dock-available"),
              presentation: node.getAttribute("data-presentation"),
              dock: dock.getBoundingClientRect().toJSON(),
              conversation: conversation?.getBoundingClientRect().toJSON(),
              window: [innerWidth, innerHeight],
            });
          });
          observer.observe(node);
          return {
            frames,
            get windowResizes() {
              return windowResizes;
            },
            close() {
              observer.disconnect();
              window.removeEventListener("resize", resized);
            },
          };
        });
        const rail = page.getByRole("separator", {
          name: "Resize the workspace fixture sidebar",
          exact: true,
        });
        try {
          await rail.focus();
          for (const key of ["End", "Home", "End", "Home"]) {
            await page.keyboard.press(key);
            await expect(rail).toBeFocused();
            await expect(row).toHaveAttribute("data-presentation", key === "End" ? "main" : "dock");
            await expect(row).toHaveAttribute("data-dock", key === "End" ? "collapsed" : "open");
            await expect(material).toBeVisible();
            if (key === "End")
              expect(await origin!.evaluate((node) => node.isConnected)).toBe(false);
            expect(
              await material.evaluate((node, original) => node === original, originalMaterial),
            ).toBe(true);
            expect(
              await originalComposer!.evaluate((node: HTMLTextAreaElement) => ({
                connected: node.isConnected,
                value: node.value,
              })),
            ).toEqual({ connected: true, value: draft });
            if (key === "Home") {
              await expect(composer).toHaveValue(draft);
              expect(
                await composer.evaluate((node, original) => node === original, originalComposer),
              ).toBe(true);
            }
            await testInfo.attach("sidebar-capacity-" + key + "-view", {
              body: await page.screenshot({ animations: "disabled" }),
              contentType: "image/png",
            });
          }
          const evidence = await samples.evaluate((sample) => ({
            frames: sample.frames,
            windowResizes: sample.windowResizes,
          }));
          await testInfo.attach("sidebar-capacity-delivery-frames", {
            body: JSON.stringify(evidence),
            contentType: "application/json",
          });
          expect(evidence.windowResizes).toBe(0);
          expect(evidence.frames.length).toBeGreaterThanOrEqual(4);
          expect(evidence.frames.map((frame) => (frame as { width: number }).width)).toEqual(
            expect.arrayContaining([600, 880]),
          );
          for (const frame of evidence.frames as Array<{
            width: number;
            available: boolean;
            presentation: string;
            open: string;
            window: number[];
          }>) {
            expect(frame.window).toEqual([1120, 728]);
            if (frame.width === 600) {
              expect(frame.available).toBe(false);
              expect(frame.presentation).toBe("main");
            } else if (frame.width === 880) {
              expect(frame.available).toBe(true);
              expect(frame.presentation).toBe("dock");
              expect(frame.open).toBe("open");
            }
          }
        } finally {
          await samples.evaluate((sample) => sample.close());
          await samples.dispose();
        }
        await page
          .getByRole("button", { name: messages["dock.action.returnToConversation"], exact: true })
          .click();
        await expect(
          page.getByRole("region", { name: messages["shell.region.conversation"], exact: true }),
        ).toBeFocused();
        await composer.focus();
        await page.keyboard.press("ControlOrMeta+ArrowDown");
        await page.keyboard.insertText(" Continue.");
        await expect(composer).toHaveValue(draft + " Continue.");
      },
    );
  }
}

const authoredDockViews = [
  trajectoryDeclaration.components.find((entry) => entry.id === "trajectory")!,
  diffDeclaration.components.find((entry) => entry.id === "diff")!,
  diagnosticsDeclaration.components.find((entry) => entry.id === "diagnostics")!,
  usageDeclaration.components.find((entry) => entry.id === "usage")!,
  reportsDeclaration.components.find((entry) => entry.id === "usage-reports")!,
  memoryDeclaration.components.find((entry) => entry.id === "memory")!,
  schedulesDeclaration.components.find((entry) => entry.id === "schedules")!,
  skillsDeclaration.components.find((entry) => entry.id === "skills")!,
];
const authoredSettingsPanes = [
  brandDeclaration.components.find((entry) => entry.id === "brand")!,
  providersDeclaration.components.find((entry) => entry.id === "settings")!,
  mcpDeclaration.components.find((entry) => entry.id === "settings")!,
  hooksDeclaration.components.find((entry) => entry.id === "settings")!,
];
for (const browserName of ["chromium", "webkit"] as const) {
  const authored = test.extend({ browserName });
  for (const profile of [
    { locale: "en", theme: "light", fontSize: 14, messages: en },
    { locale: "zh", theme: "dark", fontSize: 18, messages: zh },
  ]) {
    authored(
      `${browserName} Lab contribution names follow their declarations in ${profile.locale}`,
      async ({ page, browserName: actualBrowserName }, testInfo) => {
        expect(actualBrowserName).toBe(browserName);
        const messages: Record<string, string> = profile.messages;
        const name = (key: string) => ({ name: messages[key]!, exact: true });
        const titleFor = (entry: {
          title: string;
          localizedTitles?: Readonly<Record<string, string>>;
        }) => presentationTitle(entry.title, entry.localizedTitles, profile.locale);
        const capture = async (label: string) =>
          testInfo.attach(label, {
            body: await page.screenshot({ animations: "disabled", caret: "hide" }),
            contentType: "image/png",
          });
        try {
          await page.setViewportSize({ width: 1472, height: 900 });
          await page.goto(
            `/lab/fixture.html?${new URLSearchParams({
              fixture: "workspace",
              state: "dock-trajectory",
              "agent-state": "code-reading",
              locale: profile.locale,
              theme: profile.theme,
              "font-size": String(profile.fontSize),
              motion: "off",
              sidebar: "collapsed",
            })}`,
          );
          await expectLabReady(page);
          await page
            .getByRole("button", { name: "Collapse the workspace fixture sidebar" })
            .click();
          const composer = page.getByRole("textbox", name("composer.input.label"));
          const draft = "  Keep the authored names.\nPreserve this conversation — 保留原文。  ";
          await composer.fill(draft);
          const input = (await composer.elementHandle())!;
          const dock = page.locator(".agent-context-dock");
          await dock.getByRole("button", name("dock.action.browse")).click();
          const query = page.getByRole("combobox", name("dock.picker.placeholder"));
          for (const entry of authoredDockViews) {
            const title = titleFor(entry);
            await query.fill(title);
            const option = page.getByRole("option", { name: title, exact: true });
            const geometry = await option.evaluate((node) => {
              const clips = [];
              for (let parent = node.parentElement; parent; parent = parent.parentElement) {
                const style = getComputedStyle(parent);
                if (style.overflowX !== "visible" || style.overflowY !== "visible")
                  clips.push({
                    slot: parent.getAttribute("data-slot"),
                    overflowX: style.overflowX,
                    overflowY: style.overflowY,
                    bounds: parent.getBoundingClientRect().toJSON(),
                    clientWidth: parent.clientWidth,
                    clientHeight: parent.clientHeight,
                    scrollWidth: parent.scrollWidth,
                    scrollHeight: parent.scrollHeight,
                    padding: [
                      style.paddingTop,
                      style.paddingRight,
                      style.paddingBottom,
                      style.paddingLeft,
                    ],
                    border: [
                      style.borderTopWidth,
                      style.borderRightWidth,
                      style.borderBottomWidth,
                      style.borderLeftWidth,
                    ],
                  });
              }
              const range = document.createRange();
              range.selectNodeContents(node);
              const bounds = node.getBoundingClientRect();
              const inset = Math.min(3, bounds.width / 4, bounds.height / 4);
              const hits = [
                [bounds.left + inset, bounds.top + bounds.height / 2],
                [bounds.right - inset, bounds.top + bounds.height / 2],
                [bounds.left + bounds.width / 2, bounds.top + bounds.height / 2],
                [bounds.left + bounds.width / 2, bounds.top + inset],
                [bounds.left + bounds.width / 2, bounds.bottom - inset],
              ].map(([x, y]) => {
                const hit = document.elementFromPoint(x!, y!);
                return {
                  inside: node.contains(hit),
                  x,
                  y,
                  target: { tag: hit?.tagName, role: hit?.getAttribute("role") },
                };
              });
              const style = getComputedStyle(node);
              return {
                bounds: bounds.toJSON(),
                shape: {
                  radius: style.borderRadius,
                  corners: style.getPropertyValue("corner-shape"),
                  clip: style.clipPath,
                  transform: style.transform,
                  scale: style.scale,
                  pointerEvents: style.pointerEvents,
                },
                clips,
                hits,
                text: Array.from(range.getClientRects(), (rect) => rect.toJSON()),
                viewport: [innerWidth, innerHeight],
              };
            });
            await testInfo.attach(`authored-catalog-${entry.id}-geometry`, {
              body: JSON.stringify(geometry),
              contentType: "application/json",
            });
            try {
              const frame = {
                left: 0,
                top: 0,
                right: geometry.viewport[0]!,
                bottom: geometry.viewport[1]!,
              };
              for (const clip of geometry.clips) {
                if (clip.overflowX !== "visible") {
                  frame.left = Math.max(frame.left, clip.bounds.left + parseFloat(clip.border[3]!));
                  frame.right = Math.min(
                    frame.right,
                    clip.bounds.right - parseFloat(clip.border[1]!),
                  );
                }
                if (clip.overflowY !== "visible") {
                  frame.top = Math.max(frame.top, clip.bounds.top + parseFloat(clip.border[0]!));
                  frame.bottom = Math.min(
                    frame.bottom,
                    clip.bounds.bottom - parseFloat(clip.border[2]!),
                  );
                }
              }
              for (const rect of [geometry.bounds, ...geometry.text]) {
                expect(rect.left).toBeGreaterThanOrEqual(frame.left);
                expect(rect.right).toBeLessThanOrEqual(frame.right);
                expect(rect.top).toBeGreaterThanOrEqual(frame.top);
                expect(rect.bottom).toBeLessThanOrEqual(frame.bottom);
              }
              expect(geometry.text.length).toBeGreaterThan(0);
              expect(geometry.hits.map((hit) => hit.inside)).toEqual([
                true,
                true,
                true,
                true,
                true,
              ]);
            } finally {
              await capture(`authored-catalog-${entry.id}`);
            }
          }
          const trajectory = titleFor(authoredDockViews[0]!);
          await query.fill(trajectory);
          await page.keyboard.press("Enter");
          await expect(dock.getByRole("tab", { name: trajectory, exact: true })).toHaveAttribute(
            "aria-selected",
            "true",
          );
          const filter = dock
            .locator('[data-slot="workspace-view-body"]:visible')
            .getByRole("searchbox");
          await filter.fill("item_code_reading");
          const response = dock.getByRole("button", {
            name: profile.locale === "zh" ? /^智能体回复/ : /^Agent response/,
          });
          await response.focus();
          await page.keyboard.press("Enter");
          await expect(response).toHaveAttribute("aria-expanded", "true");
          await dock.getByRole("button", name("dock.action.expand")).click();
          const header = page.locator('[data-slot="workspace-window-header"]');
          await expect(header.locator(":scope > span[title]")).toHaveText(trajectory);
          await page.setViewportSize({ width: 373, height: 364 });
          await expect(header.locator(":scope > span[title]")).toBeInViewport({ ratio: 1 });
          await capture("authored-material-header");
          await header.getByRole("button", name("common.close")).focus();
          await page.keyboard.press("Enter");
          await page.setViewportSize({ width: 1472, height: 900 });
          await page.keyboard.press("ControlOrMeta+l");
          await expect(composer).toBeFocused();
          await expect(composer).toHaveValue(draft);
          await page.keyboard.press("ControlOrMeta+,");
          const settings = page.getByRole("tablist", name("settings.title"));
          for (const entry of authoredSettingsPanes) {
            await expect(
              settings.getByRole("tab", { name: titleFor(entry), exact: true }),
            ).toBeInViewport({ ratio: 1 });
          }
          await capture("authored-settings-navigation");
          await page.keyboard.press("ControlOrMeta+l");
          await expect(composer).toBeFocused();
          await expect(composer).toHaveValue(draft);
          expect(await composer.evaluate((node, held) => node === held, input)).toBe(true);
          expect(
            await composer.evaluate((node) => [
              (node as HTMLTextAreaElement).selectionStart,
              (node as HTMLTextAreaElement).selectionEnd,
            ]),
          ).toEqual([draft.length, draft.length]);
          await expect(filter).toHaveValue("item_code_reading");
          await expect(response).toHaveAttribute("aria-expanded", "true");
          const continuation = profile.locale === "zh" ? " 继续核对。" : " Continue reviewing.";
          await page.keyboard.type(continuation);
          await expect(composer).toHaveValue(draft + continuation);
          await capture("authored-returned-draft");

          const gallery = galleryDeclaration.components.find(
            (entry) => entry.id === "icon-gallery",
          )!;
          await page.goto(
            `/lab/fixture.html?${new URLSearchParams({
              fixture: "workspace",
              state: "full-view",
              "full-view": "package:visual:icon-gallery",
              locale: profile.locale,
              theme: profile.theme,
              "font-size": String(profile.fontSize),
              motion: "off",
              sidebar: "collapsed",
            })}`,
          );
          await expectLabReady(page);
          await page
            .getByRole("button", { name: "Collapse the workspace fixture sidebar" })
            .click();
          await expect(
            page.locator('[data-slot="workspace-window-header"] > span[title]'),
          ).toHaveText(titleFor(gallery));
          await capture("authored-gallery-header");
        } finally {
          await capture("authored-final-state");
        }
      },
    );
  }
}
