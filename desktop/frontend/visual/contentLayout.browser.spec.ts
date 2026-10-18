import { expect, test } from "./test";
import { REVIEW_DIFF } from "./diffSnapshot";

for (const theme of ["light", "dark"]) {
  test(`command search keeps its query and last option inside a short viewport in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 900, height: 360 });
    await page.goto(
      `/visual/?fixture=shell&state=populated&overlay=commands&theme=${theme}&font-size=18`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    await page.locator("html").evaluate((root) => {
      root.style.setProperty("--layout-compact-gutter", "24px");
    });
    const dialog = page.getByRole("dialog", { name: "Open command menu", exact: true });
    const query = dialog.getByRole("combobox");
    await expect(query).toBeFocused();
    await query.press("ArrowUp");
    const last = dialog.getByRole("option").last();
    await expect(last).toHaveAttribute("aria-selected", "true");
    const bounds = (await dialog.boundingBox())!;
    const lastBounds = (await last.boundingBox())!;
    const queryBounds = (await query.boundingBox())!;
    expect(bounds.y).toBeGreaterThanOrEqual(24);
    expect(bounds.y + bounds.height).toBeLessThanOrEqual(336);
    expect(lastBounds.y + lastBounds.height).toBeLessThanOrEqual(bounds.y + bounds.height);
    expect(queryBounds.y).toBeGreaterThanOrEqual(bounds.y);
    expect(queryBounds.y + queryBounds.height).toBeLessThanOrEqual(lastBounds.y);
    await query.press("ArrowDown");
    await expect(dialog.getByRole("option").first()).toHaveAttribute("aria-selected", "true");
    await query.press("Escape");
    await expect(dialog).toHaveCount(0);
  });
}

for (const overlay of ["commands", "finder"]) {
  test(`${overlay} search reveals its current result after wheel scrolling and filtering`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 560, height: 360 });
    await page.goto(
      `/visual/?fixture=shell&state=populated&overlay=${overlay}&theme=dark&font-size=18`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    const dialog = page.getByRole("dialog");
    const query = dialog.getByRole("combobox");
    const list = dialog.getByRole("listbox");
    const first = dialog.getByRole("option").first();
    await expect(query).toBeFocused();
    await first.hover();
    await expect(first).toHaveAttribute("aria-selected", "true");
    await page.mouse.wheel(0, 1000);
    await expect.poll(() => list.evaluate((node) => node.scrollTop)).toBeGreaterThan(0);
    await expect(first).not.toBeInViewport();

    await query.fill("e");
    await expect(first).toHaveAttribute("aria-selected", "true");
    await expect(query).toHaveAttribute("aria-activedescendant", (await first.getAttribute("id"))!);
    await expect(first).toBeInViewport({ ratio: 1 });
    await expect(query).toBeFocused();
    await query.fill("");
    await expect(first).toHaveAttribute("aria-selected", "true");
    await expect(first).toBeInViewport({ ratio: 1 });
    await query.press("Escape");
    await expect(dialog).toHaveCount(0);
  });
}

for (const theme of ["light", "dark"]) {
  test(`Diff keeps comparison and file controls reachable while the report scrolls in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1472, height: 900 });
    await page.goto(
      `/visual/?fixture=workspace&state=dock-review&theme=${theme}&font-size=18&review-files=200`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    const view = page.locator('[data-dock-view-id="package:visual:diff"]');
    const filter = view.getByRole("searchbox", { name: "Filter files", exact: true });
    await expect(filter).toBeVisible();
    const viewport = view.locator('[data-slot="workspace-view-body"]');
    const code = view.getByRole("region", {
      name: "Scrollable diff for src/generated/module-000.ts",
      exact: true,
    });
    const bounds = (await code.boundingBox())!;
    await page.mouse.move(bounds.x + bounds.width / 2, bounds.y + 40);
    await page.mouse.wheel(0, 600);
    await expect.poll(() => viewport.evaluate((node) => node.scrollTop)).toBe(600);
    for (const control of [
      filter,
      view.getByRole("combobox", { name: "Comparison", exact: true }),
      view.getByRole("combobox", { name: "Layout", exact: true }),
      view.getByRole("button", { name: "Refresh", exact: true }),
      view.getByRole("button", { name: "Collapse all", exact: true }),
    ]) {
      await expect(control).toBeInViewport({ ratio: 1 });
    }
    await filter.fill("module-199.ts");
    await expect(view.locator("[data-diff-file]")).toHaveCount(1);
    await expect(view.locator("[data-diff-file]")).toHaveAttribute(
      "data-diff-file",
      "src/generated/module-199.ts",
    );
    const file = view.locator("[data-diff-file]");
    const summary = file.getByRole("button", { expanded: true }).first();
    await expect(summary).toContainText("src/generated/module-199.ts");
    expect((await summary.innerText()).split("module-199.ts")).toHaveLength(2);
    await view.getByRole("button", { name: "Collapse all", exact: true }).click();
    await expect(file.getByRole("button", { expanded: false }).first()).toBeVisible();
    await view.getByRole("button", { name: "Expand all", exact: true }).click();
    await expect(file.getByRole("button", { expanded: true }).first()).toBeVisible();
  });

  test(`Diff preserves source columns across row kinds and horizontal scrolling in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1120, height: 720 });
    await page.goto(
      `/visual/?fixture=workspace&state=dock-review&theme=${theme}&font-size=${theme === "light" ? 14 : 18}`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    const view = page.locator('[data-dock-view-id="package:visual:diff"]');
    const source = REVIEW_DIFF.files![0]!.rows.filter((row) => row.type !== "hunk");
    for (const layout of ["unified", "split"] as const) {
      if (layout === "split") {
        await view.getByRole("combobox", { name: "Layout", exact: true }).click();
        await page.getByRole("option", { name: "Side by side", exact: true }).click();
      }
      const code = view
        .locator("[data-diff-file]")
        .first()
        .getByRole("region", { name: /^Scrollable diff for / });
      await expect(code.locator("table")).toHaveAttribute("data-layout", layout);
      for (const offset of [0, 80]) {
        await code.evaluate((node, offset) => {
          node.scrollLeft = offset;
        }, offset);
        const rendered = await code.locator("tbody tr").evaluateAll((rows) =>
          rows
            .filter((row) => row.querySelector("code"))
            .map((row) =>
              Array.from(row.querySelectorAll("code"), (code) => {
                const range = document.createRange();
                range.selectNodeContents(code);
                return { text: code.textContent, left: range.getBoundingClientRect().left };
              }),
            ),
        );
        expect(rendered.map((row) => row.map((cell) => cell.text))).toEqual(
          source.map((row) =>
            layout === "split"
              ? [row.type === "added" ? "" : row.code, row.type === "deleted" ? "" : row.code]
              : [row.code],
          ),
        );
        for (const row of rendered) {
          for (const [column, cell] of row.entries()) {
            if (cell.text) {
              expect(Math.abs(cell.left - rendered[0]![column]!.left)).toBeLessThan(0.5);
            }
          }
        }
        if (offset) {
          expect(await code.evaluate((node) => node.scrollLeft)).toBeGreaterThan(0);
        }
      }
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  });

  test(`a narrow file dock preserves its identity and wraps actions in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1472, height: 900 });
    await page.goto(`/visual/?fixture=workspace&state=dock-files&theme=${theme}&font-size=18`);
    await page.locator("html[data-visual-ready]").waitFor();
    await page.getByRole("treeitem", { name: "README.md", exact: true }).click();
    const view = page.locator('[data-dock-view-id="file"]');
    await expect(view.getByRole("tab", { name: "Preview", exact: true })).toBeVisible();
    const resizer = page.locator(".agent-pane-resizer");
    await resizer.focus();
    await resizer.press("Home");
    const header = view.locator('[data-slot="workspace-view-header"]');
    const identity = header.getByTitle("README.md", { exact: true });
    await expect(identity).toBeVisible();
    const bounds = (await header.boundingBox())!;
    const text = (await identity.boundingBox())!;
    expect(text.width).toBeGreaterThan(80);
    expect(text.y).toBeGreaterThanOrEqual(bounds.y);
    expect(text.x + text.width).toBeLessThanOrEqual(bounds.x + bounds.width);
    const escaped = await header.locator('button,[role="tab"]').evaluateAll((nodes) =>
      nodes
        .filter((node) => {
          const rect = node.getBoundingClientRect();
          const host = node.closest('[data-slot="workspace-view-header"]')!.getBoundingClientRect();
          return (
            rect.width > 0 &&
            (rect.x < host.x || rect.right > host.right || rect.bottom > host.bottom)
          );
        })
        .map((node) => node.getAttribute("aria-label") ?? node.textContent),
    );
    expect(escaped).toEqual([]);
    await view.getByRole("tab", { name: "Source", exact: true }).click();
    await expect(view.getByRole("tab", { name: "Source", exact: true })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await view.getByRole("tab", { name: "Preview", exact: true }).click();
    await expect(view.locator('[data-slot="document-body"]')).toContainText("Workspace notes");
  });

  test(`question options expose their distinguishing tails before selection in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 320, height: 720 });
    await page.goto(`/visual/?fixture=agent&state=question-multi&theme=${theme}&font-size=18`);
    await page.locator("html[data-visual-ready]").waitFor();
    const labels = [
      "Run all release checks for the selected workspace and include concurrency diagnostics",
      "Run all release checks for the selected workspace and include accessibility diagnostics",
    ] as const;
    for (const label of labels) {
      const choice = page.getByRole("checkbox", { name: label, exact: true });
      const text = choice.getByText(label, { exact: true });
      await text.scrollIntoViewIfNeeded();
      const overflow = await text.evaluate((node) => ({
        x: node.scrollWidth - node.clientWidth,
        y: node.scrollHeight - node.clientHeight,
      }));
      expect(overflow.x, label).toBeLessThanOrEqual(1);
      expect(overflow.y, label).toBeLessThanOrEqual(1);
      const target = (await choice.boundingBox())!;
      const content = (await text.boundingBox())!;
      expect(content.x).toBeGreaterThanOrEqual(target.x);
      expect(content.x + content.width).toBeLessThanOrEqual(target.x + target.width);
      expect(content.y + content.height).toBeLessThanOrEqual(target.y + target.height);
    }
    const last = page.getByRole("checkbox", { name: labels[1], exact: true });
    await last.click();
    await expect(last).toHaveAttribute("aria-checked", "true");
    await page.getByRole("button", { name: "Next", exact: true }).click();
    await expect(page.locator("html")).toHaveAttribute(
      "data-visual-resumed-response",
      JSON.stringify({ type: "answer", answers: [[labels[1]]] }),
    );
  });
}

test("captured Markdown tables let the file reading viewport continue scrolling", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1120, height: 720 });
  await page.goto("/visual/?fixture=workspace&state=dock-files&theme=dark&font-size=18");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.getByRole("treeitem", { name: "reading-guide.md", exact: true }).click();
  const view = page.locator('[data-dock-view-id="file"]');
  const table = view.getByRole("region", { name: /Markdown table/ }).first();
  await table.scrollIntoViewIfNeeded();
  const position = () =>
    table.evaluate((node) => {
      for (let parent = node.parentElement; parent; parent = parent.parentElement)
        if (
          parent.scrollHeight > parent.clientHeight &&
          ["auto", "scroll"].includes(getComputedStyle(parent).overflowY)
        )
          return { top: parent.scrollTop, range: parent.scrollHeight - parent.clientHeight };
      throw new Error("No vertical file reading viewport");
    });
  const before = await position();
  expect(before.range).toBeGreaterThan(0);
  const bounds = (await table.boundingBox())!;
  await page.mouse.move(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2);
  await page.mouse.wheel(0, before.top > 0 ? -240 : 240);
  await expect.poll(async () => (await position()).top).not.toBe(before.top);
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
});

test("Diff file headings retain reading context without covering focused code", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1120, height: 720 });
  await page.goto(
    "/visual/?fixture=workspace&state=dock-review&theme=dark&font-size=18&review-files=3",
  );
  await page.locator("html[data-visual-ready]").waitFor();
  const view = page.locator('[data-dock-view-id="package:visual:diff"]');
  const viewport = view.locator('[data-slot="workspace-view-body"]');
  const controls = view.locator('[data-slot="panel-controls"]');
  const files = view.locator("[data-diff-file]");
  const first = files.nth(0).getByRole("button", { expanded: true }).first();
  const firstCode = files.nth(0).getByRole("region", { name: /^Scrollable diff for / });
  const bounds = (await firstCode.boundingBox())!;
  await page.mouse.move(bounds.x + bounds.width / 2, bounds.y + 40);
  await page.mouse.wheel(0, 600);
  await expect.poll(() => viewport.evaluate((node) => node.scrollTop)).toBe(600);
  const controlsBottom = (await controls.boundingBox())!.y + (await controls.boundingBox())!.height;
  await expect(first).toBeInViewport({ ratio: 1 });
  expect((await first.boundingBox())!.y).toBeCloseTo(controlsBottom, 0);

  await first.focus();
  await first.press("Tab");
  await expect(firstCode).toBeFocused();
  const titleBottom = (await first.boundingBox())!.y + (await first.boundingBox())!.height;
  await expect
    .poll(async () => (await firstCode.locator("tbody tr").first().boundingBox())!.y)
    .toBeGreaterThanOrEqual(titleBottom);
  await page.keyboard.down("ArrowRight");
  try {
    await expect.poll(() => firstCode.evaluate((node) => node.scrollLeft)).toBeGreaterThan(0);
  } finally {
    await page.keyboard.up("ArrowRight");
  }

  const second = files.nth(1).getByRole("button", { expanded: true }).first();
  await viewport.evaluate((node) => {
    const file = node.querySelectorAll("[data-diff-file]")[1]!;
    const controls = node.querySelector('[data-slot="panel-controls"]')!;
    node.scrollTop +=
      file.getBoundingClientRect().top - controls.getBoundingClientRect().bottom + 100;
  });
  await expect(second).toBeInViewport({ ratio: 1 });
  expect((await second.boundingBox())!.y).toBeCloseTo(controlsBottom, 0);
  expect((await first.boundingBox())!.y + (await first.boundingBox())!.height).toBeLessThanOrEqual(
    controlsBottom,
  );
  await second.click();
  await expect(files.nth(1).getByRole("button", { expanded: false }).first()).toBeVisible();
  await expect(files.nth(1).getByRole("region", { name: /^Scrollable diff for / })).toHaveCount(0);
  const third = files.nth(2).getByRole("button", { expanded: true }).first();
  await third.scrollIntoViewIfNeeded();
  await expect(third).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
});

test("Diff headings remain in flow when their complete path would crowd the reading viewport", async ({
  page,
}) => {
  const longPath = `node_modules/${"layout-boundary/".repeat(32)}captured-source.ts`;
  for (const plan of [
    { width: 320, height: 360, path: undefined },
    { width: 1120, height: 720, path: longPath },
  ]) {
    await page.setViewportSize({ width: plan.width, height: plan.height });
    await page.goto(
      `/visual/?fixture=workspace&state=dock-review&theme=dark&font-size=18${plan.path ? `&review-path=${encodeURIComponent(plan.path)}` : ""}`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    if (plan.width === 320) {
      await page.getByRole("button", { name: /^Open material full width/ }).click();
      await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
    }
    const viewport = page
      .locator('[data-slot="workspace-view-body"]:visible')
      .filter({ has: page.locator("[data-diff-file]") })
      .first();
    const file = viewport.locator("[data-diff-file]").first();
    const title = file.getByRole("button", { expanded: true }).first();
    if (plan.path) {
      await expect(title).toContainText(plan.path);
      expect((await title.innerText()).split(plan.path)).toHaveLength(2);
    }
    const code = file.getByRole("region", { name: /^Scrollable diff for / });
    await code.focus();
    const controls = (await viewport.locator('[data-slot="panel-controls"]').boundingBox())!;
    const row = code.locator("tbody tr").first();
    const rowBounds = (await row.boundingBox())!;
    const viewportBounds = (await viewport.boundingBox())!;
    expect(rowBounds.y).toBeGreaterThanOrEqual(controls.y + controls.height);
    expect(rowBounds.y + rowBounds.height).toBeLessThanOrEqual(
      viewportBounds.y + viewportBounds.height,
    );
    expect((await title.boundingBox())!.y).toBeLessThan(controls.y + controls.height);
    if (plan.width === 320) {
      await page.setViewportSize({ width: 320, height: 900 });
      await expect(title).toBeInViewport({ ratio: 1 });
      const titleBounds = (await title.boundingBox())!;
      await expect
        .poll(async () => (await row.boundingBox())!.y)
        .toBeGreaterThanOrEqual(titleBounds.y + titleBounds.height);
      await expect(code).toBeFocused();
      await page.setViewportSize({ width: 320, height: 360 });
      await expect
        .poll(async () => {
          const rowBounds = (await row.boundingBox())!;
          const viewportBounds = (await viewport.boundingBox())!;
          return rowBounds.y + rowBounds.height <= viewportBounds.y + viewportBounds.height;
        })
        .toBe(true);
      const controlsBounds = (await viewport
        .locator('[data-slot="panel-controls"]')
        .boundingBox())!;
      await expect
        .poll(async () => (await title.boundingBox())!.y)
        .toBeLessThan(controlsBounds.y + controlsBounds.height);
      await expect(code).toBeFocused();
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  }
});

for (const theme of ["light", "dark"]) {
  test(`a horizontal-only code well joins the keyboard scroll path in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1472, height: 900 });
    await page.goto(`/visual/?fixture=atoms&theme=${theme}&font-size=18`);
    await page.locator("html[data-visual-ready]").waitFor();
    const specimen = page.locator('[data-atom="well-path"]');
    const well = specimen.locator("pre").last();
    await well.scrollIntoViewIfNeeded();
    const overflow = await well.evaluate((node) => ({
      horizontal: node.scrollWidth - node.clientWidth,
      vertical: node.scrollHeight - node.clientHeight,
    }));
    expect(overflow.horizontal).toBeGreaterThan(0);
    expect(overflow.vertical).toBeLessThanOrEqual(1);
    await expect(well).toHaveAttribute("tabindex", "0");
    await specimen.getByRole("button", { name: "Before long JSON", exact: true }).focus();
    await page.keyboard.press("Tab");
    await expect(well).toBeFocused();
    await page.keyboard.down("ArrowRight");
    try {
      await expect.poll(() => well.evaluate((node) => node.scrollLeft)).toBeGreaterThan(0);
    } finally {
      await page.keyboard.up("ArrowRight");
    }
  });
}
