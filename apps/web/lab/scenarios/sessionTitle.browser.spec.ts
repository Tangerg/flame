import { expect, test } from "./test";
import { expectLabReady } from "../readiness";

test("Session title editing keeps keyboard focus and cancels without a write", async ({
  page,
}, testInfo) => {
  await page.goto("/lab/fixture.html?fixture=shell&state=populated&theme=light");
  await expectLabReady(page);
  const original = "Refine Runtime protocol";
  const more = () =>
    page.getByRole("button", { name: `More actions for ${original}`, exact: true }).first();
  const current = () => page.getByRole("button", { name: new RegExp(`^${original} —`) }).first();
  const actionName = "flame.trajectory · Name this investigation";
  const dialog = page.getByRole("dialog", { name: actionName, exact: true });
  const field = dialog.getByRole("textbox", { name: "Session title", exact: true });
  await current().hover();
  await more().click();
  await page.getByRole("menuitem", { name: actionName, exact: true }).click();
  await expect(field).toBeFocused();
  await expect(field).toHaveValue(original);
  expect(await field.evaluate((node) => node.closest("button"))).toBeNull();
  await field.fill("A title that is still only a draft");
  await testInfo.attach("session-title-draft", {
    body: await page.screenshot({ animations: "disabled" }),
    contentType: "image/png",
  });
  await field.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(current()).toBeFocused();
  await current().hover();
  await more().click();
  await page.getByRole("menuitem", { name: actionName, exact: true }).click();
  await expect(field).toHaveValue(original);
  await expect(field).toBeFocused();
  await testInfo.attach("session-title-reopened", {
    body: await page.screenshot({ animations: "disabled" }),
    contentType: "image/png",
  });
  await dialog.getByRole("button", { name: "Close", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(current()).toBeFocused();
});
