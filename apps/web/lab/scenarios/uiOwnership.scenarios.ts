import { expect, test, type Locator, type Page } from "./test";
import { LONG_MODEL_NAME } from "./runtimeSnapshots";

async function readAcross(page: Page, reader: Locator, first: Locator, last: Locator) {
  await reader.scrollIntoViewIfNeeded();
  await reader.focus();
  await expect(reader).toBeFocused();
  const clip = (await reader.elementHandle())!;
  const hidden = (target: Locator) =>
    target.evaluate((node, region) => {
      const left = region.getBoundingClientRect().left + region.clientLeft;
      const bounds = node.getBoundingClientRect();
      return { left: left - bounds.left, right: bounds.right - (left + region.clientWidth) };
    }, clip);
  const transcript = page.locator(".msg-scroll-viewport");
  const outerTop = await transcript.evaluate((node) => node.scrollTop);
  expect((await hidden(last)).right).toBeGreaterThan(1);
  for (const [key, target, side] of [
    ["ArrowRight", last, "right"],
    ["ArrowLeft", first, "left"],
  ] as const) {
    let keyDowns = 0;
    const start = await reader.evaluate((node) => node.scrollLeft);
    try {
      try {
        await expect(async () => {
          keyDowns++;
          // Releasing between repeats interrupts native scrolling animations.
          await page.keyboard.down(key);
          expect((await hidden(target))[side]).toBeLessThanOrEqual(0.5);
        }).toPass({ intervals: [50], timeout: 5_000 });
      } finally {
        await page.keyboard.up(key);
      }
      await expect
        .poll(() =>
          reader.evaluate(
            (node, end) =>
              Math.abs(node.scrollLeft - (end ? node.scrollWidth - node.clientWidth : 0)),
            key === "ArrowRight",
          ),
        )
        .toBeLessThanOrEqual(0.5);
      expect((await hidden(target))[side]).toBeLessThanOrEqual(0.5);
    } finally {
      await test.info().attach(`${key}-reading.json`, {
        body: JSON.stringify({
          key,
          keyDowns,
          start,
          first: await hidden(first),
          last: await hidden(last),
          reader: await reader.evaluate((node) => ({
            focused: node === document.activeElement,
            scrollLeft: node.scrollLeft,
            clientWidth: node.clientWidth,
            scrollWidth: node.scrollWidth,
          })),
        }),
        contentType: "application/json",
      });
      await test.info().attach(`${key}-reading.png`, {
        body: await page.screenshot({ animations: "disabled", caret: "hide" }),
        contentType: "image/png",
      });
    }
    await expect(reader).toBeFocused();
  }
  expect(await reader.evaluate((node) => Math.abs(node.scrollLeft))).toBeLessThanOrEqual(0.5);
  expect(await transcript.evaluate((node) => node.scrollTop)).toBe(outerTop);
}

