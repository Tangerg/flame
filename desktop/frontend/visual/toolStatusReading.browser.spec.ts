import { expect, test } from "./test";

test("a failed process keeps its exit code when secondary tool metadata folds away", async ({
  page,
}) => {
  await page.setViewportSize({ width: 360, height: 900 });
  await page.goto("/visual/?fixture=agent&state=tool-shells&theme=dark&font-size=18");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.evaluate(async () => {
    if (!location.pathname.startsWith("/visual/"))
      throw new Error("An isolated fixture is required");
    const fixturePath = "/visual/agentSessionSnapshots.ts";
    const storePath = "/src/plugins/builtin/agent/adapters/agentStore.ts";
    const projectionPath = "/src/plugins/builtin/agent/application/session/sessionSnapshot.ts";
    const fixture = (await import(fixturePath)) as typeof import("./agentSessionSnapshots");
    const { useAgentStore } = await import(storePath);
    const { projectAgentSessionSnapshot } = await import(projectionPath);
    const snapshot = structuredClone(fixture.RUNTIME_AGENT_SESSION_SNAPSHOTS["tool-shells"]);
    snapshot.items = snapshot.items.filter(
      (item) =>
        item.type === "userMessage" ||
        item.type === "agentMessage" ||
        item.id === "item_shells_command",
    );
    const store = useAgentStore.getState();
    const refresh = store.beginViewRefresh("ses_visual", true);
    if (
      !refresh ||
      !store.commitViewRefresh(
        "ses_visual",
        refresh,
        projectAgentSessionSnapshot(fixture.projectRuntimeAgentSnapshot(snapshot)),
      )
    )
      throw new Error("The fixture snapshot could not be installed");
  });

  const tool = page.locator('[data-tool="shell"]');
  await expect(tool.getByText("exit 1", { exact: true })).toBeVisible();
  await expect(tool.getByText("8.4s", { exact: true })).toBeHidden();
  await expect(tool.getByText("failed", { exact: true })).toHaveCount(0);
  expect(await tool.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);

  await page.setViewportSize({ width: 1120, height: 900 });
  await expect(tool.getByText("exit 1", { exact: true })).toBeVisible();
  await expect(tool.getByText("8.4s", { exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
});
