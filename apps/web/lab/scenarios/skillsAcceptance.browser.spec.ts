import { expect, test, type Locator } from "./test";
import { VISUAL_SKILL_LIBRARY, VISUAL_SKILL_PROPOSALS } from "./skillSnapshots";
import { expectLabReady } from "../readiness";
for (const [decision, width, theme] of [
  ["approve", 1120, "dark"],
  ["reject", 320, "light"],
] as const) {
  test(`Skills ${decision} keeps a keyboard destination when accepted proposals leave the owner list`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width, height: 720 });
    await page.goto(
      `/lab/fixture.html?fixture=workspace&state=full-view&full-view=package:visual:skills&sidebar=collapsed&theme=${theme}&font-size=18&skills-commands=accept`,
    );
    await expectLabReady(page);
    const panel = page
      .getByRole("region", { name: "Skills", exact: true })
      .and(page.locator('[data-slot="panel"]'));
    await panel.getByRole("radio", { name: "Skill proposals", exact: true }).click();
    const search = panel.getByRole("searchbox", { name: "Search this catalog", exact: true });
    await expect(panel).toContainText("2 pending proposals");
    for (const [index, proposal] of VISUAL_SKILL_PROPOSALS.data.entries()) {
      const actions = panel.getByRole("group", {
        name: `Proposal actions for ${proposal.name}`,
        exact: true,
      });
      await actions
        .getByRole("button", { name: decision === "approve" ? "Approve" : "Reject", exact: true })
        .click();
      const confirmation = panel.getByRole("region", {
        name: "Confirm proposal decision",
        exact: true,
      });
      await expect(confirmation.getByRole("button", { name: "Cancel", exact: true })).toBeFocused();
      if (index === 0) {
        await testInfo.attach(`skills-${decision}-review`, {
          body: await page.screenshot({ animations: "disabled" }),
          contentType: "image/png",
        });
      }
      const submit = confirmation.getByRole("button", { name: `Confirm ${decision}`, exact: true });
      await submit.focus();
      await page.keyboard.press("Enter");
      await expect(confirmation).toHaveCount(0);
      await expect(
        panel.getByRole("status").filter({
          hasText: `${decision === "approve" ? "Approve" : "Reject"} ${proposal.name} completed.`,
        }),
      ).toBeVisible();
      await expect(actions).toHaveCount(0);
      const remaining = VISUAL_SKILL_PROPOSALS.data.length - index - 1;
      await expect(
        panel.getByRole("heading", {
          name: remaining === 1 ? "1 pending proposal" : `${remaining} pending proposals`,
          exact: true,
        }),
      ).toBeVisible();
      await expect.soft(search).toBeFocused();
    }
    await expect(panel.getByText("No Skills in this catalog.", { exact: true })).toBeVisible();
    await testInfo.attach(`skills-${decision}-empty`, {
      body: await page.screenshot({ animations: "disabled" }),
      contentType: "image/png",
    });
  });
}

async function visibleText(target: Locator) {
  return target.evaluate((element) => {
    const clippedFrame = (start: Element) => {
      const frame = { top: 0, left: 0, right: innerWidth, bottom: innerHeight };
      for (let parent: Element | null = start; parent; parent = parent.parentElement) {
        const style = getComputedStyle(parent);
        const bounds = parent.getBoundingClientRect();
        if (style.overflowY !== "visible") {
          frame.top = Math.max(frame.top, bounds.top + parent.clientTop);
          frame.bottom = Math.min(
            frame.bottom,
            bounds.top + parent.clientTop + parent.clientHeight,
          );
        }
        if (style.overflowX !== "visible") {
          frame.left = Math.max(frame.left, bounds.left + parent.clientLeft);
          frame.right = Math.min(frame.right, bounds.left + parent.clientLeft + parent.clientWidth);
        }
      }
      return frame;
    };
    const panel = element.closest('[data-slot="panel"]');
    if (!panel) throw new Error("skills panel is not rendered");
    const frame = clippedFrame(element);
    const readingFrame = clippedFrame(panel);
    const controls = panel.querySelector('[data-slot="panel-controls"]');
    if (controls && getComputedStyle(controls).position === "sticky") {
      frame.top = Math.max(frame.top, controls.getBoundingClientRect().bottom);
      readingFrame.top = Math.max(readingFrame.top, controls.getBoundingClientRect().bottom);
    }
    const range = document.createRange();
    range.selectNodeContents(element);
    const rects = [...range.getClientRects()]
      .filter((rect) => rect.width > 0 && rect.height > 0)
      .map(({ top, right, bottom, left }) => ({ top, right, bottom, left }));
    const unit = element.closest("article")?.getBoundingClientRect().toJSON();
    const typography = getComputedStyle(element);
    return {
      frame,
      readingFrame,
      rects,
      unit,
      margins: [typography.marginBlockStart, typography.marginBlockEnd],
      readable:
        rects.length > 0 &&
        rects.every((rect) => {
          const hit = document.elementFromPoint(
            (rect.left + rect.right) / 2,
            (rect.top + rect.bottom) / 2,
          );
          return (
            rect.top >= frame.top - 1 &&
            rect.bottom <= frame.bottom + 1 &&
            rect.left >= frame.left - 1 &&
            rect.right <= frame.right + 1 &&
            hit !== null &&
            (hit === element || element.contains(hit))
          );
        }),
    };
  });
}

