import { expect, test } from "./test";

for (const theme of ["light", "dark"]) {
  test(`Dock panels keep a quiet outline and complete context while page headings stay visible in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1472, height: 900 });
    await page.goto(`/visual/?fixture=workspace&state=dock-review&theme=${theme}&font-size=18`);
    await page.locator("html[data-visual-ready]").waitFor();
    const view = page.locator('[data-dock-view-id="package:visual:diff"]');
    const viewport = view.locator('[data-slot="workspace-view-body"]');
    const heading = view.getByRole("heading", { name: "Workspace diff", exact: true });
    await expect(heading).toHaveCount(1);
    const bounds = (await heading.boundingBox())!;
    expect(bounds.width).toBeLessThanOrEqual(1);
    expect(bounds.height).toBeLessThanOrEqual(1);

    await view.getByRole("button", { name: "Show changed files", exact: true }).click();
    for (const narrow of [false, true]) {
      if (narrow) {
        const resizer = page.locator(".agent-pane-resizer");
        await resizer.focus();
        await resizer.press("Home");
        expect((await view.boundingBox())!.width).toBeCloseTo(320, 0);
      }
      await viewport.evaluate((node) => node.scrollTo(0, 0));
      const context = view.locator('header [dir="ltr"]');
      await expect(context).toHaveText("/Users/visual/scope");
      await expect(context).toBeInViewport({ ratio: 1 });
      const filter = view.getByRole("searchbox", { name: "Filter files", exact: true });
      for (const control of [
        filter,
        view.getByRole("button", { name: "Refresh", exact: true }),
        view.getByRole("combobox", { name: "Comparison", exact: true }),
      ]) {
        await expect(control).toBeInViewport({ ratio: 1 });
      }
      await filter.fill("session.go");
      await expect(view.locator("[data-diff-file]")).toHaveCount(1);
      await expect(view.locator("[data-diff-file]").first()).toHaveAttribute(
        "title",
        "runtime/protocol/session.go",
      );
      await filter.fill("");
      await expect(view.locator("[data-diff-file]")).toHaveCount(2);
    }

    await page.setViewportSize({ width: 320, height: 900 });
    await page.goto(
      `/visual/?fixture=workspace&state=full-view&full-view=package:visual:diff&sidebar=collapsed&theme=${theme}&font-size=18`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    const full = page.getByRole("region", { name: "Workspace diff", exact: true });
    const pageTitle = full.getByRole("heading", { name: "Workspace diff", exact: true });
    expect((await pageTitle.boundingBox())!.height).toBeGreaterThan(1);
    await full.locator('header [dir="ltr"]').scrollIntoViewIfNeeded();
    await expect(full.locator('header [dir="ltr"]')).toHaveText("/Users/visual/scope");
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  });
}
