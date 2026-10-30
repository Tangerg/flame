import { expect, test, type Locator } from "./test";
async function expectReadingHierarchy(header: Locator) {
  const geometry = await header.evaluate((node) => {
    const back = node.querySelector("button")!.getBoundingClientRect();
    const identity = node.querySelector("[title]")!.getBoundingClientRect();
    const controls = node
      .querySelector('[data-slot="workspace-view-controls"]')!
      .getBoundingClientRect();
    return { back: back.toJSON(), identity: identity.toJSON(), controls: controls.toJSON() };
  });
  expect(geometry.back.right).toBeLessThanOrEqual(geometry.identity.x);
  expect(
    geometry.identity.bottom <= geometry.controls.y ||
      geometry.identity.right <= geometry.controls.x,
  ).toBe(true);
}

for (const [fontSize, theme] of [
  [14, "light"],
  [18, "dark"],
] as const) {
  test(`file identity leads its reading controls within the narrow Dock ${fontSize}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1472, height: 900 });
    await page.goto(
      `/visual/?fixture=workspace&state=dock-light&agent-state=idle&theme=${theme}&font-size=${fontSize}&locale=en`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    const view = page
      .locator('[data-dock-view-id="file"]')
      .filter({ has: page.locator('[data-slot="workspace-view-header"]') });
    const header = view.locator('[data-slot="workspace-view-header"]');
    await expectReadingHierarchy(header);
    await header.getByRole("button", { name: "Back to files", exact: true }).click();
    await page.getByRole("treeitem", { name: "README.md", exact: true }).click();
    await expect(header.getByTitle("README.md", { exact: true })).toBeVisible();
    await expectReadingHierarchy(header);

    const resizer = page.locator(".agent-pane-resizer");
    await resizer.focus();
    await resizer.press("Home");
    expect((await view.boundingBox())!.width).toBeCloseTo(320, 0);
    const identity = header.getByTitle("README.md", { exact: true });
    expect(
      await identity.evaluate((node) => {
        const range = document.createRange();
        range.selectNodeContents(node);
        const host = node.closest('[data-slot="workspace-view-header"]')!.getBoundingClientRect();
        return [...range.getClientRects()].every(
          (box) => box.width > 0 && box.left >= host.left && box.right <= host.right,
        );
      }),
    ).toBe(true);
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
  const view = page
    .locator('[data-dock-view-id="file"]')
    .filter({ has: page.locator('[data-slot="workspace-view-header"]') });
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
