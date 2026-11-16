import { expect, test } from "./test";
import { expectLabReady } from "../readiness";

for (const browserName of ["chromium", "webkit"] as const) {
  for (const lifetime of ["Activity", "Suspense"] as const) {
    for (const control of ["Controlled", "Uncontrolled"] as const) {
      test.extend({ browserName })(
        `${browserName} ${control} popup retires through ${lifetime} disconnection`,
        async ({ page }) => {
          const state = lifetime === "Activity" ? "popup-retirement" : "popup-retirement-suspense";
          await page.goto(`/lab/fixture.html?fixture=atoms&state=${state}&locale=en&motion=off`);
          await expectLabReady(page);
          const trigger = page.getByRole("button", { name: `${control} popup`, exact: true });
          const panel = page.getByRole("dialog", { name: `${control} panel` });
          const field = panel.getByRole("textbox", { name: `${control} value` });
          await trigger.click();
          await field.fill("retired draft");

          await panel.getByRole("button", { name: "Hide popup owner" }).click();
          await expect(panel).toHaveCount(0);
          const show = page.getByRole("button", { name: "Show popup owner" });
          await show.focus();
          await show.press("Enter");

          await expect(trigger).toBeVisible();
          await expect(panel).toHaveCount(0);
          await expect(show).toBeFocused();
          await trigger.click();
          await expect(field).toHaveValue("");
        },
      );
    }
  }
}
