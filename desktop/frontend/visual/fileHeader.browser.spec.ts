import { expect, test, type Locator } from "./test";

async function expectOneReadingRow(header: Locator) {
  const centres = await header.evaluate((node) => {
    const identity = node.firstElementChild!.getBoundingClientRect();
    const actions = node.lastElementChild!.getBoundingClientRect();
    return [identity.y + identity.height / 2, actions.y + actions.height / 2];
  });
  expect(Math.abs(centres[0]! - centres[1]!)).toBeLessThanOrEqual(1);
}

for (const [fontSize, theme] of [
  [14, "light"],
  [18, "dark"],
] as const) {
  test(`file identity and fitting actions share a row, then wrap within the narrow Dock ${fontSize}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1472, height: 900 });
    await page.goto(
      `/visual/?fixture=workspace&state=dock-light&agent-state=idle&theme=${theme}&font-size=${fontSize}&locale=en`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    const view = page.locator('[data-dock-view-id="file"]');
    const header = view.locator('[data-slot="workspace-view-header"]');
    await expectOneReadingRow(header);
    await header.getByRole("button", { name: "Back to files", exact: true }).click();
    await page.getByRole("treeitem", { name: "README.md", exact: true }).click();
    await expect(header.getByTitle("README.md", { exact: true })).toBeVisible();
    await expectOneReadingRow(header);

    const resizer = page.locator(".agent-pane-resizer");
    await resizer.focus();
    await resizer.press("Home");
    expect((await view.boundingBox())!.width).toBeCloseTo(320, 0);
    const identity = header.getByTitle("README.md", { exact: true });
    expect((await identity.boundingBox())!.width).toBeGreaterThan(80);
    for (const control of await header.locator('button,[role="tab"]').all()) {
      await expect(control).toBeInViewport({ ratio: 1 });
      expect(
        await control.evaluate((node) => {
          const box = node.getBoundingClientRect();
          const host = node.closest('[data-slot="workspace-view-header"]')!.getBoundingClientRect();
          return (
            box.left >= host.left &&
            box.right <= host.right &&
            box.top >= host.top &&
            box.bottom <= host.bottom &&
            node.contains(document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2))
          );
        }),
      ).toBe(true);
    }
    expect(await view.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
  });
}

test("file copy feedback follows clipboard acceptance and the current file", async ({ page }) => {
  let accept!: () => void;
  const clipboardReady = new Promise<void>((resolve) => {
    accept = resolve;
  });
  const writes: string[] = [];
  await page.exposeFunction("__writeFilePath", (text: string) => {
    writes.push(text);
    return clipboardReady;
  });
  await page.addInitScript(() => {
    const write = (window as unknown as { __writeFilePath: (text: string) => Promise<void> })
      .__writeFilePath;
    Object.defineProperty(navigator, "clipboard", { value: { writeText: write } });
  });
  await page.goto("/visual/?fixture=workspace&state=dock-light&agent-state=idle&locale=en");
  await page.locator("html[data-visual-ready]").waitFor();
  const view = page.locator('[data-dock-view-id="file"]');
  const header = view.locator('[data-slot="workspace-view-header"]');
  const path = await header.locator("[title]").first().getAttribute("title");
  try {
    await header.getByRole("button", { name: "Copy path", exact: true }).click();
    await expect.poll(() => writes).toEqual([path]);
    await expect(header.getByRole("button", { name: "Path copied", exact: true })).toHaveCount(0);
    accept();
    await expect(header.getByRole("button", { name: "Path copied", exact: true })).toBeVisible();
    await header.getByRole("button", { name: "Back to files", exact: true }).click();
    await page.getByRole("treeitem", { name: "README.md", exact: true }).click();
    await expect(header.getByTitle("README.md", { exact: true })).toBeVisible();
    await expect(header.getByRole("button", { name: "Path copied", exact: true })).toHaveCount(0);
    await header.getByRole("button", { name: "Copy path", exact: true }).click();
    await expect.poll(() => writes).toEqual([path, "README.md"]);
    await expect(header.getByRole("button", { name: "Path copied", exact: true })).toBeVisible();
  } finally {
    accept();
  }
});
