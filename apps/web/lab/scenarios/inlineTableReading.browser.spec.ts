import { expect, test } from "./test";
import { expectLabReady } from "../readiness";
for (const [width, fontSize, theme] of [
  [1472, 14, "light"],
  [320, 18, "dark"],
] as const) {
  test(`long inline tables keep their controls, headings and full reading path ${width}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 720 });
    await page.goto(
      `/lab/fixture.html?fixture=workspace&state=dock-light&agent-state=table-reading&theme=${theme}&font-size=${fontSize}`,
    );
    await expectLabReady(page);
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

const webkit = test.extend({ browserName: "webkit" });
for (const { locale, theme, fontSize, motion, lines, continuation } of [
  {
    locale: "en",
    theme: "light",
    fontSize: 14,
    motion: "full",
    lines: [
      "Keep the recorded table context.",
      "Read the final observation.",
      "Return to this conversation draft.",
    ],
    continuation: " Continued.",
  },
  {
    locale: "zh",
    theme: "dark",
    fontSize: 18,
    motion: "off",
    lines: ["保留表格阅读的上下文。", "检查最后一条记录。", "返回这份对话草稿。"],
    continuation: " 继续阅读。",
  },
] as const) {
  webkit(
    `WebKit keeps a keyboard-focused table expand action visible through reflow and draft return ${locale}`,
    async ({ page }, testInfo) => {
      const labels =
        locale === "en"
          ? {
              content: "Table content",
              copy: "Copy table",
              expand: "Expand table",
              preview: "Table preview",
              composer: "Message composer",
              regenerate: "Regenerate response",
              message: "Copy message",
            }
          : {
              content: "表格内容",
              copy: "复制表格",
              expand: "展开表格",
              preview: "表格预览",
              composer: "消息输入框",
              regenerate: "重新生成回复",
              message: "复制消息",
            };
      const draft = lines.join("\n");
      const observations: unknown[] = [];
      const table = page.locator("[data-markdown-table]");
      const reader = table.getByRole("region", { name: labels.content, exact: true });
      const copyTable = table.getByRole("button", { name: labels.copy, exact: true });
      const expand = table.getByRole("button", { name: labels.expand, exact: true });
      const composer = page.getByRole("textbox", { name: labels.composer, exact: true });
      const observe = async (label: string) => {
        const observation = await expand.evaluate((node, label: string) => {
          const viewport = node.closest<HTMLElement>(".msg-scroll-viewport")!;
          const style = getComputedStyle(viewport);
          const frame = viewport.getBoundingClientRect();
          const box = node.getBoundingClientRect();
          const hit = document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2);
          let opacity = 1;
          for (let level: Element | null = node; level; level = level.parentElement)
            opacity *= parseFloat(getComputedStyle(level).opacity);
          return {
            label,
            window: { width: innerWidth, height: innerHeight, devicePixelRatio },
            focused: node === document.activeElement,
            focusVisible: node.matches(":focus-visible"),
            bounds: { top: box.top, bottom: box.bottom, left: box.left, right: box.right },
            frame: {
              top: frame.top + (parseFloat(style.scrollPaddingTop) || 0),
              bottom: frame.bottom - (parseFloat(style.scrollPaddingBottom) || 0),
            },
            readingFrameHeight: viewport.style.getPropertyValue("--reading-frame-height"),
            composerTop: document
              .querySelector('[data-slot="composer-root"]')!
              .getBoundingClientRect().top,
            transcriptScrollTop: viewport.scrollTop,
            hit: hit !== null && node.contains(hit),
            hitTarget: hit
              ? [
                  hit.tagName.toLowerCase(),
                  hit.getAttribute("data-slot") ?? "",
                  hit.getAttribute("aria-label") ?? "",
                  [...hit.classList].join("."),
                ].join("|")
              : null,
            opacity,
          };
        }, label);
        observations.push(observation);
        return observation;
      };
      const assertVisible = async (label: string) => {
        await expect(expand).toBeFocused();
        await expect(expand).toBeInViewport({ ratio: 1 });
        const seen = await observe(label);
        expect(seen.focused).toBe(true);
        expect(seen.focusVisible).toBe(true);
        expect(seen.opacity).toBe(1);
        expect(seen.hit).toBe(true);
        expect(seen.bounds.top).toBeGreaterThanOrEqual(seen.frame.top - 0.5);
        expect(seen.bounds.bottom).toBeLessThanOrEqual(
          Math.min(seen.frame.bottom, seen.composerTop) + 0.5,
        );
      };

      try {
        await page.setViewportSize({ width: 1440, height: 900 });
        await page.goto(
          `/lab/fixture.html?fixture=workspace&state=dock-light&agent-state=table-reading&locale=${locale}&theme=${theme}&font-size=${fontSize}&motion=${motion}`,
        );
        await expectLabReady(page);
        await page.evaluate(() => document.fonts.ready);
        await page
          .getByRole("button", { name: "Collapse the workspace fixture sidebar" })
          .press("Enter");
        await expect(
          page.getByRole("button", { name: "Expand the workspace fixture sidebar" }),
        ).toHaveAttribute("aria-expanded", "false");

        await composer.focus();
        for (const [index, line] of lines.entries()) {
          if (index > 0) await page.keyboard.press("Shift+Enter");
          await page.keyboard.type(line);
        }
        await expect(composer).toHaveValue(draft);
        const input = (await composer.elementHandle())!;

        for (const stop of [
          page.getByRole("button", { name: labels.regenerate, exact: true }),
          page.getByRole("button", { name: labels.message, exact: true }).last(),
          reader,
        ]) {
          await page.keyboard.press("Shift+Tab");
          await expect(stop).toBeFocused();
        }
        const readerNode = (await reader.elementHandle())!;
        await page.keyboard.press("End");
        await expect
          .poll(() =>
            reader.evaluate((node) => node.scrollHeight - node.clientHeight - node.scrollTop),
          )
          .toBeLessThanOrEqual(1);
        await expect(
          reader.getByRole("cell", { name: "Observation 80", exact: true }),
        ).toBeInViewport();
        for (const stop of [copyTable, expand]) {
          await page.keyboard.press("Shift+Tab");
          await expect(stop).toBeFocused();
        }
        const expandNode = (await expand.elementHandle())!;
        await assertVisible("1440x900");

        const retained = async () => {
          expect(
            await expandNode.evaluate(
              (node) => node.isConnected && node === document.activeElement,
            ),
          ).toBe(true);
          expect(await expand.evaluate((node, held) => node === held, expandNode)).toBe(true);
          expect(await reader.evaluate((node, held) => node === held, readerNode)).toBe(true);
          expect(await composer.evaluate((node, held) => node === held, input)).toBe(true);
          await expect(composer).toHaveValue(draft);
        };
        // Successive CSS reading budgets exercise reflow; native page zoom is qualified separately.
        const steps = Array.from({ length: 20 }, (_, index) => ({
          width: Math.round(1440 / (1 + (index + 1) / 20)),
          height: Math.round(900 / (1 + (index + 1) / 20)),
        }));
        steps.push({ width: 560, height: 364 });
        for (const size of steps) {
          await page.setViewportSize(size);
          await page.evaluate(
            () =>
              new Promise<void>((resolve) =>
                requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
              ),
          );
          await retained();
          await assertVisible(`${size.width}x${size.height}`);
        }

        await page.keyboard.press("Enter");
        const preview = page.getByRole("dialog", { name: labels.preview, exact: true });
        const previewReader = preview.getByRole("region", { name: labels.content, exact: true });
        await expect(previewReader).toBeFocused();
        await page.keyboard.press("End");
        await expect(
          preview.getByRole("cell", { name: "Observation 80", exact: true }),
        ).toBeInViewport();
        for (let step = 0; step < 10; step++) await page.keyboard.press("ArrowRight");
        await expect
          .poll(() =>
            previewReader.evaluate((node) => node.scrollWidth - node.clientWidth - node.scrollLeft),
          )
          .toBeLessThanOrEqual(1);
        await expect(preview.getByRole("cell", { name: "80", exact: true })).toBeInViewport({
          ratio: 1,
        });
        await testInfo.attach("table-preview-tail", {
          body: await page.screenshot({ animations: "disabled" }),
          contentType: "image/png",
        });
        await page.keyboard.press("Escape");
        await expect(preview).toHaveCount(0);
        await retained();
        await assertVisible("preview-return");

        for (const stop of [
          copyTable,
          reader,
          page.getByRole("button", { name: labels.message, exact: true }).last(),
          page.getByRole("button", { name: labels.regenerate, exact: true }),
          composer,
        ]) {
          await page.keyboard.press("Tab");
          await expect(stop).toBeFocused();
        }
        expect(await composer.evaluate((node, held) => node === held, input)).toBe(true);
        await expect(composer).toHaveValue(draft);
        await page.keyboard.press(process.platform === "darwin" ? "Meta+ArrowDown" : "Control+End");
        expect(
          await composer.evaluate((node: HTMLTextAreaElement) => [
            node.selectionStart,
            node.selectionEnd,
          ]),
        ).toEqual([draft.length, draft.length]);
        await page.keyboard.type(continuation);
        await expect(composer).toHaveValue(draft + continuation);
        await expect(composer).toBeFocused();
      } finally {
        if ((await expand.count()) === 1) await observe("final");
        await testInfo.attach("table-expand-focus-observations", {
          body: JSON.stringify(observations, null, 2),
          contentType: "application/json",
        });
        await testInfo.attach("table-expand-focus-final", {
          body: await page.screenshot({ animations: "disabled" }),
          contentType: "image/png",
        });
      }
    },
  );
}
