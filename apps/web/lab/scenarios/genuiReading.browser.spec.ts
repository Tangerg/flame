import { expect, test } from "./test";
import { keepFinalSortRevealed, respectHorizontalReading } from "./genuiFocus";
import { expectLabReady } from "../readiness";

test("the Agent reading scene honors the shared collapsed-sidebar parameter", async ({ page }) => {
  await page.setViewportSize({ width: 520, height: 900 });
  await page.goto("/lab/fixture.html?fixture=agent&state=idle&sidebar=collapsed");
  await expectLabReady(page);
  const expand = page.getByRole("button", {
    name: "Expand the agent fixture sidebar",
    exact: true,
  });
  await expect(expand).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Collapse the agent fixture sidebar", exact: true }),
  ).toHaveCount(0);
  const composer = page.getByRole("textbox", { name: "Message composer", exact: true });
  await composer.fill("Keep this reading draft through viewport changes.");
  await page.setViewportSize({ width: 1120, height: 900 });
  await expect(expand).toBeVisible();
  await expect(composer).toHaveValue("Keep this reading draft through viewport changes.");
  await test.info().attach("collapsed-agent-reading-frame", {
    body: await page.screenshot({ animations: "disabled" }),
    contentType: "image/png",
  });
});

for (const theme of ["light", "dark"] as const) {
  for (const width of [360, 1280]) {
    test(`published interactive report works in ${theme} at ${width}px`, async ({ page }) => {
      await page.setViewportSize({ width, height: 960 });
      const remoteRequests: string[] = [];
      page.on("request", (request) => {
        if (!request.url().startsWith("http://127.0.0.1:")) remoteRequests.push(request.url());
      });
      await page.goto(
        `/lab/fixture.html?fixture=agent&state=genui-report&theme=${theme}&font-size=18`,
      );
      await expectLabReady(page);
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

      const sortButtons = table.getByRole("button", { name: /^Sort by / });
      await expect(sortButtons).toHaveCount(8);
      const headerWords = await sortButtons.evaluateAll((buttons) =>
        buttons.map((button) => {
          const words = [];
          const walker = document.createTreeWalker(button, NodeFilter.SHOW_TEXT);
          for (let node = walker.nextNode(); node; node = walker.nextNode()) {
            for (const match of node.textContent!.matchAll(/[A-Za-z]+/g)) {
              const range = document.createRange();
              range.setStart(node, match.index!);
              range.setEnd(node, match.index! + match[0].length);
              const rects = [...range.getClientRects()]
                .filter((rect) => rect.width > 0 && rect.height > 0)
                .map(({ left, top, width, height }) => ({ left, top, width, height }));
              const lowestTop = Math.max(...rects.map((rect) => rect.top));
              const highestBottom = Math.min(...rects.map((rect) => rect.top + rect.height));
              words.push({
                word: match[0],
                oneLine: rects.length > 0 && lowestTop < highestBottom,
                rects,
              });
            }
          }
          return { label: button.getAttribute("aria-label"), words };
        }),
      );
      await test.info().attach("published-table-header-words", {
        body: JSON.stringify(headerWords, null, 2),
        contentType: "application/json",
      });
      const headers = test.info().outputPath(`genui-${theme}-${width}-headers.png`);
      await tableRegion.screenshot({ path: headers });
      await test.info().attach("published-table-headers", { path: headers });
      expect(
        headerWords.flatMap(({ label, words }) =>
          words.length === 0
            ? [`${label}: no rendered words`]
            : words.filter((word) => !word.oneLine).map(({ word }) => `${label}: ${word}`),
        ),
      ).toEqual([]);

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
      const hintTrigger = explore.getByRole("button", { name: "Review hint", exact: true });
      await hintTrigger.click();
      const hint = page.getByRole("dialog", { name: "Review hint" });
      await expect(hint).toBeVisible();
      await page.keyboard.press("Escape");
      await expect(hint).toHaveCount(0);
      await expect(hintTrigger).toBeFocused();
      const exploration = test.info().outputPath(`genui-${theme}-${width}-explore.png`);
      await page.screenshot({ path: exploration });
      await test.info().attach("published-interactive-displays", { path: exploration });
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth - innerWidth),
      ).toBeLessThanOrEqual(1);
      expect(remoteRequests).toEqual([]);
    });

    test(`Chinese report selection survives an empty search in ${theme} at ${width}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 960 });
      await page.goto(
        `/lab/fixture.html?fixture=agent&state=genui-report&locale=zh&theme=${theme}&font-size=18&sidebar=collapsed`,
      );
      await expectLabReady(page);
      await page.evaluate(() => document.fonts.ready);
      const composer = page.getByRole("textbox", { name: "消息输入框", exact: true });
      await composer.fill("保留这条草稿，直到筛选结束。");
      await page.locator('[data-tool="genui_ui_present"]').click();
      const report = page.getByRole("region", {
        name: "Agent plugin delivery review",
        exact: true,
      });
      await expect(report).toBeVisible();

      const tableRegion = report.getByRole("region", {
        name: "Plugin delivery responsibilities",
        exact: true,
      });
      const table = tableRegion.getByRole("table", { name: "Plugin delivery responsibilities" });
      const generated = table.getByRole("checkbox", { name: "选择generated", exact: true });
      await generated.click();
      await expect(generated).toBeChecked();
      const search = report.getByRole("searchbox", {
        name: "搜索Plugin delivery responsibilities",
        exact: true,
      });
      await search.fill("no responsibility matches this query");
      await expect(table.getByRole("row")).toHaveCount(2);
      await expect(
        table.getByRole("cell", { name: "没有可显示的数据行。", exact: true }),
      ).toBeVisible();
      await expect(generated).toHaveCount(0);
      await expect(
        report.getByText("Selected evidence rows", { exact: true }).locator(".."),
      ).toContainText("1");

      const selectionHeader = table.getByRole("columnheader", { name: "选择", exact: true });
      await expect(selectionHeader).toBeVisible();
      const label = await selectionHeader.evaluate((header) => {
        const walker = document.createTreeWalker(header, NodeFilter.SHOW_TEXT);
        for (let node = walker.nextNode(); node; node = walker.nextNode()) {
          const start = node.textContent!.indexOf("选择");
          if (start < 0) continue;
          const range = document.createRange();
          range.setStart(node, start);
          range.setEnd(node, start + "选择".length);
          const rects = [...range.getClientRects()].map(({ left, top, width, height }) => ({
            left,
            top,
            width,
            height,
          }));
          const positive = rects.filter((rect) => rect.width > 0 && rect.height > 0);
          const lowestTop = Math.max(...positive.map((rect) => rect.top));
          const highestBottom = Math.min(...positive.map((rect) => rect.top + rect.height));
          return {
            rects,
            oneLine:
              positive.length > 0 && positive.length === rects.length && lowestTop < highestBottom,
          };
        }
        return { rects: [], oneLine: false };
      });
      await test.info().attach("chinese-selection-header-geometry", {
        body: JSON.stringify(label, null, 2),
        contentType: "application/json",
      });
      const emptySelection = test.info().outputPath(`genui-zh-${theme}-${width}-selection.png`);
      await tableRegion.screenshot({ path: emptySelection });
      await test.info().attach("chinese-empty-selection-table", { path: emptySelection });
      expect(label.oneLine).toBe(true);

      await report.getByRole("button", { name: "清除搜索", exact: true }).click();
      await expect(search).toBeFocused();
      await expect(search).toHaveValue("");
      await expect(generated).toBeChecked();
      await expect(composer).toHaveValue("保留这条草稿，直到筛选结束。");
    });
  }
}

for (const theme of ["light", "dark"] as const) {
  test(`Chromium keeps the focused final sort control revealed in ${theme}`, async ({ page }) => {
    await keepFinalSortRevealed(page, theme);
  });
  test(`Chromium respects manual horizontal reading in ${theme}`, async ({ page }) => {
    await respectHorizontalReading(page, theme);
  });
}

type SidebarState = "expanded" | "collapsed";

const SIDEBAR_PROFILES = [
  {
    locale: "en",
    theme: "light",
    fontSize: 14,
    sidebar: "expanded",
    composer: "Message composer",
    draft: "Keep this draft while the sidebar moves.",
    suffix: " Then continue typing.",
  },
  {
    locale: "zh",
    theme: "dark",
    fontSize: 18,
    sidebar: "collapsed",
    composer: "消息输入框",
    draft: "侧栏移动时保留这条草稿。",
    suffix: "继续输入。",
  },
] as const;

for (const profile of SIDEBAR_PROFILES) {
  test(`the Agent sidebar toggles and keeps its committed width in ${profile.locale} ${profile.theme}`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width: 1120, height: 900 });
    const query = new URLSearchParams({
      fixture: "agent",
      state: "idle",
      motion: "off",
      locale: profile.locale,
      theme: profile.theme,
      "font-size": String(profile.fontSize),
    });
    if (profile.sidebar === "collapsed") query.set("sidebar", "collapsed");
    await page.goto(`/lab/fixture.html?${query}`);
    await expectLabReady(page);

    const shell = page.locator(".agent-shell");
    const toggleFor = (state: SidebarState) =>
      page.getByRole("button", {
        name:
          state === "expanded"
            ? "Collapse the agent fixture sidebar"
            : "Expand the agent fixture sidebar",
        exact: true,
      });
    const expectSidebar = async (state: SidebarState) => {
      await expect(shell).toHaveAttribute("data-sidebar", state);
      await expect(toggleFor(state)).toBeVisible();
      await expect(toggleFor(state)).toHaveAttribute("aria-expanded", String(state === "expanded"));
      await expect(toggleFor(state === "expanded" ? "collapsed" : "expanded")).toHaveCount(0);
    };
    const expectVisibleFocus = async (state: SidebarState) => {
      await expect(toggleFor(state)).toBeFocused();
      expect(await toggleFor(state).evaluate((node) => node.matches(":focus-visible"))).toBe(true);
    };
    const sidebarWidth = () =>
      shell.evaluate((node) => getComputedStyle(node).getPropertyValue("--sidebar-width").trim());
    const capture = async (name: string) =>
      testInfo.attach(name, {
        body: await page.screenshot({ animations: "disabled" }),
        contentType: "image/png",
      });

    const composer = page.getByRole("textbox", { name: profile.composer, exact: true });
    await composer.fill(profile.draft);
    const composerNode = await composer.elementHandle();

    const toggled = profile.sidebar === "expanded" ? "collapsed" : "expanded";
    await expectSidebar(profile.sidebar);
    await capture("initial-agent-sidebar");
    await toggleFor(profile.sidebar).focus();
    await page.keyboard.press("Enter");
    await expectSidebar(toggled);
    await expectVisibleFocus(toggled);
    await page.keyboard.press("Space");
    await expectSidebar(profile.sidebar);
    await expectVisibleFocus(profile.sidebar);
    if (profile.sidebar === "collapsed") {
      await page.keyboard.press("Enter");
      await expectSidebar("expanded");
    }

    const separator = page.getByRole("separator", {
      name: "Resize the agent fixture sidebar",
      exact: true,
    });
    await separator.focus();
    await page.keyboard.press("End");
    await expect(separator).toHaveAttribute("aria-valuenow", "520");
    await expect.poll(sidebarWidth).toBe("520px");

    await toggleFor("expanded").click();
    await expectSidebar("collapsed");
    await expect(composer).toHaveValue(profile.draft);

    await page.setViewportSize({ width: 390, height: 900 });
    await expectSidebar("collapsed");
    await expect(composer).toHaveValue(profile.draft);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth - innerWidth),
    ).toBeLessThanOrEqual(1);
    await capture("narrow-collapsed-agent-sidebar");

    await toggleFor("collapsed").focus();
    await page.keyboard.press("Enter");
    await expectSidebar("expanded");
    await expect(composer).toHaveValue(profile.draft);
    await capture("narrow-expanded-agent-sidebar");

    await expect(toggleFor("expanded")).toBeFocused();
    await page.keyboard.press("Enter");
    await expectSidebar("collapsed");
    await page.setViewportSize({ width: 1120, height: 900 });
    await toggleFor("collapsed").click();
    await expectSidebar("expanded");
    await expect(separator).toHaveAttribute("aria-valuenow", "520");
    await expect.poll(sidebarWidth).toBe("520px");

    expect(await composer.evaluate((node, original) => node === original, composerNode)).toBe(true);
    await composer.focus();
    await page.keyboard.press("End");
    await page.keyboard.type(profile.suffix);
    await expect(composer).toHaveValue(profile.draft + profile.suffix);
    await capture("retained-expanded-agent-sidebar");
  });
}
