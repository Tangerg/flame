import { expect, test, type Locator } from "./test";
import { expectLabReady } from "../readiness";

export async function attachmentFrame(attachment: Locator) {
  return attachment.evaluate((node) => {
    const box = (element: Element) => {
      const { left, top, right, bottom, width, height } = element.getBoundingClientRect();
      return { left, top, right, bottom, width, height };
    };
    const frame = (element: Element) => {
      const visible = { left: 0, top: 0, right: innerWidth, bottom: innerHeight };
      const ancestors = [];
      for (let parent = element.parentElement; parent; parent = parent.parentElement) {
        const style = getComputedStyle(parent);
        const rect = parent.getBoundingClientRect();
        if (style.overflowX !== "visible") {
          visible.left = Math.max(visible.left, rect.left + parseFloat(style.borderLeftWidth));
          visible.right = Math.min(visible.right, rect.right - parseFloat(style.borderRightWidth));
        }
        if (style.overflowY !== "visible") {
          visible.top = Math.max(visible.top, rect.top + parseFloat(style.borderTopWidth));
          visible.bottom = Math.min(
            visible.bottom,
            rect.bottom - parseFloat(style.borderBottomWidth),
          );
        }
        ancestors.push({
          slot: parent.dataset.slot ?? parent.tagName.toLowerCase(),
          box: box(parent),
          clientHeight: parent.clientHeight,
          scrollHeight: parent.scrollHeight,
          scrollTop: parent.scrollTop,
          display: style.display,
          flexShrink: style.flexShrink,
          minHeight: style.minHeight,
          maxHeight: style.maxHeight,
          overflowX: style.overflowX,
          overflowY: style.overflowY,
        });
      }
      return { box: box(element), visible, ancestors };
    };
    const root = node.closest('[data-slot="composer-root"]')!;
    const input = root.querySelector("textarea")!;
    const attachment = frame(node);
    const inputStyle = getComputedStyle(input);
    const fontSize = parseFloat(inputStyle.fontSize);
    const lineHeight = parseFloat(inputStyle.lineHeight);
    const lineTop =
      input.getBoundingClientRect().top +
      input.clientTop +
      parseFloat(inputStyle.paddingTop) -
      input.scrollTop;
    return {
      attachment: attachment.box,
      buttons: [...node.querySelectorAll("button")].map((button) => ({
        name: button.getAttribute("aria-label"),
        box: box(button),
      })),
      visible: attachment.visible,
      ancestors: attachment.ancestors,
      input: {
        ...frame(input),
        clientHeight: input.clientHeight,
        scrollHeight: input.scrollHeight,
        firstLine: {
          top: lineTop + (lineHeight - fontSize) / 2,
          bottom: lineTop + (lineHeight + fontSize) / 2,
        },
      },
      footerTop: root.querySelector('[data-slot="composer-footer"]')!.getBoundingClientRect().top,
      documentOverflow: document.documentElement.scrollWidth - innerWidth,
    };
  });
}

