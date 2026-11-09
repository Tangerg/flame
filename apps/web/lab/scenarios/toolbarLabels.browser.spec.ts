import { expect, test } from "./test";
import { en } from "@/lib/i18n/locales/en";
import zh from "../../../../plugins/locale-zh/package/io.github.tangerg.flame/messages.json" with { type: "json" };
import { catalog } from "../catalog";
import { previewURL, readSelection } from "../selection";
import { expectLabReady } from "../readiness";
import type { Locator } from "@playwright/test";

async function measureControl(control: Locator, frames = 1) {
  return control.evaluate(async (node, frames) => {
    const sample = () => {
      const rect = node.getBoundingClientRect();
      const clippedBy = [];
      const ancestors = [];
      for (let parent = node.parentElement; parent; parent = parent.parentElement) {
        const style = getComputedStyle(parent);
        const box = parent.getBoundingClientRect();
        ancestors.push({
          slot: parent.getAttribute("data-slot") ?? parent.tagName,
          top: box.top,
          bottom: box.bottom,
          height: box.height,
          clientHeight: parent.clientHeight,
          scrollHeight: parent.scrollHeight,
          display: style.display,
          maxHeight: style.maxHeight,
          overflow: style.overflowY,
        });
        if (
          /(auto|scroll|hidden|clip)/.test(style.overflowY) &&
          (rect.top < box.top + parent.clientTop - 1 ||
            rect.bottom > box.top + parent.clientTop + parent.clientHeight + 1)
        )
          clippedBy.push(parent.getAttribute("data-slot") ?? parent.tagName);
      }
      const x = (rect.left + rect.right) / 2;
      const y = (rect.top + rect.bottom) / 2;
      const hits = [
        [x, y],
        [rect.left + 2, y],
        [rect.right - 2, y],
        [x, rect.top + 2],
        [x, rect.bottom - 2],
      ].map(([px, py]) => {
        const hit = document.elementFromPoint(px!, py!);
        return {
          x: px,
          y: py,
          tag: hit?.tagName,
          label: hit?.getAttribute("aria-label"),
          slot: hit?.getAttribute("data-slot"),
          contains: hit !== null && node.contains(hit),
        };
      });
      return {
        bounds: { top: rect.top, bottom: rect.bottom, width: rect.width, height: rect.height },
        clippedBy,
        ancestors,
        inViewport:
          rect.top >= 0 && rect.bottom <= innerHeight && rect.left >= 0 && rect.right <= innerWidth,
        hits,
        reachable: hits.every((hit) => hit.contains),
        focused: document.activeElement === node,
      };
    };
    const samples: ReturnType<typeof sample>[] = [];
    for (let frame = 0; frame < frames; frame++) {
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
      samples.push(sample());
    }
    return {
      ...samples.at(-1)!,
      stable: samples.every(
        (reading) =>
          reading.focused &&
          reading.clippedBy.length === 0 &&
          reading.inViewport &&
          reading.reachable &&
          reading.bounds.width >= 24 &&
          reading.bounds.height >= 24,
      ),
    };
  }, frames);
}

