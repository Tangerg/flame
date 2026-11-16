import { expect, test } from "./test";
import { expectLabReady } from "../readiness";

for (const browserName of ["chromium", "webkit"] as const) {
  test.extend({ browserName })(
    `${browserName} a close callback synchronously unmounts its owner once`,
    async ({ page }) => {
      await page.goto(
        "/lab/fixture.html?fixture=atoms&state=popup-close-unmount&locale=en&motion=off",
      );
      await expectLabReady(page);
      const trigger = page.getByRole("button", { name: "Popup to retire" });
      const panel = page.getByRole("dialog", { name: "Callback panel" });
      const field = panel.getByRole("textbox", { name: "Callback value" });
      const restore = page.getByRole("button", { name: "Restore owner" });
      const closes = page.getByRole("status", { name: "Close deliveries" });
      await trigger.click();
      await field.fill("retired draft");

      await field.press("Escape");

      await expect(trigger).toHaveCount(0);
      await expect(panel).toHaveCount(0);
      await expect(closes).toHaveText("1");
      await expect(restore).toBeFocused();
      await restore.click();
      await trigger.click();
      await expect(field).toHaveValue("");
      await field.press("Escape");
      await expect(closes).toHaveText("2");
    },
  );

  for (const lifetime of ["activity", "suspense"] as const) {
    test.extend({ browserName })(
      `${browserName} an opening cannot survive ${lifetime} reconnection inside its callback`,
      async ({ page }) => {
        await page.goto(
          `/lab/fixture.html?fixture=atoms&state=popup-opening-${lifetime}&locale=en&motion=off`,
        );
        await expectLabReady(page);
        const trigger = page.getByRole("button", { name: "Popup to retire" });
        const panel = page.getByRole("dialog", { name: "Callback panel" });
        const original = await trigger.elementHandle();

        await trigger.click();

        await expect(panel).toHaveCount(0);
        await expect(page.getByRole("button", { name: "Restore owner" })).toBeFocused();
        expect(await original?.evaluate((node) => node.isConnected)).toBe(true);
        expect(await trigger.evaluate((node, previous) => node === previous, original)).toBe(true);
        await trigger.click();
        await expect(panel).toBeVisible();
        await panel.getByRole("textbox", { name: "Callback value" }).press("Escape");
        await expect(panel).toHaveCount(0);
      },
    );
  }
}