export function registerComposerPasteContracts() {
  for (const profile of [
    {
      locale: "en",
      theme: "dark",
      composer: "Message composer",
      review: "Review pasted text",
      title: "Pasted text",
      cancel: "Cancel",
      save: "Save",
      restore: "Put back in message",
    },
    {
      locale: "zh",
      theme: "light",
      composer: "消息输入框",
      review: "查看粘贴的文本",
      title: "粘贴的文本",
      cancel: "取消",
      save: "保存",
      restore: "放回输入框",
    },
  ]) {
    test(`a complete paste attachment keeps its review and return path in ${profile.locale}`, async ({
      page,
    }, testInfo) => {
      const prefix = "Keep the typed prefix.";
      const pasted = Array.from({ length: 12 }, (_, index) => `Captured line ${index + 1}.`).join(
        "\n",
      );
      const edited = "Reviewed evidence · 修改后的正文\nKeep the final line.";
      const samples: unknown[] = [];
      await page.setViewportSize({ width: 1120, height: 728 });
      await page.goto(
        `/lab/fixture.html?fixture=workspace&state=dock-trajectory&agent-state=code-reading&locale=${profile.locale}&theme=${profile.theme}&font-size=18&motion=off`,
      );
      await expectLabReady(page);
      await page
        .getByRole("button", { name: "Collapse the workspace fixture sidebar", exact: true })
        .click();
      const input = page.getByRole("textbox", { name: profile.composer, exact: true });
      const originalInput = (await input.elementHandle())!;
      await page.setViewportSize({ width: 560, height: 364 });
      await input.focus();
      await input.evaluate((node, text) => {
        const data = new DataTransfer();
        data.setData("text/plain", text);
        node.dispatchEvent(new ClipboardEvent("paste", { clipboardData: data, bubbles: true }));
      }, pasted);
      const review = page.getByRole("button", { name: profile.review, exact: true });
      const chip = page.locator('[data-slot="chip"]').filter({ has: review });
      await expect(chip).toHaveCount(1);
      const expectCompleteAttachment = (frame: Awaited<ReturnType<typeof attachmentFrame>>) => {
        for (const target of [frame.attachment, ...frame.buttons.map((button) => button.box)]) {
          expect(target.left).toBeGreaterThanOrEqual(frame.visible.left - 0.5);
          expect(target.right).toBeLessThanOrEqual(frame.visible.right + 0.5);
          expect(target.top).toBeGreaterThanOrEqual(frame.visible.top - 0.5);
          expect(target.bottom).toBeLessThanOrEqual(frame.visible.bottom + 0.5);
        }
      };
      const checkAttachment = async (step: string, expectedDraft = prefix) => {
        await expect(async () => {
          const frame = await attachmentFrame(chip);
          samples.push({ step, ...frame });
          expectCompleteAttachment(frame);
          expect(frame.input.box.top).toBeGreaterThanOrEqual(frame.input.visible.top - 0.5);
          expect(frame.input.box.bottom).toBeLessThanOrEqual(
            Math.min(frame.input.visible.bottom, frame.footerTop) + 0.5,
          );
          expect(frame.input.scrollHeight - frame.input.clientHeight).toBeLessThanOrEqual(1);
          expect(frame.documentOverflow).toBe(0);
        }).toPass({ timeout: 3_000 });
        await expect(input).toHaveValue(expectedDraft);
        await testInfo.attach(`${step}.png`, {
          body: await page.screenshot({ animations: "disabled", caret: "hide" }),
          contentType: "image/png",
        });
      };
      try {
        await checkAttachment("first-empty-paste", "");
        await input.fill(prefix);
        await checkAttachment("short-paste");
        await page.setViewportSize({ width: 1120, height: 728 });
        await checkAttachment("wide-paste");
        await page.setViewportSize({ width: 560, height: 364 });
        await review.focus();
        await checkAttachment("focused-paste");
        await page.setViewportSize({ width: 373, height: 243 });
        await expect(async () => {
          const frame = await attachmentFrame(chip);
          samples.push({ step: "narrow-paste-review", ...frame });
          expectCompleteAttachment(frame);
        }).toPass({ timeout: 3_000 });
        await testInfo.attach("narrow-paste-review.png", {
          body: await page.screenshot({ animations: "disabled", caret: "hide" }),
          contentType: "image/png",
        });
        await input.focus();
        await expect(async () => {
          const frame = await attachmentFrame(chip);
          samples.push({ step: "narrow-paste-input", ...frame });
          expect(frame.input.box.top).toBeGreaterThanOrEqual(frame.input.visible.top - 0.5);
          expect(frame.input.box.bottom).toBeLessThanOrEqual(
            Math.min(frame.input.visible.bottom, frame.footerTop) + 0.5,
          );
          expect(frame.documentOverflow).toBe(0);
        }).toPass({ timeout: 3_000 });
        await expect(input).toHaveValue(prefix);
        await testInfo.attach("narrow-paste-input.png", {
          body: await page.screenshot({ animations: "disabled", caret: "hide" }),
          contentType: "image/png",
        });
        await page.setViewportSize({ width: 560, height: 364 });
        await review.focus();
        await review.press("Enter");
        const dialog = page.getByRole("dialog", { name: profile.title, exact: true });
        const editor = dialog.getByRole("textbox", { name: profile.title, exact: true });
        await expect(editor).toBeFocused();
        await expect(editor).toHaveValue(pasted);
        await editor.fill(edited);
        await dialog.getByRole("button", { name: profile.cancel, exact: true }).press("Enter");
        await expect(dialog).toBeHidden();
        await expect(review).toBeFocused();
        await review.press("Enter");
        await expect(editor).toHaveValue(pasted);
        await editor.fill(edited);
        await dialog.getByRole("button", { name: profile.save, exact: true }).press("Enter");
        await expect(dialog).toBeHidden();
        await expect(review).toBeFocused();
        await checkAttachment("saved-paste");
        await review.press("Enter");
        await expect(editor).toHaveValue(edited);
        await dialog.getByRole("button", { name: profile.restore, exact: true }).press("Enter");
        await expect(dialog).toBeHidden();
        await expect(chip).toHaveCount(0);
        await expect(input).toBeFocused();
        const restored = `${prefix}\n\n${edited}`;
        await expect(input).toHaveValue(restored);
        await expect(input).toHaveJSProperty("selectionStart", restored.length);
        await expect(input).toHaveJSProperty("selectionEnd", restored.length);
        await input.press("x");
        await expect(input).toHaveValue(restored + "x");
        expect(await input.evaluate((node, original) => node === original, originalInput)).toBe(
          true,
        );
      } finally {
        await testInfo.attach("paste-layout.json", {
          body: JSON.stringify(samples),
          contentType: "application/json",
        });
        await testInfo.attach("paste-final.png", {
          body: await page.screenshot({ animations: "disabled", caret: "hide" }),
          contentType: "image/png",
        });
      }
    });
  }
}