for (const environment of [
  { locale: "en", theme: "light", fontSize: 14, messages: en },
  { locale: "zh-CN", theme: "dark", fontSize: 18, messages: zh },
]) {
  test(`composer names retain current choices through keyboard selection and narrow layout in ${environment.locale}`, async ({
    page,
  }, testInfo) => {
    const messages: Record<string, string> = environment.messages;
    const modelAction = messages["composer.switchModel"]!;
    const effortAction = messages["composer.switchReasoningEffort"]!;
    const approvalAction = messages["approvals.mode.aria"]!;
    await page.setViewportSize({ width: 1440, height: 900 });
    const query = new URLSearchParams({
      panel: "business",
      story: "business-agent-scenarios",
      variant: "idle",
      locale: environment.locale,
      theme: environment.theme,
      "font-size": String(environment.fontSize),
    });
    const selection = readSelection(query.toString(), catalog);
    await page.goto(previewURL(catalog.story(selection.storyId), selection, "composer-choices"));
    await expectLabReady(page, { timeout: 5_000 });
    const draft = page.getByRole("textbox");
    const draftValue = "Keep this draft while choosing model options.";
    await draft.fill(draftValue);
    const draftNode = await draft.elementHandle();
    const measureDraft = (composerWidth: string | null = null) =>
      draft.evaluate((node: HTMLTextAreaElement, composerWidth) => {
        const composer = node.closest<HTMLElement>('[data-slot="composer-root"]')!;
        const footer = composer.querySelector(':scope > [data-slot="composer-footer"]')!;
        if (composerWidth !== null) composer.style.width = composerWidth;
        const rect = (element: Element) => {
          const { top, right, bottom, left, width, height } = element.getBoundingClientRect();
          return { top, right, bottom, left, width, height };
        };
        return {
          value: node.value,
          client: { width: node.clientWidth, height: node.clientHeight },
          scroll: {
            width: node.scrollWidth,
            height: node.scrollHeight,
            top: node.scrollTop,
            left: node.scrollLeft,
          },
          height: { inline: node.style.height, computed: getComputedStyle(node).height },
          input: rect(node),
          composer: rect(composer),
          footer: rect(footer),
        };
      }, composerWidth);
    const expectCompleteDraft = async (evidenceName: string) => {
      try {
        await expect(draft).toHaveValue(draftValue);
        expect(await draft.evaluate((node, original) => node === original, draftNode)).toBe(true);
        await expect
          .poll(async () => {
            const { client, scroll, input, composer, footer } = await measureDraft();
            return {
              fits: scroll.height - client.height <= 1 && scroll.width - client.width <= 1,
              atStart: scroll.top === 0 && scroll.left === 0,
              contained:
                input.top >= composer.top - 1 &&
                input.bottom <= composer.bottom + 1 &&
                input.left >= composer.left - 1 &&
                input.right <= composer.right + 1,
              clearOfFooter: input.bottom <= footer.top + 1,
            };
          })
          .toEqual({ fits: true, atStart: true, contained: true, clearOfFooter: true });
      } finally {
        await testInfo.attach(evidenceName, {
          body: JSON.stringify(await measureDraft(), null, 2),
          contentType: "application/json",
        });
      }
    };
    const model = page.getByRole("button", { name: new RegExp(`${modelAction}$`) });
    const effort = page.getByRole("button", { name: new RegExp(`${effortAction}$`) });
    const approval = page.getByRole("button", { name: new RegExp(`${approvalAction}$`) });
    await expect(model).toHaveAccessibleName(`GPT-5.6 Sol, ${modelAction}`);
    await expect(effort).toHaveAccessibleName(
      `${messages["model.effort.medium"]}, ${effortAction}`,
    );
    await expect(approval).toHaveAccessibleName(
      `${messages["approvals.mode.balanced"]}, ${approvalAction}`,
    );
    await testInfo.attach("composer-current-choices", {
      body: await page.screenshot({ animations: "disabled" }),
      contentType: "image/png",
    });

    await model.focus();
    await page.keyboard.press("Enter");
    const search = page.getByRole("combobox");
    await expect(search).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(model).toBeFocused();

    await effort.focus();
    await page.keyboard.press("Enter");
    await expect(page.getByRole("menuitem")).toHaveCount(6);
    await expect(page.getByRole("menuitem").first()).toBeFocused();
    await page.keyboard.press("End");
    await page.keyboard.press("ArrowUp");
    await page.keyboard.press("ArrowUp");
    await expect(
      page.getByRole("menuitem", { name: messages["model.effort.high"], exact: true }),
    ).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(effort).toHaveAccessibleName(`${messages["model.effort.high"]}, ${effortAction}`);
    await expect(effort).toBeFocused();

    await approval.focus();
    await page.keyboard.press("Enter");
    await expect(page.getByRole("menuitem").first()).toBeFocused();
    await page.keyboard.press("Home");
    await expect(page.getByRole("menuitem").first()).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(approval).toHaveAccessibleName(
      `${messages["approvals.mode.safe"]}, ${approvalAction}`,
    );
    await expect(approval).toBeFocused();

    // Measured in the same task as the width change, before ResizeObserver can deliver.
    await testInfo.attach("composer-narrow-draft-before-autosize", {
      body: JSON.stringify(await measureDraft("320px"), null, 2),
      contentType: "application/json",
    });
    await expect(effort.locator('[data-slot="composer-chip-label"]')).toBeHidden();
    await expect(approval.locator('[data-slot="composer-chip-label"]')).toBeHidden();
    await expect(effort).toHaveAccessibleName(`${messages["model.effort.high"]}, ${effortAction}`);
    await expect(approval).toHaveAccessibleName(
      `${messages["approvals.mode.safe"]}, ${approvalAction}`,
    );
    await expectCompleteDraft("composer-narrow-draft");
    await testInfo.attach("composer-narrow-choices", {
      body: await page.screenshot({ animations: "disabled" }),
      contentType: "image/png",
    });

    await model.focus();
    await page.keyboard.press("Enter");
    await search.fill("Qwen MT Plus");
    await expect(page.getByRole("option", { name: /^Qwen MT Plus/ })).toBeVisible();
    await page.keyboard.press("ArrowDown");
    await page.keyboard.press("Enter");
    await expect(model).toHaveAccessibleName(`Qwen MT Plus, ${modelAction}`);
    await expect(effort).toHaveCount(0);
    await expect(model).toBeFocused();
    await expectCompleteDraft("composer-final-draft");
  });
}

