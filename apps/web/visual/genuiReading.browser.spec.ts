import { expect, test } from "./test";

for (const theme of ["light", "dark"] as const) {
  for (const width of [360, 1280]) {
    test(`published interactive report works in ${theme} at ${width}px`, async ({ page }) => {
      await page.setViewportSize({ width, height: 960 });
      const remoteRequests: string[] = [];
      page.on("request", (request) => {
        if (!request.url().startsWith("http://127.0.0.1:")) remoteRequests.push(request.url());
      });
      await page.goto(`/visual/?fixture=agent&state=genui-report&theme=${theme}&font-size=18`);
      await page.locator("html[data-visual-ready]").waitFor();
      expect(await page.evaluate(() => Number.isInteger(Date.now()))).toBe(true);
      await page.evaluate(() => document.fonts.ready);
      await page.locator('[data-tool="genui_ui_present"]').click();
      const report = page.getByRole("region", {
        name: "Agent plugin delivery review",
        exact: true,
      });
      await expect(report).toBeVisible();
      await expect(
        report.getByRole("heading", { name: "Agent plugin delivery review" }),
      ).toBeVisible();
      await expect(page.locator("[data-openui-devtools-auto-mount]")).toHaveCount(0);

      const weeks = report.getByRole("textbox", { name: "Delivery weeks" });
      const team = report.getByRole("textbox", { name: "Delivery team" });
      const projected = report.getByText("Projected checks", { exact: true }).locator("..");
      await expect(weeks).toHaveValue("4");
      await expect(projected).toContainText("48");
      const overview = test.info().outputPath(`genui-${theme}-${width}-overview.png`);
      await page.screenshot({ path: overview });
      await test.info().attach("published-interactive-overview", { path: overview });

      await weeks.fill("-");
      await expect(report.getByText("Enter a valid number.")).toBeVisible();
      await report.getByRole("button", { name: "Apply scenario" }).click();
      await expect(report.getByText("No scenario has been applied.")).toBeVisible();
      await weeks.fill("6");
      await team.fill("Review team");
      await expect(projected).toContainText("72");
      await report.getByRole("button", { name: "Apply scenario" }).click();
      await expect(
        report.getByText("Applied scenario for Review team: 6 delivery weeks."),
      ).toBeVisible();
      await report.getByRole("button", { name: "Reset scenario" }).click();
      await expect(weeks).toHaveValue("4");
      await expect(team).toHaveValue("Runtime team");

      await report.getByRole("combobox", { name: "Delivery track" }).click();
      await page.getByRole("option", { name: "Focused", exact: true }).click();
      await expect(report.getByRole("combobox", { name: "Delivery track" })).toContainText(
        "Focused",
      );
      await report.getByRole("button", { name: "Advanced scenario controls" }).click();
      const computer = report.getByRole("checkbox", { name: "Computer use", exact: true });
      await computer.click();
      await expect(computer).toBeChecked();
      const approval = report.getByRole("checkbox", { name: "Ready for a review", exact: true });
      await approval.focus();
      await page.keyboard.press("Space");
      await expect(approval).toBeChecked();
      await report.getByRole("radio", { name: "Internal", exact: true }).focus();
      await page.keyboard.press("ArrowRight");
      await expect(report.getByRole("radio", { name: "Public", exact: true })).toBeChecked();
      const capacity = report.getByRole("slider", { name: "Capacity percentage" });
      await capacity.focus();
      await page.keyboard.press("ArrowRight");
      await expect(capacity).toHaveAttribute("aria-valuenow", "55");
      await report
        .getByRole("textbox", { name: "Review notes" })
        .fill("Inspect the original command receipt.");
      await report.getByLabel("Review date", { exact: true }).fill("2026-11-15");
      await report.getByRole("button", { name: "Advanced scenario controls" }).click();

      const identifier = report.getByText("9007199254740993", { exact: true }).first();
      await identifier.scrollIntoViewIfNeeded();
      await expect(identifier).toBeInViewport({ ratio: 1 });
      const metric = test.info().outputPath(`genui-${theme}-${width}-metric.png`);
      await page.screenshot({ path: metric });
      await test.info().attach("published-exact-identifier", { path: metric });

      const tableRegion = report.getByRole("region", {
        name: "Plugin delivery responsibilities",
        exact: true,
      });
      const table = tableRegion.getByRole("table", { name: "Plugin delivery responsibilities" });
      const pagination = report.getByRole("group", {
        name: "Plugin delivery responsibilities pages",
      });
      await expect(table.getByRole("columnheader")).toHaveCount(9);
      await expect(table.getByRole("row")).toHaveCount(3);
      await pagination.getByRole("button", { name: "Next", exact: true }).click();
      await expect(
        table.getByRole("cell", { name: "Browser automation", exact: true }),
      ).toBeVisible();
      await table.getByRole("checkbox", { name: "Select browser", exact: true }).click();
      await expect(
        report.getByText("Selected evidence rows", { exact: true }).locator(".."),
      ).toContainText("1");
      const search = report.getByRole("searchbox", {
        name: "Search Plugin delivery responsibilities",
      });
      await search.fill("Exact identifiers");
      await expect(table.getByRole("row")).toHaveCount(2);
      await expect(table.getByRole("cell", { name: "9007199254740993", exact: true })).toHaveCount(
        2,
      );
      await search.fill("");
      await table.getByRole("button", { name: "Sort by Capability", exact: true }).click();
      await expect(table.getByRole("columnheader", { name: /Capability/ })).toHaveAttribute(
        "aria-sort",
        "ascending",
      );
      await expect(table.getByRole("row").nth(1)).toContainText("Browser automation");
      await expect(
        table.getByRole("checkbox", { name: "Select browser", exact: true }),
      ).toBeChecked();

      await tableRegion.scrollIntoViewIfNeeded();
      await tableRegion.focus();
      await expect(tableRegion).toBeFocused();
      expect(
        await tableRegion.evaluate((node) => node.scrollWidth - node.clientWidth),
      ).toBeGreaterThan(0);
      await page.keyboard.down("ArrowRight");
      try {
        await expect.poll(() => tableRegion.evaluate((node) => node.scrollLeft)).toBeGreaterThan(0);
      } finally {
        await page.keyboard.up("ArrowRight");
      }
      const bounds = await report.evaluate((node) => {
        const rectangle = node.getBoundingClientRect();
        const region = node.querySelector<HTMLElement>('[role="region"]')!;
        const tableBounds = region.getBoundingClientRect();
        return {
          documentOverflow: document.documentElement.scrollWidth - innerWidth,
          reportOverflow: node.scrollWidth - node.clientWidth,
          reportLeft: rectangle.left,
          reportRight: rectangle.right,
          regionLeft: tableBounds.left,
          regionRight: tableBounds.right,
        };
      });
      expect(bounds.documentOverflow).toBeLessThanOrEqual(1);
      expect(bounds.reportOverflow).toBeLessThanOrEqual(1);
      expect(bounds.regionLeft).toBeGreaterThanOrEqual(bounds.reportLeft - 1);
      expect(bounds.regionRight).toBeLessThanOrEqual(bounds.reportRight + 1);
      expect(bounds.reportLeft).toBeGreaterThanOrEqual(0);
      expect(bounds.reportRight).toBeLessThanOrEqual(width + 1);
      await tableRegion.hover();
      await expect
        .poll(() =>
          tableRegion.evaluate(async (node) => {
            const position = node.scrollLeft;
            await new Promise<void>((resolve) =>
              requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
            );
            return node.scrollLeft === position;
          }),
        )
        .toBe(true);
      await page.mouse.wheel(6000, 0);
      await expect
        .poll(() =>
          tableRegion.evaluate((node) => node.scrollWidth - node.clientWidth - node.scrollLeft),
        )
        .toBeLessThanOrEqual(1);
      await expect(
        table.getByRole("button", { name: "Sort by Review decision", exact: true }),
      ).toBeInViewport();
      const matrix = test.info().outputPath(`genui-${theme}-${width}-table.png`);
      await page.screenshot({ path: matrix });
      await test.info().attach("published-interactive-table", { path: matrix });

      await report.getByRole("tab", { name: "Delivery evidence", exact: true }).focus();
      await page.keyboard.press("ArrowRight");
      const explore = report.getByRole("tabpanel", { name: "Explore displays" });
      await expect(explore).toBeVisible();
      await expect(explore.getByText("computer", { exact: true })).toBeVisible();
      await expect(
        explore.getByRole("img", { name: "Delivery progress illustration" }),
      ).toBeVisible();
      await expect(explore.locator("math")).toBeVisible();
      await explore.getByRole("button", { name: "Show answer", exact: true }).click();
      await expect(explore.getByText("Runtime owns execution and recovery.")).toBeVisible();
      await explore.getByRole("button", { name: "Review hint", exact: true }).click();
      await expect(explore.getByRole("dialog", { name: "Review hint" })).toBeVisible();
      await page.keyboard.press("Escape");
      await expect(explore.getByRole("dialog", { name: "Review hint" })).toHaveCount(0);
      const exploration = test.info().outputPath(`genui-${theme}-${width}-explore.png`);
      await page.screenshot({ path: exploration });
      await test.info().attach("published-interactive-displays", { path: exploration });
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth - innerWidth),
      ).toBeLessThanOrEqual(1);
      expect(remoteRequests).toEqual([]);
    });
  }
}
