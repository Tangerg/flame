import { expect, test } from "./test";
import { en } from "@/lib/i18n/locales/en";
import zh from "../../../../plugins/locale-zh/package/io.github.tangerg.flame/messages.json" with { type: "json" };
import { catalog } from "../catalog";
import { previewURL, readSelection } from "../selection";

for (const environment of [
  { locale: "en", theme: "light", fontSize: 14, messages: en },
  { locale: "zh-CN", theme: "dark", fontSize: 18, messages: zh },
]) {
  test(`composer names retain current choices through keyboard selection and narrow layout in ${environment.locale}`, async ({
    page,
  }, testInfo) => {
    const messages: Record<string, string> = environment.messages;
    const modelAction = messages["composer.switchModel"]!;
    const effortAction = messages["composer.switchReasoningEffort"]!;
    const approvalAction = messages["approvals.mode.aria"]!;
    await page.setViewportSize({ width: 1440, height: 900 });
    const query = new URLSearchParams({
      panel: "business",
      story: "business-agent-scenarios",
      variant: "idle",
      locale: environment.locale,
      theme: environment.theme,
      "font-size": String(environment.fontSize),
    });
    const selection = readSelection(query.toString(), catalog);
    await page.goto(previewURL(catalog.story(selection.storyId), selection, "composer-choices"));
    await expect(page.locator("html")).toHaveAttribute("data-lab-ready", "true");
    const draft = page.getByRole("textbox");
    await draft.fill("Keep this draft while choosing model options.");
    const model = page.getByRole("button", { name: new RegExp(`${modelAction}$`) });
    const effort = page.getByRole("button", { name: new RegExp(`${effortAction}$`) });
    const approval = page.getByRole("button", { name: new RegExp(`${approvalAction}$`) });
    await expect(model).toHaveAccessibleName(`GPT-5.6 Sol, ${modelAction}`);
    await expect(effort).toHaveAccessibleName(
      `${messages["model.effort.medium"]}, ${effortAction}`,
    );
    await expect(approval).toHaveAccessibleName(
      `${messages["approvals.mode.balanced"]}, ${approvalAction}`,
    );
    await testInfo.attach("composer-current-choices", {
      body: await page.screenshot({ animations: "disabled" }),
      contentType: "image/png",
    });

    await model.focus();
    await page.keyboard.press("Enter");
    const search = page.getByRole("combobox");
    await expect(search).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(model).toBeFocused();

    await effort.focus();
    await page.keyboard.press("Enter");
    await expect(page.getByRole("menuitem")).toHaveCount(6);
    await expect(page.getByRole("menuitem").first()).toBeFocused();
    await page.keyboard.press("End");
    await page.keyboard.press("ArrowUp");
    await page.keyboard.press("ArrowUp");
    await expect(
      page.getByRole("menuitem", { name: messages["model.effort.high"], exact: true }),
    ).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(effort).toHaveAccessibleName(`${messages["model.effort.high"]}, ${effortAction}`);
    await expect(effort).toBeFocused();

    await approval.focus();
    await page.keyboard.press("Enter");
    await expect(page.getByRole("menuitem").first()).toBeFocused();
    await page.keyboard.press("Home");
    await expect(page.getByRole("menuitem").first()).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(approval).toHaveAccessibleName(
      `${messages["approvals.mode.safe"]}, ${approvalAction}`,
    );
    await expect(approval).toBeFocused();

    await page.locator('[data-slot="composer-root"]').evaluate((element) => {
      element.style.width = "320px";
    });
    await expect(effort.locator('[data-slot="composer-chip-label"]')).toBeHidden();
    await expect(approval.locator('[data-slot="composer-chip-label"]')).toBeHidden();
    await expect(effort).toHaveAccessibleName(`${messages["model.effort.high"]}, ${effortAction}`);
    await expect(approval).toHaveAccessibleName(
      `${messages["approvals.mode.safe"]}, ${approvalAction}`,
    );
    await testInfo.attach("composer-narrow-choices", {
      body: await page.screenshot({ animations: "disabled" }),
      contentType: "image/png",
    });

    await model.focus();
    await page.keyboard.press("Enter");
    await search.fill("Qwen MT Plus");
    await expect(page.getByRole("option", { name: /^Qwen MT Plus/ })).toBeVisible();
    await page.keyboard.press("ArrowDown");
    await page.keyboard.press("Enter");
    await expect(model).toHaveAccessibleName(`Qwen MT Plus, ${modelAction}`);
    await expect(effort).toHaveCount(0);
    await expect(model).toBeFocused();
    await expect(draft).toHaveValue("Keep this draft while choosing model options.");
  });
}

test("the composer preserves the model and lets secondary labels yield without measurement", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/lab/fixture.html?fixture=agent&state=idle&theme=light");
  await page.locator("html[data-visual-ready]").waitFor();
  const composer = page.locator('[data-slot="composer-root"]');
  const model = page.getByRole("button", { name: /Switch model$/ });
  const approval = page.getByRole("button", { name: /Approval mode$/ });
  await expect(model.locator('[data-slot="composer-chip-label"]')).toBeVisible();
  await expect(approval.locator('[data-slot="composer-chip-label"]')).toBeVisible();
  await composer.evaluate((node) => {
    node.style.width = "320px";
  });
  await expect(model.locator('[data-slot="composer-chip-label"]')).toBeVisible();
  await expect(approval.locator('[data-slot="composer-chip-label"]')).toBeHidden();
  await model.locator('[data-slot="composer-chip-label"]').evaluate((node) => {
    node.textContent = "A much longer model label changed without resizing the window";
  });
  expect(await composer.evaluate((node) => node.scrollWidth - node.clientWidth)).toBeLessThan(2);
  const effort = page.getByRole("button", { name: /Switch reasoning effort$/ });
  await expect(effort).toBeVisible();
  await expect(effort.locator('[data-slot="composer-chip-label"]')).toBeHidden();
});

test("high-risk approval remains explicit in a narrow composer", async ({ page }) => {
  await page.goto("/lab/fixture.html?fixture=agent&state=idle&theme=light");
  await page.locator("html[data-visual-ready]").waitFor();
  const approval = page.getByRole("button", { name: /Approval mode$/ });
  await approval.click();
  await page.getByRole("menuitem", { name: /^Auto\b/ }).click();
  const composer = page.locator('[data-slot="composer-root"]');
  await composer.evaluate((node) => {
    node.style.width = "280px";
  });
  await expect(approval.locator('[data-slot="composer-chip-label"]')).toBeVisible();
  await expect(approval).toHaveText("Auto");
  expect(await composer.evaluate((node) => node.scrollWidth - node.clientWidth)).toBeLessThan(2);
});

test("navigation density does not rescale reading or input geometry", async ({ page }) => {
  const measures = [];
  for (const density of ["compact", "spacious"]) {
    await page.goto(`/lab/fixture.html?fixture=agent&state=idle&theme=light&density=${density}`);
    await page.locator("html[data-visual-ready]").waitFor();
    measures.push(
      await page.evaluate(() => {
        const root = getComputedStyle(document.documentElement);
        const input = document.querySelector('[data-slot="composer-root"]')!;
        return {
          row: root.getPropertyValue("--density-row-height"),
          reading: root.getPropertyValue("--reading-gutter-wide"),
          input: input.getBoundingClientRect().height,
        };
      }),
    );
  }
  expect(measures[0]!.row).not.toBe(measures[1]!.row);
  expect(measures[0]!.reading).toBe(measures[1]!.reading);
  expect(measures[0]!.input).toBe(measures[1]!.input);
});
