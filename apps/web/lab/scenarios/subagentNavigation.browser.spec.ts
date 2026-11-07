import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "./test";
test("subagent navigation keeps keyboard focus with its selected task and restores its list entry", async ({
  page,
}, testInfo) => {
  await page.goto("/lab/fixture.html?fixture=agent&theme=light&state=delegated&resume-ready=1");
  await page.locator("html[data-visual-ready]").waitFor();
  const trigger = page.locator("#item_delegate").getByRole("button", { name: /^Sub-agent/ });
  await trigger.focus();
  await trigger.press("Enter");
  const dock = page.getByRole("complementary", { name: "Subagents", exact: true });
  const panel = page.getByRole("region", { name: "Subagents", exact: true });
  const back = panel.getByRole("button", { name: "All subagents", exact: true });
  await expect(back).toBeVisible();
  // Inspection arrival focuses the workspace frame; the task's local controls are a separate stop.
  await expect(dock).toBeFocused();
  expect(await dock.evaluate((node) => node.matches(":focus-visible"))).toBe(true);
  await testInfo.attach("subagent-arrival.png", {
    body: await dock.screenshot(),
    contentType: "image/png",
  });

  await page.keyboard.press("Escape");
  await expect(trigger).toBeFocused();
  await trigger.press("Enter");
  await expect(back).toBeVisible();
  await expect(dock).toBeFocused();
  expect(await dock.evaluate((node) => node.matches(":focus-visible"))).toBe(true);
  await page.keyboard.press("Tab");
  await expect(
    dock.getByRole("button", { name: "Back to conversation", exact: true }),
  ).toBeFocused();

  await back.focus();
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

  const parentTranscript = panel.getByRole("region", {
    name: "Audit Agent Framework ownership",
    exact: true,
  });
  await expect(parentTranscript).toBeVisible();
  const nestedLink = parentTranscript.locator(
    '[data-slot="delegated-run-link"][data-run-id="run_nested"]',
  );
  await expect(nestedLink).toHaveCount(1);
  const nested = nestedLink.getByRole("button", { name: /^Sub-agent/ });
  await expect(nested).toBeVisible();
  await nested.focus();
  await nested.press("Enter");
  await expect(
    panel.getByRole("region", { name: "Verify package dependencies", exact: true }),
  ).toBeVisible();
  await expect(panel.locator('[data-slot="subagent-identity"]')).toHaveText(
    "Verify package dependencies",
  );
  await expect(back).toBeFocused();
  await back.press("Enter");
  const nestedTask = panel.getByRole("button", { name: "Verify package dependencies Finished" });
  await expect(nestedTask).toBeFocused();
  const overview = panel.getByRole("group", { name: "Subagents", exact: true });
  await expect(overview.locator('[data-slot="delegated-run-link"]')).toHaveCount(6);
  await expect(overview.getByText("Finished · 5", { exact: true })).toBeVisible();
  await nestedTask.press("Enter");

  await page.evaluate(async () => {
    const path = "/lab/scenarios/fixtureRuntime.tsx";
    const { visualClient } = (await import(path)) as typeof import("./fixtureRuntime");
    visualClient.navigation.back();
  });
  await expect(nestedTask).toBeFocused();
  await page.evaluate(async () => {
    const path = "/lab/scenarios/fixtureRuntime.tsx";
    const { visualClient } = (await import(path)) as typeof import("./fixtureRuntime");
    visualClient.navigation.forward();
  });
  await expect(back).toBeFocused();
  const accessibility = await new AxeBuilder({ page })
    .include('[role="region"][aria-label="Subagents"]')
    .analyze();
  expect(accessibility.violations).toEqual([]);

  const composer = page.getByRole("textbox", { name: "Message composer" });
  await composer.focus();
  await page.evaluate(async () => {
    const path = "/src/features/workspace/public/navigation.ts";
    const { openWorkspaceSubagentRun } = (await import(
      path
    )) as typeof import("@/features/workspace/public/navigation");
    const fixturePath = "/lab/scenarios/fixtureRuntime.tsx";
    const { visualClient } = (await import(fixturePath)) as typeof import("./fixtureRuntime");
    openWorkspaceSubagentRun(visualClient.workspace.access, "run_child");
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
    const fixturePath = "/lab/scenarios/fixtureRuntime.tsx";
    const { visualClient } = (await import(fixturePath)) as typeof import("./fixtureRuntime");
    openWorkspaceSubagentRun(visualClient.workspace.access, null);
  });
  await expect(task).toBeVisible();
  await expect(composer).toBeFocused();
});
