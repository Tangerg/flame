import { expect, test } from "./test";
for (const width of [1120, 320]) {
  for (const action of ["Edit", "Delete"] as const) {
    test(`schedule ${action.toLowerCase()} returns to search when a remote update retires its opener ${width}`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: width === 1120 ? 720 : 900 });
      await page.goto(
        `/visual/?fixture=workspace&state=${width === 1120 ? "dock-schedules" : "full-view&full-view=package:visual:schedules"}&theme=${width === 1120 ? "dark" : "light"}&font-size=18`,
      );
      await page.locator("html[data-visual-ready]").waitFor();
      if (width === 320) {
        await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
      }
      const panel = page
        .getByRole("region", {
          name: "Schedules",
          exact: true,
          includeHidden: true,
        })
        .and(page.locator('[data-slot="panel"]'));
      const search = panel.getByRole("searchbox", {
        name: "Search this page",
        exact: true,
        includeHidden: true,
      });
      const opener = panel.getByRole("button", {
        name: "More actions for Nightly dependency audit",
        exact: true,
        includeHidden: true,
      });
      await opener.press("Enter");
      await page.getByRole("menuitem", { name: action, exact: true }).press("Enter");
      const form = page.getByRole(action === "Edit" ? "dialog" : "alertdialog", {
        name:
          action === "Edit" ? "Edit Nightly dependency audit" : "Delete Nightly dependency audit?",
        exact: true,
      });
      if (action === "Edit") {
        await expect(form.getByRole("textbox", { name: "Title", exact: true })).toBeFocused();
        await form
          .getByRole("textbox", { name: "Title", exact: true })
          .fill("Retain remote edit draft");
      } else {
        await expect(form.getByRole("button", { name: "Cancel", exact: true })).toBeFocused();
      }
      await expect(
        page.getByRole("searchbox", { name: "Search this page", exact: true }),
      ).toHaveCount(0);
      await page.keyboard.press("Tab");
      expect(await form.evaluate((node) => node.contains(document.activeElement))).toBe(true);
      await page.evaluate(async () => {
        const modulePath = "/visual/createVisualRuntimeTransport.ts";
        const { VISUAL_REMOVE_SCHEDULE_EVENT }: typeof import("./createVisualRuntimeTransport") =
          await import(modulePath);
        window.dispatchEvent(
          new CustomEvent(VISUAL_REMOVE_SCHEDULE_EVENT, { detail: "sch_nightly" }),
        );
      });
      await expect(opener).toHaveCount(0);
      if (action === "Edit")
        await expect(form.getByRole("textbox", { name: "Title", exact: true })).toHaveValue(
          "Retain remote edit draft",
        );
      await page.keyboard.press("Escape");
      await expect(form).toHaveCount(0);
      await expect(search).toBeFocused();
      await expect(search).toHaveValue("");
      await panel
        .getByRole("button", { name: "More actions for Weekly maintenance notes", exact: true })
        .press("Enter");
      await page.getByRole("menuitem", { name: "Edit", exact: true }).press("Enter");
      const next = page.getByRole("dialog", { name: "Edit Weekly maintenance notes", exact: true });
      await expect(next.getByRole("textbox", { name: "Title", exact: true })).toHaveValue(
        "Weekly maintenance notes",
      );
      await next.getByRole("button", { name: "Cancel", exact: true }).press("Enter");
      expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
    });
  }

  test(`a new schedule draft keeps its owner until cancellation ${width}`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      "/visual/?fixture=workspace&state=full-view&full-view=package:visual:schedules&theme=light",
    );
    await page.locator("html[data-visual-ready]").waitFor();
    const panel = page
      .getByRole("region", { name: "Schedules", exact: true, includeHidden: true })
      .and(page.locator('[data-slot="panel"]'));
    const add = panel.getByRole("button", {
      name: "Add schedule",
      exact: true,
      includeHidden: true,
    });
    const templates = panel.getByRole("button", {
      name: "Schedule templates",
      exact: true,
      includeHidden: true,
    });
    const recordActions = panel.getByRole("button", {
      name: /^More actions for /,
      includeHidden: true,
    });
    await expect(
      panel.getByRole("heading", { name: "Nightly dependency audit", exact: true }),
    ).toBeVisible();
    await add.press("Enter");
    const editor = page.getByRole("dialog", { name: "New schedule", exact: true });
    const title = editor.getByRole("textbox", { name: "Title", exact: true });
    await expect(title).toBeFocused();
    await title.fill("Keep my new draft");
    await expect(add).toBeDisabled();
    await expect(templates).toBeDisabled();
    for (const trigger of await recordActions.all()) await expect(trigger).toBeDisabled();
    for (const run of await panel
      .getByRole("button", { name: "Run now", exact: true, includeHidden: true })
      .all())
      await expect(run).toBeDisabled();
    await expect(title).toHaveValue("Keep my new draft");
    await editor.getByRole("button", { name: "Cancel", exact: true }).press("Enter");
    await expect(editor).toHaveCount(0);
    await expect(add).toBeFocused();
    await expect(templates).toBeEnabled();
    for (const trigger of await recordActions.all()) await expect(trigger).toBeEnabled();
    await templates.press("Enter");
    await expect(
      page.getByRole("menuitem", { name: "Hourly progress check", exact: true }),
    ).toBeFocused();
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
    const panel = page
      .getByRole("region", { name: "Schedules", exact: true, includeHidden: true })
      .and(page.locator('[data-slot="panel"]'));
    const add = panel.getByRole("button", {
      name: "Add schedule",
      exact: true,
      includeHidden: true,
    });
    const templates = panel.getByRole("button", {
      name: "Schedule templates",
      exact: true,
      includeHidden: true,
    });
    const nightly = panel.getByRole("button", {
      name: "More actions for Nightly dependency audit",
      exact: true,
      includeHidden: true,
    });
    const weekly = panel.getByRole("button", {
      name: "More actions for Weekly maintenance notes",
      exact: true,
      includeHidden: true,
    });
    await nightly.press("Enter");
    await page.getByRole("menuitem", { name: "Edit", exact: true }).press("Enter");
    const editor = page.getByRole("dialog", { name: "Edit Nightly dependency audit", exact: true });
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
    const confirmation = page.getByRole("alertdialog", {
      name: "Delete Nightly dependency audit?",
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
    const weeklyEditor = page.getByRole("dialog", {
      name: "Edit Weekly maintenance notes",
      exact: true,
    });
    await expect(weeklyEditor.getByRole("textbox", { name: "Title", exact: true })).toHaveValue(
      "Weekly maintenance notes",
    );
    await weeklyEditor.getByRole("button", { name: "Cancel", exact: true }).press("Enter");
    await expect(weekly).toBeFocused();
    await expect(
      panel.getByRole("heading", { name: "Nightly dependency audit", exact: true }),
    ).toBeVisible();
  });
}
