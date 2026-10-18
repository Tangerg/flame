import { expect, test } from "./test";

for (const [theme, fontSize, width] of [
  ["light", 14, 1120],
  ["dark", 18, 800],
] as const) {
  test(`generic tool results stay primary and scrollable before and after highlighting in ${theme}`, async ({
    page,
  }) => {
    let releaseHighlight!: () => void;
    const highlight = new Promise<void>((resolve) => (releaseHighlight = resolve));
    let requested = false;
    await page.route("**/node_modules/.vite/deps/shiki.js*", async (route) => {
      requested = true;
      await highlight;
      await route.continue();
    });
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      `/visual/?fixture=agent&state=tool-agentic&theme=${theme}&font-size=${fontSize}`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    await page.evaluate(() => document.fonts.ready);
    await page.getByRole("button", { name: /calls/ }).first().click();
    const trigger = page.locator('[data-tool="inspect_workspace"]');
    await trigger.click();
    const result = page.getByRole("region", { name: "Result", exact: true });
    const argumentsButton = page.getByRole("button", { name: "Arguments", exact: true });
    const source = () => result;
    const assertResult = async () => {
      const value = JSON.parse(await source().innerText());
      expect(value).toHaveProperty("summary", { status: "reviewed", files: 24 });
      expect(value).toHaveProperty("files.length", 24);
      expect(value).toHaveProperty("files.23", {
        path: "runtime/internal/session/state_24.go",
        status: "reviewed",
      });
      const dimensions = await result.evaluate((node) => ({
        height: node.clientHeight,
        maximum: Number.parseFloat(getComputedStyle(node).maxHeight),
        content: node.scrollHeight,
      }));
      expect(dimensions.height).toBeLessThanOrEqual(dimensions.maximum);
      expect(dimensions.content).toBeGreaterThan(dimensions.height);
      expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
    };
    try {
      await expect.poll(() => requested).toBe(true);
      await expect(argumentsButton).toHaveAttribute("aria-expanded", "false");
      await expect(source().locator("code .line")).toHaveCount(0);
      await assertResult();
      const before = (await result.boundingBox())!;
      await result.focus();
      await expect(result).toBeFocused();
      releaseHighlight();
      await result.locator("code .line").first().waitFor();
      await assertResult();
      const after = (await result.boundingBox())!;
      expect({ width: after.width, height: after.height }).toEqual({
        width: before.width,
        height: before.height,
      });
      await expect(result).toBeFocused();
      await page.keyboard.press("Tab");
      await expect(argumentsButton).toBeFocused();
      await result.focus();
      await result.press("PageDown");
      await expect.poll(() => result.evaluate((node) => node.scrollTop)).toBeGreaterThan(0);
      await page.evaluate(() => {
        Object.defineProperty(navigator, "clipboard", {
          configurable: true,
          value: {
            writeText: async (value: string) => {
              (window as unknown as Record<string, unknown>).inspectorCopied = value;
            },
          },
        });
      });
      await result.locator("..").getByRole("button", { name: "Copy code", exact: true }).click();
      await expect
        .poll(() =>
          page.evaluate(() => (window as unknown as Record<string, unknown>).inspectorCopied),
        )
        .toBe(await source().innerText());

      await argumentsButton.click();
      const args = page.getByRole("region", { name: "Arguments", exact: true });
      await args.locator("code .line").first().waitFor();
      const value = JSON.parse(await args.innerText());
      expect(value).toEqual({
        workspace: {
          root: `/Users/visual/Flame workspaces/${"application-boundary-".repeat(12)}review`,
          include: ["runtime/internal/session/**", "desktop/frontend/src/**"],
        },
        checks: ["ownership", "dependency-direction"],
        notes: "Keep <component> tags and source paths as literal text.",
      });
      await expect(args.locator("component")).toHaveCount(0);
      const frame = args.locator("..");
      await expect(frame.getByRole("button", { name: "Copy code", exact: true })).toHaveCount(0);
      const copyArguments = trigger
        .locator("..")
        .getByRole("button", { name: "Copy arguments", exact: true });
      await expect(copyArguments).toHaveCount(1);
      await trigger.hover();
      await copyArguments.click();
      expect(
        JSON.parse(
          (await page.evaluate(
            () => (window as unknown as Record<string, unknown>).inspectorCopied,
          )) as string,
        ),
      ).toEqual(value);
      const wrap = frame.getByRole("button", { name: "Enable word wrap", exact: true });
      await wrap.click();
      await expect
        .poll(() => args.evaluate((node) => node.scrollWidth - node.clientWidth))
        .toBeLessThanOrEqual(1);
      await expect(
        page.getByRole("button", { name: "Disable word wrap", exact: true }),
      ).toHaveCount(2);
      expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
    } finally {
      releaseHighlight();
    }
  });
}
