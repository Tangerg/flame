import { expect, test } from "./test";

for (const theme of ["light", "dark"]) {
  for (const width of [1120, 320]) {
    test(`brand assets load without overflow ${theme} ${width}`, async ({ page }) => {
      await page.setViewportSize({ width, height: 900 });
      const locale = width === 320 ? "zh" : "en";
      await page.goto(
        `/visual/?fixture=workspace&state=settings&pane=brand&theme=${theme}&locale=${locale}`,
      );
      await page.locator("html[data-visual-ready]").waitFor();
      const panel = page.getByRole("tabpanel");
      await expect(
        panel.getByRole("heading", {
          name: locale === "zh" ? "品牌形象" : "Brand identity",
          level: 1,
          exact: true,
        }),
      ).toBeVisible();
      await expect(panel.getByRole("img")).toHaveCount(3);
      await expect
        .poll(() =>
          panel.getByRole("img").evaluateAll((images) =>
            images.every((image) => {
              const img = image as HTMLImageElement;
              return img.complete && img.naturalWidth > 0;
            }),
          ),
        )
        .toBe(true);
      await expect(panel.getByText("#FB6E30", { exact: true })).toBeVisible();
      await expect(panel.getByText("251, 110, 48", { exact: true })).toBeVisible();
      expect(await panel.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
      expect(
        await page.locator("html").evaluate((node) => node.scrollWidth - node.clientWidth),
      ).toBe(0);
    });
  }
}

test("brand settings can be found, copied and downloaded", async ({
  page,
  context,
  browserName,
}) => {
  await context.grantPermissions(
    browserName === "webkit" ? ["clipboard-read"] : ["clipboard-read", "clipboard-write"],
  );
  await page.goto(
    "/visual/?fixture=workspace&state=settings&pane=appearance&theme=light&locale=en",
  );
  await page.locator("html[data-visual-ready]").waitFor();
  await page
    .getByRole("searchbox", { name: "Search settings…", exact: true })
    .fill("Brand identity");
  await page.getByRole("tab", { name: "Brand identity", exact: true }).click();
  const panel = page.getByRole("tabpanel");
  await panel.getByRole("button", { name: "Copy HEX", exact: true }).click();
  await expect(panel.getByRole("button", { name: "Copied", exact: true })).toBeVisible();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe("#FB6E30");

  const downloadEvent = page.waitForEvent("download");
  const save = panel.getByRole("button", { name: "Download Primary logo", exact: true });
  await save.click();
  const download = await downloadEvent;
  expect(download.suggestedFilename()).toBe("flame-image.png");
  expect(await download.failure()).toBeNull();
  await expect(save).toBeFocused();
});
