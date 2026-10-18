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

test("media-only paragraphs own image spacing across wrapped and unavailable images", async ({
  page,
}) => {
  const gallery = page.locator('[data-markdown-image-grid="true"]');
  const spacing = () =>
    gallery.evaluate((group) => {
      const children = Array.from(group.children);
      if (children.length !== 2) throw new Error("Canonical gallery must contain two images");
      const first = children[0]!.getBoundingClientRect();
      const second = children[1]!.getBoundingClientRect();
      const sameRow = first.top === second.top;
      const style = getComputedStyle(group);
      const reading = group.closest(".msg-scroll-viewport")!;
      return {
        sameRow,
        gap: sameRow ? second.left - first.right : second.top - first.bottom,
        parentGap: Number.parseFloat(sameRow ? style.columnGap : style.rowGap),
        margins: children.map((child) => {
          const style = getComputedStyle(child);
          return [style.marginBlockStart, style.marginBlockEnd];
        }),
        readingOverflow: reading.scrollWidth - reading.clientWidth,
        pageOverflow: document.documentElement.scrollWidth - innerWidth,
      };
    });

  for (const width of [1472, 1120]) {
    await page.setViewportSize({ width, height: 720 });
    await page.goto(
      "/visual/?fixture=workspace&state=dock-trajectory&agent-state=long-content&theme=light&font-size=14",
    );
    await page.locator("html[data-visual-ready]").waitFor();
    await expect(gallery.locator("img")).toHaveCount(2);
    for (const image of await gallery.locator("img").all()) {
      await image.evaluate((image: HTMLImageElement) => image.decode());
    }
    await gallery.scrollIntoViewIfNeeded();
    const measured = await spacing();
    expect(measured.sameRow).toBe(width === 1472);
    expect(measured.gap).toBe(measured.parentGap);
    expect(measured.margins).toEqual([
      ["0px", "0px"],
      ["0px", "0px"],
    ]);
    expect(measured.readingOverflow).toBe(0);
    expect(measured.pageOverflow).toBe(0);
  }

  await gallery
    .locator("img")
    .first()
    .evaluate((image) => image.dispatchEvent(new Event("error")));
  const unavailable = gallery.getByRole("img", { name: "Inline architecture", exact: true });
  await expect(unavailable).toContainText("Image unavailable");
  await expect(gallery.locator("img")).toHaveCount(1);
  const failed = await spacing();
  expect(failed.margins).toEqual([
    ["0px", "0px"],
    ["0px", "0px"],
  ]);
  expect(failed.gap).toBe(failed.parentGap);
  expect(failed.readingOverflow).toBe(0);
  expect(failed.pageOverflow).toBe(0);
});
