import { expect, test } from "./test";
test("a folded user message reveals keyboard focus above the composer and folds cleanly again", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1120, height: 1000 });
  await page.goto("/visual/?fixture=agent&state=idle&theme=light&font-size=18");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.evaluate(async () => {
    if (!location.pathname.startsWith("/visual/"))
      throw new Error("An isolated fixture is required");
    const fixturePath = "/visual/agentSessionSnapshots.ts";
    const storePath = "/src/features/agent/adapters/agentStore.ts";
    const projectionPath = "/src/features/agent/application/session/sessionSnapshot.ts";
    const fixture = (await import(fixturePath)) as typeof import("./agentSessionSnapshots");
    const { useAgentStore } = await import(storePath);
    const { projectAgentSessionSnapshot } = await import(projectionPath);
    const snapshot = structuredClone(fixture.RUNTIME_AGENT_SESSION_SNAPSHOTS.idle);
    const prompt = snapshot.items.find((item) => item.type === "userMessage");
    if (!prompt) throw new Error("The fixture must contain an accepted user message");
    const image =
      "data:image/svg+xml," +
      encodeURIComponent(
        '<svg xmlns="http://www.w3.org/2000/svg" width="160" height="80"><rect width="160" height="80" fill="#3674e8"/></svg>',
      );
    const text =
      Array.from(
        { length: 12 },
        (_, index) =>
          `Observation ${index + 1}: keep the accepted request and its context visible.`,
      ).join("\n\n") + `\n\n![Captured detail](${image})`;
    snapshot.items = [{ ...prompt, content: [{ type: "text", text }] }];
    const store = useAgentStore.getState();
    const refresh = store.beginViewRefresh("ses_visual", true);
    if (
      !refresh ||
      !store.commitViewRefresh(
        "ses_visual",
        refresh,
        projectAgentSessionSnapshot(fixture.projectRuntimeAgentSnapshot(snapshot)),
      )
    )
      throw new Error("The fixture snapshot could not be installed");
  });
  const fold = page.locator('[data-slot="user-message-fold"]');
  const toggle = page.getByRole("button", { name: "Show full message", exact: true });
  await expect(fold).toHaveAttribute("data-folded", "");
  await toggle.focus();
  await page.keyboard.press("Shift+Tab");

  const image = fold.getByRole("button", { name: "Captured detail", exact: true });
  await expect(image).toBeFocused();
  await expect(fold).not.toHaveAttribute("data-folded");
  await expect(fold).toHaveCSS("mask-image", "none");
  expect(await fold.evaluate((node) => node.scrollTop)).toBe(0);
  const focused = (await image.boundingBox())!;
  const viewport = (await page.locator(".msg-scroll-viewport").boundingBox())!;
  const composer = (await page.locator('[data-slot="composer-root"]').boundingBox())!;
  expect(focused.y).toBeGreaterThanOrEqual(viewport.y);
  expect(focused.y + focused.height).toBeLessThanOrEqual(composer.y);

  await page.keyboard.press("Enter");
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(image).toBeFocused();

  await page.keyboard.press("Tab");
  const collapse = page.getByRole("button", { name: "Show less", exact: true });
  await expect(collapse).toBeFocused();
  await page.keyboard.press("Space");
  await expect(toggle).toBeFocused();
  await expect(fold).toHaveAttribute("data-folded", "");
  expect(await fold.evaluate((node) => node.scrollTop)).toBe(0);
});

for (const [width, height, theme] of [
  [1120, 720, "light"],
  [560, 360, "dark"],
] as const) {
  test(`a ${theme} diagram keeps its frame fitted and its zoomed canvas operable`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height });
    await page.goto(`/visual/?fixture=agent&state=long-content&theme=${theme}&font-size=14`);
    await page.locator("html[data-visual-ready]").waitFor();
    const open = page.getByRole("button", { name: "Enlarge diagram", exact: true });
    await open.focus();
    await page.keyboard.press("Enter");
    const dialog = page.getByRole("dialog", { name: "Diagram", exact: true });
    const canvas = dialog.getByRole("region", { name: "Diagram", exact: true });
    const image = canvas.getByRole("img", { name: "Diagram", exact: true });
    await expect(dialog).toBeVisible();
    await expect(image).toBeVisible();
    const frame = await dialog.evaluate((node) => {
      const bounds = node.getBoundingClientRect();
      return {
        overflowX: node.scrollWidth - node.clientWidth,
        overflowY: node.scrollHeight - node.clientHeight,
        contained:
          bounds.left >= 0 &&
          bounds.top >= 0 &&
          bounds.right <= innerWidth &&
          bounds.bottom <= innerHeight,
      };
    });
    expect(frame).toEqual({ overflowX: 0, overflowY: 0, contained: true });
    const svg = image.locator("svg");
    const naturalWidth = await svg.evaluate(
      (node) => (node as SVGSVGElement).viewBox.baseVal.width,
    );
    await dialog.getByRole("button", { name: "100%", exact: true }).click();
    expect((await svg.boundingBox())!.width).toBeCloseTo(naturalWidth, 0);
    for (let step = 0; step < 3; step++)
      await dialog.getByRole("button", { name: "Zoom in diagram", exact: true }).click();
    await expect(dialog.getByRole("button", { name: "300%", exact: true })).toBeVisible();
    expect((await svg.boundingBox())!.width).toBeCloseTo(naturalWidth * 3, 0);
    expect(await dialog.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
    expect(await dialog.evaluate((node) => node.scrollHeight - node.clientHeight)).toBe(0);

    const close = dialog.getByRole("button", { name: "Close", exact: true });
    await close.focus();
    await page.keyboard.press("Tab");
    await expect(canvas).toBeFocused();
    await page.keyboard.down("ArrowRight");
    try {
      await expect.poll(() => canvas.evaluate((node) => node.scrollLeft)).toBeGreaterThan(0);
    } finally {
      await page.keyboard.up("ArrowRight");
    }
    const prior = await canvas.evaluate((node) => node.scrollLeft);
    const bounds = (await canvas.boundingBox())!;
    await page.mouse.move(bounds.x + bounds.width * 0.7, bounds.y + bounds.height * 0.6);
    await page.mouse.down();
    await page.mouse.move(bounds.x + bounds.width * 0.7 - 60, bounds.y + bounds.height * 0.6, {
      steps: 8,
    });
    await page.mouse.up();
    await expect.poll(() => canvas.evaluate((node) => node.scrollLeft)).toBeGreaterThan(prior);

    await dialog.getByRole("button", { name: "Fit", exact: true }).click();
    await expect.poll(() => canvas.evaluate((node) => node.scrollLeft)).toBe(0);
    const fitted = (await svg.boundingBox())!;
    const area = (await canvas.boundingBox())!;
    expect(fitted.width).toBeLessThanOrEqual(area.width);
    expect(fitted.height).toBeLessThanOrEqual(area.height);
    await close.click();
    await expect(dialog).toHaveCount(0);
    await expect(open).toBeFocused();
  });
}
