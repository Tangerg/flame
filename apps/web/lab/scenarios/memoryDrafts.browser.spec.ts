import { expect, test } from "./test";
import { expectLabReady } from "../readiness";
import { en as memoryEn } from "../../../../plugins/memory/component/locales/en";
import { zh as memoryZh } from "../../../../plugins/memory/component/locales/zh";
import { en } from "../../src/lib/i18n/locales/en";
import zh from "../../../../plugins/locale-zh/package/io.github.tangerg.flame/messages.json" with { type: "json" };
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
      await page.goto(`/lab/fixture.html?${query}`);
      await expectLabReady(page);
      if (width === 320)
        await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
      const panel = page
        .getByRole("region", { name: "Agent memory", exact: true })
        .and(page.locator('[data-slot="panel"]'));
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

const webkit = test.extend({ browserName: "webkit" });
for (const profile of [
  {
    locale: "en",
    theme: "light",
    fontSize: 14,
    width: 1120,
    height: 720,
    memory: memoryEn,
    messages: en,
    draft: "Continue after checking User memory.",
    continuation: " Keep the captured scope.",
  },
  {
    locale: "zh",
    theme: "dark",
    fontSize: 18,
    width: 640,
    height: 400,
    memory: memoryZh,
    messages: zh as Record<string, string>,
    draft: "核对用户记忆后继续。",
    continuation: "保留记录的范围。",
  },
] as const) {
  const { locale, theme, fontSize, width, height, memory, messages, draft, continuation } = profile;
  webkit(
    `Memory scope, edit and conversation return stay coherent in ${locale}/${theme}`,
    async ({ page }, testInfo) => {
      const named = (key: string) => ({ name: memory[key]!, exact: true });
      const capture = async (name: string) =>
        testInfo.attach(`memory-${name}-${locale}`, {
          body: await page.screenshot({ animations: "disabled" }),
          contentType: "image/png",
        });
      await page.setViewportSize({ width, height });
      await page.goto(
        `/lab/fixture.html?fixture=workspace&state=dock-agent-memory&locale=${locale}&theme=${theme}&font-size=${fontSize}`,
      );
      await expectLabReady(page);
      await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
      const composer = page.getByRole("textbox", {
        name: messages["composer.input.label"],
        exact: true,
      });
      await composer.fill(draft);
      await page
        .getByRole("button", {
          name: messages[width === 640 ? "dock.action.openFull" : "dock.action.expand"],
          exact: true,
        })
        .click();
      const panel = page
        .getByRole("region", named("panel.title"))
        .and(page.locator('[data-slot="panel"]'));
      const projectRecords = [
        "mem_00000000000000000000000000000001",
        "mem_00000000000000000000000000000002",
      ];
      for (const id of projectRecords)
        await expect(panel.locator(`[data-memory-id="${id}"]`)).toBeVisible();
      const user = panel.getByRole("radio", named("segment.user"));
      const project = panel.getByRole("radio", named("segment.project"));
      await user.click();
      await expect(user).toBeChecked();
      const record = panel.locator("[data-memory-id]").first();
      await record
        .getByRole("button", { name: new RegExp(`^${memory["record.details"]}`) })
        .click();
      await record.locator("dl").scrollIntoViewIfNeeded();
      await capture("user-identity");
      await expect(record.getByText(memory["scope.user"]!, { exact: true })).toBeVisible();
      await expect(record.getByText(memory["origin.user"]!, { exact: true })).toBeVisible();
      for (const id of projectRecords)
        await expect(panel.locator(`[data-memory-id="${id}"]`)).toHaveCount(0);
      const userID = await record.getAttribute("data-memory-id");
      const canonical = await record.getByRole("paragraph").first().innerText();
      const edit = record.getByRole("button", named("edit.action"));
      await edit.click();
      const form = panel.getByRole("region", named("edit"));
      const content = form.getByRole("textbox", named("content.label"));
      await expect(content).toBeFocused();
      await expect(content).toHaveValue(canonical);
      await expect(form.getByText(memory["scope.user"]!, { exact: true })).toBeVisible();
      await expect(project).toBeDisabled();
      const localDraft = `${draft}\n\n/Users/visual/${"retained-memory-".repeat(8)}review.md`;
      await content.fill(localDraft);
      await page.setViewportSize({ width: 320, height: 600 });
      await expect(content).toHaveValue(localDraft);
      await page.keyboard.press("Tab");
      const cancel = form.getByRole("button", named("cancel"));
      await expect(cancel).toBeFocused();
      const cancelBox = (await cancel.boundingBox())!;
      expect(cancelBox.x).toBeGreaterThanOrEqual(0);
      expect(cancelBox.y).toBeGreaterThanOrEqual(0);
      expect(cancelBox.x + cancelBox.width).toBeLessThanOrEqual(320);
      expect(cancelBox.y + cancelBox.height).toBeLessThanOrEqual(600);
      expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
      await capture("user-edit-320");
      await page.setViewportSize({ width, height });
      await expect(content).toHaveValue(localDraft);
      await expect(cancel).toBeFocused();
      await page.keyboard.press("Enter");
      await expect(form).toHaveCount(0);
      await expect(edit).toBeFocused();
      await expect(record.getByRole("paragraph").first()).toHaveText(canonical);
      await user.press("ArrowLeft");
      await expect(project).toBeChecked();
      for (const id of projectRecords)
        await expect(panel.locator(`[data-memory-id="${id}"]`)).toBeVisible();
      await expect(panel.locator(`[data-memory-id="${userID}"]`)).toHaveCount(0);
      await capture("project-return");
      await page.keyboard.press("Escape");
      await expect(composer).toBeVisible();
      await expect(composer).toHaveValue(draft);
      await composer.focus();
      await page.keyboard.press("ControlOrMeta+A");
      await page.keyboard.press("ArrowRight");
      await page.keyboard.type(continuation);
      await expect(composer).toHaveValue(draft + continuation);
      await capture("continued-draft");
    },
  );
}
