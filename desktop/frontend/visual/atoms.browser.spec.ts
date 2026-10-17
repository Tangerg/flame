import { contrastRatio, WCAG_AA_NON_TEXT } from "@/plugins/builtin/theme/kit/legibility";
import { expect, test, type Locator, type Page } from "./test";

const THEMES = ["light", "dark"] as const;

async function openGallery(page: Page, theme: (typeof THEMES)[number]) {
  await page.setViewportSize({ width: 1440, height: 1300 });
  await page.goto(`/visual/?fixture=atoms&theme=${theme}`);
  await page.locator("html[data-visual-ready]").waitFor();
  await expect(page.locator('[data-slot="atoms-gallery"]')).toBeVisible();
}

async function paint(page: Page, selector: string, property: string): Promise<string> {
  return page
    .locator(selector)
    .first()
    .evaluate((node, name) => {
      const probe = document.createElement("div");
      probe.style.color = `color-mix(in srgb, ${getComputedStyle(node).getPropertyValue(name)} 100%, transparent)`;
      document.body.append(probe);
      const resolved = getComputedStyle(probe).color;
      probe.remove();
      const srgb = /color\(srgb ([\d.e-]+) ([\d.e-]+) ([\d.e-]+)/.exec(resolved);
      const rgb = /rgba?\((\d+), (\d+), (\d+)/.exec(resolved);
      const channels = srgb
        ? srgb.slice(1).map((value) => Math.round(Number(value) * 255))
        : rgb!.slice(1).map(Number);
      return `#${channels.map((channel) => channel.toString(16).padStart(2, "0")).join("")}`;
    }, property);
}

async function controlPaint(control: Locator) {
  return control.evaluate((node) => {
    const style = getComputedStyle(node);
    return {
      background: style.backgroundColor,
      color: style.color,
      border: style.borderColor,
    };
  });
}

async function restingPaint(page: Page, control: Locator) {
  await control.scrollIntoViewIfNeeded();
  await page.mouse.move(1, 1);
  expect(await control.evaluate((node) => node.matches(":hover"))).toBe(false);
  return controlPaint(control);
}

async function hoveredPaint(control: Locator) {
  await control.hover();
  expect(await control.evaluate((node) => node.matches(":hover"))).toBe(true);
  return controlPaint(control);
}

async function expectStableHover(page: Page, control: Locator) {
  const resting = await restingPaint(page, control);
  expect(await hoveredPaint(control)).toEqual(resting);
}

test("a long button label stays inside its complete click target", async ({ page }) => {
  await openGallery(page, "light");
  const button = page.locator('[data-atom="button"]').getByRole("button").first();
  await expect(button).toHaveText("Primary");
  const restingHeight = await button.evaluate((node) => node.getBoundingClientRect().height);
  await button.evaluate((node) => {
    node.textContent = "Q7fJ2xL9pN4vR8mK3sT6wY1zB5cD0eG".repeat(7);
    node.style.width = "160px";
  });
  const bounds = await button.evaluate((node) => ({
    width: node.getBoundingClientRect().width,
    height: node.getBoundingClientRect().height,
    horizontalOverflow: node.scrollWidth - node.clientWidth,
    verticalOverflow: node.scrollHeight - node.clientHeight,
  }));
  expect(bounds.width).toBeLessThanOrEqual(160);
  expect(bounds.height).toBeGreaterThan(restingHeight);
  expect(bounds.horizontalOverflow).toBeLessThanOrEqual(1);
  expect(bounds.verticalOverflow).toBeLessThanOrEqual(1);
});

for (const theme of THEMES) {
  test(`disabled and pending buttons keep their paint while enabled buttons answer hover in ${theme}`, async ({
    page,
  }) => {
    await openGallery(page, theme);
    const buttons = page.locator('[data-atom="button"]');
    for (const label of ["Primary", "Soft", "Outline", "Ghost", "Delete"]) {
      const enabled = buttons.getByRole("button", { name: label, exact: true });
      const resting = await restingPaint(page, enabled);
      expect(await hoveredPaint(enabled)).not.toEqual(resting);

      const disabled = buttons.getByRole("button", { name: `${label} disabled`, exact: true });
      await expect(disabled).toHaveJSProperty("disabled", true);
      await expectStableHover(page, disabled);

      const pending = buttons.getByRole("button", { name: `${label} pending`, exact: true });
      await expect(pending).toHaveJSProperty("disabled", false);
      await expect(pending).toHaveAttribute("aria-disabled", "true");
      await expectStableHover(page, pending);
    }
  });

  test(`unavailable text, segment and choice controls keep their paint on hover in ${theme}`, async ({
    page,
  }) => {
    await openGallery(page, theme);
    const controls = page.locator('[data-atom="unavailable-controls"]');
    const disabledText = controls.getByRole("button", {
      name: "Disabled text action",
      exact: true,
    });
    await expect(disabledText).toHaveJSProperty("disabled", true);
    await expectStableHover(page, disabledText);
    const pendingText = controls.getByRole("button", { name: "Pending text action", exact: true });
    await expect(pendingText).toHaveJSProperty("disabled", false);
    await expect(pendingText).toHaveAttribute("aria-disabled", "true");
    await expectStableHover(page, pendingText);

    const tabs = controls.getByRole("tablist", { name: "Unavailable diagnostics", exact: true });
    for (const label of ["Trace", "Metrics"]) {
      const tab = tabs.getByRole("tab", { name: label, exact: true });
      await expect(tab).toBeDisabled();
      await expectStableHover(page, tab);
    }
    const choice = controls.getByRole("radio", { name: "Unavailable choice", exact: true });
    await expect(choice).toBeDisabled();
    await expectStableHover(page, choice);

    const enabled = [
      page.locator('[data-atom="text-button"]').getByRole("button", { name: "Muted", exact: true }),
      page.getByRole("tablist", { name: "Diagnostics", exact: true }).getByRole("tab", {
        name: "Metrics",
        exact: true,
      }),
      page.locator('[data-atom="choice-list-single"]').getByRole("radio", {
        name: "alpha",
        exact: true,
      }),
    ];
    for (const control of enabled) {
      const resting = await restingPaint(page, control);
      expect(await hoveredPaint(control)).not.toEqual(resting);
    }
  });

  test(`unavailable field triggers keep their paint and enabled pickers answer hover in ${theme}`, async ({
    page,
  }) => {
    await openGallery(page, theme);
    const triggers = page.locator('[data-atom="select-trigger"]');
    const disabled = triggers.getByRole("button", { name: "Disabled selection", exact: true });
    await expect(disabled).toHaveJSProperty("disabled", true);
    await expectStableHover(page, disabled);
    const pending = triggers.getByRole("button", { name: "Pending selection", exact: true });
    await expect(pending).toHaveJSProperty("disabled", false);
    await expect(pending).toHaveAttribute("aria-disabled", "true");
    await expectStableHover(page, pending);
    const disabledSelect = triggers.getByRole("combobox", {
      name: "Disabled status",
      exact: true,
    });
    await expect(disabledSelect).toHaveJSProperty("disabled", true);
    await expectStableHover(page, disabledSelect);

    const enabled = [
      triggers.getByRole("button", { name: "gpt-5 · high", exact: true }),
      page.getByRole("combobox", { name: "Memory status", exact: true }),
    ];
    for (const trigger of enabled) {
      const resting = await restingPaint(page, trigger);
      expect((await hoveredPaint(trigger)).color).not.toBe(resting.color);
    }
    const select = enabled[1]!;
    await select.click();
    await expect(page.getByRole("listbox")).toBeVisible();
    await page.getByRole("option", { name: "Active", exact: true }).click();
    await expect(select).toHaveText("Active");
  });

  test(`menu hover highlights enabled items and leaves disabled items unchanged in ${theme}`, async ({
    page,
  }) => {
    await openGallery(page, theme);
    await page.getByRole("button", { name: "Record actions", exact: true }).click();
    const menu = page.getByRole("menu", { name: "Record actions", exact: true });
    const enabled = menu.getByRole("menuitem", { name: "Edit record", exact: true });
    const resting = await restingPaint(page, enabled);
    expect((await hoveredPaint(enabled)).background).not.toBe(resting.background);
    const disabled = menu.getByRole("menuitem", { name: "Delete record", exact: true });
    await expect(disabled).toHaveAttribute("data-disabled", "");
    await expectStableHover(page, disabled);
  });

  test(`every atom renders in ${theme}`, async ({ page }) => {
    await openGallery(page, theme);
    const panel = page.getByRole("region", { name: "Shared Memory controls" });
    await expect(panel.getByText("Agent memory", { exact: true })).toBeVisible();
    await expect(panel.getByRole("combobox", { name: "Memory status" })).toHaveText(
      "Pending review",
    );
  });

  test(`an unchecked control states its boundary against the ${theme} canvas`, async ({ page }) => {
    await openGallery(page, theme);
    const canvas = await paint(page, '[data-slot="atoms-gallery"]', "background-color");
    const boundaries = {
      "switch off track": await paint(
        page,
        '[data-atom="switch"] [role="switch"]:not([data-checked])',
        "background-color",
      ),
      "checkbox box": await paint(
        page,
        '[data-atom="checkbox"] [role="checkbox"]:not([data-checked])',
        "border-top-color",
      ),
      "slider thumb": await paint(page, '[data-atom="slider"] [data-index]', "border-top-color"),
    };
    for (const [control, ink] of Object.entries(boundaries)) {
      expect(contrastRatio(ink, canvas), control).toBeGreaterThanOrEqual(WCAG_AA_NON_TEXT);
    }
  });

  test(`hover keeps the selected segment's fill in ${theme}`, async ({ page }) => {
    await openGallery(page, theme);
    const chip = '[data-atom="segmented"] [role="tab"][data-active]';
    const resting = await paint(page, chip, "background-color");
    const selected = page.locator('[data-atom="segmented"] [role="tab"][data-active]');
    await selected.hover();
    await expect.poll(() => selected.evaluate((node) => node.matches(":hover"))).toBe(true);
    expect(await paint(page, chip, "background-color")).toBe(resting);
    const well = await paint(
      page,
      '[data-atom="segmented"] [data-slot="segmented"]',
      "background-color",
    );
    expect(resting).not.toBe(well);
  });
}
