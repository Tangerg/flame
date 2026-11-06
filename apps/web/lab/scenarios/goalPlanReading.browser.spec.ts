import { expect, test, type Locator } from "./test";
import { en } from "@/lib/i18n/locales/en";
import zh from "../../../../plugins/locale-zh/package/io.github.tangerg.flame/messages.json" with { type: "json" };
import { catalog } from "../catalog";
import { previewURL, readSelection } from "../selection";

async function expectPainted(surface: Locator) {
  await expect(surface).toBeVisible();
  await expect(surface).toBeInViewport({ ratio: 1 });
  await expect
    .poll(() =>
      surface.evaluate((element) => {
        let opacity = 1;
        for (let node: Element | null = element; node; node = node.parentElement) {
          opacity *= Number(getComputedStyle(node).opacity);
        }
        return opacity;
      }),
    )
    .toBe(1);
}

for (const environment of [
  { locale: "en", theme: "light", fontSize: 14, messages: en },
  { locale: "zh-CN", theme: "dark", fontSize: 18, messages: zh },
]) {
  test(`Goal and Plan readers preserve keyboard return and independent drafts in ${environment.locale}`, async ({
    page,
  }, testInfo) => {
    const messages: Record<string, string> = environment.messages;
    const openScene = async (story: string, variant: string) => {
      await page.setViewportSize({ width: 1440, height: 900 });
      const query = new URLSearchParams({
        panel: "business",
        story,
        variant,
        locale: environment.locale,
        theme: environment.theme,
        "font-size": String(environment.fontSize),
      });
      const selection = readSelection(query.toString(), catalog);
      await page.goto(previewURL(catalog.story(selection.storyId), selection, "goal-plan-reading"));
      await expect(page.locator("html")).toHaveAttribute("data-lab-ready", "true");
      await page.evaluate(() => document.fonts.ready);
    };

    await openScene("business-agent-scenarios", "running");
    const composer = page.getByRole("textbox", {
      name: messages["composer.input.label"],
      exact: true,
    });
    const draft = "Keep this independent draft while reading the Goal and Plan.";
    await composer.fill(draft);
    const goal = page.locator('[data-goal="summary"]');
    const objective = await goal.getAttribute("title");
    expect(objective).toBeTruthy();
    await goal.focus();
    await goal.press("Enter");
    const editor = page.getByRole("dialog", {
      name: messages["goal.edit.title"],
      exact: true,
    });
    const field = editor.getByRole("textbox", {
      name: messages["goal.edit.label"],
      exact: true,
    });
    await expect(field).toHaveValue(objective!);
    await expect(field).toBeFocused();
    const changed = `${objective}\nA local review draft that will be cancelled.`;
    await field.fill(changed);
    await page.setViewportSize({ width: 560, height: 900 });
    await expect(field).toHaveValue(changed);
    expect(await editor.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
    await expectPainted(editor);
    await testInfo.attach("goal-editor-narrow", {
      body: await page.screenshot({ animations: "disabled" }),
      contentType: "image/png",
    });
    await page.keyboard.press("Escape");
    await expect(editor).toHaveCount(0);
    await expect(goal).toBeFocused();
    await expect(composer).toHaveValue(draft);
    await goal.press("Enter");
    await expect(field).toHaveValue(objective!);
    await editor.getByRole("button", { name: messages["common.cancel"], exact: true }).click();
    await expect(goal).toBeFocused();

    const plan = page.locator('[data-slot="active-plan-pill"]');
    await plan.focus();
    await plan.press("Enter");
    const planDialog = page.getByRole("dialog", {
      name: messages["tools.family.plan"],
      exact: true,
    });
    const reader = planDialog.getByRole("region");
    await expect(reader).toBeFocused();
    await expect(reader).not.toHaveAccessibleName("");
    await expect(reader.getByRole("listitem")).toHaveCount(3);
    for (const status of ["done", "active", "pending"]) {
      await expect(
        reader.getByRole("img", {
          name: messages[`plan.step.${status}`],
          exact: true,
        }),
      ).toHaveCount(1);
    }
    const current = reader.locator('li[aria-current="step"]');
    await expect(current).toHaveCount(1);
    await expect(
      current.getByRole("img", {
        name: messages["plan.step.active"],
        exact: true,
      }),
    ).toBeVisible();
    expect(await reader.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
    await expectPainted(planDialog);
    await testInfo.attach("plan-reader-narrow", {
      body: await page.screenshot({ animations: "disabled" }),
      contentType: "image/png",
    });
    await page.keyboard.press("Escape");
    await expect(planDialog).toHaveCount(0);
    await expect(plan).toBeFocused();
    await expect(composer).toHaveValue(draft);

    await openScene("business-workspace-scenarios", "dock-light");
    const workspaceDraft = "Keep this separate workspace draft while reading its Goal.";
    await composer.fill(workspaceDraft);
    const capturedObjective = await goal.getAttribute("title");
    expect(capturedObjective).toBeTruthy();
    await goal.focus();
    await goal.press("Enter");
    const readonly = page.getByRole("dialog", {
      name: messages["goal.edit.label"],
      exact: true,
    });
    const objectiveReader = readonly.getByRole("region", {
      name: messages["goal.edit.label"],
      exact: true,
    });
    await expect(objectiveReader).toBeFocused();
    await expect(objectiveReader).toHaveText(capturedObjective!);
    await expect(readonly.getByRole("textbox")).toHaveCount(0);
    await expect(
      readonly.getByRole("button", {
        name: messages["goal.edit.save"],
        exact: true,
      }),
    ).toHaveCount(0);
    await page.setViewportSize({ width: 560, height: 900 });
    expect(await objectiveReader.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
    await expectPainted(readonly);
    await testInfo.attach("goal-reader-narrow", {
      body: await page.screenshot({ animations: "disabled" }),
      contentType: "image/png",
    });
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.keyboard.press("Escape");
    await expect(readonly).toHaveCount(0);
    await expect(goal).toBeFocused();
    await expect(composer).toHaveValue(workspaceDraft);
  });
}
