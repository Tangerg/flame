import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "./test";
test("subagent navigation keeps keyboard focus with its selected task and restores its list entry", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=agent&theme=light&state=delegated&resume-ready=1");
  await page.locator("html[data-visual-ready]").waitFor();
  const trigger = page.locator("#item_delegate").getByRole("button", { name: /^Sub-agent/ });
  await trigger.focus();
  await trigger.press("Enter");
  const panel = page.locator('[data-dock-view-id="subagents"]');
  const back = panel.getByRole("button", { name: "All subagents", exact: true });
  await expect(back).toBeFocused();
  expect(await back.evaluate((node) => node.matches(":focus-visible"))).toBe(true);
  expect(
    await back.evaluate((node) => parseFloat(getComputedStyle(node).outlineWidth)),
  ).toBeGreaterThan(0);
  await back.press("Tab");
  await expect(
    panel.locator("header").getByRole("button", { name: "Cancel this run" }),
  ).toBeFocused();
  await page.keyboard.press("Shift+Tab");
  await expect(back).toBeFocused();
  await back.press("Enter");
  const task = panel.getByRole("button", { name: "Audit Agent Framework ownership Needs input" });
  await expect(task).toBeFocused();
  await task.press("Enter");
  await expect(back).toBeFocused();

  const nested = panel
    .locator('[data-run-id="run_nested"]')
    .getByRole("button", { name: /^Sub-agent/ });
  await nested.focus();
  await nested.press("Enter");
  await expect(
    panel.getByRole("region", { name: "Verify package dependencies", exact: true }),
  ).toBeVisible();
  await expect(back).toBeFocused();
  await back.press("Enter");
  const nestedTask = panel.getByRole("button", { name: "Verify package dependencies Finished" });
  await expect(nestedTask).toBeFocused();
  await expect(panel.getByText("Finished · 5", { exact: true })).toBeVisible();
  await nestedTask.press("Enter");

  await page.evaluate(async () => {
    const path = "/src/lib/navigation.ts";
    const { navigator } = (await import(path)) as typeof import("@/lib/navigation");
    navigator().back();
  });
  await expect(nestedTask).toBeFocused();
  await page.evaluate(async () => {
    const path = "/src/lib/navigation.ts";
    const { navigator } = (await import(path)) as typeof import("@/lib/navigation");
    navigator().forward();
  });
  await expect(back).toBeFocused();
  const accessibility = await new AxeBuilder({ page })
    .include('[data-dock-view-id="subagents"]')
    .analyze();
  expect(accessibility.violations).toEqual([]);

  const composer = page.getByRole("textbox", { name: "Message composer" });
  await composer.focus();
  await page.evaluate(async () => {
    const path = "/src/features/workspace/public/navigation.ts";
    const { openWorkspaceSubagentRun } = (await import(
      path
    )) as typeof import("@/features/workspace/public/navigation");
    openWorkspaceSubagentRun("run_child");
  });
  await expect(
    panel.getByRole("region", { name: "Audit Agent Framework ownership", exact: true }),
  ).toBeVisible();
  await expect(composer).toBeFocused();
  await page.evaluate(async () => {
    const path = "/src/features/workspace/public/navigation.ts";
    const { openWorkspaceSubagentRun } = (await import(
      path
    )) as typeof import("@/features/workspace/public/navigation");
    openWorkspaceSubagentRun(null);
  });
  await expect(task).toBeVisible();
  await expect(composer).toBeFocused();
});
