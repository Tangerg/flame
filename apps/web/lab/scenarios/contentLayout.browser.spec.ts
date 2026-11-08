import { expect, test } from "./test";
import { REVIEW_DIFF } from "./diffSnapshot";
import { expectLabReady } from "../readiness";
for (const theme of ["light", "dark"]) {
  test(`command search keeps its query and last option inside a short viewport in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 900, height: 360 });
    await page.goto(
      `/lab/fixture.html?fixture=shell&state=populated&overlay=commands&theme=${theme}&font-size=18`,
    );
    await expectLabReady(page);
    await page.locator("html").evaluate((root) => {
      root.style.setProperty("--layout-compact-gutter", "24px");
    });
    const dialog = page.getByRole("dialog", { name: "Open command menu", exact: true });
    const query = dialog.getByRole("combobox");
    await expect(query).toBeFocused();
    await query.press("ArrowUp");
    const last = dialog.getByRole("option").last();
    await expect(last).toHaveAttribute("aria-selected", "true");
    const bounds = (await dialog.boundingBox())!;
    const lastBounds = (await last.boundingBox())!;
    const queryBounds = (await query.boundingBox())!;
    expect(bounds.y).toBeGreaterThanOrEqual(24);
    expect(bounds.y + bounds.height).toBeLessThanOrEqual(336);
    expect(lastBounds.y + lastBounds.height).toBeLessThanOrEqual(bounds.y + bounds.height);
    expect(queryBounds.y).toBeGreaterThanOrEqual(bounds.y);
    expect(queryBounds.y + queryBounds.height).toBeLessThanOrEqual(lastBounds.y);
    await query.press("ArrowDown");
    await expect(dialog.getByRole("option").first()).toHaveAttribute("aria-selected", "true");
    await query.press("Escape");
    await expect(dialog).toHaveCount(0);
  });
}

for (const overlay of ["commands", "finder"]) {
  test(`${overlay} search reveals its current result after wheel scrolling and filtering`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 560, height: 360 });
    await page.goto(
      `/lab/fixture.html?fixture=shell&state=populated&overlay=${overlay}&theme=dark&font-size=18`,
    );
    await expectLabReady(page);
    const dialog = page.getByRole("dialog");
    const query = dialog.getByRole("combobox");
    const first = dialog.getByRole("option").first();
    await expect(query).toBeFocused();
    await first.hover();
    await expect(first).toHaveAttribute("aria-selected", "true");
    const top = (await first.boundingBox())!.y;
    await page.mouse.wheel(0, 1000);
    await expect.poll(async () => (await first.boundingBox())!.y).toBeLessThan(top);
    await expect(first).not.toBeInViewport();

    await query.fill("e");
    await expect(first).toHaveAttribute("aria-selected", "true");
    await expect(query).toHaveAttribute("aria-activedescendant", (await first.getAttribute("id"))!);
    await expect(first).toBeInViewport({ ratio: 1 });
    await expect(query).toBeFocused();
    await query.fill("");
    await expect(first).toHaveAttribute("aria-selected", "true");
    await expect(query).toHaveAttribute("aria-activedescendant", (await first.getAttribute("id"))!);
    await expect(first).toBeInViewport({ ratio: 1 });
    await expect(query).toBeFocused();
    await query.press("Escape");
    await expect(dialog).toHaveCount(0);
  });
}

for (const theme of ["light", "dark"]) {
  test(`Diff change counts share the worktree summary's semantic ink in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1472, height: 900 });
    await page.goto(
      `/lab/fixture.html?fixture=workspace&state=dock-review&theme=${theme}&font-size=${theme === "light" ? 14 : 18}&accent=${theme === "light" ? "%23FB6E30" : "%238b5cf6"}`,
    );
    await expectLabReady(page);
    const view = page.locator('[data-dock-view-id="package:visual:diff"]');
    const summary = view.locator('[data-slot="panel-controls"]');
    const worktree = page.getByRole("button", {
      name: "Review working tree changes (2) +25 −9",
      exact: true,
    });
    const inks = async (root: typeof summary, added: string, removed: string) =>
      root.evaluate(
        (node, values) =>
          values.map((value) => {
            const label = [...node.querySelectorAll("span")].find(
              (element) => element.textContent === value,
            );
            return label ? getComputedStyle(label).color : null;
          }),
        [added, removed],
      );
    const file = await inks(summary, "+18", "−6");
    expect(file).not.toContain(null);
    expect(file[0]).not.toBe(file[1]);
    expect(file).toEqual(await inks(worktree, "+25", "−9"));
    await view.getByRole("button", { name: "Show changed files", exact: true }).click();
    const current = view.locator('[data-diff-file][aria-current="true"]');
    await expect(current).toBeFocused();
    expect(await inks(current, "+18", "−6")).toEqual(file);
    await worktree.click();
    await expect(view.getByRole("heading", { name: "2 changed files", exact: true })).toBeVisible();
    await expect(view.locator("[data-diff-file]")).toHaveCount(2);
    await expect(summary).toContainText("Uncommitted changes · Workspace · Baseline: HEAD");
  });

  test(`Diff statistics distinguish known zeroes and unrecorded counts in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1120, height: 900 });
    await page.goto(`/lab/fixture.html?fixture=atoms&theme=${theme}&font-size=18`);
    await expectLabReady(page);
    const specimen = page.locator('[data-atom="diff-stat"]');
    await specimen.scrollIntoViewIfNeeded();
    const counts = specimen.locator('[data-slot="diff-stat"]');
    await expect(counts).toHaveText(["+12 −4", "+0 −0", "+0 −?", "+? −?", "+1234567 −7654321"]);
    const partial = counts.nth(2);
    const quiet = await partial
      .getByText("+0", { exact: true })
      .evaluate((node) => getComputedStyle(node).color);
    expect(
      await partial
        .getByText("−?", { exact: true })
        .evaluate((node) => getComputedStyle(node).color),
    ).toBe(quiet);
    const large = counts.last();
    await large.evaluate((node) => {
      node.style.inlineSize = "10ch";
    });
    const added = (await large.getByText("+1234567", { exact: true }).boundingBox())!;
    const removed = (await large.getByText("−7654321", { exact: true }).boundingBox())!;
    expect(removed.y).toBeGreaterThanOrEqual(added.y + added.height);
    expect(await large.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
  });

  test(`Diff opens the requested file and returns to its list identity in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1472, height: 900 });
    await page.goto(
      `/lab/fixture.html?fixture=workspace&state=dock-review&theme=${theme}&font-size=${theme === "light" ? 14 : 18}${theme === "dark" ? "&accent=%238b5cf6" : ""}`,
    );
    await expectLabReady(page);
    const view = page.locator('[data-dock-view-id="package:visual:diff"]');
    await expect(view.getByRole("heading", { name: "DockResizer.tsx", exact: true })).toBeVisible();
    await expect(view.getByRole("table")).toHaveCount(1);
    const code = view.getByRole("region", { name: /^Scrollable diff for / });
    const source = REVIEW_DIFF.files![0]!.rows.filter((row) => row.type !== "hunk").map(
      (row) => row.code,
    );
    await expect(code.locator("tbody code")).toHaveText(source);
    const background = await code.evaluate((node) => getComputedStyle(node).backgroundColor);
    await code.hover();
    expect(await code.evaluate((node) => getComputedStyle(node).backgroundColor)).toBe(background);
    await view.getByRole("button", { name: "Show changed files", exact: true }).click();
    const current = view.locator('[data-diff-file][aria-current="true"]');
    await expect(current).toHaveAttribute("title", REVIEW_DIFF.files![0]!.path);
    await expect(current).toBeFocused();
    await expect(view.getByRole("table")).toHaveCount(0);
    await current.press("Enter");
    await expect(code.locator("tbody code")).toHaveText(source);
    await expect(view.getByRole("heading", { name: "DockResizer.tsx", exact: true })).toBeFocused();
  });

  test(`Diff keeps scope and file navigation reachable while its list and reader scroll in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1472, height: 900 });
    await page.goto(
      `/lab/fixture.html?fixture=workspace&state=dock-review&theme=${theme}&font-size=18&review-files=200`,
    );
    await expectLabReady(page);
    const view = page.locator('[data-dock-view-id="package:visual:diff"]');
    const filter = view.getByRole("searchbox", { name: "Filter files", exact: true });
    const viewport = view.locator('[data-slot="workspace-view-body"]');
    const controls = view.locator('[data-slot="panel-controls"]');
    await view.getByRole("button", { name: "Show changed files", exact: true }).click();
    await expect(view.locator("[data-diff-file]")).toHaveCount(200);
    const acceptedScope = controls.getByText(
      /^Uncommitted changes · Workspace · Baseline: HEAD · 12345678 · Read at /,
    );
    const readAt = (await controls.locator("time").getAttribute("datetime"))!;
    await view.locator("[data-diff-file]").first().hover();
    await page.mouse.wheel(0, 600);
    await expect.poll(() => viewport.evaluate((node) => node.scrollTop)).toBeGreaterThan(0);
    for (const control of [
      filter,
      view.getByRole("combobox", { name: "Comparison", exact: true }),
      view.getByRole("button", { name: "Refresh", exact: true }),
    ]) {
      await expect(control).toBeInViewport({ ratio: 1 });
    }
    await filter.fill("module-199.ts");
    const row = view.locator("[data-diff-file]");
    await expect(row).toHaveCount(1);
    await expect(row).toHaveAttribute("title", "src/generated/module-199.ts");
    await row.click();
    await expect(view.getByRole("table")).toHaveCount(1);
    await expect(view.getByRole("heading", { name: "module-199.ts", exact: true })).toBeFocused();
    await view.getByRole("region", { name: /^Scrollable diff for / }).hover();
    await page.mouse.wheel(0, 600);
    await expect.poll(() => viewport.evaluate((node) => node.scrollTop)).toBeGreaterThan(0);
    for (const control of [
      view.getByRole("button", { name: "Back to changed files", exact: true }),
      view.getByRole("combobox", { name: "Diff layout", exact: true }),
      view.getByRole("button", { name: "Refresh", exact: true }),
      acceptedScope,
    ]) {
      await expect(control).toBeInViewport({ ratio: 1 });
    }
    await expect(controls.locator("time")).toHaveAttribute("datetime", readAt);
    await page.keyboard.press("Escape");
    await expect(filter).toHaveValue("module-199.ts");
    await expect(row).toBeFocused();
    await expect(view.getByRole("combobox", { name: "Comparison", exact: true })).toBeInViewport({
      ratio: 1,
    });
  });

  for (const locale of ["de", "zh"] as const) {
    const labels =
      locale === "de"
        ? {
            composer: "Nachrichteneditor",
            showFiles: "Geänderte Dateien anzeigen",
            filter: "Dateien filtern",
            openMain: "In voller Breite öffnen",
            layout: "Diff-Layout",
            reader: "Scrollbarer Diff für src/generated/module-199.ts",
            back: "Zurück zu geänderten Dateien",
          }
        : {
            composer: "消息输入框",
            showFiles: "显示已更改文件",
            filter: "筛选文件",
            openMain: "在主区打开",
            layout: "Diff 布局",
            reader: "src/generated/module-199.ts 的可滚动 diff",
            back: "返回已更改文件",
          };
    test(`a zoomed full-width Diff reader paints its final source line below its controls in ${theme} ${locale}`, async ({
      page,
    }, testInfo) => {
      await page.setViewportSize({ width: 1472, height: 900 });
      await page.goto(
        `/lab/fixture.html?fixture=workspace&state=dock-review&theme=${theme}&font-size=18&locale=${locale}&review-files=200`,
      );
      await expectLabReady(page);
      const draft = "Entwurf beim Diff-Lesen behalten. Keep this diff reading draft.";
      const composer = page.getByRole("textbox", { name: labels.composer, exact: true });
      await composer.fill(draft);
      const view = page.locator('[data-dock-view-id="package:visual:diff"]');
      await view.getByRole("button", { name: labels.showFiles, exact: true }).click();
      const filter = view.getByRole("searchbox", { name: labels.filter, exact: true });
      await filter.fill("module-19");
      await expect(view.locator("[data-diff-file]")).toHaveCount(10);
      const readAt = (await view
        .locator('[data-slot="panel-controls"] time')
        .getAttribute("datetime"))!;
      await view.locator('[data-diff-file][title="src/generated/module-199.ts"]').press("Enter");
      await expect(view.getByRole("heading", { name: "module-199.ts", exact: true })).toBeFocused();

      await page.getByRole("button", { name: labels.openMain, exact: true }).click();
      await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
      const body = page.locator('[data-slot="workspace-view-body"]:visible');
      const controls = body.locator('[data-slot="panel-controls"]');
      const heading = body.getByRole("heading", { name: "module-199.ts", exact: true });
      const reader = body.getByRole("region", {
        name: labels.reader,
        exact: true,
      });
      await expect(heading).toBeVisible();

      await page.setViewportSize({ width: 373, height: 243 });
      await heading.focus();
      await page.keyboard.press("Tab");
      await expect(body.getByRole("combobox", { name: labels.layout, exact: true })).toBeFocused();
      await page.keyboard.press("Tab");
      await expect(reader).toBeFocused();

      // Wide source scrolls horizontally, so only the visible part of the line must be hit-testable.
      const finalLine = () =>
        reader
          .locator("tbody code")
          .last()
          .evaluate((code) => {
            const box = (rect: DOMRect) => ({
              top: rect.top,
              bottom: rect.bottom,
              left: rect.left,
              right: rect.right,
            });
            const viewport = code.closest('[data-slot="workspace-view-body"]')!;
            const body = viewport.getBoundingClientRect();
            const well = code.closest('[role="region"]')!.getBoundingClientRect();
            const line = code.getBoundingClientRect();
            const range = document.createRange();
            range.selectNodeContents(code);
            const glyphs = range.getBoundingClientRect();
            const left = Math.max(glyphs.left, well.left, body.left, 0);
            const right = Math.min(glyphs.right, well.right, body.right, innerWidth);
            const hits = [line.top + 2, (line.top + line.bottom) / 2, line.bottom - 2].map((y) => {
              const hit = document.elementFromPoint((left + right) / 2, y);
              return {
                y,
                code: !!hit && code.contains(hit),
                slot: hit?.closest("[data-slot]")?.getAttribute("data-slot") ?? null,
              };
            });
            const controls = viewport.querySelector('[data-slot="panel-controls"]');
            return {
              window: { width: innerWidth, height: innerHeight },
              body: box(body),
              scroll: {
                top: viewport.scrollTop,
                range: viewport.scrollHeight - viewport.clientHeight,
              },
              controls: controls && box(controls.getBoundingClientRect()),
              text: code.textContent,
              line: box(line),
              visible: { left, right },
              hits,
            };
          });
      for (const [step, height] of [243, 720, 243].entries()) {
        await page.setViewportSize({ width: 373, height });
        await page.keyboard.press("End");
        let geometry: Awaited<ReturnType<typeof finalLine>>;
        try {
          await expect
            .poll(() =>
              body.evaluate((node) => node.scrollHeight - node.clientHeight - node.scrollTop),
            )
            .toBeLessThanOrEqual(1);
        } finally {
          const screenshot = await page.screenshot({
            animations: "disabled",
            caret: "hide",
            scale: "device",
          });
          geometry = await finalLine();
          await testInfo.attach(`diff-final-line-${theme}-${locale}-${step}-${height}.json`, {
            body: JSON.stringify(geometry, null, 2),
            contentType: "application/json",
          });
          await testInfo.attach(`diff-final-line-${theme}-${locale}-${step}-${height}.png`, {
            body: screenshot,
            contentType: "image/png",
          });
        }
        expect(geometry.text).toBe("export const value24 = compute(199, 24); // after");
        expect(geometry.line.top).toBeGreaterThanOrEqual(Math.max(geometry.body.top, 0) - 0.5);
        expect(geometry.line.bottom).toBeLessThanOrEqual(
          Math.min(geometry.body.bottom, geometry.window.height) + 0.5,
        );
        expect(geometry.visible.right - geometry.visible.left).toBeGreaterThan(0);
        expect(
          geometry.hits.every((hit) => hit.code),
          JSON.stringify(geometry.hits),
        ).toBe(true);
        if (height === 720) {
          for (const control of [
            body.getByRole("button", { name: labels.back, exact: true }),
            body.getByRole("combobox", { name: labels.layout, exact: true }),
            heading,
          ]) {
            await expect(control).toBeInViewport({ ratio: 1 });
          }
        }
        await expect(reader).toBeFocused();
      }

      await expect(controls.locator("time")).toHaveAttribute("datetime", readAt);
      await page.keyboard.press("Escape");
      await expect(body.getByRole("searchbox", { name: labels.filter, exact: true })).toHaveValue(
        "module-19",
      );
      await expect(body.locator("[data-diff-file]")).toHaveCount(10);
      const current = body.locator('[data-diff-file][aria-current="true"]');
      await expect(current).toHaveAttribute("title", "src/generated/module-199.ts");
      await expect(current).toBeFocused();
      await expect(
        page.getByRole("textbox", { name: labels.composer, exact: true, includeHidden: true }),
      ).toHaveValue(draft);
    });
  }

  test(`Diff preserves source columns across row kinds and horizontal scrolling in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1120, height: 720 });
    await page.goto(
      `/lab/fixture.html?fixture=workspace&state=dock-review&theme=${theme}&font-size=${theme === "light" ? 14 : 18}`,
    );
    await expectLabReady(page);
    const view = page.locator('[data-dock-view-id="package:visual:diff"]');
    const lines = REVIEW_DIFF.files![0]!.rows.flatMap((row, index) =>
      row.type === "hunk" ? [] : [{ type: row.type, line: String(index), text: row.code }],
    );
    const identities = (type: "added" | "deleted" | null) =>
      lines.filter((line) => line.type !== type).map(({ line, text }) => ({ line, text }));
    const deletion = lines.find((line) => line.type === "deleted")!;
    const [firstAddition, unpairedAddition] = lines.filter((line) => line.type === "added");
    for (const layout of ["unified", "split"] as const) {
      if (layout === "split") {
        await view.getByRole("combobox", { name: "Diff layout", exact: true }).click();
        await page.getByRole("option", { name: "Side by side", exact: true }).click();
      }
      const code = view.getByRole("region", { name: /^Scrollable diff for / });
      await expect(code.locator("table")).toHaveAttribute("data-layout", layout);
      let origin = 0;
      for (const offset of [0, 80]) {
        await code.evaluate((node, offset) => {
          node.scrollLeft = offset;
        }, offset);
        const rendered = await code.locator("tbody tr").evaluateAll((rows) =>
          rows
            .filter((row) => row.querySelector("code"))
            .map((row) =>
              Array.from(row.querySelectorAll("code"), (code) => {
                const range = document.createRange();
                range.selectNodeContents(code);
                return {
                  line: code.closest("td")!.getAttribute("data-diff-line"),
                  text: code.textContent,
                  left: range.getBoundingClientRect().left,
                };
              }),
            ),
        );
        const column = (index: number) => rendered.map((row) => row[index]!);
        const sourced = (cells: ReturnType<typeof column>) =>
          cells.filter((cell) => cell.line !== null).map(({ line, text }) => ({ line, text }));
        const pairOf = (line: string) =>
          rendered.find((row) => row.some((cell) => cell.line === line))!.map((cell) => cell.line);
        if (layout === "unified") {
          expect(rendered.map((row) => row.length)).toEqual(lines.map(() => 1));
          expect(sourced(column(0))).toEqual(identities(null));
        } else {
          expect(rendered.every((row) => row.length === 2)).toBe(true);
          expect(sourced(column(0))).toEqual(identities("added"));
          expect(sourced(column(1))).toEqual(identities("deleted"));
          expect(rendered.every((row) => row.some((cell) => cell.line !== null))).toBe(true);
          for (const cell of rendered.flat()) {
            if (cell.line === null) expect(cell.text).toBe("");
          }
          for (const { line } of lines.filter((item) => item.type === "context")) {
            expect(pairOf(line)).toEqual([line, line]);
          }
          expect(pairOf(deletion.line)).toEqual([deletion.line, firstAddition!.line]);
          expect(pairOf(unpairedAddition!.line)).toEqual([null, unpairedAddition!.line]);
        }
        for (const index of rendered[0]!.keys()) {
          const filled = column(index).filter((cell) => cell.text);
          for (const cell of filled) {
            expect(Math.abs(cell.left - filled[0]!.left)).toBeLessThan(0.5);
          }
        }
        const anchor = column(0).find((cell) => cell.text)!.left;
        if (offset) {
          const scrolled = await code.evaluate((node) => node.scrollLeft);
          expect(scrolled).toBeGreaterThan(0);
          expect(Math.abs(anchor - (origin - scrolled))).toBeLessThan(0.5);
        } else {
          origin = anchor;
        }
      }
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  });

  test(`a narrow file dock preserves its identity and wraps actions in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1472, height: 900 });
    await page.goto(
      `/lab/fixture.html?fixture=workspace&state=dock-files&theme=${theme}&font-size=18`,
    );
    await expectLabReady(page);
    await page.getByRole("treeitem", { name: "README.md", exact: true }).click();
    const view = page.locator('[data-dock-view-id="file"]');
    await expect(view.getByRole("radio", { name: "Preview", exact: true })).toBeVisible();
    const resizer = page.locator(".agent-pane-resizer");
    await resizer.focus();
    await resizer.press("Home");
    const header = view.locator('[data-slot="workspace-view-header"]');
    const identity = header.getByTitle("README.md", { exact: true });
    await expect(identity).toBeVisible();
    const bounds = (await header.boundingBox())!;
    const text = (await identity.boundingBox())!;
    expect(text.width).toBeGreaterThan(80);
    expect(text.y).toBeGreaterThanOrEqual(bounds.y);
    expect(text.x + text.width).toBeLessThanOrEqual(bounds.x + bounds.width);
    const escaped = await header.locator('button,[role="tab"]').evaluateAll((nodes) =>
      nodes
        .filter((node) => {
          const rect = node.getBoundingClientRect();
          const host = node.closest('[data-slot="workspace-view-header"]')!.getBoundingClientRect();
          return (
            rect.width > 0 &&
            (rect.x < host.x || rect.right > host.right || rect.bottom > host.bottom)
          );
        })
        .map((node) => node.getAttribute("aria-label") ?? node.textContent),
    );
    expect(escaped).toEqual([]);
    await view.getByRole("radio", { name: "Source", exact: true }).click();
    await expect(view.getByRole("radio", { name: "Source", exact: true })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    await view.getByRole("radio", { name: "Preview", exact: true }).click();
    await expect(view.locator('[data-slot="document-body"]')).toContainText("Workspace notes");
  });

  test(`question options expose their distinguishing tails before selection in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 320, height: 720 });
    await page.goto(
      `/lab/fixture.html?fixture=agent&state=question-multi&theme=${theme}&font-size=18`,
    );
    await expectLabReady(page);
    const labels = [
      "Run all release checks for the selected workspace and include concurrency diagnostics",
      "Run all release checks for the selected workspace and include accessibility diagnostics",
    ] as const;
    for (const label of labels) {
      const choice = page.getByRole("checkbox", { name: label, exact: true });
      const text = choice.getByText(label, { exact: true });
      await text.scrollIntoViewIfNeeded();
      const overflow = await text.evaluate((node) => ({
        x: node.scrollWidth - node.clientWidth,
        y: node.scrollHeight - node.clientHeight,
      }));
      expect(overflow.x, label).toBeLessThanOrEqual(1);
      expect(overflow.y, label).toBeLessThanOrEqual(1);
      const target = (await choice.boundingBox())!;
      const content = (await text.boundingBox())!;
      expect(content.x).toBeGreaterThanOrEqual(target.x);
      expect(content.x + content.width).toBeLessThanOrEqual(target.x + target.width);
      expect(content.y + content.height).toBeLessThanOrEqual(target.y + target.height);
    }
    const last = page.getByRole("checkbox", { name: labels[1], exact: true });
    await last.click();
    await expect(last).toHaveAttribute("aria-checked", "true");
    await page.getByRole("button", { name: "Submit answers", exact: true }).click();
    await expect(page.locator("html")).toHaveAttribute(
      "data-visual-resumed-response",
      JSON.stringify({ type: "answer", answers: [[labels[1]]] }),
    );
  });
}

