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
    const dialog = page.getByRole("dialog", { name: "Configure inputs", exact: true });
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
      await page.setViewportSize({ width, height: 568 });
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
      await field.fill("invalid JSON");
      await submit.click();
      await expect(dialog.getByRole("alert")).toBeVisible();
      await expect(field).toHaveValue("invalid JSON");
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
          scrolls: body.scrollHeight > body.clientHeight,
          headOutsideBody: head.bottom <= body.getBoundingClientRect().top,
          actionInFooter: action.top >= footer.top && action.bottom <= footer.bottom,
        };
      });
      expect(dimensions.left).toBeGreaterThanOrEqual(16);
      expect(dimensions.right).toBeLessThanOrEqual(width - 16);
      expect(dimensions.top).toBeGreaterThanOrEqual(16);
      expect(dimensions.bottom).toBeLessThanOrEqual(552);
      expect(dimensions.overflow).toBe(0);
      expect(dimensions.headOutsideBody).toBe(true);
      expect(dimensions.actionInFooter).toBe(true);
      if (width === 320) expect(dimensions.scrolls).toBe(true);
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
  test(`schedule editor groups task, timing and execution without escaping ${width}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      "/visual/?fixture=workspace&state=full-view&full-view=package:visual:schedules&theme=light&font-size=18",
    );
    await page.locator("html[data-visual-ready]").waitFor();
    await page.getByRole("button", { name: "Add schedule", exact: true }).click();
    const editor = page.getByRole("region", { name: "Schedule editor", exact: true });
    await expect(editor.getByRole("group", { name: "Task", exact: true })).toBeVisible();
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
  });
}

for (const width of [1120, 320]) {
  test(`objective editor keeps the footer visible while its body scrolls ${width}`, async ({
    page,
  }) => {
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
    const body = dialog.locator('[data-slot="dialog-body"]');
    expect(await body.evaluate((node) => node.scrollHeight > node.clientHeight)).toBe(true);
    await body.evaluate((node) => {
      node.scrollTop = node.scrollHeight;
    });
    const actions = await dialog.locator("footer").boundingBox();
    expect(actions!.y + actions!.height).toBeLessThanOrEqual(484);
    const content = await dialog.boundingBox();
    expect(content!.x).toBeGreaterThanOrEqual(16);
    expect(content!.x + content!.width).toBeLessThanOrEqual(width - 16);
    await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(dialog).toHaveCount(0);
  });
}
