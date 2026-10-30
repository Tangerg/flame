import { expect, test } from "./test";
import AxeBuilder from "@axe-core/playwright";
for (const theme of ["light", "dark"]) {
  test(`plugin dialogs finish painting with full motion ${theme}`, async ({ page }) => {
    await page.emulateMedia({ reducedMotion: "no-preference" });
    await page.goto(
      `/visual/?fixture=workspace&state=settings&pane=plugins&theme=${theme}&motion=full`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    await page.getByRole("button", { name: "Configure inputs", exact: true }).first().click();
    const dialog = page.getByRole("dialog", { name: /^Configure inputs: / });
    await expect(dialog).toBeVisible();
    await expect(dialog).toHaveCSS("opacity", "1");
    await expect(dialog).toHaveCSS("scale", "none");
    const isFrontmost = await dialog.evaluate((node) => {
      const box = node.getBoundingClientRect();
      return node.contains(
        document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2),
      );
    });
    expect(isFrontmost).toBe(true);
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
  });
}

for (const theme of ["light", "dark"]) {
  for (const width of [1120, 320]) {
    test(`JSON import keeps its input, errors and actions reachable ${theme} ${width}`, async ({
      page,
    }) => {
      const height = width === 320 ? 420 : 568;
      await page.setViewportSize({ width, height });
      await page.goto(
        `/visual/?fixture=workspace&state=settings&pane=mcp-servers&theme=${theme}&font-size=18`,
      );
      await page.locator("html[data-visual-ready]").waitFor();
      const trigger = page.getByRole("button", { name: "Import from JSON", exact: true });
      await trigger.click();
      const dialog = page.getByRole("dialog", { name: "Import from JSON", exact: true });
      const field = dialog.getByRole("textbox", { name: "JSON configuration", exact: true });
      const submit = dialog.getByRole("button", { name: "Import", exact: true });
      await expect(field).toBeFocused();
      await expect(submit).toBeDisabled();
      const invalid = "invalid JSON\n".repeat(100);
      await field.fill(invalid);
      await submit.click();
      await expect(dialog.getByRole("alert")).toBeVisible();
      await expect(field).toHaveValue(invalid);
      await expect(field).toHaveAttribute("aria-invalid", "true");
      await field.scrollIntoViewIfNeeded();
      const dimensions = await dialog.evaluate((node) => {
        const box = node.getBoundingClientRect();
        const body = node.querySelector('[data-slot="dialog-body"]')!;
        const footer = node.querySelector("footer")!.getBoundingClientRect();
        const head = node.querySelector("header")!.getBoundingClientRect();
        const action = node
          .querySelector<HTMLButtonElement>('button[type="submit"]')!
          .getBoundingClientRect();
        return {
          left: box.left,
          right: box.right,
          top: box.top,
          bottom: box.bottom,
          overflow: node.scrollWidth - node.clientWidth,
          bodyOverflow: getComputedStyle(body).overflowY,
          editorScrolls:
            node.querySelector("textarea")!.scrollHeight >
            node.querySelector("textarea")!.clientHeight,
          headOutsideBody: head.bottom <= body.getBoundingClientRect().top,
          actionInFooter: action.top >= footer.top && action.bottom <= footer.bottom,
        };
      });
      expect(dimensions.left).toBeGreaterThanOrEqual(16);
      expect(dimensions.right).toBeLessThanOrEqual(width - 16);
      expect(dimensions.top).toBeGreaterThanOrEqual(16);
      expect(dimensions.bottom).toBeLessThanOrEqual(height - 16);
      expect(dimensions.overflow).toBe(0);
      expect(dimensions.headOutsideBody).toBe(true);
      expect(dimensions.actionInFooter).toBe(true);
      expect(dimensions.bodyOverflow).toBe("auto");
      expect(dimensions.editorScrolls).toBe(true);
      const accessibility = await new AxeBuilder({ page })
        .include('[role="dialog"]')
        .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
        .analyze();
      expect(accessibility.violations).toEqual([]);
      await page.keyboard.press("Escape");
      await expect(dialog).toHaveCount(0);
      await expect(trigger).toBeFocused();
    });
  }
}

