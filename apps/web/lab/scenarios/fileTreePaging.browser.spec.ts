import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "./test";
import { expectLabReady } from "../readiness";

for (const browserName of ["chromium", "webkit"] as const) {
  test.extend({ browserName })(
    `${browserName} continues a real directory cursor through the virtual tree`,
    async ({ page }) => {
      await page.goto(
        "/lab/fixture.html?fixture=workspace&state=dock-files&agent-state=idle&directory-state=paged&locale=en&motion=off",
      );
      await expectLabReady(page);
      const tree = page.getByRole("treegrid", { name: "Files", exact: true });
      const folder = tree.getByRole("row", { name: "apps", exact: true });
      await folder.focus();
      await folder.press("ArrowRight");
      await expect(tree.getByRole("row", { name: "page-001.txt", exact: true })).toBeVisible();
      await expect
        .poll(async () => {
          await tree.evaluate((node) => {
            node.scrollTop = node.scrollHeight;
          });
          return tree.getByRole("row", { name: "page-120.txt", exact: true }).count();
        })
        .toBe(1);
      await expect(tree.getByRole("row", { name: "page-120.txt", exact: true })).toBeVisible();
      expect(await tree.getByRole("row").count()).toBeLessThan(100);
      const audit = await new AxeBuilder({ page })
        .include('[role="treegrid"]')
        .withTags(["wcag2a", "wcag2aa"])
        .analyze();
      expect(audit.violations).toEqual([]);
    },
  );

  test.extend({ browserName })(
    `${browserName} exposes budget continuation outside the file collection`,
    async ({ page }) => {
      await page.goto(
        "/lab/fixture.html?fixture=workspace&state=dock-files&agent-state=idle&directory-state=budget&locale=en&motion=off",
      );
      await expectLabReady(page);
      const tree = page.getByRole("treegrid", { name: "Files", exact: true });
      const folder = tree.getByRole("row", { name: "apps", exact: true });
      await folder.focus();
      await folder.press("ArrowRight");
      const more = page.getByRole("button", { name: "Load more files", exact: true });
      await expect(more).toBeVisible();
      expect(await more.evaluate((node) => Boolean(node.closest('[role="treegrid"]')))).toBe(false);
      for (
        let step = 0;
        step < 8 && !(await more.evaluate((node) => node === document.activeElement));
        step++
      )
        await page.keyboard.press("Tab");
      await expect(more).toBeFocused();
      await more.press("Enter");
      await expect(tree.locator('[data-file-path][aria-level="2"]').first()).toBeVisible();
      await expect(more).toHaveCount(0);
      await expect(folder).toHaveAttribute("aria-expanded", "true");
    },
  );
}
