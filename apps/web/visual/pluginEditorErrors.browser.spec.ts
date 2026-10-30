import { expect, test } from "./test";
for (const theme of ["light", "dark"]) {
  for (const width of [1120, 320]) {
    test(`plugin input failures stay with the modal editor ${theme} ${width}`, async ({ page }) => {
      const height = 568;
      await page.setViewportSize({ width, height });
      await page.goto(
        `/visual/?fixture=workspace&state=settings&pane=plugins&theme=${theme}&font-size=18`,
      );
      await page.locator("html[data-visual-ready]").waitFor();
      const trigger = page.getByRole("button", { name: "Configure inputs", exact: true }).first();
      await trigger.click();
      const dialog = page.getByRole("dialog", { name: /^Configure inputs: / });
      const field = dialog.getByRole("textbox", { name: "Input changes (JSON)", exact: true });
      await expect(field).toBeFocused();
      await field.fill("{");
      await field.press("Control+Enter");
      const message = dialog.getByRole("alert");
      await expect(message).toContainText("Enter valid JSON with unique property names.");
      await expect(page.locator('[role="alert"]')).toHaveCount(1);
      await expect(field).toHaveValue("{");
      await expect(field).toHaveAttribute("aria-invalid", "true");
      await expect(field).toHaveAttribute("aria-describedby", (await message.getAttribute("id"))!);
      await expect(message).toBeInViewport({ ratio: 1 });
      await expect(field).toBeInViewport({ ratio: 1 });
      await expect(dialog.getByText("Input changes (JSON)", { exact: true })).toBeInViewport({
        ratio: 1,
      });
      const shortHeight = (await field.boundingBox())!.height;
      const box = (await dialog.boundingBox())!;
      expect(box.x).toBeGreaterThanOrEqual(16);
      expect(box.x + box.width).toBeLessThanOrEqual(width - 16);
      expect(box.y).toBeGreaterThanOrEqual(16);
      expect(box.y + box.height).toBeLessThanOrEqual(height - 16);
      expect(await dialog.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);

      const input = JSON.stringify({ digest: "2".repeat(64) });
      await field.fill(input);
      await field.press("Control+Enter");
      await expect(message).toContainText("“digest” belongs to the reviewed release");
      await expect(field).toHaveValue(input);
      await field.fill("long input line\n".repeat(2000));
      await field.scrollIntoViewIfNeeded();
      await expect(field).toBeInViewport({ ratio: 1 });
      expect((await field.boundingBox())!.height).toBeGreaterThan(shortHeight);
      expect(await field.evaluate((node) => node.scrollHeight > node.clientHeight)).toBe(true);
      await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
      await expect(dialog).toHaveCount(0);
      await expect(trigger).toBeFocused();
      await trigger.click();
      await expect(field).toBeFocused();
      await expect(field).not.toHaveAttribute("aria-invalid", "true");
      await expect(dialog.getByRole("alert")).toHaveCount(0);
      await page.keyboard.press("Escape");
      await expect(trigger).toBeFocused();
    });
  }
}
