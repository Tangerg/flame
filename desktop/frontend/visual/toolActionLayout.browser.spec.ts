import { expect, test } from "./test";

for (const grouped of [true, false]) {
  test(`a ${grouped ? "grouped" : "single"} tool without actions uses its complete row`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 800, height: 900 });
    await page.goto("/visual/?fixture=agent&state=tool-search&theme=dark&font-size=18&locale=en");
    await page.locator("html[data-visual-ready]").waitFor();
    if (grouped) {
      await page.getByRole("button", { name: /calls/ }).first().click();
    } else {
      await page.evaluate(async () => {
        const fixturePath = "/visual/agentSessionSnapshots.ts";
        const storePath = "/src/plugins/builtin/agent/adapters/agentStore.ts";
        const projectionPath = "/src/plugins/builtin/agent/application/session/sessionSnapshot.ts";
        const fixture = (await import(fixturePath)) as typeof import("./agentSessionSnapshots");
        const { useAgentStore } = await import(storePath);
        const { projectAgentSessionSnapshot } = await import(projectionPath);
        const snapshot = structuredClone(fixture.RUNTIME_AGENT_SESSION_SNAPSHOTS["tool-search"]);
        snapshot.items = snapshot.items.filter(
          (item) => item.type !== "toolCall" || item.tool.name === "glob",
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
          throw new Error("The canonical tool snapshot could not be installed");
      });
    }
    const tool = page.locator('[data-tool="glob"]');
    const row = grouped ? tool.locator("..") : tool.locator('[data-slot="agent-activity-header"]');
    const trigger = grouped ? tool : row.getByRole("button");
    await expect(row.getByRole("button")).toHaveCount(1);
    const bounds = (await row.boundingBox())!;
    const target = (await trigger.boundingBox())!;
    expect(Math.abs(bounds.x + bounds.width - target.x - target.width)).toBeLessThanOrEqual(1);
    expect(
      await trigger.evaluate(
        (element, point) => {
          const hit = document.elementFromPoint(point.x, point.y);
          return hit !== null && element.contains(hit);
        },
        { x: bounds.x + bounds.width - 2, y: bounds.y + bounds.height / 2 },
      ),
    ).toBe(true);
    await page.mouse.click(bounds.x + bounds.width - 2, bounds.y + bounds.height / 2);
    await expect(trigger).toHaveAttribute("aria-expanded", "true");
    await expect(page.locator('[data-slot="recorded-result-viewport"]')).toContainText(
      "runtime/internal/session/store_test.go",
    );
  });
}
