import { expect, test } from "./test";
const label =
  "Audit application-owned cancellation, transaction boundaries and dependency graph verification across the complete Agent Framework";

for (const [fontSize, theme] of [
  [14, "light"],
  [18, "dark"],
] as const) {
  test(`a narrow subagent header keeps the selected identity and commands together ${fontSize}`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width: 1472, height: 900 });
    await page.goto(
      `/visual/?fixture=workspace&state=dock-subagents&theme=${theme}&font-size=${fontSize}&locale=en`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    const panel = page.locator('[data-dock-view-id="subagents"]');
    await expect(panel.getByText("6 subagents · 1 running", { exact: true })).toBeVisible();
    await panel
      .getByRole("button", { name: "Audit Agent Framework ownership Needs input" })
      .click();
    const resizer = page.locator(".agent-pane-resizer");
    await resizer.focus();
    await resizer.press("Home");
    expect((await panel.boundingBox())!.width).toBeCloseTo(320, 0);

    await page.evaluate(async (label) => {
      const path = "/visual/main.tsx";
      const { visualClient } = (await import(path)) as typeof import("./main");
      const store = visualClient.agent.projection.store.getState();
      const view = store.sessions.ses_visual!.view;
      const refresh = store.beginViewRefresh("ses_visual", true);
      if (!refresh) throw new Error("Expected a refresh token");
      store.commitViewRefresh("ses_visual", refresh, {
        ...view,
        toolCalls: {
          ...view.toolCalls,
          item_delegate: { ...view.toolCalls.item_delegate!, fn: label },
        },
      });
    }, label);

    const header = panel.locator("header");
    const identity = header.getByText(label, { exact: true });
    await expect(identity).toHaveText(label);
    await expect(resizer).toBeFocused();
    await expect(panel.getByRole("region", { name: label, exact: true })).toBeAttached();
    await expect(header.getByText("Needs input", { exact: true })).toBeInViewport({ ratio: 1 });
    await expect(panel.getByText("6 subagents · 1 running", { exact: true })).toHaveCount(0);
    expect(await identity.evaluate((node) => node.scrollHeight > node.clientHeight)).toBe(true);
    expect(
      await identity.evaluate(
        (node) => node.clientHeight / parseFloat(getComputedStyle(node).lineHeight),
      ),
    ).toBeLessThanOrEqual(2.05);
    await expect(identity).toHaveAttribute("title", label);
    for (const button of await header.getByRole("button").all()) {
      await expect(button).toBeInViewport({ ratio: 1 });
      expect(
        await button.evaluate((node) => {
          const box = node.getBoundingClientRect();
          const host = node.closest("header")!.getBoundingClientRect();
          return (
            box.left >= host.left &&
            box.right <= host.right &&
            node.contains(document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2))
          );
        }),
      ).toBe(true);
    }
    expect(await panel.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
    await page.screenshot({ path: testInfo.outputPath("narrow-selected-task.png") });
    const back = header.getByRole("button", { name: "All subagents", exact: true });
    await back.focus();
    await back.press("Enter");
    const task = panel.getByRole("button", { name: `${label} Needs input`, exact: true });
    await expect(task.getByText(label, { exact: true })).toBeInViewport({ ratio: 1 });
    await expect(task).toBeFocused();
    await task.press("Enter");
    await expect(back).toBeFocused();
    await expect(identity).toBeInViewport({ ratio: 1 });
    await header.getByRole("button", { name: "Cancel this run", exact: true }).click();
    await expect(page.locator("html")).toHaveAttribute("data-visual-canceled-run", "run_child");
    await header.getByRole("button", { name: "All subagents", exact: true }).click();
    await expect(header.getByText("6 subagents · 1 running", { exact: true })).toBeVisible();
    await expect(header.getByRole("button", { name: "All subagents", exact: true })).toHaveCount(0);
  });
}