for (const width of [1120, 320]) {
  test(`schedule editor keeps task fields and timing and execution groups within its dialog ${width}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      "/visual/?fixture=workspace&state=full-view&full-view=package:visual:schedules&theme=light&font-size=18",
    );
    await page.locator("html[data-visual-ready]").waitFor();
    const add = page.getByRole("button", { name: "Add schedule", exact: true });
    await add.click();
    const editor = page.getByRole("dialog", { name: "New schedule", exact: true });
    const title = editor.getByRole("textbox", { name: "Title", exact: true });
    await expect(title).toBeFocused();
    const heading = (await editor
      .getByRole("heading", { name: "New schedule", exact: true })
      .boundingBox())!;
    expect((await title.boundingBox())!.y).toBeGreaterThanOrEqual(heading.y + heading.height);
    await expect(editor.getByRole("textbox", { name: "Instructions", exact: true })).toBeVisible();
    await expect(
      editor.getByRole("group", { name: "Timing and workspace", exact: true }),
    ).toBeVisible();
    await expect(editor.getByRole("group", { name: "Execution", exact: true })).toBeVisible();
    const escaped = await editor.evaluate((node) => {
      const box = node.getBoundingClientRect();
      return [
        ...node.querySelectorAll(
          "button,input:not([type=hidden]):not([aria-hidden=true]),textarea",
        ),
      ]
        .filter((control) => {
          const bounds = control.getBoundingClientRect();
          return bounds.width > 0 && (bounds.left < box.left || bounds.right > box.right);
        })
        .map((control) => control.getAttribute("aria-label") ?? control.textContent);
    });
    expect(escaped).toEqual([]);
    await expect(editor.getByRole("button", { name: "Save schedule", exact: true })).toBeDisabled();
    await editor.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(editor).toHaveCount(0);
    await expect(add).toBeFocused();
  });
}

test("long panels keep one filter zone reachable and remeasure it after wrapping", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1120, height: 720 });
  await page.goto(
    "/visual/?fixture=workspace&state=full-view&full-view=package:visual:icon-gallery&theme=light&font-size=18",
  );
  await page.locator("html[data-visual-ready]").waitFor();
  const panel = page.getByRole("region", { name: "Flame icon gallery", exact: true });
  const viewport = page.locator('[data-slot="workspace-view-body"]').filter({ has: panel });
  const controls = panel.locator('[data-slot="panel-controls"]');
  const windowHeader = page.locator('[data-slot="workspace-window-header"]');
  await expect(controls).toHaveCount(1);
  expect(await viewport.evaluate((node) => node.scrollHeight > node.clientHeight)).toBe(true);
  const height = () => controls.evaluate((node) => node.getBoundingClientRect().height);
  const projected = () =>
    panel.evaluate((node) =>
      parseFloat(getComputedStyle(node).getPropertyValue("--panel-controls-height")),
    );
  const wide = await height();
  await viewport.evaluate((node) => {
    node.scrollTop = node.scrollHeight;
  });
  await expect(controls).toBeInViewport({ ratio: 1 });
  const headerBounds = (await windowHeader.boundingBox())!;
  expect((await controls.boundingBox())!.y).toBeGreaterThanOrEqual(
    headerBounds.y + headerBounds.height,
  );
  await expect(panel.getByRole("searchbox", { name: "Filter icons" })).toBeInViewport({
    ratio: 1,
  });
  await expect(panel.getByRole("heading", { level: 1 })).not.toBeInViewport();
  await page.setViewportSize({ width: 320, height: 720 });
  await expect.poll(height).toBeGreaterThan(wide);
  await expect.poll(projected).toBe(await height());
  await expect(controls).toBeInViewport({ ratio: 1 });
  const narrowHeader = (await windowHeader.boundingBox())!;
  expect((await controls.boundingBox())!.y).toBeGreaterThanOrEqual(
    narrowHeader.y + narrowHeader.height,
  );
  await page.goto(
    "/visual/?fixture=workspace&state=full-view&full-view=package:visual:trajectory&theme=light&font-size=18",
  );
  await page.locator("html[data-visual-ready]").waitFor();
  const trajectory = page.getByRole("region", { name: "Session trajectory", exact: true });
  const anchor = trajectory.getByRole("region", { name: "Trajectory observations", exact: true });
  await anchor.evaluate((node) => node.scrollIntoView({ block: "start" }));
  const bottom = (await trajectory.locator('[data-slot="panel-controls"]').boundingBox())!;
  expect((await anchor.boundingBox())!.y).toBeGreaterThanOrEqual(bottom.y + bottom.height);
});

test("a menu-opened memory confirmation returns focus without changing the record", async ({
  page,
}) => {
  await page.goto(
    "/visual/?fixture=workspace&state=full-view&full-view=package:visual:memory&theme=light",
  );
  await page.locator("html[data-visual-ready]").waitFor();
  const records = page.getByRole("region", { name: "Recorded memory", exact: true });
  const trigger = records.getByRole("button", { name: /^More actions for / });
  await trigger.click();
  await page.getByRole("menuitem", { name: "Delete", exact: true }).click();
  const confirm = page.getByRole("region", { name: "Confirm memory deletion", exact: true });
  const cancel = confirm.getByRole("button", { name: "Cancel", exact: true });
  await expect(cancel).toBeFocused();
  await cancel.press("Enter");
  await expect(confirm).toHaveCount(0);
  await expect(trigger).toBeFocused();
  await expect(records.getByRole("article")).toHaveCount(2);
});

for (const width of [1120, 320]) {
  test(`objective editor keeps long input and its footer reachable ${width}`, async ({ page }) => {
    await page.setViewportSize({ width, height: 500 });
    await page.goto("/visual/?fixture=agent&theme=dark&state=running&font-size=18");
    await page.locator("html[data-visual-ready]").waitFor();
    await page.getByRole("button", { name: "Edit goal", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "Edit goal", exact: true });
    const field = dialog.getByRole("textbox", { name: "Goal", exact: true });
    await expect(field).toBeFocused();
    expect(
      await field.evaluate((node) => (node as HTMLTextAreaElement).form?.getAttribute("role")),
    ).toBe("dialog");
    await field.fill("Review the workspace while preserving accepted work.\n".repeat(1000));
    expect(await field.evaluate((node) => node.scrollHeight > node.clientHeight)).toBe(true);
    await field.evaluate((node) => {
      node.scrollTop = node.scrollHeight;
    });
    await expect(field).toBeInViewport({ ratio: 1 });
    for (const button of await dialog.locator("footer").getByRole("button").all())
      await expect(button).toBeInViewport({ ratio: 1 });
    const actions = await dialog.locator("footer").boundingBox();
    expect(actions!.y + actions!.height).toBeLessThanOrEqual(484);
    const content = await dialog.boundingBox();
    expect(content!.x).toBeGreaterThanOrEqual(16);
    expect(content!.x + content!.width).toBeLessThanOrEqual(width - 16);
    await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(dialog).toHaveCount(0);
  });
}
