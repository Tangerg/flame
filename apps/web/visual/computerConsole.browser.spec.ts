import { expect, test } from "./test";

for (const theme of ["light", "dark"] as const) {
  for (const width of [360, 1280]) {
    test(`published computer console supports permission and original results in ${theme} at ${width}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 960 });
      const remoteRequests: string[] = [];
      page.on("request", (request) => {
        if (!request.url().startsWith("http://127.0.0.1:")) remoteRequests.push(request.url());
      });
      await page.goto(`/visual/?fixture=computer&theme=${theme}&font-size=18`);
      await page.locator("html[data-visual-ready]").waitFor();
      const panel = page.getByRole("region", { name: "Computer use", exact: true });
      await expect(panel.getByText("isolated-desktop.example", { exact: true })).toBeVisible();
      await expect(
        panel.getByText(/operator console outside the controlled desktop/),
      ).toBeVisible();
      const opening = test.info().outputPath(`computer-${theme}-${width}-open.png`);
      await page.screenshot({ path: opening });
      await test.info().attach("published-computer-open-boundary", { path: opening });
      await panel.getByRole("textbox", { name: "Lease duration in seconds" }).fill("300");
      await panel.getByRole("button", { name: "Authorize and open control" }).click();
      const editor = panel.getByRole("textbox", { name: "Tool arguments (JSON)" });
      await expect(editor).toBeEnabled();
      await editor.fill('{"code":"await fixture.inspect();"}');
      await panel.getByRole("button", { name: "Review operation", exact: true }).click();
      await expect(panel.getByRole("button", { name: "Back to editor" })).toBeFocused();
      await panel.getByRole("button", { name: "Execute reviewed operation" }).click();
      await expect(
        panel.getByText("Allow control of the isolated example editor?", { exact: true }),
      ).toBeVisible();
      await expect(panel.getByRole("button", { name: "Always allow" })).toHaveCount(0);
      await expect(panel.getByRole("button", { name: "Stop current operation" })).toBeEnabled();
      await expect(panel.getByRole("button", { name: "End control lease" })).toBeEnabled();
      await panel.getByRole("button", { name: "Allow once", exact: true }).focus();
      const approval = test.info().outputPath(`computer-${theme}-${width}-approval.png`);
      const permissionHeading = panel.getByRole("heading", {
        name: "Permission request",
        exact: true,
      });
      await expect
        .poll(() =>
          permissionHeading.evaluate((node) => {
            const controls = node
              .closest('[data-slot="panel"]')
              ?.querySelector('[data-slot="panel-controls"]');
            if (!controls) throw new Error("Missing panel controls");
            return node.getBoundingClientRect().top - controls.getBoundingClientRect().bottom;
          }),
        )
        .toBeGreaterThanOrEqual(0);
      await expect(
        panel.getByText("Allow control of the isolated example editor?", { exact: true }),
      ).toBeInViewport({ ratio: 1 });
      await page.screenshot({ path: approval });
      await test.info().attach("published-computer-permission", { path: approval });
      await panel.getByRole("button", { name: "Allow for this session" }).click();
      await expect(
        panel.getByText("Allow control of the isolated example editor?", { exact: true }),
      ).toHaveCount(0);
      const history = panel
        .getByRole("heading", { name: "Original outcomes", exact: true })
        .locator("../../..");
      const record = history.getByRole("button", { name: /^[a-f0-9-]{36}$/ });
      await expect(record).toHaveCount(1);
      await record.click();
      await expect(
        panel.getByText("The original isolated fixture operation completed.", { exact: true }),
      ).toBeVisible();
      await expect(panel.getByRole("img", { name: "Captured image 2" })).toBeVisible();
      await expect(
        panel.getByText("Recorded image; this is not a live desktop.", { exact: true }),
      ).toBeVisible();
      const outcome = test.info().outputPath(`computer-${theme}-${width}-outcome.png`);
      await panel.screenshot({ path: outcome });
      await test.info().attach("published-computer-original-outcome", { path: outcome });
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth - innerWidth),
      ).toBeLessThanOrEqual(1);
      expect(remoteRequests.filter((url) => !url.startsWith("data:"))).toEqual([]);
      await panel.getByRole("button", { name: "End control lease" }).click();
      await expect(panel.getByText("Closed", { exact: true })).toBeVisible();
    });
  }
}

test("published console stops an outstanding permission without replaying its action", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=computer&theme=light");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.getByRole("button", { name: "Authorize and open control" }).click();
  await page
    .getByRole("textbox", { name: "Tool arguments (JSON)" })
    .fill('{"code":"await fixture.inspect();"}');
  await page.getByRole("button", { name: "Review operation", exact: true }).click();
  await page.getByRole("button", { name: "Execute reviewed operation" }).click();
  await expect(
    page.getByText("Allow control of the isolated example editor?", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Stop current operation" }).click();
  await expect(page.getByText("Closed", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Check original outcome" })).toBeVisible();
  await page.getByRole("button", { name: "Check original outcome" }).click();
  await expect(page.getByRole("button", { name: "Check original outcome" })).toBeVisible();
  await expect(
    page.getByText("Allow control of the isolated example editor?", { exact: true }),
  ).toHaveCount(0);
});