test("the composer preserves the model and lets secondary labels yield without measurement", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/lab/fixture.html?fixture=agent&state=idle&theme=light");
  await expectLabReady(page);
  const composer = page.locator('[data-slot="composer-root"]');
  const model = page.getByRole("button", { name: /Switch model$/ });
  const approval = page.getByRole("button", { name: /Approval mode$/ });
  await expect(model.locator('[data-slot="composer-chip-label"]')).toBeVisible();
  await expect(approval.locator('[data-slot="composer-chip-label"]')).toBeVisible();
  await composer.evaluate((node) => {
    node.style.width = "320px";
  });
  await expect(model.locator('[data-slot="composer-chip-label"]')).toBeVisible();
  await expect(approval.locator('[data-slot="composer-chip-label"]')).toBeHidden();
  await model.locator('[data-slot="composer-chip-label"]').evaluate((node) => {
    node.textContent = "A much longer model label changed without resizing the window";
  });
  expect(await composer.evaluate((node) => node.scrollWidth - node.clientWidth)).toBeLessThan(2);
  const effort = page.getByRole("button", { name: /Switch reasoning effort$/ });
  await expect(effort).toBeVisible();
  await expect(effort.locator('[data-slot="composer-chip-label"]')).toBeHidden();
});

test("high-risk approval remains explicit in a narrow composer", async ({ page }) => {
  await page.goto("/lab/fixture.html?fixture=agent&state=idle&theme=light");
  await expectLabReady(page);
  const approval = page.getByRole("button", { name: /Approval mode$/ });
  await approval.click();
  await page.getByRole("menuitem", { name: /^Auto\b/ }).click();
  const composer = page.locator('[data-slot="composer-root"]');
  await composer.evaluate((node) => {
    node.style.width = "280px";
  });
  await expect(approval.locator('[data-slot="composer-chip-label"]')).toBeVisible();
  await expect(approval).toHaveText("Auto");
  expect(await composer.evaluate((node) => node.scrollWidth - node.clientWidth)).toBeLessThan(2);
});

