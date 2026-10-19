import { expect, test, type Locator } from "./test";

async function expectCompleteText(text: Locator) {
  await text.scrollIntoViewIfNeeded();
  const clipped = await text.evaluate((node) => {
    const range = document.createRange();
    range.selectNodeContents(node);
    const fragments = [...range.getClientRects()].filter((rect) => rect.width > 0);
    for (let parent: Element | null = node; parent; parent = parent.parentElement) {
      if (!["hidden", "clip", "auto", "scroll"].includes(getComputedStyle(parent).overflowX))
        continue;
      const bounds = parent.getBoundingClientRect();
      if (fragments.some((rect) => rect.left < bounds.left || rect.right > bounds.right))
        return true;
    }
    return false;
  });
  expect(clipped).toBe(false);
}

for (const theme of ["light", "dark"]) {
  for (const width of [1120, 320]) {
    test(`delegated task names remain readable in the list and selected transcript ${theme} ${width}`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 900 });
      const query = new URLSearchParams({
        fixture: "workspace",
        state: width === 320 ? "full-view" : "dock-subagents",
        "full-view": "subagents",
        "agent-state": "delegated",
        theme,
        "font-size": "18",
      });
      await page.goto(`/visual/?${query}`);
      await page.locator("html[data-visual-ready]").waitFor();
      if (width === 320) {
        await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
        await expect(
          page.getByRole("button", { name: "Expand the workspace fixture sidebar" }),
        ).toHaveAttribute("aria-expanded", "false");
      }
      const mainView = width === 320 ? "subagents" : "";
      await expect(page.getByTestId("active-main-view")).toHaveText(mainView);
      const panel = page.getByRole("region", { name: "Subagents", exact: true });
      for (const name of [
        "Audit Agent Framework ownership",
        "Verify package dependencies",
        ...[1, 2, 3, 4].map((index) => `Survey the boundary in parallel · Sub-agent ${index} of 4`),
      ]) {
        await expectCompleteText(panel.getByText(name, { exact: true }));
      }
      const name = "Audit Agent Framework ownership";
      await panel.getByRole("button", { name: `${name} Needs input`, exact: true }).click();
      await expect(page.getByTestId("active-main-view")).toHaveText(mainView);
      const transcript = panel.getByRole("region", { name, exact: true });
      await expect(transcript).toBeVisible();
      const header = panel.locator("header");
      await expectCompleteText(header.getByText(name, { exact: true }));
      await expect(header.getByText("Needs input", { exact: true })).toBeVisible();
      const cancel = header.getByRole("button", { name: "Cancel this run", exact: true }).first();
      await expect(cancel).toBeInViewport({ ratio: 1 });
      const bounds = (await panel.boundingBox())!;
      const action = (await cancel.boundingBox())!;
      expect(action.x).toBeGreaterThanOrEqual(bounds.x);
      expect(action.x + action.width).toBeLessThanOrEqual(bounds.x + bounds.width);
      expect(await panel.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
      expect(
        await page.locator("html").evaluate((node) => node.scrollWidth - node.clientWidth),
      ).toBe(0);
      await panel.getByRole("button", { name: "Back to subagents", exact: true }).click();
      await expect(page.getByTestId("active-main-view")).toHaveText(mainView);
      await expectCompleteText(panel.getByText(name, { exact: true }));
      const longName = "Survey the boundary in parallel · Sub-agent 1 of 4";
      await panel.getByRole("button", { name: longName }).click();
      const completed = panel.getByRole("region", { name: longName, exact: true });
      await expect(completed).toBeVisible();
      await expectCompleteText(header.getByText(longName, { exact: true }));
      await expect(header.getByText("Canceled", { exact: true })).toBeVisible();
      await expect(page.getByTestId("active-main-view")).toHaveText(mainView);
    });
  }
}
