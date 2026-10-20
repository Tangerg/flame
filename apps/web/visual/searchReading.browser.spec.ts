import { contrastRatio, WCAG_AA_TEXT } from "@/features/theme/kit/legibility";
import { expect, test, type Locator, type Page } from "./test";
async function openOutput(page: Page, query = "") {
  await page.goto(`/visual/?fixture=agent&state=tool-shells${query}`);
  await page.locator("html[data-visual-ready]").waitFor();
  await page.getByRole("button", { name: /steps/ }).first().click();
  const command = page.locator('[data-tool="shell"]').first();
  await command.getByRole("button").first().click();
  await command.getByRole("button", { name: /earlier line/ }).click();
  const output = command.getByRole("region", { name: "Tool output", exact: true });
  await expect(output).toBeVisible();
  return { command, output };
}

for (const [theme, accent] of [
  ["light", ""],
  ["dark", "&accent=%238b5cf6"],
] as const) {
  test(`output find identifies its current line and clears without leaving search in ${theme}`, async ({
    page,
  }) => {
    const { command, output } = await openOutput(page, `&theme=${theme}${accent}`);
    await output.press("ControlOrMeta+f");
    const find = command.getByRole("searchbox", { name: "Find in output" });
    await expect(find).toBeFocused();
    await find.fill("Rollback");
    const current = output.locator("[data-current-match]");
    await expect(command.getByText("1 of 3")).toBeVisible();
    await expect(current).toContainText("=== RUN   TestRollbackOnFlushFailure");
    const rowHeight = (await current.boundingBox())!.height;
    const paint = await current.evaluate((node) => {
      const style = getComputedStyle(node);
      return { background: style.backgroundColor, rule: style.boxShadow };
    });
    expect(paint.background).not.toBe("rgba(0, 0, 0, 0)");
    expect(paint.rule).not.toBe("none");
    await find.press("Enter");
    await expect(command.getByText("2 of 3")).toBeVisible();
    await expect(current).toHaveText("    store_test.go:214: expected rollback, got commit");
    await expect(find).toBeFocused();
    await find.press("Shift+Enter");
    await expect(command.getByText("1 of 3")).toBeVisible();
    expect((await current.boundingBox())!.height).toBe(rowHeight);

    await command.getByRole("button", { name: "Clear search", exact: true }).click();
    await expect(find).toHaveValue("");
    await expect(find).toBeFocused();
    await expect(current).toHaveCount(0);
    await expect(command.getByRole("button", { name: "Clear search" })).toHaveCount(0);
    await find.fill("no such captured line");
    await expect(command.getByText("No matches", { exact: true })).toBeVisible();
    await find.press("Escape");
    await expect(find).toHaveCount(0);
    await expect(output).toBeFocused();
  });

  test(`message search uses readable theme emphasis and retires its ranges in ${theme}`, async ({
    page,
  }) => {
    await page.goto(`/visual/?fixture=agent&state=long-content&theme=${theme}${accent}`);
    await page.locator("html[data-visual-ready]").waitFor();
    const pairs = await page.evaluate(async () => {
      const rangesPath = "/src/features/chat/chat-search/adapters/messageRanges.ts";
      const paintPath = "/src/features/chat/chat-search/adapters/searchHighlights.ts";
      const { findMessageRanges } = await import(rangesPath);
      const { paintChatSearchHighlights } = await import(paintPath);
      const ranges: Range[] = findMessageRanges("Runtime");
      if (ranges.length < 2) throw new Error("The captured answer must contain repeated evidence");
      paintChatSearchHighlights(ranges, 1);
      if (CSS.highlights.get("search-match-active")?.has(ranges[1]!) !== true)
        throw new Error("The actual highlight registry must identify the current range");
      const node = ranges[1]!.startContainer.parentElement!;
      const canvas = document.createElement("canvas");
      canvas.width = canvas.height = 1;
      const context = canvas.getContext("2d", { willReadFrequently: true })!;
      const resolve = (colors: string[]) => {
        context.clearRect(0, 0, 1, 1);
        for (const color of colors) {
          context.fillStyle = color;
          context.fillRect(0, 0, 1, 1);
        }
        const pixel = context.getImageData(0, 0, 1, 1).data;
        if (pixel[3] !== 255) throw new Error("Contrast needs the composed opaque plane");
        return `rgb(${pixel[0]}, ${pixel[1]}, ${pixel[2]})`;
      };
      const planes: string[] = [];
      for (let at: Element | null = node; at; at = at.parentElement)
        planes.unshift(getComputedStyle(at).backgroundColor);
      return ["search-match", "search-match-active"].map((name) => {
        const style = getComputedStyle(node, `::highlight(${name})`);
        return {
          name,
          foreground: resolve([style.color]),
          background: resolve([...planes, style.backgroundColor]),
        };
      });
    });
    for (const pair of pairs)
      expect(contrastRatio(pair.foreground, pair.background), pair.name).toBeGreaterThanOrEqual(
        WCAG_AA_TEXT,
      );
    await page.evaluate(async () => {
      const path = "/src/features/chat/chat-search/adapters/searchHighlights.ts";
      const { clearChatSearchHighlights } = await import(path);
      clearChatSearchHighlights();
    });
    expect(
      await page.evaluate(() =>
        ["search-match", "search-match-active"].some((name) => CSS.highlights.has(name)),
      ),
    ).toBe(false);
  });
}

