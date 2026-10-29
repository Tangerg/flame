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
  await page.goto("/visual/?fixture=agent&state=idle&locale=en");
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