test("captured Markdown tables let the file reading viewport continue scrolling", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1120, height: 720 });
  await page.goto("/lab/fixture.html?fixture=workspace&state=dock-files&theme=dark&font-size=18");
  await expectLabReady(page);
  await page.getByRole("treeitem", { name: "reading-guide.md", exact: true }).click();
  const view = page.locator('[data-dock-view-id="file"]');
  const table = view.getByRole("region", { name: /Markdown table/ }).first();
  await table.scrollIntoViewIfNeeded();
  const position = () =>
    table.evaluate((node) => {
      for (let parent = node.parentElement; parent; parent = parent.parentElement)
        if (
          parent.scrollHeight > parent.clientHeight &&
          ["auto", "scroll"].includes(getComputedStyle(parent).overflowY)
        )
          return { top: parent.scrollTop, range: parent.scrollHeight - parent.clientHeight };
      throw new Error("No vertical file reading viewport");
    });
  const before = await position();
  expect(before.range).toBeGreaterThan(0);
  const bounds = (await table.boundingBox())!;
  await page.mouse.move(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2);
  await page.mouse.wheel(0, before.top > 0 ? -240 : 240);
  await expect.poll(async () => (await position()).top).not.toBe(before.top);
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
});

test("Diff keeps one file identity above focused code while navigating a review", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1120, height: 720 });
  await page.goto(
    "/lab/fixture.html?fixture=workspace&state=dock-review&theme=dark&font-size=18&review-files=3",
  );
  await expectLabReady(page);
  const view = page.locator('[data-dock-view-id="package:visual:diff"]');
  const viewport = view.locator('[data-slot="workspace-view-body"]');
  const controls = view.locator('[data-slot="panel-controls"]');
  await view.getByRole("button", { name: "Show changed files", exact: true }).click();
  await expect(view.locator("[data-diff-file]")).toHaveCount(3);
  await view.locator("[data-diff-file]").first().click();
  const heading = view.getByRole("heading", { name: "module-000.ts", exact: true });
  const code = view.getByRole("region", { name: /^Scrollable diff for / });
  await code.hover();
  await page.mouse.wheel(0, 600);
  await expect.poll(() => viewport.evaluate((node) => node.scrollTop)).toBeGreaterThan(0);
  await expect(heading).toBeInViewport({ ratio: 1 });
  await code.focus();
  const controlsBottom = (await controls.boundingBox())!.y + (await controls.boundingBox())!.height;
  await expect
    .poll(async () => (await code.locator("tbody tr").first().boundingBox())!.y)
    .toBeGreaterThanOrEqual(controlsBottom);
  await page.keyboard.down("ArrowRight");
  try {
    await expect.poll(() => code.evaluate((node) => node.scrollLeft)).toBeGreaterThan(0);
  } finally {
    await page.keyboard.up("ArrowRight");
  }
  await view.getByRole("button", { name: "Next file", exact: true }).click();
  await expect(view.getByRole("heading", { name: "module-001.ts", exact: true })).toBeFocused();
  await expect(view.getByRole("table")).toHaveCount(1);
  await view.getByRole("button", { name: "Back to changed files", exact: true }).click();
  await expect(view.locator('[data-diff-file][aria-current="true"]')).toHaveAttribute(
    "title",
    "src/generated/module-001.ts",
  );
  await expect(view.locator('[data-diff-file][aria-current="true"]')).toBeFocused();
});

