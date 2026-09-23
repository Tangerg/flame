import { contrastRatio, WCAG_AA_NON_TEXT } from "@/plugins/builtin/theme/kit/legibility";
import { expect, test, type Page } from "./test";

const THEMES = ["light", "dark"] as const;

async function openGallery(page: Page, theme: (typeof THEMES)[number]) {
  await page.setViewportSize({ width: 1440, height: 1300 });
  await page.goto(`/visual/?fixture=atoms&theme=${theme}`);
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

for (const theme of THEMES) {
  test(`every atom renders in ${theme}`, async ({ page }) => {
    await openGallery(page, theme);
    await expect(page).toHaveScreenshot(`atoms-${theme}.png`, { fullPage: true });
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
    const chip = '[data-atom="segmented"] [role="tab"][data-active] > span:first-child';
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