test("a virtual output search locates text across ANSI spans without changing row metrics", async ({
  page,
}) => {
  const { command, output } = await openOutput(page, "&theme=light&long-output=1");
  await output.press("ControlOrMeta+f");
  const find = command.getByRole("searchbox", { name: "Find in output" });
  await find.fill("line 31415");
  const current = output.locator("[data-current-match]");
  await expect(current).toHaveText("build output line 31415");
  await expect(current).toBeInViewport();
  await expect(find).toBeFocused();
  expect(await output.locator("[data-output-line]").count()).toBeLessThan(100);
  const ink = current.locator("[data-output-line] > span");
  expect(await ink.count()).toBeGreaterThan(1);
  expect(await ink.last().evaluate((node) => getComputedStyle(node).color)).not.toBe(
    await output.evaluate((node) => getComputedStyle(node).color),
  );
  await command.evaluate((node) => (node as HTMLElement).style.setProperty("--fs-code", "16px"));
  await expect
    .poll(() =>
      current.evaluate((node) =>
        Math.abs(
          node.getBoundingClientRect().height - parseFloat(getComputedStyle(node).lineHeight),
        ),
      ),
    )
    .toBeLessThan(0.1);
  await expect(current).toBeInViewport();
});

async function expectVerticalContinuation(page: Page, inner: Locator, outer: Locator) {
  const outerBox = (await outer.boundingBox())!;
  await page.mouse.move(outerBox.x + 10, outerBox.y + 10);
  await page.mouse.wheel(0, -120);
  await inner.scrollIntoViewIfNeeded();
  const outerTop = () => outer.evaluate((node) => node.scrollTop);
  const pointInside = async () => {
    const box = (await inner.boundingBox())!;
    const viewport = (await outer.boundingBox())!;
    const top = Math.max(box.y, viewport.y, 0);
    const bottom = Math.min(
      box.y + box.height,
      viewport.y + viewport.height,
      page.viewportSize()!.height,
    );
    expect(bottom - top).toBeGreaterThan(10);
    await page.mouse.move(box.x + box.width / 2, (top + bottom) / 2);
  };
  await inner.evaluate((node) => {
    node.scrollTop = node.scrollHeight;
  });
  await pointInside();
  const beforeDown = await outerTop();
  await page.mouse.wheel(0, 160);
  await expect.poll(outerTop).toBeGreaterThan(beforeDown);
  await inner.evaluate((node) => {
    node.scrollTop = 0;
  });
  await pointInside();
  const beforeUp = await outerTop();
  await page.mouse.wheel(0, -160);
  await expect.poll(outerTop).toBeLessThan(beforeUp);
}

test("bounded output, inspector code and Dock evidence release vertical scrolling at their edges", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1180, height: 640 });
  const { output } = await openOutput(page, "&theme=light&long-output=1");
  await expectVerticalContinuation(page, output, page.locator(".msg-scroll-viewport"));

  await page.goto("/visual/?fixture=agent&state=tool-agentic&theme=light");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.getByRole("button", { name: /calls/ }).first().click();
  await page.locator('[data-tool="inspect_workspace"]').click();
  const result = page.getByRole("region", { name: "Result", exact: true });
  await expect(result.locator("code .line").first()).toBeVisible();
  await expectVerticalContinuation(page, result, page.locator(".msg-scroll-viewport"));

  await page.goto(
    "/visual/?fixture=workspace&state=dock-trajectory&agent-state=prose-blocks&theme=dark&font-size=18",
  );
  await page.locator("html[data-visual-ready]").waitFor();
  const dock = page.locator('[data-dock-view-id="package:visual:trajectory"]');
  const record = dock.locator('[data-trajectory-record="item:item_reading_blocks"]');
  await record.getByRole("button").first().click();
  await record.getByRole("button", { name: "Recorded evidence", exact: true }).click();
  const evidence = record.getByRole("region", { name: "Recorded evidence", exact: true });
  const outer = dock.locator('[data-slot="panel"]').locator("..");
  await expectVerticalContinuation(page, evidence, outer);
});
