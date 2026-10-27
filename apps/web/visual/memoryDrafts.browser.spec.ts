import { expect, test } from "./test";
for (const theme of ["light", "dark"]) {
  for (const width of [1120, 320]) {
    test(`memory forms preserve their draft and return to their opener ${theme} ${width}`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 900 });
      const query = new URLSearchParams({
        fixture: "workspace",
        state: width === 1120 ? "dock-agent-memory" : "full-view",
        "font-size": "18",
        theme,
      });
      if (width === 320) query.set("full-view", "package:visual:memory");
      await page.goto(`/visual/?${query}`);
      await page.locator("html[data-visual-ready]").waitFor();
      if (width === 320)
        await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
      const panel = page.getByRole("region", { name: "Agent memory", exact: true });
      const record = panel.locator('[data-memory-id="mem_00000000000000000000000000000001"]');
      const edit = record.getByRole("button", { name: "Edit", exact: true });
      await expect(edit).toBeEnabled();
      await record.getByRole("button", { name: /Record details/, expanded: false }).click();
      await expect(record.getByText("Day", { exact: true })).toHaveCount(0);
      await expect(record.getByText("Source Session", { exact: true })).toBeVisible();
      const add = panel.getByRole("button", { name: "Add memory", exact: true });
      await add.click();
      const editor = panel.getByRole("region", { name: "Add memory", exact: true });
      const content = editor.getByRole("textbox", { name: "Memory content", exact: true });
      await expect(content).toBeFocused();
      await content.fill("Keep this unsaved memory draft.\n\nPreserve these authored lines.");
      await expect(edit).toBeDisabled();
      await expect(panel.getByRole("radio", { name: "User", exact: true })).toBeDisabled();
      await expect(panel.getByRole("button", { name: "Approve", exact: true })).toBeEnabled();
      const actions = record.getByRole("button", { name: /^More actions for / });
      await actions.click();
      await expect(page.getByRole("menuitem", { name: "Delete", exact: true })).toBeDisabled();
      await expect(page.getByRole("menuitem", { name: "Unpin", exact: true })).toBeEnabled();
      await page.keyboard.press("Escape");
      await expect(content).toHaveValue(
        "Keep this unsaved memory draft.\n\nPreserve these authored lines.",
      );
      expect(await editor.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
      await editor.getByRole("button", { name: "Cancel", exact: true }).click();
      await expect(editor).toHaveCount(0);
      await expect(add).toBeFocused();
      await edit.click();
      const existing = panel.getByRole("region", { name: "Edit memory", exact: true });
      await expect(existing.getByRole("textbox", { name: "Memory content" })).toBeFocused();
      await existing.getByRole("button", { name: "Cancel", exact: true }).click();
      await expect(edit).toBeFocused();
      await edit.click();
      await expect(existing.getByRole("textbox", { name: "Memory content" })).toBeFocused();
      const search = panel.getByRole("searchbox", { name: "Search this page", exact: true });
      const filterQuery = "no-record-matches-this-query";
      await search.fill(filterQuery);
      await expect(record).toHaveCount(0);
      await existing.getByRole("button", { name: "Cancel", exact: true }).click();
      await expect(existing).toHaveCount(0);
      await expect(search).toBeFocused();
      await expect(search).toHaveValue(filterQuery);
      await search.fill("");
      await expect(edit).toBeEnabled();
      await actions.click();
      await page.getByRole("menuitem", { name: "Delete", exact: true }).click();
      const confirmation = panel.getByRole("region", {
        name: "Confirm memory deletion",
        exact: true,
      });
      await expect(confirmation).toContainText(
        "The compaction cutpoint is chosen by the Runtime, never by the client.",
      );
      const cancel = confirmation.getByRole("button", { name: "Cancel", exact: true });
      await expect(cancel).toBeFocused();
      await expect(add).toBeDisabled();
      await expect(edit).toBeDisabled();
      await expect(panel.getByRole("radio", { name: "User", exact: true })).toBeDisabled();
      expect(await confirmation.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
      await cancel.click();
      await expect(confirmation).toHaveCount(0);
      await expect(actions).toBeFocused();
      expect(await panel.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
    });
  }
}
