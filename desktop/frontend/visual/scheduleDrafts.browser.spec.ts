import { expect, test } from "./test";

for (const width of [1120, 320]) {
  test(`a new schedule draft keeps its owner until cancellation ${width}`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      "/visual/?fixture=workspace&state=full-view&full-view=package:visual:schedules&theme=light",
    );
    await page.locator("html[data-visual-ready]").waitFor();
    const panel = page.getByRole("region", { name: "Schedules", exact: true });
    const add = panel.getByRole("button", { name: "Add schedule", exact: true });
    const templates = panel.getByRole("button", { name: "Schedule templates", exact: true });
    const recordActions = panel.getByRole("button", { name: /^More actions for / });
    await expect(
      panel.getByRole("heading", { name: "Nightly dependency audit", exact: true }),
    ).toBeVisible();
    await add.press("Enter");
    const editor = panel.getByRole("region", { name: "Schedule editor", exact: true });
    const title = editor.getByRole("textbox", { name: "Title", exact: true });
    await expect(title).toBeFocused();
    await title.fill("Keep my new draft");
    await expect(add).toBeDisabled();
    await expect(templates).toBeDisabled();
    for (const trigger of await recordActions.all()) await expect(trigger).toBeDisabled();
    for (const run of await panel.getByRole("button", { name: "Run now", exact: true }).all())
      await expect(run).toBeDisabled();
    await expect(title).toHaveValue("Keep my new draft");
    await editor.getByRole("button", { name: "Cancel", exact: true }).press("Enter");
    await expect(editor).toHaveCount(0);
    await expect(add).toBeFocused();
    await expect(templates).toBeEnabled();
    for (const trigger of await recordActions.all()) await expect(trigger).toBeEnabled();
    await templates.press("Enter");
    await page.keyboard.press("End");
    await expect(
      page.getByRole("menuitem", { name: "Weekly maintenance review", exact: true }),
    ).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(title).toBeFocused();
    await expect(title).toHaveValue("Weekly maintenance review");
    await editor.getByRole("button", { name: "Cancel", exact: true }).press("Enter");
    await expect(templates).toBeFocused();
    await expect(add).toBeEnabled();
  });

  test(`record edits and deletion confirmations keep their draft until cancellation ${width}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      "/visual/?fixture=workspace&state=full-view&full-view=package:visual:schedules&theme=light",
    );
    await page.locator("html[data-visual-ready]").waitFor();
    const panel = page.getByRole("region", { name: "Schedules", exact: true });
    const add = panel.getByRole("button", { name: "Add schedule", exact: true });
    const templates = panel.getByRole("button", { name: "Schedule templates", exact: true });
    const nightly = panel.getByRole("button", {
      name: "More actions for Nightly dependency audit",
      exact: true,
    });
    const weekly = panel.getByRole("button", {
      name: "More actions for Weekly maintenance notes",
      exact: true,
    });
    await nightly.press("Enter");
    await page.getByRole("menuitem", { name: "Edit", exact: true }).press("Enter");
    const editor = panel.getByRole("region", { name: "Schedule editor", exact: true });
    const title = editor.getByRole("textbox", { name: "Title", exact: true });
    await expect(title).toBeFocused();
    await expect(title).toHaveValue("Nightly dependency audit");
    await title.fill("Keep my existing edit");
    await expect(add).toBeDisabled();
    await expect(templates).toBeDisabled();
    await expect(nightly).toBeDisabled();
    await expect(weekly).toBeDisabled();
    await expect(title).toHaveValue("Keep my existing edit");
    await editor.getByRole("button", { name: "Cancel", exact: true }).press("Enter");
    await expect(editor).toHaveCount(0);
    await expect(nightly).toBeFocused();
    await nightly.press("Enter");
    await page.getByRole("menuitem", { name: "Delete", exact: true }).press("Enter");
    const confirmation = panel.getByRole("region", {
      name: "Confirm schedule deletion",
      exact: true,
    });
    const cancel = confirmation.getByRole("button", { name: "Cancel", exact: true });
    await expect(cancel).toBeFocused();
    await expect(add).toBeDisabled();
    await expect(templates).toBeDisabled();
    await expect(nightly).toBeDisabled();
    await expect(weekly).toBeDisabled();
    await cancel.press("Enter");
    await expect(confirmation).toHaveCount(0);
    await expect(nightly).toBeFocused();
    await expect(add).toBeEnabled();
    await expect(templates).toBeEnabled();
    await weekly.press("Enter");
    await page.getByRole("menuitem", { name: "Edit", exact: true }).press("Enter");
    await expect(title).toHaveValue("Weekly maintenance notes");
    await editor.getByRole("button", { name: "Cancel", exact: true }).press("Enter");
    await expect(weekly).toBeFocused();
    await expect(
      panel.getByRole("heading", { name: "Nightly dependency audit", exact: true }),
    ).toBeVisible();
  });
}
