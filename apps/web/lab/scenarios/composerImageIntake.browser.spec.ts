import { expect, test } from "./test";
import { IMAGE_EXPORT_FIXTURES } from "./imageExportFixtures";

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    const nativeRead = FileReader.prototype.readAsDataURL;
    const pending: (() => Promise<void>)[] = [];
    Object.defineProperty(globalThis, "finishComposerImageReads", { value: pending });
    FileReader.prototype.readAsDataURL = function (file) {
      pending.push(
        () =>
          new Promise<void>((resolve) => {
            this.addEventListener("loadend", () => resolve(), { once: true });
            nativeRead.call(this, file);
          }),
      );
    };
  });
  await page.goto("/lab/fixture.html?fixture=agent&state=idle&locale=en");
  await page.locator("html[data-visual-ready]").waitFor();
});

async function finishReading(page: import("./test").Page) {
  await page.evaluate(async () => {
    const pending = (globalThis as unknown as { finishComposerImageReads: (() => Promise<void>)[] })
      .finishComposerImageReads;
    if (pending.length !== 1) throw new Error("expected one delayed native image read");
    await pending.shift()!();
  });
}

const image = IMAGE_EXPORT_FIXTURES.find((fixture) => fixture.mime === "image/png")!;

test("pending image input survives an immediate keyboard send and becomes ready", async ({
  page,
}) => {
  const composer = page.locator('[data-slot="composer-root"]');
  const input = composer.getByRole("textbox", { name: "Message composer" });
  const status = composer.getByRole("status");
  await expect(status).toBeEmpty();
  await input.fill("include the selected image");
  await page.getByLabel("Attach image", { exact: true }).setInputFiles({
    name: "selected.png",
    mimeType: image.mime,
    buffer: Buffer.from(image.base64, "base64"),
  });
  await expect(status).toHaveText("Reading selected.png…");
  const send = composer.getByRole("button", { name: "Send", exact: true });
  await expect(send).toBeDisabled();
  await input.press("Enter");
  await expect(input).toHaveValue("include the selected image");
  await expect(composer.getByRole("button", { name: "Remove image" })).toBeVisible();
  await finishReading(page);
  await expect(status).toHaveText("selected.png is ready");
  await expect(composer.getByRole("img", { name: "selected.png", exact: true })).toBeVisible();
  await expect(send).toBeEnabled();
  await expect(input).toHaveValue("include the selected image");
});

test("keyboard removal cancels image intent before native FileReader finishes", async ({
  page,
}) => {
  const composer = page.locator('[data-slot="composer-root"]');
  const input = composer.getByRole("textbox", { name: "Message composer" });
  await input.fill("keep this text");
  await page.getByLabel("Attach image", { exact: true }).setInputFiles({
    name: "cancelled.png",
    mimeType: image.mime,
    buffer: Buffer.from(image.base64, "base64"),
  });
  const remove = composer.getByRole("button", { name: "Remove image" });
  await remove.focus();
  await remove.press("Enter");
  await expect(input).toBeFocused();
  await finishReading(page);
  await expect(composer.getByRole("img", { name: "cancelled.png" })).toHaveCount(0);
  await expect(composer.getByRole("status")).toBeEmpty();
  await expect(input).toHaveValue("keep this text");
});

const longName =
  "quarterly-architecture-review-whiteboard-session-annotated-dependency-graph-FINAL-SUFFIX-7f3c.png";

