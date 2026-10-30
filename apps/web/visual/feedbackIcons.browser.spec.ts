import { expect, test } from "./test";
for (const pane of ["providers", "mcp-servers"] as const) {
  for (const width of [1120, 320]) {
    test(`${pane} feedback keeps its declared icon viewport while text wraps at ${width}`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: width === 1120 ? 720 : 900 });
      const theme = width === 1120 ? "light" : "dark";
      await page.goto(
        `/visual/?fixture=workspace&state=settings&pane=${pane}&theme=${theme}&font-size=18&locale=de`,
      );
      await page.locator("html[data-visual-ready]").waitFor();
      await page.evaluate(() => document.fonts.ready);
      const panel = page.getByRole("tabpanel");
      if (pane === "providers") {
        const row = panel.getByRole("article").filter({
          has: page.getByLabel("openai API-Schlüssel", { exact: true }),
        });
        await row.getByRole("button", { name: "Testen", exact: true }).click();
      } else {
        await panel.getByRole("button", { name: "Server hinzufügen", exact: true }).click();
        await panel
          .getByRole("textbox", { name: "Servername", exact: true })
          .fill("workspace-tools");
        await panel.getByRole("textbox", { name: "Befehl", exact: true }).fill("npx");
        await panel.getByRole("button", { name: "Testen", exact: true }).click();
      }
      const alert = panel.getByRole("alert");
      await expect(alert).toHaveText(
        pane === "providers"
          ? 'Visual Runtime has no response for "providers.test"'
          : 'Visual Runtime has no response for "mcp.servers.test"',
      );
      await alert.scrollIntoViewIfNeeded();
      const icon = alert.locator("svg");
      const dimensions = await icon.evaluate((node) => {
        const box = node.getBoundingClientRect();
        return {
          width: box.width,
          height: box.height,
          declaredWidth: Number(node.getAttribute("width")),
          declaredHeight: Number(node.getAttribute("height")),
        };
      });
      expect(dimensions.declaredWidth).toBeGreaterThan(0);
      expect(dimensions.width).toBe(dimensions.declaredWidth);
      expect(dimensions.height).toBe(dimensions.declaredHeight);
      expect(dimensions.width).toBe(dimensions.height);
      const textLayout = await alert.evaluate((node) => ({
        height: node.getBoundingClientRect().height,
        lineHeight: parseFloat(getComputedStyle(node).lineHeight),
        whiteSpace: getComputedStyle(node).whiteSpace,
        overflow: node.scrollWidth - node.clientWidth,
      }));
      expect(textLayout.whiteSpace).toBe("normal");
      if (width === 320) expect(textLayout.height).toBeGreaterThan(textLayout.lineHeight);
      expect(textLayout.overflow).toBe(0);
      expect(
        await page.locator("html").evaluate((node) => node.scrollWidth - node.clientWidth),
      ).toBe(0);
    });
  }
}