for (const browserName of ["chromium", "webkit"] as const) {
  const capacityTest = test.extend({ browserName });
  capacityTest.describe(`${browserName} composer action capacity`, () => {
    for (const environment of [
      { locale: "en", theme: "light", fontSize: 14, messages: en },
      { locale: "zh", theme: "dark", fontSize: 18, messages: zh },
    ]) {
      for (const state of ["idle", "running"]) {
        capacityTest(
          `composer action rows remain complete in ${environment.locale} ${state}`,
          async ({ page }, testInfo) => {
            const messages: Record<string, string> = environment.messages;
            await page.setViewportSize({ width: 1120, height: 728 });
            const query = new URLSearchParams({
              fixture: "agent",
              state,
              locale: environment.locale,
              theme: environment.theme,
              "font-size": String(environment.fontSize),
            });
            await page.goto(`/lab/fixture.html?${query}`);
            await expectLabReady(page);
            const draft = page.getByRole("textbox", { name: messages["composer.input.label"] });
            const draftValue = "  Keep this draft while the controls reflow.  ";
            await draft.fill(draftValue);
            const draftNode = await draft.elementHandle();
            const composer = page.locator('[data-slot="composer-root"]');
            const footer = composer.locator('[data-slot="composer-footer"]');
            const approval = footer.getByRole("button", {
              name: new RegExp(`${messages["approvals.mode.aria"]}$`),
            });
            await approval.click();
            await page
              .getByRole("menuitem", {
                name: new RegExp(`^${messages["approvals.mode.auto"]}`),
              })
              .click();
            await expect(approval.locator('[data-slot="composer-chip-label"]')).toBeVisible();
            await expect(
              footer.getByRole("button", {
                name: messages[
                  state === "running" ? "composer.action.steer" : "composer.action.send"
                ],
                exact: true,
              }),
            ).toBeVisible();
            if (state === "running") {
              await expect(
                footer.getByRole("button", { name: messages["composer.action.stop"], exact: true }),
              ).toBeVisible();
            }

            const measure = () =>
              footer.evaluate((node, running) => {
                const frame = node.closest<HTMLElement>('[data-slot="composer-root"]')!;
                const rect = (element: Element) => {
                  const { top, right, bottom, left, width, height } =
                    element.getBoundingClientRect();
                  return { top, right, bottom, left, width, height };
                };
                const contains = (outer: DOMRect, inner: DOMRect) =>
                  inner.left >= outer.left - 1 &&
                  inner.right <= outer.right + 1 &&
                  inner.top >= outer.top - 1 &&
                  inner.bottom <= outer.bottom + 1;
                const frameBox = frame.getBoundingClientRect();
                const input = frame.querySelector<HTMLTextAreaElement>("textarea")!;
                const inputBox = input.getBoundingClientRect();
                const viewport = new DOMRect(0, 0, innerWidth, innerHeight);
                const buttons = [...node.querySelectorAll("button")];
                const actions = buttons
                  .slice(running ? -2 : -1)
                  .map((button) => button.getBoundingClientRect());
                return {
                  frame: rect(frame),
                  footer: rect(node),
                  overflow: frame.scrollWidth - frame.clientWidth,
                  draftFits:
                    contains(frameBox, inputBox) &&
                    inputBox.bottom <= node.getBoundingClientRect().top + 1 &&
                    input.scrollWidth - input.clientWidth <= 1 &&
                    input.scrollHeight - input.clientHeight <= 1,
                  actionsAtEnd:
                    Math.abs(
                      actions.at(-1)!.right -
                        (frameBox.right - parseFloat(getComputedStyle(node).paddingRight)),
                    ) <= 1,
                  actionsTogether: actions.every(
                    (action) => Math.abs(action.top - actions[0]!.top) <= 1,
                  ),
                  actions: buttons.map((button) => {
                    const box = button.getBoundingClientRect();
                    const x = (box.left + box.right) / 2;
                    const y = (box.top + box.bottom) / 2;
                    return {
                      name: button.getAttribute("aria-label"),
                      rect: rect(button),
                      contained: contains(frameBox, box) && contains(viewport, box),
                      glyphs: [...button.querySelectorAll("svg")].every((glyph) =>
                        contains(box, glyph.getBoundingClientRect()),
                      ),
                      reachable: [
                        [x, y],
                        [box.left + 2, y],
                        [box.right - 2, y],
                        [x, box.top + 2],
                        [x, box.bottom - 2],
                      ].every(([px, py]) => {
                        const hit = document.elementFromPoint(px!, py!);
                        return hit !== null && button.contains(hit);
                      }),
                    };
                  }),
                };
              }, state === "running");

            for (const width of [320, 280, 1120]) {
              await page.setViewportSize({ width, height: 728 });
              try {
                await expect(draft).toHaveValue(draftValue);
                expect(await draft.evaluate((node, original) => node === original, draftNode)).toBe(
                  true,
                );
                await expect
                  .poll(async () => {
                    const reading = await measure();
                    return {
                      fits: reading.overflow <= 1,
                      draftFits: reading.draftFits,
                      actionsAtEnd: reading.actionsAtEnd,
                      actionsTogether: reading.actionsTogether,
                      complete: reading.actions.every(
                        (action) => action.contained && action.glyphs && action.reachable,
                      ),
                    };
                  })
                  .toEqual({
                    fits: true,
                    draftFits: true,
                    actionsAtEnd: true,
                    actionsTogether: true,
                    complete: true,
                  });
                await approval.focus();
                await page.keyboard.press("Enter");
                await expect(page.getByRole("menuitem").first()).toBeFocused();
                await page.keyboard.press("Escape");
                await expect(approval).toBeFocused();
                await expect(draft).toHaveValue(draftValue);
              } finally {
                await testInfo.attach(`composer-action-capacity-${width}`, {
                  body: JSON.stringify(await measure(), null, 2),
                  contentType: "application/json",
                });
                await testInfo.attach(`composer-action-rows-${width}`, {
                  body: await page.screenshot({ animations: "disabled" }),
                  contentType: "image/png",
                });
              }
            }
          },
        );
      }
    }
  });

  for (const environment of [
    { locale: "en", theme: "light", fontSize: 14, messages: en },
    { locale: "zh", theme: "dark", fontSize: 18, messages: zh },
  ]) {
    capacityTest(
      `${browserName} short composer keeps editing and Goal controls reachable in ${environment.locale}`,
      async ({ page }, testInfo) => {
        const messages: Record<string, string> = environment.messages;
        await page.setViewportSize({ width: 1120, height: 728 });
        await page.goto(
          `/lab/fixture.html?${new URLSearchParams({
            fixture: "agent",
            state: "running",
            locale: environment.locale,
            theme: environment.theme,
            "font-size": String(environment.fontSize),
          })}`,
        );
        await expectLabReady(page);
        await page
          .getByRole("button", { name: "Collapse the agent fixture sidebar", exact: true })
          .click();
        const draft = page.getByRole("textbox", { name: messages["composer.input.label"] });
        const footer = page.locator('[data-slot="composer-footer"]');
        const approval = footer.getByRole("button", {
          name: new RegExp(`${messages["approvals.mode.aria"]}$`),
        });
        await approval.click();
        await page
          .getByRole("menuitem", { name: new RegExp(`^${messages["approvals.mode.auto"]}`) })
          .click();
        const value = "  Keep this draft while the controls reflow.  ";
        await draft.fill(value);
        await draft.evaluate((node: HTMLTextAreaElement) => node.setSelectionRange(2, 12));
        const original = await draft.elementHandle();
        const expectReachable = async (control: Locator, name: string) => {
          try {
            await expect(control).toBeFocused();
            await expect
              .poll(async () => {
                const reading = await measureControl(control, 8);
                return {
                  unclipped: reading.clippedBy.length === 0,
                  inViewport: reading.inViewport,
                  reachable: reading.reachable,
                  size: reading.bounds.width >= 24 && reading.bounds.height >= 24,
                  stable: reading.stable,
                };
              })
              .toEqual({
                unclipped: true,
                inViewport: true,
                reachable: true,
                size: true,
                stable: true,
              });
          } finally {
            await testInfo.attach(name, {
              body: JSON.stringify(await measureControl(control), null, 2),
              contentType: "application/json",
            });
            await testInfo.attach(`${name}-view`, {
              body: await page.screenshot({ animations: "disabled" }),
              contentType: "image/png",
            });
          }
          await expect(control).toBeFocused();
          const captured = await measureControl(control);
          await testInfo.attach(`${name}-after-capture`, {
            body: JSON.stringify(captured, null, 2),
            contentType: "application/json",
          });
          expect(captured.clippedBy).toEqual([]);
          expect(captured.inViewport && captured.reachable).toBe(true);
        };
        await page.setViewportSize({ width: 373, height: 243 });
        await expectReachable(draft, "short-focused-draft");
        await expect(draft).toHaveValue(value);
        expect(
          await draft.evaluate((node: HTMLTextAreaElement) => [
            node.selectionStart,
            node.selectionEnd,
          ]),
        ).toEqual([2, 12]);
        for (const [index, button] of (await footer.getByRole("button").all()).entries()) {
          await page.keyboard.press("Tab");
          await expectReachable(button, `short-footer-${index}`);
        }
        const stop = footer.getByRole("button", {
          name: messages["composer.action.stop"],
          exact: true,
        });
        await expect(stop).toBeFocused();
        await page.setViewportSize({ width: 1120, height: 728 });
        await expectReachable(stop, "restored-stop");
        await page.setViewportSize({ width: 373, height: 243 });
        await expectReachable(stop, "short-focused-stop");
        const model = footer.getByRole("button", {
          name: new RegExp(`${messages["composer.switchModel"]}$`),
        });
        await model.focus();
        await expectReachable(model, "short-model");
        await page.keyboard.press("Enter");
        await expect(page.getByRole("combobox")).toBeFocused();
        await page.keyboard.press("Escape");
        await expectReachable(model, "short-model-return");
        await draft.focus();
        await page.keyboard.press("Shift+Tab");
        const edit = page.getByRole("button", { name: messages["goal.action.edit"], exact: true });
        await expectReachable(edit, "short-edit-goal");
        await page.keyboard.press("Enter");
        await expect(page.getByRole("dialog", { name: messages["goal.edit.title"] })).toBeVisible();
        await page.keyboard.press("Escape");
        await expectReachable(edit, "short-goal-return");
        for (const action of ["goal.action.pause", "goal.action.clear"]) {
          await page.keyboard.press("Shift+Tab");
          await expectReachable(
            page.getByRole("button", { name: messages[action], exact: true }),
            `short-${action}`,
          );
        }
        const scrollport = page.locator('[data-slot="composer-scrollport"]');
        const port = (await scrollport.boundingBox())!;
        await page.mouse.move(port.x + port.width / 2, port.y + port.height / 2);
        await page.mouse.wheel(0, 1000);
        await expect.poll(() => scrollport.evaluate((node) => node.scrollTop)).toBeGreaterThan(0);
        await scrollport.evaluate(async (node) => {
          let previous = node.scrollTop;
          let stable = 0;
          for (let frame = 0; frame < 120; frame++) {
            await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
            const current = node.scrollTop;
            stable = current === previous ? stable + 1 : 0;
            previous = current;
            if (stable === 8) return;
          }
          throw new Error("Composer wheel reading did not settle");
        });
        await page.setViewportSize({ width: 390, height: 243 });
        await expect.poll(() => scrollport.evaluate((node) => node.scrollTop)).toBeGreaterThan(0);
        await page.keyboard.press("Shift+Tab");
        const summary = page.locator('[data-slot="composer-top-tray-surface"] button').first();
        await expectReachable(summary, "short-goal-after-wheel");
        await page.setViewportSize({ width: 1120, height: 728 });
        await draft.focus();
        await expectReachable(draft, "restored-draft");
        await expect(draft).toHaveValue(value);
        expect(await draft.evaluate((node, previous) => node === previous, original)).toBe(true);
        await page.keyboard.press("ControlOrMeta+ArrowDown");
        await page.keyboard.insertText(" Continue editing.");
        await expect(draft).toHaveValue(`${value} Continue editing.`);
        expect(await page.evaluate(() => ({ x: window.scrollX, y: window.scrollY }))).toEqual({
          x: 0,
          y: 0,
        });
      },
    );
  }
}

test("navigation density does not rescale reading or input geometry", async ({ page }) => {
  const measures = [];
  for (const density of ["compact", "spacious"]) {
    await page.goto(`/lab/fixture.html?fixture=agent&state=idle&theme=light&density=${density}`);
    await expectLabReady(page);
    measures.push(
      await page.evaluate(() => {
        const root = getComputedStyle(document.documentElement);
        const input = document.querySelector('[data-slot="composer-root"]')!;
        return {
          row: root.getPropertyValue("--density-row-height"),
          reading: root.getPropertyValue("--reading-gutter-wide"),
          input: input.getBoundingClientRect().height,
        };
      }),
    );
  }
  expect(measures[0]!.row).not.toBe(measures[1]!.row);
  expect(measures[0]!.reading).toBe(measures[1]!.reading);
  expect(measures[0]!.input).toBe(measures[1]!.input);
});
