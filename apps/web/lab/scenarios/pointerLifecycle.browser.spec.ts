import { expect, test, type Locator, type Page } from "./test";
import { expectLabReady } from "../readiness";

async function drag(page: Page, viewport: Locator) {
  const box = (await viewport.boundingBox())!;
  const x = box.x + box.width * 0.7;
  const y = box.y + box.height * 0.6;
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.mouse.move(x - 50, y - 50, { steps: 5 });
  await expect(viewport).toHaveAttribute("data-panning", "");
  return { x: x - 50, y: y - 50 };
}

async function wheelTakeover(page: Page, viewport: Locator) {
  const position = await drag(page, viewport);
  await page.mouse.wheel(80, 0);
  await expect(viewport).not.toHaveAttribute("data-panning");
  const accepted = await viewport.evaluate(async (node) => {
    // Wheel dispatch completes before native compositor scrolling has painted.
    await new Promise<void>((resolve) =>
      requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
    );
    return [node.scrollLeft, node.scrollTop];
  });
  await page.mouse.move(position.x - 20, position.y - 10, { steps: 2 });
  await page.mouse.up();
  expect(await viewport.evaluate((node) => [node.scrollLeft, node.scrollTop])).toEqual(accepted);
  await page.mouse.wheel(80, 0);
  await expect
    .poll(() => viewport.evaluate((node) => node.scrollLeft))
    .toBeGreaterThan(accepted[0]!);
}

async function resizeBy(page: Page, handle: Locator, delta: number) {
  const box = (await handle.boundingBox())!;
  const x = box.x + box.width / 2;
  const y = box.y + box.height / 2;
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.mouse.move(x + delta, y, { steps: 4 });
  return { x, y };
}