for (const browserName of ["chromium", "webkit"] as const) {
  const skillsTest = test.extend({ browserName });
  for (const profile of [
    {
      locale: "en",
      theme: "light",
      fontSize: 14,
      panel: "Skills",
      library: "Personal Skill library",
      available: "Available Skills",
      search: "Search this catalog",
      clear: "Clear search",
      refresh: "Refresh",
      archive: "Archive",
      restore: "Restore",
      active: "Active",
      archived: "Archived",
      total: "2 user-managed Skills",
      material: "Open material full width",
      close: "Close",
      composer: "Message composer",
      conversation: "Conversation",
      draft: "  Keep the personal library reading draft.  ",
    },
    {
      locale: "zh",
      theme: "dark",
      fontSize: 18,
      panel: "技能",
      library: "个人技能库",
      available: "可用技能",
      search: "搜索此目录",
      clear: "清除搜索",
      refresh: "刷新",
      archive: "归档",
      restore: "恢复",
      active: "活跃",
      archived: "已归档",
      total: "2 个用户管理的技能",
      material: "全宽打开材料",
      close: "关闭",
      composer: "消息输入框",
      conversation: "对话",
      draft: "  读完个人技能库后继续原草稿。  ",
    },
  ] as const) {
    skillsTest(
      `Skills personal library keeps command feedback readable and returns to its draft in ${profile.locale} ${browserName}`,
      async ({ page }, testInfo) => {
        testInfo.setTimeout(60_000);
        const evidence: { name: string; geometry: Awaited<ReturnType<typeof visibleText>> }[] = [];
        const capture = async (name: string) =>
          testInfo.attach(`skills-personal-${profile.locale}-${browserName}-${name}`, {
            body: await page.screenshot({ animations: "disabled" }),
            contentType: "image/png",
          });
        const readable = async (target: Locator, name: string) => {
          await expect
            .poll(
              async () => {
                const geometry = await visibleText(target);
                evidence.push({ name, geometry });
                return geometry.readable;
              },
              { message: `${name} stays completely readable without another scroll or focus move` },
            )
            .toBe(true);
        };
        try {
          await page.setViewportSize({ width: 1120, height: 728 });
          await page.goto(
            `/lab/fixture.html?fixture=workspace&state=dock-skills&locale=${profile.locale}&theme=${profile.theme}&font-size=${profile.fontSize}`,
          );
          await expectLabReady(page);
          await page
            .getByRole("button", { name: "Collapse the workspace fixture sidebar", exact: true })
            .click();
          const composer = page.getByRole("textbox", { name: profile.composer, exact: true });
          await composer.fill(profile.draft);
          const panel = page
            .getByRole("region", { name: profile.panel, exact: true })
            .and(page.locator('[data-slot="panel"]'));
          const personal = panel.getByRole("radio", { name: profile.library, exact: true });
          await personal.click();
          const catalog = panel.getByRole("region", { name: profile.library, exact: true });
          const records = catalog.getByRole("article");
          await expect(records).toHaveCount(VISUAL_SKILL_LIBRARY.data.length);
          for (const item of VISUAL_SKILL_LIBRARY.data) {
            const row = records.filter({
              has: page.getByRole("heading", { name: item.name, exact: true }),
            });
            await expect(row.getByText(item.description!, { exact: true })).toBeVisible();
            await expect(
              row.getByText(item.lifecycle === "active" ? profile.active : profile.archived, {
                exact: true,
              }),
            ).toBeVisible();
            await expect(
              row.getByRole("button", {
                name: item.lifecycle === "active" ? profile.archive : profile.restore,
                exact: true,
              }),
            ).toBeEnabled();
          }
          await expect(catalog.locator("[aria-expanded]")).toHaveCount(0);
          await capture("initial");

          await page.setViewportSize({ width: 560, height: 364 });
          await page.getByRole("button", { name: profile.material, exact: true }).click();
          await expect(personal).toBeChecked();
          const search = panel.getByRole("searchbox", { name: profile.search, exact: true });
          const clear = panel.getByRole("button", { name: profile.clear, exact: true });
          const refresh = panel.getByRole("button", { name: profile.refresh, exact: true });
          const enterAction = async (name: string) => {
            await expect(search).toBeFocused();
            await page.keyboard.press("Tab");
            await expect(clear).toBeFocused();
            await page.keyboard.press("Tab");
            await expect(refresh).toBeFocused();
            await page.keyboard.press("Tab");
            const action = catalog.getByRole("button", { name, exact: true });
            await expect(action).toBeFocused();
            await page.keyboard.press("Enter");
            await expect(action).toBeEnabled();
            await expect(action).toBeFocused();
            return action;
          };
          const backToSearch = async () => {
            for (let index = 0; index < 3; index++) await page.keyboard.press("Shift+Tab");
            await expect(search).toBeFocused();
          };
          await search.fill("old-format");
          const restore = await enterAction(profile.restore);
          const restoreFeedback = panel.getByRole("status").filter({
            hasText: 'Visual Runtime has no response for "skills.library.restore"',
          });
          await expect(restoreFeedback).toBeVisible();
          await expect(records).toHaveCount(1);
          await expect(catalog.getByText(profile.archived, { exact: true })).toBeVisible();
          await expect(search).toHaveValue("old-format");
          await readable(restoreFeedback, "restore feedback at 560px");
          await readable(
            catalog.getByRole("heading", { name: "old-format-notes", exact: true }),
            "archived identity at 560px",
          );
          await readable(restore, "Restore action at 560px");
          await capture("restore-refused-560");

          await page.setViewportSize({ width: 373, height: 243 });
          await backToSearch();
          await page.keyboard.press("Escape");
          await expect(search).toHaveValue("");
          await expect(search).toBeFocused();
          await search.fill("risk-first");
          const archive = await enterAction(profile.archive);
          const archiveFeedback = panel.getByRole("status").filter({
            hasText: 'Visual Runtime has no response for "skills.library.archive"',
          });
          await expect(archiveFeedback).toBeVisible();
          await expect(catalog.getByText(profile.active, { exact: true })).toBeVisible();
          await expect(search).toHaveValue("risk-first");
          await readable(archive, "Archive action at 373px");
          const capacity = await visibleText(archive);
          evidence.push({ name: "archive reading capacity at 373px", geometry: capacity });
          if (!capacity.unit) throw new Error("personal library row is not rendered");
          if (
            capacity.unit.height <=
            capacity.readingFrame.bottom - capacity.readingFrame.top + 1
          ) {
            await readable(archiveFeedback, "archive feedback at 373px");
            await readable(
              catalog.getByRole("heading", { name: "risk-first-review", exact: true }),
              "active identity at 373px",
            );
          } else {
            const feedbackGeometry = await visibleText(archiveFeedback);
            evidence.push({ name: "oversized feedback before wheel", geometry: feedbackGeometry });
            const bottom = Math.max(...feedbackGeometry.rects.map((rect) => rect.bottom));
            const top = Math.min(...feedbackGeometry.rects.map((rect) => rect.top));
            const delta =
              bottom > feedbackGeometry.readingFrame.bottom
                ? bottom - feedbackGeometry.readingFrame.bottom + 1
                : top - feedbackGeometry.readingFrame.top - 1;
            const bounds = (await archive.boundingBox())!;
            await page.mouse.move(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2);
            await page.mouse.wheel(0, delta);
            await readable(
              archiveFeedback,
              "oversized archive feedback after wheel reading at 373px",
            );
            await expect(archive).toBeFocused();
          }
          await capture("archive-refused-373");
          expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(
            0,
          );

          const actionBounds = (await archiveFeedback.boundingBox())!;
          await page.mouse.move(
            actionBounds.x + actionBounds.width / 2,
            actionBounds.y + actionBounds.height / 2,
          );
          await page.mouse.wheel(0, -400);
          await capture("wheel-reading");
          await backToSearch();
          await clear.click();
          await expect(search).toBeFocused();
          await expect(search).toHaveValue("");
          await refresh.click();
          await expect(
            panel.getByRole("heading", { name: profile.total, exact: true }),
          ).toBeVisible();
          await expect(records).toHaveCount(2);
          await personal.focus();
          await page.keyboard.press("ArrowLeft");
          await page.keyboard.press("ArrowLeft");
          await expect(
            panel.getByRole("radio", { name: profile.available, exact: true }),
          ).toBeChecked();
          await expect(archiveFeedback).toHaveCount(0);
          await page.keyboard.press("ArrowRight");
          await page.keyboard.press("ArrowRight");
          await expect(personal).toBeChecked();
          await expect(search).toHaveValue("");
          await expect(records).toHaveCount(2);
          const close = page.getByRole("button", { name: profile.close, exact: true });
          await close.focus();
          await page.keyboard.press("Enter");
          await expect(
            page.getByRole("region", { name: profile.conversation, exact: true }),
          ).toBeFocused();
          await expect(composer).toHaveValue(profile.draft);
          await composer.focus();
          await page.keyboard.press(
            process.platform === "darwin" ? "Meta+ArrowDown" : "Control+End",
          );
          await page.keyboard.insertText("x");
          await expect(composer).toHaveValue(profile.draft + "x");
          await capture("continued-draft");
        } catch (error) {
          await capture("failure");
          throw error;
        } finally {
          await testInfo.attach(`skills-personal-${profile.locale}-${browserName}-geometry`, {
            body: JSON.stringify(evidence, null, 2),
            contentType: "application/json",
          });
        }
      },
    );
  }
}
