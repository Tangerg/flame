import { expect, test, type Locator } from "./test";
import { en } from "@/lib/i18n/locales/en";
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
        for (let visit = 0; visit < 2; visit++) {
          if (visit === 1) {
            const previousTop = await scrollport.evaluate((node) => node.scrollTop);
            const bounds = (await scrollport.boundingBox())!;
            await page.mouse.move(bounds.x + bounds.width - 1, bounds.y + bounds.height / 2);
            await page.mouse.wheel(0, -12);
            await expect
              .poll(async () => {
                const positions = (await composerReturnFrames(scrollport)).map(
                  (frame) => frame.top,
                );
                return (
                  Math.max(...positions) - Math.min(...positions) < 1 &&
                  positions.at(-1)! > 0 &&
                  positions.at(-1)! < previousTop
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
  await expect(observe).not.toBeChecked();
  await expect(output).toHaveText("");
  await expect(readingOutput).toHaveText("");
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
