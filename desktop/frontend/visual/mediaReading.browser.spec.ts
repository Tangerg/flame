import { expect, test } from "./test";
import { LONG_RESPONSE_MARKDOWN } from "./agentSessionSnapshots";

test("file image references keep encoded payloads in Source and out of the reading flow", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1120, height: 1000 });
  const remoteRequests: string[] = [];
  page.on("request", (request) => {
    if (request.url().startsWith("https://tracker.example/")) remoteRequests.push(request.url());
  });
  await page.goto("/visual/?fixture=workspace&state=dock-files&theme=dark&font-size=18");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.getByRole("treeitem", { name: "reading-guide.md", exact: true }).click();
  const view = page.locator('[data-dock-view-id="file"]');
  const reading = view.locator('[data-slot="document-body"]');
  await expect(reading.getByRole("heading", { name: "Architecture review" })).toBeVisible();
  await expect(reading).toContainText("[Image: Inline architecture]");
  await expect(reading).toContainText("[Image: Inline detail]");
  await expect(reading.getByText("(embedded image data)", { exact: true })).toHaveCount(2);
  await expect(reading).not.toContainText("data:image/");
  await expect(reading).toContainText("https://tracker.example/pixel.png");
  await expect(reading.locator("img,a,iframe,object,embed")).toHaveCount(0);
  expect(await reading.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);

  await view.getByRole("tab", { name: "Source", exact: true }).click();
  const payload = LONG_RESPONSE_MARKDOWN.match(/data:image\/svg\+xml;base64,[^)]+/)![0];
  await expect(view.locator('[data-quote-source="file"]')).toContainText(payload);
  await view.getByRole("tab", { name: "Preview", exact: true }).click();
  await expect(reading.getByText("(embedded image data)", { exact: true })).toHaveCount(2);
  expect(remoteRequests).toEqual([]);
});
