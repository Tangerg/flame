import { expect, test } from "./test";

for (const browserName of ["chromium", "webkit"] as const) {
  test.extend({ browserName })(
    `${browserName} retains a native command through reflow and pointer activation`,
    async ({ page }) => {
      await page.goto("/lab/native-reading.html");
      const command = page.getByRole("button", { name: "Record native command", exact: true });
      await command.focus();
      await page
        .getByRole("button", { name: "Grow panel content", exact: true })
        .evaluate((node: HTMLButtonElement) => node.click());
      await expect(command).toBeInViewport({ ratio: 1 });
      await command.click();
      await expect(page.getByText("Recorded native commands: 1", { exact: true })).toBeVisible();
      await expect(command).toBeFocused();
    },
  );
  test.extend({ browserName })(
    `${browserName} bounds native content sizing and keeps rows usable without enhancement`,
    async ({ page }, testInfo) => {
      await page.goto("/lab/native-reading.html");
      await page.getByRole("button", { name: "Open native dialog", exact: true }).click();
      const field = page.getByRole("textbox", { name: "Autosized note", exact: true });
      const value = Array.from(
        { length: 200 },
        (_, index) => `Native line ${index} 中文 9007199254740993`,
      ).join("\n");
      await field.fill(value);
      const capability = await field.evaluate((node: HTMLTextAreaElement) => ({
        userAgent: navigator.userAgent,
        fieldSizing: CSS.supports("field-sizing", "content"),
        height: node.getBoundingClientRect().height,
        maxHeight: parseFloat(getComputedStyle(node).maxHeight),
        clientHeight: node.clientHeight,
        scrollHeight: node.scrollHeight,
      }));
      await testInfo.attach("native-fields.json", {
        body: JSON.stringify(capability, null, 2),
        contentType: "application/json",
      });
      expect(capability.height).toBeLessThanOrEqual(capability.maxHeight + 1);
      expect(capability.scrollHeight).toBeGreaterThan(capability.clientHeight);
      await field.evaluate((node: HTMLTextAreaElement) => {
        node.style.fieldSizing = "fixed";
        node.style.fontSize = "24px";
      });
      await page.setViewportSize({ width: 820, height: 560 });
      await expect(field).toHaveValue(value);
      await expect(field).toBeInViewport({ ratio: 1 });
      await field.press("ControlOrMeta+End");
      await field.press("Enter");
      await field.press("N");
      await expect(field).toHaveValue(value + "\nN");
    },
  );
  test.extend({ browserName })(
    `${browserName} keeps native panel focus and selection through upper reflow`,
    async ({ page }) => {
      await page.goto("/lab/native-reading.html");
      const field = page.getByRole("textbox", { name: "Panel note", exact: true });
      await field.fill("Native draft 保留 9007199254740993");
      await field.focus();
      await field.evaluate((node: HTMLTextAreaElement) =>
        node.setSelectionRange(7, 12, "backward"),
      );
      await page
        .getByRole("button", { name: "Grow panel content", exact: true })
        .evaluate((node: HTMLButtonElement) => node.click());
      await expect(page.getByText("Captured content before the field.", { exact: true })).toHaveCSS(
        "height",
        "1100px",
      );
      await expect(field).toBeFocused();
      await expect(field).toBeInViewport({ ratio: 1 });
      expect(
        await field.evaluate((node: HTMLTextAreaElement) => [
          node.selectionStart,
          node.selectionEnd,
          node.selectionDirection,
        ]),
      ).toEqual([7, 12, "backward"]);
      await expect(field).toHaveValue("Native draft 保留 9007199254740993");
      await page.setViewportSize({ width: 860, height: 520 });
      await expect(field).toBeFocused();
      await expect(field).toBeInViewport({ ratio: 1 });
    },
  );
  test.extend({ browserName })(
    `${browserName} leaves deliberate native panel scrolling with the reader`,
    async ({ page }) => {
      await page.goto("/lab/native-reading.html");
      const field = page.getByRole("textbox", { name: "Panel note", exact: true });
      await field.focus();
      await page
        .getByRole("button", { name: "Grow panel content", exact: true })
        .evaluate((node: HTMLButtonElement) => node.click());
      const viewport = page.locator("[data-native-panel]");
      await viewport.hover();
      await page.mouse.wheel(0, -1200);
      await expect(field).not.toBeInViewport();
      const top = await viewport.evaluate((node) => node.scrollTop);
      await page.setViewportSize({ width: 860, height: 600 });
      await expect(field).toBeFocused();
      await expect(field).not.toBeInViewport();
      expect(
        Math.abs((await viewport.evaluate((node) => node.scrollTop)) - top),
      ).toBeLessThanOrEqual(2);
    },
  );
  test.extend({ browserName })(
    `${browserName} preserves native dialog fields and returns focus through Base UI`,
    async ({ page }) => {
      await page.goto("/lab/native-reading.html");
      const trigger = page.getByRole("button", { name: "Open native dialog", exact: true });
      await trigger.click();
      const field = page.getByRole("textbox", { name: "Dialog note", exact: true });
      await field.fill("Dialog draft 保留");
      await field.focus();
      await field.evaluate((node: HTMLTextAreaElement) => node.setSelectionRange(3, 8));
      await page
        .locator("nav button")
        .filter({ hasText: "Grow dialog content" })
        .evaluate((node: HTMLButtonElement) => node.click());
      await expect(field).toBeFocused();
      await expect(field).toBeInViewport({ ratio: 1 });
      await expect(field).toHaveValue("Dialog draft 保留");
      expect(
        await field.evaluate((node: HTMLTextAreaElement) => [
          node.selectionStart,
          node.selectionEnd,
        ]),
      ).toEqual([3, 8]);
      await page.setViewportSize({ width: 820, height: 560 });
      await expect(field).toBeInViewport({ ratio: 1 });
      await page.keyboard.press("Escape");
      await expect(page.getByRole("dialog")).toHaveCount(0);
      await expect(trigger).toBeFocused();
    },
  );
  test.extend({ browserName })(
    `${browserName} renders a bounded code body below its normal-flow caption`,
    async ({ page }) => {
      await page.goto("/lab/native-reading.html");
      const reader = page.getByRole("region", { name: "Native code reader", exact: true });
      await reader.scrollIntoViewIfNeeded();
      await expect(reader.locator("diffs-container").locator("[data-line]").first()).toBeVisible();
      const dimensions = await reader.evaluate((node) => ({
        height: node.getBoundingClientRect().height,
        scroll: node.scrollHeight,
        width: node.clientWidth,
      }));
      expect(dimensions.height).toBeGreaterThan(60);
      expect(dimensions.height).toBeLessThanOrEqual(450);
      expect(dimensions.scroll).toBeGreaterThan(dimensions.height);
    },
  );
}
