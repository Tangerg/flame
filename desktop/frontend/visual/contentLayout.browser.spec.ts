import { expect, test } from "./test";

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
    await viewport.evaluate((node) => {
      node.scrollTop = 600;
    });
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
