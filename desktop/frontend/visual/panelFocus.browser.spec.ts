import { expect, test, type Locator, type Page } from "./test";

async function openMemory(
  page: Page,
  theme: string,
  { width = 560, height = 360, filters = false } = {},
) {
  await page.setViewportSize({ width, height });
  await page.goto(
    `/visual/?fixture=workspace&state=full-view&full-view=package:visual:memory&font-size=18&theme=${theme}`,
  );
  await page.locator("html[data-visual-ready]").waitFor();
  await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
  await page.evaluate(() => document.fonts.ready);
  const panel = page.getByRole("region", { name: "Agent memory", exact: true });
  if (filters) {
    await panel.getByRole("button", { name: /^Filters\b/ }).click();
    await panel.getByRole("combobox", { name: "Memory status", exact: true }).click();
    await page.getByRole("option", { name: "Pending review", exact: true }).click();
  }
  const add = panel.getByRole("button", { name: "Add memory", exact: true });
  await expect(add).toBeEnabled();
  await add.press("Enter");
  const editor = panel.getByRole("region", { name: "Add memory", exact: true });
  const content = editor.getByRole("textbox", { name: "Memory content", exact: true });
  await expect(content).toBeFocused();
  return { panel, add, editor, content };
}

async function expectFirstLineVisible(content: Locator) {
  const reading = await content.evaluate((element) => {
    const field = element as HTMLTextAreaElement;
    const style = getComputedStyle(field);
    const bounds = field.getBoundingClientRect();
    const controls = field
      .closest('[data-slot="panel"]')!
      .querySelector('[data-slot="panel-controls"]')!
      .getBoundingClientRect();
    const top =
      bounds.top +
      parseFloat(style.borderTopWidth) +
      parseFloat(style.paddingTop) -
      field.scrollTop;
    const bottom = top + parseFloat(style.lineHeight);
    const hit = document.elementFromPoint(bounds.left + bounds.width / 2, (top + bottom) / 2);
    return {
      top,
      bottom,
      controlsBottom: controls.bottom,
      viewportBottom: innerHeight,
      hit: hit === field,
    };
  });
  expect(reading.top).toBeGreaterThanOrEqual(reading.controlsBottom);
  expect(reading.bottom).toBeLessThanOrEqual(reading.viewportBottom);
  expect(reading.hit).toBe(true);
}

for (const theme of ["light", "dark"]) {
  test(`panel body focus keeps typed text above the sticky controls after autofocus and Tab ${theme}`, async ({
    page,
  }) => {
    const { add, editor, content } = await openMemory(page, theme);
    const draft = "Retain this first memory line.";
    await page.keyboard.type(draft);
    await expect(content).toHaveValue(draft);
    await expectFirstLineVisible(content);
    await content.press("Tab");
    const cancel = editor.getByRole("button", { name: "Cancel", exact: true });
    await expect(cancel).toBeFocused();
    await page.keyboard.press("Shift+Tab");
    await expect(content).toBeFocused();
    await expectFirstLineVisible(content);
    await expect(content).toHaveValue(draft);
    await content.press("Tab");
    await cancel.press("Enter");
    await expect(add).toBeFocused();
  });
}

test("visible body and toolbar controls keep their existing scroll position", async ({ page }) => {
  const { panel, editor, content } = await openMemory(page, "light", {
    width: 1120,
    height: 720,
  });
  const viewport = page.locator('[data-slot="workspace-view-body"]').filter({ has: panel });
  const before = await viewport.evaluate((node) => node.scrollTop);
  await content.focus();
  await expectFirstLineVisible(content);
  expect(await viewport.evaluate((node) => node.scrollTop)).toBe(before);
  const search = panel.getByRole("searchbox", { name: "Search this page", exact: true });
  await search.focus();
  await expect(search).toBeFocused();
  expect(await viewport.evaluate((node) => node.scrollTop)).toBe(before);
  await editor.getByRole("button", { name: "Cancel", exact: true }).click();
});

test.describe("compact Memory editing", () => {
  test.use({ hasTouch: true });
  test("opening a Memory editor with expanded filters keeps its first line readable", async ({
    page,
  }) => {
    const { panel, content } = await openMemory(page, "dark", {
      width: 360,
      height: 360,
      filters: true,
    });
    await expect(panel.getByRole("button", { name: /^Filters\b/ })).toContainText("1 active");
    await expect(content).toBeFocused();
    await expectFirstLineVisible(content);
  });
});

test("a body-owned Select portal retains its own focus and scrolling", async ({ page }) => {
  await page.setViewportSize({ width: 560, height: 360 });
  await page.goto(
    "/visual/?fixture=workspace&state=full-view&full-view=package:visual:schedules&font-size=18&theme=dark",
  );
  await page.locator("html[data-visual-ready]").waitFor();
  await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
  const panel = page.getByRole("region", { name: "Schedules", exact: true });
  const add = panel.getByRole("button", { name: "Add schedule", exact: true });
  await expect(add).toBeEnabled();
  await add.click();
  const editor = panel.getByRole("region", { name: "Schedule editor", exact: true });
  await page.evaluate(async () => {
    const modulePath = "/visual/runtimeSnapshots.ts";
    const { VISUAL_MODELS }: typeof import("./runtimeSnapshots") = await import(modulePath);
    const original = VISUAL_MODELS[0]!;
    VISUAL_MODELS.push(
      ...Array.from({ length: 20 }, (_, index) => ({
        ...original,
        id: `panel-focus-${index}`,
        displayName: `Panel focus model ${index}`,
      })),
    );
  });
  const refresh = editor.getByRole("button", { name: "Refresh model choices", exact: true });
  await refresh.click();
  await expect(refresh).toBeEnabled();
  const model = editor.getByRole("combobox", { name: "Model", exact: true });
  const popup = page.getByRole("listbox");
  const last = popup.getByRole("option", { name: "openai / Panel focus model 19", exact: true });
  await model.click();
  await expect(
    popup.getByRole("option", { name: "Runtime default model", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("End");
  await expect(last).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(model).toContainText("Panel focus model 19");
  await model.click();
  await expect(last).toBeFocused();
  await expect(popup.getByRole("option")).toHaveCount(22);
  const candidate = await popup.getByRole("option").evaluateAll((options) => {
    const controls = document
      .querySelector('[data-slot="panel-controls"]')!
      .getBoundingClientRect();
    const popup = options[0]!.closest('[role="listbox"]')!.getBoundingClientRect();
    return options.findIndex((option) => {
      const box = option.getBoundingClientRect();
      return box.top >= popup.top && box.bottom <= popup.bottom && box.top < controls.bottom;
    });
  });
  expect(candidate).toBeGreaterThanOrEqual(0);
  const option = popup.getByRole("option").nth(candidate);
  expect(await option.evaluate((node) => node.closest('[data-slot="panel-body"]'))).toBeNull();
  const viewport = page.locator('[data-slot="workspace-view-body"]').filter({ has: panel });
  const bodyScroll = await viewport.evaluate((node) => node.scrollTop);
  const popupScroll = await popup.evaluate((node) => node.scrollTop);
  await option.focus();
  await expect(option).toBeFocused();
  expect(await viewport.evaluate((node) => node.scrollTop)).toBe(bodyScroll);
  expect(await popup.evaluate((node) => node.scrollTop)).toBe(popupScroll);
  await page.keyboard.press("Escape");
  await expect(model).toBeFocused();
  await editor.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(add).toBeFocused();
});
