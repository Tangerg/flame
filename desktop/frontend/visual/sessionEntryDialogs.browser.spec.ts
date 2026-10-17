import { expect, test } from "./test";

test("Add project opens the production directory dialog and cancellation returns focus", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=shell&state=populated&theme=light");
  await page.locator("html[data-visual-ready]").waitFor();
  const trigger = page.getByRole("button", { name: "Add project", exact: true });
  await trigger.click();
  const dialog = page.getByRole("dialog", { name: "New session", exact: true });
  const field = dialog.getByRole("textbox", {
    name: "Runtime workspace directory",
    exact: true,
  });
  await expect(field).toBeFocused();
  await expect(dialog.getByRole("button", { name: "Create session", exact: true })).toBeDisabled();
  await dialog.getByRole("button", { name: "Browse this computer", exact: true }).click();
  await expect(field).toHaveValue("/Users/visual/selected-project");
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(trigger).toBeFocused();
});

test("the investigation action captures its Session through the production row menu", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=shell&state=populated&theme=dark");
  await page.locator("html[data-visual-ready]").waitFor();
  const row = page.getByRole("button", { name: /^Refine Runtime protocol/ });
  await row.click({ button: "right" });
  await page
    .getByRole("menuitem", { name: "flame.trajectory · Name this investigation", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "flame.trajectory · Name this investigation",
    exact: true,
  });
  await expect(dialog.getByRole("textbox", { name: "Session title", exact: true })).toHaveValue(
    "Refine Runtime protocol",
  );
  await expect(dialog.locator("form")).toHaveCount(1);
  await dialog.getByRole("button", { name: "Captured session", exact: true }).click();
  await expect(dialog.getByText("visual-shell-active", { exact: true })).toBeVisible();
  await dialog.getByRole("button", { name: "Close", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(row).toBeFocused();
});
