import { expect, test, type Page } from "./test";
import { expectLabReady } from "../readiness";

async function files(page: Page, image = true) {
  return page.evaluateHandle((image) => {
    const transfer = new DataTransfer();
    transfer.items.add(new File(["notes"], "notes.txt", { type: "text/plain" }));
    if (image) {
      const bytes = Uint8Array.from(
        atob(
          "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
        ),
        (char) => char.charCodeAt(0),
      );
      transfer.items.add(new File([bytes], "capture.png", { type: "image/png" }));
    }
    return transfer;
  }, image);
}

for (const browserName of ["chromium", "webkit"] as const) {
  test.extend({ browserName })(
    `${browserName} external files have one accepting Composer target and retain their original source`,
    async ({ page }) => {
      await page.goto("/lab/fixture.html?fixture=agent&state=idle&theme=light");
      await expectLabReady(page);
      await page.evaluate(async () => {
        const path = "/lab/scenarios/fixtureRuntime.tsx";
        const { visualClient } = await import(path);
        visualClient.composer.start();
      });
      const surface = page.locator('[data-slot="composer-root"]');
      const cue = surface.locator('[data-slot="composer-drop-cue"]');
      const attached = surface.getByRole("button", { name: "Remove image", exact: true });
      const transfer = await files(page);
      await surface.dispatchEvent("dragenter", { dataTransfer: transfer, relatedTarget: null });
      await surface.dispatchEvent("dragover", { dataTransfer: transfer });
      await expect(cue).toContainText("supported images will be attached");
      await surface.dispatchEvent("drop", { dataTransfer: transfer });
      await expect(cue).toHaveCount(0);
      await expect(attached).toHaveCount(1);
      await surface.dispatchEvent("drop", { dataTransfer: transfer });
      await expect(attached).toHaveCount(1);

      const textOnly = await files(page, false);
      await surface.dispatchEvent("dragenter", { dataTransfer: textOnly, relatedTarget: null });
      await surface.dispatchEvent("dragover", { dataTransfer: textOnly });
      await expect(cue).toContainText("Drop files here");
      await surface.dispatchEvent("drop", { dataTransfer: textOnly });
      await expect(attached).toHaveCount(1);

      const outside = await files(page);
      await surface.dispatchEvent("dragenter", { dataTransfer: outside, relatedTarget: null });
      await page.locator("h1").dispatchEvent("dragover", { dataTransfer: outside });
      await page.locator("h1").dispatchEvent("drop", { dataTransfer: outside });
      await expect(cue).toHaveCount(0);
      await expect(attached).toHaveCount(1);

      const retired = await files(page);
      await surface.dispatchEvent("dragenter", { dataTransfer: retired, relatedTarget: null });
      await surface.dispatchEvent("dragover", { dataTransfer: retired });
      await expect(cue).toBeVisible();
      await page.evaluate(async () => {
        const path = "/lab/scenarios/fixtureRuntime.tsx";
        const { visualClient } = await import(path);
        visualClient.composer.store.getState().loadSession("successor-draft");
      });
      await expect(cue).toHaveCount(0);
      await surface.dispatchEvent("dragover", { dataTransfer: retired });
      await surface.dispatchEvent("drop", { dataTransfer: retired });
      await expect(attached).toHaveCount(0);
      const fresh = await files(page);
      await surface.dispatchEvent("dragenter", { dataTransfer: fresh, relatedTarget: null });
      await surface.dispatchEvent("dragover", { dataTransfer: fresh });
      await surface.dispatchEvent("drop", { dataTransfer: fresh });
      await expect(attached).toHaveCount(1);
      await transfer.dispose();
      await textOnly.dispose();
      await outside.dispose();
      await retired.dispose();
      await fresh.dispose();
    },
  );

  test.extend({ browserName })(
    `${browserName} native Dock dragging and keyboard menus advance the same canonical order`,
    async ({ page }) => {
      await page.setViewportSize({ width: 1472, height: 900 });
      await page.goto("/lab/fixture.html?fixture=workspace&state=dock-light&theme=dark");
      await expectLabReady(page);
      const strip = page.locator(".agent-dock-tabs");
      const fileTitle = await strip.locator('[data-dock-tab-id="file"]').innerText();
      const order = () =>
        strip
          .getByRole("tab")
          .evaluateAll((tabs) => tabs.map((tab) => tab.getAttribute("data-dock-tab-id")));
      expect(await order()).toEqual(["file", "package:visual:diff", "package:visual:trajectory"]);
      await strip
        .locator('[data-dock-tab-row="file"]')
        .dragTo(strip.locator('[data-dock-tab-row="package:visual:trajectory"]'));
      await expect
        .poll(order)
        .toEqual(["package:visual:diff", "package:visual:trajectory", "file"]);
      await expect(strip.getByRole("status")).toHaveText(`${fileTitle} moved to position 3 of 3`);
      const file = strip.locator('[data-dock-tab-id="file"]');
      await file.focus();
      await file.press("Shift+F10");
      await page.getByRole("menuitem", { name: "Move panel left", exact: true }).click();
      await expect
        .poll(order)
        .toEqual(["package:visual:diff", "file", "package:visual:trajectory"]);
      await expect(file).toBeFocused();
      await expect(strip.getByRole("status")).toHaveText(`${fileTitle} moved to position 2 of 3`);
      await file.press("ArrowRight");
      await expect(
        strip.getByRole("tab", { name: "Session trajectory", exact: true }),
      ).toBeFocused();
      await expect(page.getByTestId("active-dock-view")).toHaveText("package:visual:trajectory");
      const background = strip.locator('[data-dock-tab-row="package:visual:diff"]');
      await background.hover();
      await background.getByRole("button").click();
      await expect.poll(order).toEqual(["file", "package:visual:trajectory"]);
      await expect(page.getByTestId("active-dock-view")).toHaveText("package:visual:trajectory");
    },
  );
}
