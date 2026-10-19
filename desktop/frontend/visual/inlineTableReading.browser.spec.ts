import { expect, test } from "./test";

for (const [width, fontSize, theme] of [
  [1472, 14, "light"],
  [320, 18, "dark"],
] as const) {
  test(`long inline tables keep their controls, headings and full reading path ${width}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 720 });
    await page.goto(
      `/visual/?fixture=workspace&state=dock-light&agent-state=table-reading&theme=${theme}&font-size=${fontSize}`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    await page.evaluate(() => document.fonts.ready);
    if (width === 320) {
      await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
      await expect(
        page.getByRole("button", { name: "Expand the workspace fixture sidebar" }),
      ).toHaveAttribute("aria-expanded", "false");
    }

    const transcript = page.locator(".msg-scroll-viewport");
    const table = page.locator("[data-markdown-table]");
    const reader = table.locator(".md-table-scroller");
    expect(await reader.evaluate((node) => node.scrollHeight - node.clientHeight)).toBeGreaterThan(
      0,
    );
    await expect(reader).toHaveAttribute("role", "region");
    await expect(reader).toHaveAccessibleName("Table content");
    await expect(reader.getByRole("row")).toHaveCount(81);
    await transcript.evaluate((node) => node.scrollTo(0, 0));
    await table.scrollIntoViewIfNeeded();
    await table.hover();
    const frame = (await reader.boundingBox())!;
    const heading = reader.getByRole("columnheader", { name: "Observation", exact: true });
    for (const name of ["Expand table", "Copy table"]) {
      const action = table.getByRole("button", { name, exact: true });
      await expect(action).toBeInViewport({ ratio: 1 });
      const bounds = (await action.boundingBox())!;
      expect(bounds.y + bounds.height).toBeLessThanOrEqual(frame.y);
      expect(
        await action.evaluate((node) => {
          const box = node.getBoundingClientRect();
          return node.contains(
            document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2),
          );
        }),
      ).toBe(true);
    }

    await reader.focus();
    const outerTop = await transcript.evaluate((node) => node.scrollTop);
    await page.keyboard.down("ArrowDown");
    try {
      await expect.poll(() => reader.evaluate((node) => node.scrollTop)).toBeGreaterThan(0);
    } finally {
      await page.keyboard.up("ArrowDown");
    }
    expect(await transcript.evaluate((node) => node.scrollTop)).toBe(outerTop);
    await reader.evaluate((node) => node.scrollTo(0, node.scrollHeight));
    await expect(
      reader.getByRole("cell", { name: "Observation 80", exact: true }),
    ).toBeInViewport();
    expect((await heading.boundingBox())!.y).toBeCloseTo(frame.y, 0);
    await expect(table.getByRole("button", { name: "Expand table", exact: true })).toBeInViewport();

    await transcript.evaluate((node) => node.scrollTo(0, 0));
    const visible = (await reader.boundingBox())!;
    await page.mouse.move(visible.x + visible.width / 2, visible.y + 80);
    await page.mouse.wheel(0, 240);
    await expect.poll(() => transcript.evaluate((node) => node.scrollTop)).toBeGreaterThan(0);
    await expect(
      page.getByText("The explanation continues after the recorded evidence.", { exact: true }),
    ).toBeInViewport();
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  });
}
