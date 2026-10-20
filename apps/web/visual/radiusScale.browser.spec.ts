import { expect, test, type Locator, type Page } from "./test";
async function open(page: Page, theme: string, fixture: string) {
  await page.goto(`/visual/?${fixture}&theme=${theme}&font-size=14`);
  await page.locator("html[data-visual-ready]").waitFor();
  await page.evaluate(() => document.fonts.ready);
}

async function chooseCorners(page: Page, theme: string, choice: string, reload = false) {
  await open(page, theme, "fixture=workspace&state=settings&pane=appearance");
  const corners = page.getByRole("tablist", { name: "Corner radius", exact: true });
  const tab = corners.getByRole("tab", { name: choice, exact: true });
  await tab.click();
  await expect(tab).toHaveAttribute("aria-selected", "true");
  if (reload) {
    await page.reload();
    await page.locator("html[data-visual-ready]").waitFor();
    await expect(tab).toHaveAttribute("aria-selected", "true");
  }
}

async function shape(target: Locator) {
  await target.scrollIntoViewIfNeeded();
  await expect(target).toBeInViewport();
  return target.evaluate((node) => {
    const box = node.getBoundingClientRect();
    const radius = parseFloat(getComputedStyle(node).borderTopLeftRadius);
    return {
      width: box.width,
      height: box.height,
      radius,
      paintedRadius: Math.min(radius, box.width / 2, box.height / 2),
    };
  });
}

function expectRound(reading: Awaited<ReturnType<typeof shape>>) {
  expect(reading.paintedRadius).toBe(Math.min(reading.width, reading.height) / 2);
}

for (const theme of ["light", "dark"]) {
  test(`saved corner choices reach ordinary controls without changing circular geometry ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1472, height: 1000 });
    const readings: Record<string, Awaited<ReturnType<typeof shape>>>[] = [];
    for (const choice of ["Sharp", "Default", "Soft"]) {
      await chooseCorners(page, theme, choice, true);
      await open(page, theme, "fixture=atoms");
      const chip = page.locator('[data-slot="chip"]').filter({ hasText: "main.go" });
      const segmented = page.locator('[data-atom="segmented"] [data-slot="segmented"]');
      const current = {
        icon: await shape(
          page.locator('[data-atom="icon-button"]').getByRole("button", {
            name: "Search",
            exact: true,
          }),
        ),
        text: await shape(
          page.locator('[data-atom="button"]').getByRole("button", {
            name: "Primary",
            exact: true,
          }),
        ),
        inline: await shape(
          page.locator('[data-atom="text-button"]').getByRole("button", {
            name: "Muted",
            exact: true,
          }),
        ),
        chip: await shape(chip),
        field: await shape(page.getByRole("textbox", { name: "Empty", exact: true })),
        segmented: await shape(segmented),
        segment: await shape(segmented.getByRole("tab", { name: "Trace", exact: true })),
      };
      expect(current.icon.width).toBe(24);
      expect(current.icon.height).toBe(24);
      if (choice === "Sharp") {
        expect.soft(current.icon.paintedRadius).toBeLessThan(current.icon.height / 2);
        expect.soft(current.chip.paintedRadius).toBeLessThan(current.chip.height / 2);
      }

      const toggle = page.getByRole("switch", { name: "On", exact: true });
      const radio = page
        .locator('[data-atom="choice-list-single"]')
        .getByRole("radio", { name: "beta", exact: true });
      for (const circle of [
        chip.getByRole("button", { name: "Remove", exact: true }),
        page.locator('[data-atom="status"] > div > span[aria-hidden="true"]').first(),
        radio.locator(':scope > span[aria-hidden="true"]'),
        toggle,
        toggle.locator(":scope > span"),
      ]) {
        expectRound(await shape(circle));
      }

      await page.getByRole("button", { name: "Record actions", exact: true }).click();
      const menu = page.getByRole("menu", { name: "Record actions", exact: true });
      const menuShape = await shape(menu);
      await page.keyboard.press("Escape");

      await open(page, theme, "fixture=foundation&sidebar=collapsed");
      const textXs = await shape(page.getByRole("button", { name: "Agent", exact: true }));
      expect(textXs.height).toBe(24);
      if (choice === "Sharp") {
        expect.soft(textXs.paintedRadius).toBeLessThan(textXs.height / 2);
      }
      const status = await shape(
        page.locator('[data-slot="agent-status"]').filter({ hasText: "Ready" }),
      );
      readings.push({ ...current, menu: menuShape, textXs, status });
    }
    await test.info().attach("corner-control-readings", {
      body: JSON.stringify(readings, null, 2),
      contentType: "application/json",
    });
    const [sharp, normal, soft] = readings;
    for (const name of Object.keys(sharp!)) {
      expect
        .soft(sharp![name]!.radius, `${name}: Sharp must affect the rendered corner`)
        .toBeLessThan(normal![name]!.radius);
      expect
        .soft(soft![name]!.radius, `${name}: Soft must affect the rendered corner`)
        .toBeGreaterThan(normal![name]!.radius);
    }
  });

  test(`saved corner choices reach content and modal surfaces while explicit round controls stay round ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1120, height: 720 });
    const readings: {
      card: Awaited<ReturnType<typeof shape>>;
      modal: Awaited<ReturnType<typeof shape>>;
      image: Awaited<ReturnType<typeof shape>>;
    }[] = [];
    for (const choice of ["Sharp", "Soft"]) {
      await chooseCorners(page, theme, choice);
      await open(
        page,
        theme,
        "fixture=workspace&state=full-view&full-view=package:visual:schedules",
      );
      await page.getByRole("button", { name: "Add schedule", exact: true }).click();
      const editor = page.getByRole("region", { name: "Schedule editor", exact: true });
      const card = await shape(editor);
      await editor.getByRole("button", { name: "Cancel", exact: true }).click();

      await open(page, theme, "fixture=workspace&state=settings&pane=plugins");
      await page.getByRole("button", { name: "Configure inputs", exact: true }).first().click();
      const dialog = page.getByRole("dialog", { name: "Configure inputs", exact: true });
      const modal = await shape(dialog);
      await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
      await expect(dialog).toHaveCount(0);

      await open(page, theme, "fixture=agent&state=running");
      expectRound(await shape(page.getByRole("button", { name: "Stop", exact: true })));
      await open(page, theme, "fixture=agent&state=long-content");
      const preview = page.getByRole("button", { name: "Inline architecture", exact: true });
      const image = await shape(preview);
      expect(image.radius).toBe((await shape(preview.locator("img"))).radius);
      readings.push({ card, modal, image });
    }
    await test.info().attach("corner-surface-readings", {
      body: JSON.stringify(readings, null, 2),
      contentType: "application/json",
    });
    expect(readings[0]!.card.paintedRadius).toBeLessThan(readings[1]!.card.paintedRadius);
    expect(readings[0]!.modal.paintedRadius).toBeLessThan(readings[1]!.modal.paintedRadius);
    expect(readings[0]!.image.paintedRadius).toBeLessThan(readings[1]!.image.paintedRadius);
  });
}