test("Diff keeps long directories outside the sticky file controls", async ({ page }) => {
  const longPath = `node_modules/${"layout-boundary/".repeat(32)}captured-source.ts`;
  for (const width of [1120, 320]) {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      `/lab/fixture.html?fixture=workspace&state=full-view&full-view=package:visual:diff&sidebar=collapsed&theme=dark&font-size=18&review-path=${encodeURIComponent(longPath)}`,
    );
    await expectLabReady(page);
    const viewport = page.locator('[data-slot="workspace-view-body"]:visible').first();
    await viewport.getByRole("button", { name: "Show changed files", exact: true }).click();
    await expect(viewport.locator("[data-diff-file]")).toHaveCount(2);
    await viewport.locator("[data-diff-file]").first().click();
    const controls = viewport.locator('[data-slot="panel-controls"]');
    await expect(
      controls.getByRole("heading", { name: "captured-source.ts", exact: true }),
    ).toBeVisible();
    await expect(controls).not.toContainText("layout-boundary");
    const reader = viewport.getByRole("region", { name: `Changes in ${longPath}`, exact: true });
    await expect(reader).toContainText(longPath);
    const code = reader.getByRole("region", { name: /^Scrollable diff for / });
    await code.focus();
    const controlsBounds = (await controls.boundingBox())!;
    const row = (await code.locator("tbody tr").first().boundingBox())!;
    const viewportBounds = (await viewport.boundingBox())!;
    expect(row.y).toBeGreaterThanOrEqual(controlsBounds.y + controlsBounds.height);
    expect(row.y + row.height).toBeLessThanOrEqual(viewportBounds.y + viewportBounds.height);
    await expect(code).toBeFocused();
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  }
});

for (const theme of ["light", "dark"]) {
  test(`a horizontal-only code well joins the keyboard scroll path in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1472, height: 900 });
    await page.goto(`/lab/fixture.html?fixture=atoms&theme=${theme}&font-size=18`);
    await expectLabReady(page);
    const specimen = page.locator('[data-atom="well-path"]');
    const well = specimen.locator("pre").last();
    await well.scrollIntoViewIfNeeded();
    const overflow = await well.evaluate((node) => ({
      horizontal: node.scrollWidth - node.clientWidth,
      vertical: node.scrollHeight - node.clientHeight,
    }));
    expect(overflow.horizontal).toBeGreaterThan(0);
    expect(overflow.vertical).toBeLessThanOrEqual(1);
    await expect(well).toHaveAttribute("tabindex", "0");
    await specimen.getByRole("button", { name: "Before long JSON", exact: true }).focus();
    await page.keyboard.press("Tab");
    await expect(well).toBeFocused();
    await page.keyboard.down("ArrowRight");
    try {
      await expect.poll(() => well.evaluate((node) => node.scrollLeft)).toBeGreaterThan(0);
    } finally {
      await page.keyboard.up("ArrowRight");
    }
  });
}
