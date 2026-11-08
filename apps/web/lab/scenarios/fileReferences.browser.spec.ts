import { expect, test } from "./test";
import { type Query } from "@tanstack/react-query";
import { expectLabReady } from "../readiness";
const REFERENCE = "apps/web/src/features/chat/message/ui/markdown/markdownComponents.tsx:12:3";

for (const [width, fontSize, theme] of [
  [1472, 14, "light"],
  [800, 18, "dark"],
] as const) {
  test(`file references read as part of their sentence at ${width}px in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 1000 });
    await page.goto(
      `/lab/fixture.html?fixture=agent&state=prose-blocks&theme=${theme}&font-size=${fontSize}`,
    );
    await expectLabReady(page);
    const reading = page.locator(".md").filter({ hasText: "Source references" });
    const reference = reading.getByRole("button", { name: REFERENCE, exact: true });
    await reference.scrollIntoViewIfNeeded();
    await page.evaluate(() => document.fonts.ready);

    const flow = await reference.evaluate((node) => {
      const paragraph = node.parentElement!;
      const prefix = document.createRange();
      prefix.selectNodeContents(paragraph);
      prefix.setEndBefore(node);
      const before = [...prefix.getClientRects()].at(-1)!;
      const fragments = [...node.getClientRects()];
      const bounds = paragraph.getBoundingClientRect();
      const selection = window.getSelection()!;
      const range = document.createRange();
      range.selectNodeContents(paragraph);
      selection.removeAllRanges();
      selection.addRange(range);
      const selected = selection.toString();
      selection.removeAllRanges();
      return {
        firstLineOffset: Math.abs(fragments[0]!.top - before.top),
        hasWrapped: fragments.some((rect) => rect.top > fragments[0]!.top + 1),
        fits: fragments.every(
          (rect) => rect.left >= bounds.left - 1 && rect.right <= bounds.right + 1,
        ),
        selected,
        text: paragraph.textContent,
        overflow: paragraph.scrollWidth - paragraph.clientWidth,
      };
    });
    const voice = (node: Element) => {
      const style = getComputedStyle(node);
      return { fontFamily: style.fontFamily, fontSize: style.fontSize };
    };
    expect(await reference.evaluate(voice)).toEqual(
      await reading.locator('[data-slot="inline-code"]').first().evaluate(voice),
    );
    await expect(reference).toHaveCSS("text-decoration-line", "underline");
    await expect(reference).not.toHaveCSS("text-decoration-color", "rgba(0, 0, 0, 0)");

    expect(flow.firstLineOffset).toBeLessThan(fontSize / 2);
    expect(flow.hasWrapped).toBe(true);
    expect(flow.fits).toBe(true);
    expect(flow.overflow).toBe(0);
    expect(flow.selected).toBe(flow.text);
    expect(flow.selected).toContain(`${REFERENCE}. Continue reading`);
    await expect(
      reading.getByRole("button", { name: "src/main.ts:12:3", exact: true }),
    ).toBeVisible();
    await expect(
      reading.getByRole("button", { name: "apps/web/package.json", exact: true }),
    ).toHaveText("apps/web/package.json");
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  });
}

for (const [width, fontSize, theme] of [
  [1472, 14, "light"],
  [1120, 18, "dark"],
] as const) {
  test(`dragging a file reference keeps selection and copy in ${theme}`, async ({
    page,
    context,
    browserName,
  }) => {
    await context.grantPermissions(
      browserName === "webkit" ? ["clipboard-read"] : ["clipboard-read", "clipboard-write"],
    );
    await page.setViewportSize({ width, height: 1000 });
    await page.goto(
      `/lab/fixture.html?fixture=workspace&state=dock-files&agent-state=prose-blocks&theme=${theme}&font-size=${fontSize}`,
    );
    await expectLabReady(page);
    const reading = page.locator(".md").filter({ hasText: "Source references" });
    const fileTree = page.getByRole("treeitem", { name: "reading-guide.md", exact: true });

    for (const label of ["README.md:15:2", REFERENCE]) {
      const reference = reading.getByRole("button", { name: label, exact: true });
      await reference.scrollIntoViewIfNeeded();
      await page.evaluate(() => document.fonts.ready);
      const points = await reference.evaluate((node) => {
        const walker = document.createTreeWalker(node, NodeFilter.SHOW_TEXT);
        const texts: Text[] = [];
        while (walker.nextNode()) texts.push(walker.currentNode as Text);
        const start = document.createRange();
        start.setStart(texts[0]!, 0);
        start.setEnd(texts[0]!, 1);
        const end = document.createRange();
        const last = texts.at(-1)!;
        end.setStart(last, last.length - 1);
        end.setEnd(last, last.length);
        const a = start.getBoundingClientRect();
        const b = end.getBoundingClientRect();
        return {
          start: { x: a.right - 1, y: a.top + a.height / 2 },
          end: { x: b.right - 1, y: b.top + b.height / 2 },
        };
      });
      await page.mouse.move(points.start.x, points.start.y);
      await page.mouse.down();
      await page.mouse.move(points.end.x, points.end.y, { steps: 24 });
      await page.mouse.up();
      await page.evaluate(
        () => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))),
      );

      await expect.poll(() => page.evaluate(() => getSelection()?.toString())).toBe(label.slice(1));
      await expect(fileTree).toBeVisible();
      await page.keyboard.press("ControlOrMeta+C");
      await expect
        .poll(() => page.evaluate(() => navigator.clipboard.readText()))
        .toBe(label.slice(1));

      const viewport = page.locator(".msg-scroll-viewport");
      const bounds = (await viewport.boundingBox())!;
      await viewport.click({ position: { x: 6, y: points.end.y - bounds.y } });
      await expect.poll(() => page.evaluate(() => getSelection()?.isCollapsed)).toBe(true);
      await expect(fileTree).toBeVisible();
    }

    await reading.getByRole("button", { name: "README.md:15:2", exact: true }).click();
    await expect(
      page.locator('[data-quote-source="file"][data-quote-path="README.md"]'),
    ).toBeVisible();
  });
}

test("inline references retain keyboard navigation to the recorded file and line", async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 1472, height: 1000 });
  await page.goto(
    "/lab/fixture.html?fixture=workspace&state=dock-files&agent-state=prose-blocks&locale=en&theme=light&font-size=18&motion=off",
  );
  await expectLabReady(page);
  await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
  const draft = "Compare the README verification steps with the package scripts";
  const composer = page.getByRole("textbox", { name: "Message composer", exact: true });
  await composer.fill(draft);
  const reading = page.locator(".md").filter({ hasText: "Source references" });
  const reference = reading.getByRole("button", { name: "README.md:15:2", exact: true });
  await reference.focus();
  await expect(reference).toBeFocused();
  await reference.evaluate((node) => {
    const range = document.createRange();
    range.selectNodeContents(node);
    getSelection()!.removeAllRanges();
    getSelection()!.addRange(range);
  });
  await reference.press("Enter");
  const view = page
    .locator('[data-dock-view-id="file"]')
    .filter({ has: page.locator('[data-slot="workspace-view-body"]') });
  const sourceMode = view.getByRole("radio", { name: "Source", exact: true });
  const previewMode = view.getByRole("radio", { name: "Preview", exact: true });
  await expect(sourceMode).toHaveAttribute("aria-checked", "true");
  const source = view.locator('[data-quote-source="file"][data-quote-path="README.md"]');
  await expect(source.locator('[data-quote-line="15"]')).toBeVisible();
  await previewMode.click();
  await expect(previewMode).toHaveAttribute("aria-checked", "true");
  await expect(source).toHaveCount(0);
  await sourceMode.click();
  await expect(sourceMode).toHaveAttribute("aria-checked", "true");
  await expect(source.locator('[data-quote-line="15"]')).toBeVisible();

  await page.setViewportSize({ width: 720, height: 450 });
  await expect(
    page.getByRole("button", { name: "Back to conversation", exact: true }),
  ).toBeVisible();
  await expect(
    view.locator('[data-slot="workspace-view-header"]').getByTitle("README.md", { exact: true }),
  ).toBeVisible();

  const goToLine = view.getByRole("button", { name: "Go to line", exact: true });
  await goToLine.click();
  const lineField = page.getByRole("textbox", { name: "Go to line", exact: true });
  await lineField.fill("17");
  await lineField.press("Enter");
  await expect(lineField).toBeHidden();
  await expect(goToLine).toBeFocused();
  await expect(sourceMode).toHaveAttribute("aria-checked", "true");
  await expect(source.locator('[data-quote-line="17"]')).toBeVisible();

  const reader = view.getByRole("region", { name: "README.md", exact: true });
  await page.keyboard.press("Tab");
  await expect(view.getByRole("button", { name: "Copy path", exact: true })).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(reader).toBeFocused();
  await reader.press("PageDown");
  await expect(source.locator('[data-quote-line="24"]')).toBeInViewport({ ratio: 1 });
  // A short file may already sit at its native edge, so only the edge is required.
  await expect
    .poll(() => reader.evaluate((node) => node.scrollHeight - node.clientHeight - node.scrollTop))
    .toBeLessThanOrEqual(1);
  await expect(source.locator('[data-quote-line="17"]')).toBeVisible();
  const sourceChecked = await sourceMode.getAttribute("aria-checked");
  const readmeReading = await reader.evaluate((node) => {
    const viewport = node.getBoundingClientRect();
    const top = viewport.top + node.clientTop;
    const bottom = top + node.clientHeight;
    const lines = [...node.querySelectorAll("[data-quote-line]")]
      .filter((row) => {
        const box = row.getBoundingClientRect();
        return box.bottom > top && box.top < bottom;
      })
      .map((row) => Number(row.getAttribute("data-quote-line")));
    return {
      viewport: { width: innerWidth, height: innerHeight },
      scrollTop: node.scrollTop,
      clientHeight: node.clientHeight,
      scrollHeight: node.scrollHeight,
      firstVisibleLine: lines[0],
      lastVisibleLine: lines.at(-1),
      focused: node.ownerDocument.activeElement === node,
    };
  });
  await testInfo.attach("readme-source-reading.png", {
    body: await page.screenshot({ animations: "disabled", caret: "hide" }),
    contentType: "image/png",
  });

  await reader.press("Escape");
  await expect(reference).toBeFocused();
  await expect(reference).toBeInViewport();
  await expect(composer).toHaveValue(draft);
  const readmeReturnedTo = await reference.evaluate(
    (node) => document.activeElement === node && node.getAttribute("aria-label"),
  );

  const next = reading.getByRole("button", {
    name: "apps/web/package.json",
    exact: true,
  });
  await next.focus();
  await next.evaluate((node) => {
    const range = document.createRange();
    range.selectNodeContents(node);
    getSelection()!.removeAllRanges();
    getSelection()!.addRange(range);
  });
  const transcript = page.locator(".msg-scroll-viewport");
  const transcriptScrollTop = await transcript.evaluate((node) => node.scrollTop);
  await next.press("Space");
  const packageSource = view.locator(
    '[data-quote-source="file"][data-quote-path="apps/web/package.json"]',
  );
  await expect(packageSource).toBeVisible();
  const packageReader = view.getByRole("region", { name: "apps/web/package.json", exact: true });
  await packageReader.focus();
  await expect(packageReader).toBeFocused();
  await testInfo.attach("package-source-reading.png", {
    body: await page.screenshot({ animations: "disabled", caret: "hide" }),
    contentType: "image/png",
  });
  await expect(packageSource).toContainText('"name": "@flame/web"');
  await packageReader.press("Escape");
  await expect(next).toBeFocused();
  await expect(next).toBeInViewport();
  await expect.poll(() => transcript.evaluate((node) => node.scrollTop)).toBe(transcriptScrollTop);
  await expect(composer).toHaveValue(draft);
  const packageReturnedTo = await next.evaluate(
    (node) => document.activeElement === node && node.getAttribute("aria-label"),
  );
  const transcriptScrollTopAfter = await transcript.evaluate((node) => node.scrollTop);

  const suffix = " before the next release";
  await composer.focus();
  await composer.evaluate((node: HTMLTextAreaElement) =>
    node.setSelectionRange(node.value.length, node.value.length),
  );
  await page.keyboard.type(suffix);
  await expect(composer).toHaveValue(`${draft}${suffix}`);
  await testInfo.attach("returned-conversation-draft.png", {
    body: await page.screenshot({ animations: "disabled", caret: "hide" }),
    contentType: "image/png",
  });
  await testInfo.attach("file-inspection-return.json", {
    body: JSON.stringify(
      {
        readme: { sourceChecked, reading: readmeReading, returnedTo: readmeReturnedTo },
        package: {
          returnedTo: packageReturnedTo,
          transcriptScrollTopBefore: transcriptScrollTop,
          transcriptScrollTopAfter,
        },
        draft: await composer.inputValue(),
      },
      null,
      2,
    ),
    contentType: "application/json",
  });
});

for (const [fontSize, codeFontSize, theme] of [
  [14, 20, "light"],
  [18, 10, "dark"],
] as const) {
  test(`file source keeps line numbers with the code scale in ${theme}`, async ({ page }) => {
    await page.setViewportSize({ width: 1120, height: 900 });
    await page.goto(
      `/lab/fixture.html?fixture=workspace&state=dock-files&theme=${theme}&font-size=${fontSize}`,
    );
    await expectLabReady(page);
    await page.getByRole("treeitem", { name: "reading-guide.md", exact: true }).click();
    const view = page.locator('[data-dock-view-id="file"]');
    await view.getByRole("radio", { name: "Source", exact: true }).click();
    const source = view.locator('[data-quote-source="file"]');
    await expect(source).toBeVisible();
    await page.evaluate(async (size) => {
      if (!location.pathname.startsWith("/lab/"))
        throw new Error("An isolated fixture is required");
      const fixturePath = "/lab/scenarios/fixtureRuntime.tsx";
      const { visualClient } = (await import(fixturePath)) as typeof import("./fixtureRuntime");
      visualClient.theme.store.getState().setCodeFontSize(size);
    }, codeFontSize);
    await expect(source).toHaveCSS("font-size", `${codeFontSize}px`);
    const gutter = source.locator('[data-quote-line="2"] > span').first();
    await expect(gutter).toHaveCSS("font-size", `${codeFontSize}px`);
    await expect(gutter).toHaveCSS(
      "line-height",
      await source.evaluate((node) => getComputedStyle(node).lineHeight),
    );

    await page.evaluate(async () => {
      if (!location.pathname.startsWith("/lab/"))
        throw new Error("An isolated fixture is required");
      const clientPath = "/src/lib/queryClient.ts";
      const queryPath = "/src/features/workspace/application/workspaceQueries.ts";
      const { queryClient } = await import(clientPath);
      const { WORKSPACE_READ_FILE_KEY } = await import(queryPath);
      await queryClient.cancelQueries({ queryKey: [WORKSPACE_READ_FILE_KEY] });
      queryClient.setQueriesData(
        { queryKey: [WORKSPACE_READ_FILE_KEY] },
        { content: "first\n\n  final", startLine: 999998, endLine: 1000000, totalLines: 1000000 },
      );
    });
    await expect(source.locator('[data-quote-line="1000000"]')).toBeVisible();
    await page.evaluate(() => document.fonts.ready);
    const rows = await source.evaluate((node) =>
      [...node.querySelectorAll("[data-quote-line]")].map((row) => {
        const gutter = row.children[0]!;
        const code = row.children[1]!;
        const range = document.createRange();
        range.selectNodeContents(gutter);
        const number = range.getBoundingClientRect();
        const slot = gutter.getBoundingClientRect();
        return {
          number: gutter.textContent,
          text: code.textContent,
          numberFits: number.left >= slot.left - 1 && number.right <= slot.right + 1,
          codeLeft: code.getBoundingClientRect().left,
          numberRight: number.right,
          height: row.getBoundingClientRect().height,
          lineHeight: Number.parseFloat(getComputedStyle(code).lineHeight),
        };
      }),
    );
    expect(rows.map((row) => row.number)).toEqual(["999998", "999999", "1000000"]);
    expect(rows.map((row) => row.text?.trimEnd())).toEqual(["first", "", "  final"]);
    for (const row of rows) {
      expect(row.numberFits).toBe(true);
      expect(row.numberRight).toBeLessThan(row.codeLeft);
      expect(row.height).toBeCloseTo(row.lineHeight, 1);
      expect(row.codeLeft).toBe(rows[0]!.codeLeft);
    }
    expect(await source.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  });
}

test("file page changes keep keyboard focus, reset reading position and copy the current material", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/lab/fixture.html?fixture=workspace&state=dock-files&locale=en");
  await expectLabReady(page);
  await page.getByRole("treeitem", { name: "README.md", exact: true }).click();
  const view = page.locator('[data-dock-view-id="file"]');
  await view.getByRole("radio", { name: "Source", exact: true }).click();
  const source = view.locator('[data-quote-source="file"]');
  await expect(source).toBeVisible();
  const suffix = await page.evaluate(async () => {
    if (!location.pathname.startsWith("/lab/")) throw new Error("An isolated fixture is required");
    const clientPath = "/src/lib/queryClient.ts";
    const queryPath = "/src/features/workspace/application/workspaceQueries.ts";
    const { queryClient } = await import(clientPath);
    const { WORKSPACE_READ_FILE_KEY } = await import(queryPath);
    await queryClient.cancelQueries({ queryKey: [WORKSPACE_READ_FILE_KEY] });
    const query = queryClient
      .getQueryCache()
      .findAll({ queryKey: [WORKSPACE_READ_FILE_KEY] })
      .find((entry: Query) => entry.isActive());
    if (!query) throw new Error("The current file read is missing");
    const params = query.queryKey[1] as { cwd?: string; path: string };
    const lines = Array.from(
      { length: 100 },
      (_, index) => `source line ${index + 1}: ${"material ".repeat(12)}`,
    );
    lines.push("");
    const first = {
      content: lines.slice(0, 80).join("\n"),
      startLine: 1,
      endLine: 80,
      totalLines: lines.length,
      truncated: true,
    };
    const last = {
      content: lines.slice(80).join("\n"),
      startLine: 81,
      endLine: 101,
      totalLines: lines.length,
      truncated: true,
    };
    const nextKey = [WORKSPACE_READ_FILE_KEY, { ...params, startLine: 81, endLine: 280 }];
    const previousKey = [
      WORKSPACE_READ_FILE_KEY,
      { ...params, startLine: 1, endLine: 80, fromEnd: true },
    ];
    for (const key of [query.queryKey, nextKey, previousKey])
      queryClient.setQueryDefaults(key, { staleTime: Infinity });
    queryClient.setQueryData(nextKey, last);
    queryClient.setQueryData(previousKey, first);
    queryClient.setQueryData(query.queryKey, first);
    return last.content;
  });
  const reader = view.getByRole("region", { name: "README.md", exact: true });
  const next = view.getByRole("button", { name: /more/i });
  await next.focus();
  await next.press("Enter");
  await expect(source.locator('[data-quote-line="81"]')).toBeVisible();
  await expect(source.locator('[data-quote-line="1"]')).toHaveCount(0);
  await expect(reader).toBeFocused();
  await expect.poll(() => reader.evaluate((node) => node.scrollTop)).toBe(0);
  await reader.press("ControlOrMeta+A");
  await reader.press("ControlOrMeta+C");
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(suffix);
  const previous = view.getByRole("button", { name: /earlier/i });
  await previous.focus();
  await previous.press("Enter");
  await expect(source.locator('[data-quote-line="1"]')).toBeVisible();
  await expect(source.locator('[data-quote-line="81"]')).toHaveCount(0);
  await expect(reader).toBeFocused();
  await expect.poll(() => reader.evaluate((node) => node.scrollTop)).toBe(0);
});
