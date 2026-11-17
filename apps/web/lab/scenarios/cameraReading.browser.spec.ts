import { expect, test } from "./test";
import { expectLabReady } from "../readiness";

for (const browserName of ["chromium", "webkit"] as const) {
  test.extend({ browserName })(
    `${browserName} keeps gallery focus while the camera owns fit, zoom and keyboard pan`,
    async ({ page }) => {
      await page.goto("/lab/fixture.html?fixture=agent&state=long-content&locale=en&motion=off");
      await expectLabReady(page);
      const trigger = page.getByRole("button", { name: "Inline architecture", exact: true });
      await trigger.scrollIntoViewIfNeeded();
      await trigger.click();
      const dialog = page.getByRole("dialog", { name: "Inline architecture", exact: true });
      const image = dialog.getByRole("img");
      await image.evaluate((node: HTMLImageElement) => node.decode());
      const fitted = (await image.boundingBox())!.width;
      const zoom = dialog.getByRole("button", { name: "Zoom in image", exact: true });
      await zoom.click();
      await expect.poll(async () => (await image.boundingBox())!.width).toBeGreaterThan(fitted);
      await zoom.press("ArrowRight");
      const next = page.getByRole("dialog", { name: "Inline detail", exact: true });
      await expect(next).toBeVisible();
      await expect(next.getByRole("button", { name: "Zoom in image", exact: true })).toBeFocused();
      const viewport = next.getByRole("region", { name: "Inline detail", exact: true });
      const detail = next.getByRole("img");
      await detail.evaluate((node: HTMLImageElement) => node.decode());
      await expect
        .poll(async () => {
          const frame = (await viewport.boundingBox())!;
          const bounds = (await detail.boundingBox())!;
          return bounds.width <= frame.width + 1 && bounds.height <= frame.height + 1;
        })
        .toBe(true);
      await viewport.focus();
      for (let index = 0; index < 40; index++) await viewport.press("+");
      await expect
        .poll(async () => (await detail.boundingBox())!.height)
        .toBeGreaterThan((await viewport.boundingBox())!.height);
      const top = (await detail.boundingBox())!.y;
      await viewport.press("ArrowDown");
      await expect.poll(async () => (await detail.boundingBox())!.y).not.toBe(top);
      await expect(next).toBeVisible();
      await expect(viewport).toBeFocused();
      await next.getByRole("button", { name: "Fit image", exact: true }).click();
      await expect
        .poll(async () => (await detail.boundingBox())!.height)
        .toBeLessThanOrEqual((await viewport.boundingBox())!.height + 1);
      await page.keyboard.press("Escape");
      await expect(next).toHaveCount(0);
      await expect(trigger).toBeFocused();
    },
  );

  test.extend({ browserName })(
    `${browserName} keeps Mermaid text selectable in its camera`,
    async ({ page }) => {
      await page.goto("/lab/fixture.html?fixture=agent&state=long-content&locale=en&motion=off");
      await expectLabReady(page);
      const trigger = page.getByRole("button", { name: "Enlarge diagram", exact: true });
      await page.locator('[data-slot="mermaid-stage"]').hover();
      await trigger.click();
      const dialog = page.getByRole("dialog", { name: "Diagram", exact: true });
      const viewport = dialog.getByRole("region", { name: "Diagram", exact: true });
      await viewport.focus();
      await expect(viewport).toBeFocused();
      const graphic = viewport.getByRole("img", { name: "Diagram", exact: true });
      const literal = await graphic.evaluate((node) => {
        const text = node.querySelector("text");
        if (!text?.textContent) throw new Error("expected selectable diagram text");
        const selection = node.ownerDocument.getSelection()!;
        selection.selectAllChildren(text);
        return selection.toString();
      });
      expect(literal.length).toBeGreaterThan(0);
      await dialog.getByRole("button", { name: "Zoom in diagram", exact: true }).click();
      await expect.poll(() => page.evaluate(() => getSelection()?.toString())).toBe(literal);
      await expect(viewport.getByRole("img", { name: "Diagram" })).toBeVisible();
      await dialog.getByRole("button", { name: "Pan diagram", exact: true }).click();
      await expect(dialog.getByRole("button", { name: "Pan diagram" })).toHaveAttribute(
        "aria-pressed",
        "true",
      );
      await page.keyboard.press("Escape");
      await expect(dialog).toHaveCount(0);
      await expect(trigger).toBeFocused();
    },
  );
}
