import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "./test";
import { expectLabReady } from "../readiness";

for (const browserName of ["chromium", "webkit"] as const) {
  test.extend({ browserName })(
    `${browserName} file tree keeps navigation separate from activation`,
    async ({ page }) => {
      await page.goto(
        "/lab/fixture.html?fixture=workspace&state=dock-files&agent-state=idle&locale=en&motion=off",
      );
      await expectLabReady(page);
      const tree = page.getByRole("tree", { name: "Files", exact: true });
      const folder = tree.getByRole("treeitem", { name: "apps", exact: true });
      await folder.focus();
      await folder.press("ArrowRight");
      await expect(folder).toHaveAttribute("aria-expanded", "true");
      await expect(tree.locator('[role="treeitem"][aria-level="2"]').first()).toBeVisible();
      await folder.press("ArrowRight");
      const child = tree.locator('[role="treeitem"]:focus');
      await expect(child).toHaveAttribute("aria-level", "2");
      const name = await child.getAttribute("aria-label");
      await expect(page.getByRole("button", { name: "Back to files", exact: true })).toBeHidden();
      await child.press("Enter");
      const back = page.getByRole("button", { name: "Back to files", exact: true });
      await expect(back).toBeFocused();
      await back.press("Enter");
      const returned = tree.getByRole("treeitem", { name: name!, exact: true });
      await expect(returned).toBeFocused();
      await expect(returned).toHaveAttribute("aria-current", "true");
      await returned.press("ArrowLeft");
      await expect(folder).toBeFocused();
      await folder.press(" ");
      await expect(folder).toHaveAttribute("aria-expanded", "false");
      await folder.press(" ");
      await expect(folder).toHaveAttribute("aria-expanded", "true");
      await folder.click({ button: "right" });
      const copy = page.getByRole("menuitem", { name: "Copy path", exact: true });
      await expect(copy).toBeVisible();
      await page.keyboard.press("Escape");
      await expect(folder).toBeFocused();
      const audit = await new AxeBuilder({ page })
        .include('[role="tree"]')
        .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
        .analyze();
      expect(audit.violations).toEqual([]);
    },
  );

  test.extend({ browserName })(
    `${browserName} a file preview retires the retained tree menu`,
    async ({ page }) => {
      await page.goto(
        "/lab/fixture.html?fixture=workspace&state=dock-files&agent-state=idle&locale=en&motion=off",
      );
      await expectLabReady(page);
      const tree = page.getByRole("tree", { name: "Files", exact: true });
      const folder = tree.getByRole("treeitem", { name: "apps", exact: true });
      await folder.focus();
      await folder.press("ArrowRight");
      const file = tree.locator('[role="treeitem"][aria-level="2"]').first();
      await expect(file).toBeVisible();
      const path = await file.getAttribute("title");
      expect(path).toBeTruthy();
      await file.focus();
      await file.click({ button: "right" });
      await expect(page.getByRole("menuitem", { name: "Copy path", exact: true })).toBeVisible();

      await page.evaluate(async (path) => {
        const runtimePath = "/lab/scenarios/fixtureRuntime.tsx";
        const navigationPath = "/src/features/workspace/application/navigation.ts";
        const [{ visualClient }, { openWorkspaceFile }] = await Promise.all([
          import(runtimePath),
          import(navigationPath),
        ]);
        openWorkspaceFile(visualClient.workspace.access, path);
      }, path!);
      const back = page.getByRole("button", { name: "Back to files", exact: true });
      await expect(back).toBeFocused();
      await expect(page.getByRole("menu")).toHaveCount(0);
      await back.press("Enter");
      await expect(file).toBeFocused();
      await expect(page.getByRole("menu")).toHaveCount(0);
      await expect(folder).toHaveAttribute("aria-expanded", "true");
    },
  );
}
