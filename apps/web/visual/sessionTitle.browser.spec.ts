import { expect, test } from "./test";

test("Session title editing keeps keyboard focus and cancels without a write", async ({ page }) => {
  await page.goto("/visual/?fixture=shell&state=populated&theme=light");
  await page.locator("html[data-visual-ready]").waitFor();
  const original = "Refine Runtime protocol";
  const more = () =>
    page.getByRole("button", { name: `More actions for ${original}`, exact: true }).first();
  const current = () => page.getByRole("button", { name: new RegExp(`^${original} —`) }).first();
  await current().hover();
  await more().click();
  await page.getByRole("menuitem", { name: "Rename", exact: true }).click();
  const field = page.getByRole("textbox", { name: "Session title", exact: true });
  await expect(field).toBeFocused();
  await expect(field).toHaveValue(original);
  await field.fill("A title that is still only a draft");
  await field.press("Escape");
  await expect(field).toHaveCount(0);
  await expect(current()).toBeFocused();
  await current().hover();
  await more().click();
  await page.getByRole("menuitem", { name: "Rename", exact: true }).click();
  await expect(field).toHaveValue(original);
  await expect(field).toBeFocused();
  await field.press("Enter");
  await expect(field).toHaveCount(0);
  await expect(current()).toBeFocused();
});
