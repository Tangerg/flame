import { expect, test } from "./test";
for (const theme of ["light", "dark"] as const) {
  test(`Mermaid paints its theme with local fonts in ${theme}`, async ({ page }) => {
    const fontRequests: string[] = [];
    await page.route("https://fonts.googleapis.com/**", (route) => {
      fontRequests.push(route.request().url());
    });
    await page.goto(`/visual/?fixture=agent&theme=${theme}&state=long-content`, {
      waitUntil: "domcontentloaded",
    });
    await page.locator("html[data-visual-ready]").waitFor();
    const diagram = page.getByRole("img", { name: "Diagram" });
    await expect(diagram).toBeVisible();
    await page.evaluate(
      () =>
        new Promise<void>((resolve) =>
          requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
        ),
    );
    const colors = await diagram.evaluate((root) => {
      const text = root.querySelector("text")!;
      const node = root.querySelector("rect")!;
      return {
        text: getComputedStyle(text).fill,
        node: getComputedStyle(node).fill,
        expectedText: getComputedStyle(root).color,
        font: getComputedStyle(text).fontFamily,
      };
    });
    expect.soft(fontRequests).toEqual([]);
    expect.soft(colors.text).toBe(colors.expectedText);
    expect.soft(colors.node).not.toBe(colors.text);
    expect.soft(colors.font).toContain("system-ui");
    const alignment = await diagram.evaluate((stage) => {
      const frame = stage.getBoundingClientRect();
      const svg = stage.querySelector("svg")!.getBoundingClientRect();
      return Math.abs((svg.left + svg.right) / 2 - (frame.left + frame.right) / 2);
    });
    expect(alignment).toBeLessThan(0.5);
  });
}