for (const width of [320, 1120]) {
  for (const theme of ["light", "dark"]) {
    test(`long pending image name stays contained and keyboard-removable at ${width}px ${theme}`, async ({
      page,
    }, testInfo) => {
      await page.setViewportSize({ width, height: 900 });
      await page.goto(
        `/lab/fixture.html?fixture=agent&state=idle&locale=en&theme=${theme}&font-size=18&motion=off`,
      );
      await page.locator("html[data-visual-ready]").waitFor();
      const composer = page.locator('[data-slot="composer-root"]');
      const input = composer.getByRole("textbox", { name: "Message composer" });
      const status = composer.getByRole("status");
      const draft = "describe the whiteboard before sending";
      await input.fill(draft);
      await page.getByLabel("Attach image", { exact: true }).setInputFiles({
        name: longName,
        mimeType: image.mime,
        buffer: Buffer.from(image.base64, "base64"),
      });
      await expect(status).toHaveText(`Reading ${longName}…`);
      await expect(composer.getByRole("button", { name: "Send", exact: true })).toBeDisabled();
      const remove = composer.getByRole("button", { name: "Remove image" });
      const chip = composer
        .locator('[data-slot="chip"]')
        .filter({ has: page.getByRole("button", { name: "Remove image" }) });
      await expect(chip).toHaveCount(1);
      await testInfo.attach(`pending-long-image-${width}-${theme}.png`, {
        body: await page.screenshot({ animations: "disabled" }),
        contentType: "image/png",
      });

      const geometry = await chip.evaluate((node) => {
        const box = (element: Element) => {
          const { left, top, right, bottom } = element.getBoundingClientRect();
          return { left, top, right, bottom };
        };
        const root = node.closest('[data-slot="composer-root"]')!;
        const tray = node.closest('[data-slot="composer-attachments"]')!;
        const text = Array.from(node.querySelectorAll("span")).find(
          (span) => span.textContent === node.textContent,
        )!;
        const clips: { element: string; box: ReturnType<typeof box> }[] = [];
        for (let element = node.parentElement; element; element = element.parentElement) {
          const style = getComputedStyle(element);
          if (style.overflowX !== "visible" || style.overflowY !== "visible") {
            const bounds = element.getBoundingClientRect();
            clips.push({
              element: element.dataset.slot ?? element.tagName.toLowerCase(),
              box: {
                left: bounds.left + element.clientLeft,
                top: bounds.top + element.clientTop,
                right: bounds.left + element.clientLeft + element.clientWidth,
                bottom: bounds.top + element.clientTop + element.clientHeight,
              },
            });
          }
          if (element === root) break;
        }
        const visible = [box(tray), box(root), ...clips.map((clip) => clip.box)].reduce((a, b) => ({
          left: Math.max(a.left, b.left),
          top: Math.max(a.top, b.top),
          right: Math.min(a.right, b.right),
          bottom: Math.min(a.bottom, b.bottom),
        }));
        return {
          chip: box(node),
          remove: box(node.querySelector('button[aria-label="Remove image"]')!),
          text: { ...box(text), clientWidth: text.clientWidth, scrollWidth: text.scrollWidth },
          tray: box(tray),
          composer: box(root),
          clips,
          visible,
        };
      });
      await testInfo.attach(`pending-long-image-${width}-${theme}.json`, {
        body: JSON.stringify(geometry, null, 2),
        contentType: "application/json",
      });
      expect(geometry.text.scrollWidth, "the long name is truncated").toBeGreaterThan(
        geometry.text.clientWidth,
      );
      for (const [name, rect] of [
        ["chip", geometry.chip],
        ["remove", geometry.remove],
      ] as const) {
        expect(rect.left, `${name} left`).toBeGreaterThanOrEqual(geometry.visible.left - 1);
        expect(rect.top, `${name} top`).toBeGreaterThanOrEqual(geometry.visible.top - 1);
        expect(rect.right, `${name} right`).toBeLessThanOrEqual(geometry.visible.right + 1);
        expect(rect.bottom, `${name} bottom`).toBeLessThanOrEqual(geometry.visible.bottom + 1);
      }

      await input.focus();
      await page.keyboard.press("Shift+Tab");
      await expect(remove).toBeFocused();
      const tooltip = page.getByRole("tooltip");
      await expect(tooltip).toHaveText(await status.innerText());
      await expect(tooltip).toBeVisible();
      await testInfo.attach(`pending-long-image-${width}-${theme}-focused.png`, {
        body: await page.screenshot({ animations: "disabled" }),
        contentType: "image/png",
      });
      await remove.press("Enter");
      await expect(input).toBeFocused();
      await expect(input).toHaveValue(draft);
      await expect(status).toBeEmpty();
      await finishReading(page);
      await expect(composer.getByRole("img", { name: longName })).toHaveCount(0);
      await expect(remove).toHaveCount(0);
      await expect(status).toBeEmpty();
      await expect(input).toHaveValue(draft);
      await testInfo.attach(`pending-long-image-${width}-${theme}-removed.png`, {
        body: await page.screenshot({ animations: "disabled" }),
        contentType: "image/png",
      });
    });
  }
}

for (const theme of ["light", "dark"]) {
  test(`failed image exposes its complete recovery hint and retains the draft ${theme}`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width: 320, height: 360 });
    await page.goto(
      `/lab/preview.html?story=business-attachments&variant=failed&locale=en&theme=${theme}&font-size=18&motion=off`,
    );
    await page.locator('html[data-lab-ready="true"]').waitFor();
    const input = page.getByRole("textbox", { name: "Message composer", exact: true });
    const status = page.getByRole("status");
    const message = await status.innerText();
    expect(message).toContain("Remove it and choose it again.");
    const draft = await input.inputValue();
    await input.focus();
    await page.keyboard.press("Shift+Tab");
    const remove = page.getByRole("button", { name: "Remove image", exact: true });
    await expect(remove).toBeFocused();
    const tooltip = page.getByRole("tooltip");
    await expect(tooltip).toHaveText(message);
    await expect(tooltip).toBeVisible();
    await testInfo.attach(`failed-image-recovery-${theme}.png`, {
      body: await page.screenshot({ animations: "disabled" }),
      contentType: "image/png",
    });
    await page.keyboard.press("Enter");
    await expect(remove).toHaveCount(0);
    await expect(input).toBeFocused();
    await expect(input).toHaveValue(draft);
    await expect(status).toBeEmpty();
    await testInfo.attach(`failed-image-return-${theme}.png`, {
      body: await page.screenshot({ animations: "disabled" }),
      contentType: "image/png",
    });
  });
}