for (const browserName of ["chromium", "webkit"] as const) {
  test.extend({ browserName })(
    `${browserName} diagram pan cancels and yields to native wheel scrolling and Fit`,
    async ({ page }) => {
      await page.goto("/lab/fixture.html?fixture=agent&state=long-content&theme=light");
      await expectLabReady(page);
      await page.getByRole("button", { name: "Enlarge diagram", exact: true }).focus();
      await page.keyboard.press("Enter");
      const dialog = page.getByRole("dialog", { name: "Diagram", exact: true });
      const viewport = dialog.getByRole("region", { name: "Diagram", exact: true });
      for (let i = 0; i < 3; i++)
        await dialog.getByRole("button", { name: "Zoom in diagram", exact: true }).click();
      await viewport.evaluate((node) => {
        node.scrollLeft = 0;
        node.scrollTop = 0;
      });
      await drag(page, viewport);
      expect(await viewport.evaluate((node) => node.scrollLeft)).toBeGreaterThan(0);
      await page.keyboard.press("Escape");
      await page.mouse.up();
      await expect(dialog).toBeVisible();
      expect(await viewport.evaluate((node) => [node.scrollLeft, node.scrollTop])).toEqual([0, 0]);
      await expect(viewport).not.toHaveAttribute("data-panning");
      await wheelTakeover(page, viewport);
      await drag(page, viewport);
      await dialog
        .getByRole("button", { name: "Fit", exact: true })
        .evaluate((node: HTMLButtonElement) => node.click());
      await expect(viewport).not.toHaveAttribute("data-panning");
      await page.mouse.up();
      await expect
        .poll(() => viewport.evaluate((node) => [node.scrollLeft, node.scrollTop]))
        .toEqual([0, 0]);
    },
  );

  test.extend({ browserName })(
    `${browserName} image pan restores cancellation and yields to wheel and keyboard commands`,
    async ({ page }) => {
      await page.setViewportSize({ width: 560, height: 360 });
      await page.goto("/lab/fixture.html?fixture=agent&state=long-content&theme=dark");
      await expectLabReady(page);
      await page.getByRole("button", { name: "Inline architecture", exact: true }).focus();
      await page.keyboard.press("Enter");
      const dialog = page.getByRole("dialog", { name: "Inline architecture", exact: true });
      const viewport = dialog.locator("[data-image-zoom]");
      await viewport.locator("img").evaluate((image: HTMLImageElement) => image.decode());
      for (let i = 0; i < 5; i++)
        await dialog.getByRole("button", { name: "Zoom in image", exact: true }).click();
      await viewport.evaluate((node) => {
        node.scrollLeft = 0;
        node.scrollTop = 0;
      });
      await drag(page, viewport);
      expect(await viewport.evaluate((node) => node.scrollTop)).toBeGreaterThan(0);
      await page.keyboard.press("Escape");
      await page.mouse.up();
      await expect(dialog).toBeVisible();
      expect(await viewport.evaluate((node) => [node.scrollLeft, node.scrollTop])).toEqual([0, 0]);
      await wheelTakeover(page, viewport);
      await drag(page, viewport);
      await page.keyboard.press("ArrowDown");
      await expect(viewport).not.toHaveAttribute("data-panning");
      const accepted = await viewport.evaluate((node) => node.scrollTop);
      await page.mouse.up();
      expect(await viewport.evaluate((node) => node.scrollTop)).toBe(accepted);
      expect(accepted).toBeGreaterThan(50);
    },
  );

  test.extend({ browserName })(
    `${browserName} resize cancellation preserves the preference and keyboard commits one intent`,
    async ({ page }) => {
      await page.goto("/lab/fixture.html?fixture=shell&state=populated&theme=light");
      await expectLabReady(page);
      const handle = page.getByRole("separator", { name: "Resize the work index" });
      const persisted = page.getByTestId("persisted-sidebar-width");
      await expect(persisted).toHaveText("275");
      const box = (await handle.boundingBox())!;
      await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
      await page.mouse.down();
      await page.mouse.move(box.x + 50, box.y + box.height / 2);
      await expect(handle).not.toHaveAttribute("aria-valuenow", "275");
      await handle.focus();
      await page.keyboard.press("Escape");
      await page.mouse.up();
      await expect(handle).toHaveAttribute("aria-valuenow", "275");
      await expect(persisted).toHaveText("275");
      await handle.press("ArrowRight");
      await expect(persisted).toHaveText("283");
      await handle.press("Shift+ArrowRight");
      await expect(persisted).toHaveText("307");
      await handle.press("Home");
      await expect(persisted).toHaveText((await handle.getAttribute("aria-valuemin"))!);
      await handle.press("End");
      await expect(persisted).toHaveText((await handle.getAttribute("aria-valuemax"))!);
    },
  );

  for (const layout of [
    {
      name: "sidebar",
      fixture: "shell&state=populated",
      handle: "Resize the work index",
      owner: ".agent-shell",
      surface: ".agent-drawer-gap",
      property: "--sidebar-width",
      preference: "persisted-sidebar-width",
      direction: 1,
    },
    {
      name: "Dock",
      fixture: "workspace&state=dock-light",
      handle: "Resize right workspace",
      owner: ".agent-dock-row",
      surface: ".agent-context-dock",
      property: "--dock-measure",
      preference: "persisted-dock-ratio",
      direction: -1,
    },
  ]) {
    test.extend({ browserName })(
      `${browserName} ${layout.name} keeps accepted layout separate from a retired preview`,
      async ({ page }) => {
        await page.goto(`/lab/fixture.html?fixture=${layout.fixture}&theme=light`);
        await expectLabReady(page);
        const handle = page.getByRole("separator", { name: layout.handle });
        const owner = page.locator(layout.owner);
        const surface = page.locator(layout.surface);
        const preference = page.getByTestId(layout.preference);
        const before = await preference.textContent();
        const accepted = Number(await handle.getAttribute("aria-valuenow"));
        const minimum = Number(await handle.getAttribute("aria-valuemin"));
        const delta = accepted - minimum >= 32 ? -24 : 24;
        await expect
          .poll(async () => (await surface.boundingBox())!.width)
          .toBeCloseTo(accepted, 0);
        const position = await resizeBy(page, handle, delta * layout.direction);
        await expect(handle).toHaveAttribute("aria-valuenow", String(accepted + delta));
        await expect(preference).toHaveText(before!);
        expect(
          await owner.evaluate(
            (node, property) => node.style.getPropertyValue(property),
            layout.property,
          ),
        ).toBe(`${accepted}px`);
        await expect
          .poll(async () => (await surface.boundingBox())!.width)
          .toBeCloseTo(accepted + delta, 0);

        await page.evaluate(() =>
          window.dispatchEvent(new PageTransitionEvent("pagehide", { persisted: true })),
        );
        await expect(owner).not.toHaveAttribute("data-resizing");
        await expect(handle).toHaveAttribute("aria-valuenow", String(accepted));
        await expect
          .poll(async () => (await surface.boundingBox())!.width)
          .toBeCloseTo(accepted, 0);
        await page.mouse.move(position.x + 2 * delta * layout.direction, position.y);
        await page.mouse.up();
        await expect(preference).toHaveText(before!);

        await resizeBy(page, handle, delta * layout.direction);
        await page.mouse.up();
        await expect(preference).not.toHaveText(before!);
        await expect(handle).toHaveAttribute("aria-valuenow", String(accepted + delta));
        await expect
          .poll(async () => (await surface.boundingBox())!.width)
          .toBeCloseTo(accepted + delta, 0);
      },
    );
  }
}