export function registerUiOwnershipContracts() {
  test("wide formulas reach their final term and return with horizontal arrows", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 320, height: 900 });
    await page.goto("/lab/fixture.html?fixture=agent&state=math-content&theme=dark&font-size=18");
    await page.locator("html[data-visual-ready]").waitFor();
    const formulas = page.getByRole("region", { name: "Mathematical expression", exact: true });
    await expect(formulas).toHaveCount(2);
    const objective = formulas.first();
    await readAcross(
      page,
      objective,
      objective.locator(".katex-html .katex-base").first(),
      objective.locator(".katex-html .katex-tag"),
    );
    const matrix = formulas.nth(1);
    await readAcross(
      page,
      matrix,
      matrix.locator(".katex-html .katex-base").first(),
      matrix.locator(".katex-html .katex-base").last(),
    );
  });

  test("an inline table reaches its last column and returns with horizontal arrows", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 320, height: 1200 });
    await page.goto("/lab/fixture.html?fixture=agent&state=narrative&theme=dark&font-size=18");
    await page.locator("html[data-visual-ready]").waitFor();
    const reader = page
      .locator("[data-markdown-table]")
      .filter({ hasText: "Strategy" })
      .getByRole("region", { name: "Table content", exact: true });
    const columns = reader.getByRole("row").first().getByRole("columnheader");
    await readAcross(page, reader, columns.first(), columns.last());
  });

  test("an expanded table reaches its last column and returns with horizontal arrows", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 320, height: 900 });
    await page.goto(
      "/lab/fixture.html?fixture=agent&state=table-reading&theme=dark&font-size=18&sidebar=collapsed",
    );
    await page.locator("html[data-visual-ready]").waitFor();
    const expand = page.getByRole("button", { name: "Expand table", exact: true });
    await expand.focus();
    await page.keyboard.press("Enter");
    const dialog = page.getByRole("dialog", { name: "Table preview", exact: true });
    const reader = dialog.getByRole("region", { name: "Table content", exact: true });
    await expect(reader).toBeFocused();
    const columns = reader.getByRole("row").first().getByRole("columnheader");
    await readAcross(page, reader, columns.first(), columns.last());
    await page.keyboard.press("Escape");
    await expect(expand).toBeFocused();
  });

  test("autosize follows content, width, font and reset after a hidden mount", async ({ page }) => {
    await page.goto("/lab/fixture.html?fixture=atoms&state=input-contracts&theme=light");
    await page.getByRole("button", { name: "Show notes" }).click();
    const note = page.getByRole("textbox", { name: "Controlled note", exact: true });
    const height = () => note.evaluate((element) => element.getBoundingClientRect().height);
    const initial = await height();
    expect(initial).toBeGreaterThan(10);
    await note.fill("one\ntwo\nthree\nfour");
    await expect.poll(height).toBeGreaterThan(initial * 2);
    await note.fill(Array.from({ length: 30 }, (_, index) => `Line ${index}`).join("\n"));
    await expect.poll(height).toBe(160);
    expect(await note.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(
      true,
    );
    await note.fill("Words that wrap into multiple lines when the note becomes narrow.");
    const wide = await height();
    await page.getByRole("button", { name: "Toggle note width" }).click();
    await expect.poll(height).toBeGreaterThan(wide);
    await note.fill("Short");
    const smallFont = await height();
    await page.getByRole("button", { name: "Toggle note font" }).click();
    await expect.poll(height).toBeGreaterThan(smallFont);
    const uncontrolled = page.getByRole("textbox", { name: "Uncontrolled note", exact: true });
    await uncontrolled.fill("one\ntwo\nthree\nfour\nfive");
    await page.getByRole("button", { name: "Reset notes" }).click();
    await expect(note).toHaveValue("A short note");
    await expect(uncontrolled).toHaveValue("A short note");
    await expect
      .poll(() => uncontrolled.evaluate((element) => element.getBoundingClientRect().height))
      .toBeLessThan(initial * 2);
  });

  test("search mutation paths obey the field state with real keyboard activation", async ({
    page,
  }) => {
    await page.goto("/lab/fixture.html?fixture=atoms&state=input-contracts&theme=light");
    for (const state of ["disabled", "readOnly", "pending"]) {
      const clear = page.getByRole("button", { name: `Clear ${state} search`, exact: true });
      await expect(clear).toBeDisabled();
      await clear.dispatchEvent("click");
      await expect(
        page.getByRole("searchbox", { name: `${state} search`, exact: true }),
      ).toHaveValue("fixed");
    }
    const clear = page.getByRole("button", { name: "Clear editable search", exact: true });
    await clear.focus();
    await page.keyboard.press("Enter");
    await expect(page.getByRole("searchbox", { name: "Editable search" })).toHaveValue("");
    await expect(clear).toHaveCount(0);
  });

  test("Dock menu movement and closing restore focus without iterator helpers", async ({
    page,
  }) => {
    await page.addInitScript(() => {
      const prototype = Object.getPrototypeOf(Object.getPrototypeOf([][Symbol.iterator]()));
      Object.defineProperty(prototype, "find", { value: undefined, configurable: true });
    });
    await page.goto("/lab/fixture.html?fixture=workspace&state=dock-light&theme=light");
    await page.locator("html[data-visual-ready]").waitFor();
    const trajectory = page.getByRole("tab", { name: "Session trajectory", exact: true });
    await trajectory.focus();
    await page.keyboard.press("Shift+F10");
    await expect(page.getByRole("menuitem", { name: "Move panel right" })).toBeDisabled();
    await page.getByRole("menuitem", { name: "Move panel left" }).click();
    await expect(page.getByTestId("dock-view-ids")).toHaveText(
      "file,package:visual:trajectory,package:visual:diff",
    );
    await expect(trajectory).toBeFocused();
    await page.keyboard.press("Delete");
    const diff = page.getByRole("tab", { name: "Workspace diff", exact: true });
    await expect(diff).toBeFocused();
    await page.keyboard.press("Shift+F10");
    await page.getByRole("menuitem", { name: "Close all panels" }).click();
    await expect(page.getByRole("button", { name: "Browse panels" })).toBeFocused();
    await expect(page.getByRole("tab")).toHaveCount(0);
  });

  test("a canceled Dock resize restores preview and leaves the preference untouched", async ({
    page,
  }) => {
    await page.goto("/lab/fixture.html?fixture=workspace&state=dock-light&theme=light");
    await page.locator("html[data-visual-ready]").waitFor();
    const ratio = page.getByTestId("persisted-dock-ratio");
    const before = await ratio.textContent();
    const row = page.locator(".agent-dock-row");
    const property = await row.evaluate((element) =>
      (element as HTMLElement).style.getPropertyValue("--dock-measure"),
    );
    const separator = page.getByRole("separator", { name: "Resize right workspace" });
    const box = await separator.boundingBox();
    await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2);
    await page.mouse.down();
    await page.mouse.move(box!.x - 80, box!.y + box!.height / 2, { steps: 4 });
    await page.keyboard.press("Escape");
    await page.mouse.up();
    await expect(ratio).toHaveText(before!);
    expect(
      await row.evaluate((element) =>
        (element as HTMLElement).style.getPropertyValue("--dock-measure"),
      ),
    ).toBe(property);
  });

  test("image zoom anchors a pixel and pan is independent of event segmentation", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 640, height: 360 });
    await page.goto("/lab/fixture.html?fixture=agent&state=long-content&theme=light");
    await page.locator("html[data-visual-ready]").waitFor();
    const trigger = page.getByRole("button", { name: "Inline architecture", exact: true });
    await trigger.scrollIntoViewIfNeeded();
    await trigger.click();
    const dialog = page.getByRole("dialog", { name: "Inline architecture", exact: true });
    const pan = dialog.getByRole("region", { name: "Inline architecture" });
    const image = dialog.locator("img");
    for (let step = 0; step < 4; step++)
      await page.getByRole("button", { name: "Zoom in image" }).click();
    await expect(pan).toHaveAttribute("data-image-zoom", "300");
    const box = await pan.boundingBox();
    const point = { x: box!.x + box!.width / 2 + 8, y: box!.y + box!.height / 2 };
    const before = await image.boundingBox();
    const coordinate = {
      x: (point.x - before!.x) / before!.width,
      y: (point.y - before!.y) / before!.height,
    };
    await page.mouse.move(point.x, point.y);
    await page.mouse.wheel(0, -100);
    await expect(pan).toHaveAttribute("data-image-zoom", "400");
    const after = await image.boundingBox();
    expect(Math.abs(after!.x + coordinate.x * after!.width - point.x)).toBeLessThan(1.5);
    expect(Math.abs(after!.y + coordinate.y * after!.height - point.y)).toBeLessThan(1.5);
    const drag = async (steps: number) => {
      await pan.evaluate((element) => {
        element.scrollLeft = 100;
        element.scrollTop = 30;
      });
      await page.mouse.move(point.x, point.y);
      await page.mouse.down();
      await page.mouse.move(point.x - 30, point.y - 20, { steps });
      await page.mouse.up();
      return pan.evaluate((element) => ({ x: element.scrollLeft, y: element.scrollTop }));
    };
    const single = await drag(1);
    const segmented = await drag(3);
    expect(segmented).toEqual(single);
    expect(single.x).toBeGreaterThan(100);
    await pan.focus();
    await page.keyboard.press("ArrowRight");
    await expect(dialog).toBeVisible();
    await expect
      .poll(() => pan.evaluate((element) => element.scrollLeft))
      .toBeGreaterThan(single.x);
    await page.setViewportSize({ width: 700, height: 400 });
    await expect(pan).toHaveAttribute("data-image-zoom", "100");
    const next = page.getByRole("button", { name: "Next image", exact: true });
    await next.click();
    await expect(page.getByRole("dialog", { name: "Inline detail", exact: true })).toBeVisible();
    await expect(next).toBeFocused();
    await page.keyboard.press("ArrowLeft");
    await expect(dialog).toBeVisible();
    await expect(pan).toHaveAttribute("data-image-zoom", "100");
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
    await expect(trigger).toBeFocused();
  });

  for (const theme of ["light", "dark"]) {
    test(`keyboard-opened schedule Model stays open ${theme}`, async ({
      page,
      browserName,
    }, testInfo) => {
      await page.setViewportSize({ width: 373, height: 243 });
      await page.goto(
        `/lab/fixture.html?fixture=workspace&state=full-view&full-view=package:visual:schedules&sidebar=collapsed&theme=${theme}&locale=zh&font-size=18`,
      );
      await page.locator("html[data-visual-ready]").waitFor();
      const panel = page
        .getByRole("region", { name: "定时任务", exact: true })
        .and(page.locator('[data-slot="panel"]'));
      const add = panel.getByRole("button", { name: "添加定时任务", exact: true });
      await expect(add).toBeEnabled();
      await add.click();
      const editor = page.getByRole("dialog", { name: "新建定时任务", exact: true });
      const body = editor.locator('[data-slot="dialog-body"]');
      const title = editor.getByRole("textbox", { name: "标题", exact: true });
      await expect(title).toBeFocused();
      const draft = "  Padded first-open draft  ";
      await title.fill(draft);
      await expect(title).toHaveValue(draft);

      const model = editor.getByRole("combobox", { name: "模型", exact: true });
      for (const next of [
        editor.getByRole("textbox", { name: "指令", exact: true }),
        editor.getByRole("textbox", { name: "Cron 表达式", exact: true }),
        editor.getByRole("textbox", { name: "时区", exact: true }),
        editor.getByRole("textbox", { name: "工作区路径", exact: true }),
        editor.getByRole("checkbox", { name: "已启用", exact: true }),
        model,
      ]) {
        await page.keyboard.press("Tab");
        await expect(next).toBeFocused();
      }

      const modelPlacement = () =>
        model.evaluate((control) => {
          const viewport = control.closest('[data-slot="dialog-body"]')!.getBoundingClientRect();
          const box = control.getBoundingClientRect();
          const hit = document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2);
          return {
            inside:
              box.top >= viewport.top &&
              box.bottom <= viewport.bottom &&
              box.left >= viewport.left &&
              box.right <= viewport.right,
            outside: box.top >= viewport.bottom || box.bottom <= viewport.top,
            hit: hit !== null && control.contains(hit),
          };
        });
      await testInfo.attach(`schedule-model-before-${browserName}-${theme}.png`, {
        body: await page.screenshot({ animations: "disabled", caret: "hide" }),
        contentType: "image/png",
      });
      expect(await modelPlacement()).toMatchObject({ inside: true, hit: true });

      const popup = page.getByRole("listbox");
      await page.keyboard.press("Enter");
      try {
        await expect(popup).toBeVisible();
        await expect(model).toHaveAttribute("aria-expanded", "true");
        await expect(
          popup.getByRole("option", { name: "Runtime 默认模型", exact: true }),
        ).toBeFocused();
        await expect
          .poll(() =>
            popup.evaluate((element) => {
              let opacity = 1;
              for (let node: Element | null = element; node; node = node.parentElement)
                opacity *= Number(getComputedStyle(node).opacity);
              return opacity;
            }),
          )
          .toBe(1);
      } finally {
        await testInfo.attach(`schedule-model-after-${browserName}-${theme}.png`, {
          body: await page.screenshot({ animations: "disabled", caret: "hide" }),
          contentType: "image/png",
        });
        await testInfo.attach(`schedule-model-after-${browserName}-${theme}.json`, {
          body: JSON.stringify({
            expanded: await model.getAttribute("aria-expanded"),
            listboxes: await popup.count(),
            active: await page.evaluate(() => {
              const active = document.activeElement;
              return (
                active &&
                `${active.tagName} ${active.getAttribute("role") ?? ""} ${active.getAttribute("data-slot") ?? ""}`
              );
            }),
            placement: await modelPlacement(),
          }),
          contentType: "application/json",
        });
      }

      const sol = popup.getByRole("option", { name: "openai / GPT-5.6 Sol", exact: true });
      await page.keyboard.press("ArrowDown");
      await expect(sol).toBeFocused();
      await page.keyboard.press("Enter");
      await expect(popup).toBeHidden();
      await expect(model).toContainText("openai / GPT-5.6 Sol");
      await expect(model).toBeFocused();
      await expect(editor).toBeVisible();
      await expect(title).toHaveValue(draft);

      await page.keyboard.press("Enter");
      await expect(popup).toBeVisible();
      await page.keyboard.press("Escape");
      await expect(popup).toBeHidden();
      await expect(model).toBeFocused();
      await expect(editor).toBeVisible();

      await page.keyboard.press("Enter");
      await expect(popup).toBeVisible();
      const corner = await body.evaluate((node) => {
        const box = node.getBoundingClientRect();
        return { x: box.left + node.clientLeft + 8, y: box.top + node.clientTop + 8 };
      });
      const popupBox = (await popup.boundingBox())!;
      expect(
        corner.x >= popupBox.x &&
          corner.x <= popupBox.x + popupBox.width &&
          corner.y >= popupBox.y &&
          corner.y <= popupBox.y + popupBox.height,
      ).toBe(false);
      await page.mouse.move(corner.x, corner.y);
      await page.mouse.wheel(0, -10_000);
      await expect(popup).toBeHidden();
      await expect(body).toBeFocused();
      await expect.poll(async () => (await modelPlacement()).outside).toBe(true);
      await page.evaluate(
        () => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))),
      );
      expect((await modelPlacement()).outside).toBe(true);
      await expect(title).toHaveValue(draft);

      await editor.getByRole("button", { name: "取消", exact: true }).click();
      await expect(add).toBeFocused();
    });
  }
  for (const theme of ["light", "dark"]) {
    test(`long choices retain the task and conversation at narrow frames ${theme}`, async ({
      page,
    }, testInfo) => {
      await page.setViewportSize({ width: 1120, height: 728 });
      await page.goto(
        `/lab/fixture.html?fixture=workspace&state=dock-schedules&locale=de&font-size=18&theme=${theme}&long-models=1`,
      );
      await page.locator("html[data-visual-ready]").waitFor();
      await page.evaluate(() => document.fonts.ready);
      await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
      const composer = page.getByRole("textbox", { name: "Nachrichteneditor", exact: true });
      const conversationDraft = "Keep this draft while reviewing the schedule editor.";
      await composer.fill(conversationDraft);
      await page.getByRole("button", { name: "In voller Breite öffnen", exact: true }).click();
      const add = page.getByRole("button", { name: "Zeitplan hinzufügen", exact: true });
      await add.click();
      const editor = page.getByRole("dialog", { name: "Neuer Zeitplan", exact: true });
      const title = editor.getByRole("textbox", { name: "Titel", exact: true });
      const instructions = editor.getByRole("textbox", { name: "Anweisungen", exact: true });
      const cron = editor.getByRole("textbox", { name: "Cron-Ausdruck", exact: true });
      const model = editor.getByRole("combobox", { name: "Modell", exact: true });
      const refresh = editor.getByRole("button", {
        name: "Modellauswahl aktualisieren",
        exact: true,
      });
      const cancel = editor.getByRole("button", { name: "Abbrechen", exact: true });
      const save = editor.getByRole("button", { name: "Zeitplan speichern", exact: true });
      const taskTitle = "Wöchentliche Wartungsdurchsicht im Arbeitsbereich";
      const taskInstructions =
        "Prüfe die Ausrichtung und bewahre diesen Entwurf.\n\nFasse Risiken und nächste Schritte zusammen.";
      const geometry: unknown[] = [];
      const measure = async (state: string) => {
        const result = await editor.evaluate((node) => {
          const rect = (element: Element) => element.getBoundingClientRect().toJSON();
          const body = node.querySelector('[data-slot="dialog-body"]')!;
          const footer = node.querySelector("footer")!;
          const focusedText = document
            .createTreeWalker(document.activeElement!, NodeFilter.SHOW_TEXT)
            .nextNode();
          const firstLetter = document.createRange();
          if (focusedText) {
            firstLetter.setStart(focusedText, 0);
            firstLetter.setEnd(focusedText, 1);
          }
          return {
            dialog: rect(node),
            body: rect(body),
            footer: rect(footer),
            viewport: { width: innerWidth, height: innerHeight },
            overflow: node.scrollWidth - node.clientWidth,
            bodyOverflow: body.scrollWidth - body.clientWidth,
            actions: [...footer.querySelectorAll("button")].map((button) => {
              const range = document.createRange();
              range.selectNodeContents(button);
              return {
                name: button.textContent,
                box: rect(button),
                text: range.getBoundingClientRect().toJSON(),
              };
            }),
            focused: rect(document.activeElement!),
            firstLetter: focusedText ? firstLetter.getBoundingClientRect().toJSON() : null,
          };
        });
        geometry.push({ state, ...result });
        return result;
      };
      try {
        await expect(title).toBeFocused();
        await title.fill(taskTitle);
        await instructions.fill(taskInstructions);
        await cron.fill("0 9 * * 1");
        await expect(save).toBeEnabled();
        for (const frame of [
          { width: 560, height: 364 },
          { width: 373, height: 243 },
        ]) {
          await page.setViewportSize(frame);
          const measured = await measure(`frame-${frame.width}`);
          expect(measured.overflow).toBe(0);
          expect(measured.bodyOverflow).toBe(0);
          expect(measured.dialog.left).toBeGreaterThanOrEqual(0);
          expect(measured.dialog.top).toBeGreaterThanOrEqual(0);
          expect(measured.dialog.right).toBeLessThanOrEqual(frame.width);
          expect(measured.dialog.bottom).toBeLessThanOrEqual(frame.height);
          expect(measured.body.bottom).toBeLessThanOrEqual(measured.footer.top + 0.5);
          expect(measured.actions.map((action) => action.name)).toEqual([
            "Abbrechen",
            "Zeitplan speichern",
          ]);
          expect(measured.actions[0]!.box.bottom).toBe(measured.actions[1]!.box.bottom);
          for (const action of measured.actions) {
            expect(action.box.left).toBeGreaterThanOrEqual(measured.dialog.left);
            expect(action.box.right).toBeLessThanOrEqual(measured.dialog.right);
            expect(action.box.top).toBeGreaterThanOrEqual(measured.footer.top);
            expect(action.box.bottom).toBeLessThanOrEqual(frame.height);
            expect(action.text.left).toBeGreaterThanOrEqual(action.box.left);
            expect(action.text.right).toBeLessThanOrEqual(action.box.right);
            expect(action.text.top).toBeGreaterThanOrEqual(action.box.top);
            expect(action.text.bottom).toBeLessThanOrEqual(action.box.bottom);
          }
          await testInfo.attach(`schedule-frame-${frame.width}`, {
            body: await page.screenshot(),
            contentType: "image/png",
          });
        }
        await expect(cron).toBeFocused();
        for (let step = 0; step < 4; step++) await page.keyboard.press("Tab");
        await expect(model).toBeFocused();
        const focused = await measure("model-focus");
        expect(focused.focused.top).toBeGreaterThanOrEqual(focused.body.top - 0.5);
        expect(focused.focused.bottom).toBeLessThanOrEqual(focused.footer.top + 0.5);
        await page.keyboard.press("Enter");
        const popup = page.getByRole("listbox");
        await expect(popup).toHaveCSS("opacity", "1");
        await expect(
          popup.getByRole("option", { name: "Runtime-Standardmodell", exact: true }),
        ).toBeFocused();
        await page.keyboard.press("ArrowDown");
        const option = popup.getByRole("option", {
          name: `openai / ${LONG_MODEL_NAME}`,
          exact: true,
        });
        await expect(option).toBeFocused();
        const optionBounds = await option.boundingBox();
        const popupBounds = await popup.boundingBox();
        geometry.push({ state: "long-option", optionBounds, popupBounds });
        expect(optionBounds).not.toBeNull();
        expect(popupBounds).not.toBeNull();
        expect(optionBounds!.y).toBeGreaterThanOrEqual(popupBounds!.y - 0.5);
        expect(optionBounds!.y + optionBounds!.height).toBeLessThanOrEqual(
          popupBounds!.y + popupBounds!.height + 0.5,
        );
        await testInfo.attach("schedule-long-option", {
          body: await page.screenshot(),
          contentType: "image/png",
        });
        await page.keyboard.press("Enter");
        await expect(model).toHaveText(`openai / ${LONG_MODEL_NAME}`);
        await model.click();
        await expect(option).toBeFocused();
        await page.keyboard.press("Escape");
        await expect(model).toBeFocused();
        const selected = await measure("long-selection-return");
        expect(selected.overflow).toBe(0);
        expect(selected.bodyOverflow).toBe(0);
        expect(selected.firstLetter).not.toBeNull();
        expect(selected.firstLetter!.top).toBeGreaterThanOrEqual(selected.body.top - 0.5);
        expect(selected.firstLetter!.bottom).toBeLessThanOrEqual(selected.body.bottom + 0.5);
        await page.keyboard.press("Tab");
        await expect(
          editor.getByRole("combobox", { name: "Denkintensität", exact: true }),
        ).toBeFocused();
        await page.keyboard.press("Tab");
        await expect(refresh).toBeFocused();
        const refreshed = await measure("refresh-focus");
        expect(refreshed.focused.top).toBeGreaterThanOrEqual(refreshed.body.top - 0.5);
        expect(refreshed.focused.bottom).toBeLessThanOrEqual(refreshed.body.bottom + 0.5);
        await page.keyboard.press("Enter");
        await expect(refresh).toBeEnabled();
        await expect(refresh).toBeFocused();
        await expect(title).toHaveValue(taskTitle);
        await expect(instructions).toHaveValue(taskInstructions);
        await expect(cron).toHaveValue("0 9 * * 1");
        await expect(model).toHaveText(`openai / ${LONG_MODEL_NAME}`);
        await testInfo.attach("schedule-refreshed-draft", {
          body: await page.screenshot(),
          contentType: "image/png",
        });
        await page.keyboard.press("Tab");
        await expect(cancel).toBeFocused();
        await page.keyboard.press("Enter");
        await expect(editor).toHaveCount(0);
        await expect(add).toBeFocused();
        await page.getByRole("button", { name: "Schließen", exact: true }).click();
        await expect(composer).toHaveValue(conversationDraft);
        await composer.press("End");
        await page.keyboard.type(" Then continue it.");
        await expect(composer).toHaveValue(`${conversationDraft} Then continue it.`);
        await testInfo.attach("schedule-conversation-return", {
          body: await page.screenshot(),
          contentType: "image/png",
        });
      } finally {
        if (await editor.count()) await measure("final-editor");
        await testInfo.attach("schedule-geometry", {
          body: JSON.stringify(geometry, null, 2),
          contentType: "application/json",
        });
      }
    });
  }
}
