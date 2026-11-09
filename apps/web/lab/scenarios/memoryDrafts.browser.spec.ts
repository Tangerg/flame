import { expect, test } from "./test";
import { expectLabReady } from "../readiness";
import { en as memoryEn } from "../../../../plugins/memory/component/locales/en";
import { zh as memoryZh } from "../../../../plugins/memory/component/locales/zh";
import { en } from "../../src/lib/i18n/locales/en";
import zh from "../../../../plugins/locale-zh/package/io.github.tangerg.flame/messages.json" with { type: "json" };
import { VISUAL_LONG_MEMORY_PAGE, VISUAL_LONG_USER_MEMORY_PAGE } from "./componentSnapshots";
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

for (const engine of ["chromium", "webkit"] as const) {
  const memoryTest = test.extend({ browserName: engine });
  for (const profile of [
    { locale: "en", theme: "light", fontSize: 14, memory: memoryEn, messages: en },
    { locale: "zh", theme: "dark", fontSize: 18, memory: memoryZh, messages: zh },
  ] as const) {
    memoryTest(
      `long Memory reading, local drafts and scope return ${engine}/${profile.locale}`,
      async ({ page }, testInfo) => {
        const named = (key: string) => ({ name: profile.memory[key]!, exact: true });
        const capture = async (label: string) =>
          testInfo.attach(`long-memory-${label}`, {
            body: await page.screenshot({ animations: "disabled", caret: "hide" }),
            contentType: "image/png",
          });
        const draft = `${profile.locale} recorded Memory return`;
        const continuation = " · continued";
        await page.setViewportSize({ width: 1120, height: 720 });
        await page.goto(
          `/lab/fixture.html?fixture=workspace&state=dock-agent-memory-recorded&locale=${profile.locale}&theme=${profile.theme}&font-size=${profile.fontSize}`,
        );
        await expectLabReady(page);
        await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
        const composer = page.getByRole("textbox", {
          name: profile.messages["composer.input.label"],
          exact: true,
        });
        await composer.fill(draft);
        await page
          .getByRole("button", { name: profile.messages["dock.action.expand"], exact: true })
          .press("Enter");
        const panel = page
          .getByRole("region", named("panel.title"))
          .and(page.locator('[data-slot="panel"]'));
        const rows = panel.locator("[data-memory-id]");
        await expect(rows).toHaveCount(VISUAL_LONG_MEMORY_PAGE.Rows.length);
        const search = panel.getByRole("searchbox", named("search.label"));
        const project = panel.getByRole("radio", named("segment.project"));
        const user = panel.getByRole("radio", named("segment.user"));
        const pathRow = VISUAL_LONG_MEMORY_PAGE.Rows.find((row) =>
          row.Content.includes("recorded-compaction-boundary-"),
        )!;
        await search.fill("recorded-compaction-boundary-");
        await expect(rows).toHaveCount(1);
        const path = rows.getByRole("paragraph").first();
        await expect(path).toHaveText(pathRow.Content);
        const geometry = [];
        for (const viewport of [
          { width: 1120, height: 720 },
          { width: 320, height: 600 },
        ]) {
          await page.setViewportSize(viewport);
          await path.scrollIntoViewIfNeeded();
          const reading = await path.evaluate((element) => {
            const bounds = element.getBoundingClientRect();
            const range = document.createRange();
            range.selectNodeContents(element);
            return {
              whiteSpace: getComputedStyle(element).whiteSpace,
              left: bounds.left,
              right: bounds.right,
              lines: [...range.getClientRects()].map((rect) => ({
                left: rect.left,
                right: rect.right,
              })),
            };
          });
          expect(reading.whiteSpace).toBe("pre-wrap");
          expect(reading.lines.length).toBeGreaterThan(1);
          for (const line of reading.lines) {
            expect(line.left).toBeGreaterThanOrEqual(reading.left - 1);
            expect(line.right).toBeLessThanOrEqual(reading.right + 1);
          }
          expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(
            0,
          );
          geometry.push({ viewport, reading });
          await capture(`path-${viewport.width}`);
        }
        await testInfo.attach("long-memory-reading.json", {
          body: JSON.stringify(geometry),
          contentType: "application/json",
        });
        await page.setViewportSize({ width: 1120, height: 720 });
        await search.fill("no-recorded-memory-matches");
        await expect(rows).toHaveCount(0);
        const clear = panel.getByRole("button", named("catalog.empty.filtered.action"));
        await clear.press("Enter");
        await expect(search).toBeFocused();
        await expect(search).toHaveValue("");
        await expect(rows).toHaveCount(VISUAL_LONG_MEMORY_PAGE.Rows.length);

        const projectRow = VISUAL_LONG_MEMORY_PAGE.Rows.find(
          (row) => row.Status === "active" && row.Content.includes("\n\n\n"),
        )!;
        const record = panel.locator(`[data-memory-id="${projectRow.ID}"]`);
        const edit = record.getByRole("button", named("edit.action"));
        await edit.press("Enter");
        const form = panel.getByRole("region", named("edit"));
        const content = form.getByRole("textbox", named("content.label"));
        await expect(content).toBeFocused();
        await expect(content).toHaveValue(projectRow.Content);
        const localDraft = `${projectRow.Content}\n\n${draft}`;
        await content.fill(localDraft);
        await page.setViewportSize({ width: 320, height: 600 });
        await expect(content).toHaveValue(localDraft);
        await expect(project).toBeDisabled();
        await page.keyboard.press("Tab");
        const cancel = form.getByRole("button", named("cancel"));
        await expect(cancel).toBeFocused();
        const cancelBounds = (await cancel.boundingBox())!;
        expect(cancelBounds.x).toBeGreaterThanOrEqual(0);
        expect(cancelBounds.y).toBeGreaterThanOrEqual(0);
        expect(cancelBounds.x + cancelBounds.width).toBeLessThanOrEqual(320);
        expect(cancelBounds.y + cancelBounds.height).toBeLessThanOrEqual(600);
        await capture("project-edit-320");
        const cancelReading = () =>
          cancel.evaluate((element) => {
            const bounds = element.getBoundingClientRect();
            const hit = document.elementFromPoint(
              bounds.left + bounds.width / 2,
              bounds.top + bounds.height / 2,
            );
            const centerHit = element.contains(hit);
            return {
              viewport: { width: innerWidth, height: innerHeight },
              bounds: {
                top: bounds.top,
                bottom: bounds.bottom,
                left: bounds.left,
                right: bounds.right,
              },
              focused: document.activeElement === element,
              centerHit,
              complete:
                bounds.left >= 0 &&
                bounds.top >= 0 &&
                bounds.right <= innerWidth &&
                bounds.bottom <= innerHeight &&
                centerHit,
            };
          });
        for (const viewport of [
          { width: 560, height: 364 },
          { width: 720, height: 450 },
        ]) {
          await page.setViewportSize(viewport);
          await expect(cancel).toBeFocused();
          const label = `project-cancel-${viewport.width}-${viewport.height}`;
          try {
            await expect.poll(async () => (await cancelReading()).complete).toBe(true);
            await expect(content).toHaveValue(localDraft);
          } finally {
            await testInfo.attach(`${label}.json`, {
              body: JSON.stringify(await cancelReading()),
              contentType: "application/json",
            });
            await capture(label);
          }
        }
        await cancel.press("Enter");
        await expect(form).toHaveCount(0);
        await expect(edit).toBeFocused();
        await expect(record.getByRole("paragraph").first()).toHaveText(projectRow.Content);

        await page.setViewportSize({ width: 1120, height: 720 });
        await project.press("ArrowRight");
        await expect(user).toBeChecked();
        await expect(rows).toHaveCount(VISUAL_LONG_USER_MEMORY_PAGE.Rows.length);
        await expect(panel.locator(`[data-memory-id="${projectRow.ID}"]`)).toHaveCount(0);
        const userRow = VISUAL_LONG_USER_MEMORY_PAGE.Rows[0]!;
        const userRecord = panel.locator(`[data-memory-id="${userRow.ID}"]`);
        await userRecord
          .getByRole("button", { name: new RegExp(`^${profile.memory["record.details"]}`) })
          .press("Enter");
        const facts = userRecord.locator("dl");
        await facts.scrollIntoViewIfNeeded();
        await expect(facts.getByText(profile.memory["scope.user"]!, { exact: true })).toBeVisible();
        await expect(
          facts.getByText(profile.memory["origin.user"]!, { exact: true }),
        ).toBeVisible();
        await expect(facts.getByText(profile.memory["fact.session"]!, { exact: true })).toHaveCount(
          0,
        );
        await capture("user-identity");
        await userRecord.getByRole("button", named("edit.action")).press("Enter");
        await expect(content).toHaveValue(userRow.Content);
        const userDraft = `${userRow.Content}\n\n${draft}`;
        await content.fill(userDraft);
        await search.fill("no-recorded-memory-matches");
        await expect(userRecord).toHaveCount(0);
        await expect(content).toHaveValue(userDraft);
        await cancel.press("Enter");
        await expect(form).toHaveCount(0);
        await expect(search).toBeFocused();
        await expect(search).toHaveValue("no-recorded-memory-matches");
        await clear.press("Enter");
        await expect(search).toBeFocused();
        await expect(userRecord.getByRole("paragraph").first()).toHaveText(userRow.Content);
        await user.press("ArrowLeft");
        await expect(project).toBeChecked();
        await expect(rows).toHaveCount(VISUAL_LONG_MEMORY_PAGE.Rows.length);
        await page.keyboard.press("Escape");
        await expect(composer).toBeVisible();
        await expect(composer).toHaveValue(draft);
        await composer.press("ControlOrMeta+A");
        await page.keyboard.press("ArrowRight");
        await page.keyboard.type(continuation);
        await expect(composer).toHaveValue(draft + continuation);
        await capture("continued-conversation");
      },
    );
  }
}
