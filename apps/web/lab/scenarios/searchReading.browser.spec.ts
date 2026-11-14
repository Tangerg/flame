import { contrastRatio, WCAG_AA_TEXT } from "@/features/theme/kit/legibility";
import { expect, test, type Locator, type Page } from "./test";
import { expectLabReady } from "../readiness";
import { en } from "@/lib/i18n/locales/en";
import zh from "../../../../plugins/locale-zh/package/io.github.tangerg.flame/messages.json" with { type: "json" };
async function openOutput(page: Page, query = "") {
  await page.goto(`/lab/fixture.html?fixture=agent&state=tool-shells${query}`);
  await expectLabReady(page);
  await page.getByRole("button", { name: /steps/ }).first().click();
  const command = page.locator('[data-tool="shell"]').first();
  await command.getByRole("button").first().click();
  await command.getByRole("button", { name: /earlier line/ }).click();
  const output = command.getByRole("region", { name: "Tool output", exact: true });
  await expect(output).toBeVisible();
  return { command, output };
}

test("message search excludes retained nested thoughts until their owners reveal them", async ({
  page,
}) => {
  await page.goto("/lab/fixture.html?fixture=agent&theme=light&state=narrative");
  await expectLabReady(page);
  const query = "the component may unmount mid-flight";
  const matches = () =>
    page.evaluate(async (query) => {
      const path = "/src/features/chat/search/adapters/messageRanges.ts";
      const { findMessageMatches } = await import(path);
      return findMessageMatches(query).map(({ range }: { range: Range }) => ({
        text: range.toString(),
        visible: [...range.getClientRects()].some((rect) => rect.width > 0 && rect.height > 0),
        hidden: Boolean(range.startContainer.parentElement?.closest("[inert], [hidden]")),
      }));
    }, query);
  await expect.poll(matches).toEqual([]);
  const process = page.getByRole("button", { name: /read · 1 search · 1 write/ }).first();
  await process.click();
  const thought = page.getByRole("button", { name: "Thought", exact: true });
  await expect(thought).toHaveAttribute("aria-expanded", "false");
  await expect.poll(matches).toEqual([]);
  await thought.click();
  await expect.poll(matches).toEqual([{ text: query, visible: true, hidden: false }]);
  await thought.click();
  await expect(thought).toBeFocused();
  await expect(page.locator(".msg-content").filter({ hasText: query })).toHaveCount(1);
  await expect.poll(matches).toEqual([]);
  await thought.click();
  await expect.poll(matches).toEqual([{ text: query, visible: true, hidden: false }]);
  await process.click();
  await expect.poll(matches).toEqual([]);
  await process.click();
  await expect.poll(matches).toEqual([{ text: query, visible: true, hidden: false }]);
});

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
    await page.goto(`/lab/fixture.html?fixture=agent&state=long-content&theme=${theme}${accent}`);
    await expectLabReady(page);
    const pairs = await page.evaluate(async () => {
      const rangesPath = "/src/features/chat/search/adapters/messageRanges.ts";
      const paintPath = "/src/features/chat/search/adapters/searchHighlights.ts";
      const { findMessageMatches } = (await import(
        rangesPath
      )) as typeof import("@/features/chat/search/adapters/messageRanges");
      const { paintChatSearchHighlights } = await import(paintPath);
      const ranges = findMessageMatches("Runtime").map((match) => match.range);
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
      const path = "/src/features/chat/search/adapters/searchHighlights.ts";
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
  const outerTop = () => outer.evaluate((node) => node.scrollTop);
  const outerBox = (await outer.boundingBox())!;
  await page.mouse.move(outerBox.x + 10, outerBox.y + 10);
  const initialTop = await outerTop();
  if (initialTop > 0) {
    await page.mouse.wheel(0, -120);
    await expect.poll(outerTop).toBeLessThan(initialTop);
  }
  await inner.scrollIntoViewIfNeeded();
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
  await inner.press("End");
  await expect(inner).toBeFocused();
  await expect
    .poll(() => inner.evaluate((node) => node.scrollHeight - node.clientHeight - node.scrollTop))
    .toBeLessThanOrEqual(1);
  await pointInside();
  const beforeDown = await outerTop();
  await page.mouse.wheel(0, 160);
  await expect.poll(outerTop).toBeGreaterThan(beforeDown);
  await inner.press("Home");
  await expect.poll(() => inner.evaluate((node) => node.scrollTop)).toBe(0);
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

  await page.goto("/lab/fixture.html?fixture=agent&state=tool-agentic&theme=light");
  await expectLabReady(page);
  await page.getByRole("button", { name: "4 search · 2 run 6 steps", exact: true }).click();
  await page.locator('[data-tool="inspect_workspace"]').click();
  const result = page.getByRole("region", { name: "Result", exact: true });
  await expect(result.locator("code .line").first()).toBeVisible();
  await expectVerticalContinuation(page, result, page.locator(".msg-scroll-viewport"));

  await page.goto(
    "/lab/fixture.html?fixture=workspace&state=dock-trajectory&agent-state=prose-blocks&theme=dark&font-size=18",
  );
  await expectLabReady(page);
  const dock = page.locator('[data-dock-view-id="package:visual:trajectory"]');
  const record = dock.locator('[data-trajectory-record="item:item_reading_blocks"]');
  await record.getByRole("button").first().click();
  await record.getByRole("button", { name: "Recorded evidence", exact: true }).click();
  const evidence = record.getByRole("region", { name: "Recorded evidence", exact: true });
  const outer = dock.locator('[data-slot="panel"]').locator("..");
  await expectVerticalContinuation(page, evidence, outer);
});

for (const browserName of ["chromium", "webkit"] as const) {
  for (const profile of [
    {
      locale: "en",
      theme: "light",
      fontSize: 14,
      messages: en,
      draft:
        "  Keep this search draft.\nFind the recorded evidence.\n\nReturn without sending.\n  End  ",
      continuation: " Continue after Find.",
    },
    {
      locale: "zh",
      theme: "dark",
      fontSize: 18,
      messages: zh,
      draft: "  保留这份搜索草稿。\n查找已记录的证据。\n\n返回后不要发送。\n  结束  ",
      continuation: "继续核对。",
    },
  ] as const) {
    test.extend({ browserName })(
      `${browserName} conversation Find opens from the draft and retires with material in ${profile.locale}`,
      async ({ page, browserName: actualBrowserName }, testInfo) => {
        expect(actualBrowserName).toBe(browserName);
        const messages: Record<string, string> = profile.messages;
        const name = (key: string) => ({ name: messages[key]!, exact: true });
        const capture = async (label: string) =>
          testInfo.attach(`chat-search-${label}.png`, {
            body: await page.screenshot({ animations: "disabled", caret: "hide" }),
            contentType: "image/png",
          });
        await page.setViewportSize({ width: 1120, height: 755 });
        const link = new URLSearchParams({
          fixture: "workspace",
          state: "dock-trajectory",
          "agent-state": "code-reading",
          locale: profile.locale,
          theme: profile.theme,
          "font-size": String(profile.fontSize),
        });
        await page.goto(`/lab/fixture.html?${link}`);
        await expectLabReady(page);
        await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
        const conversation = page.getByRole("region", name("shell.region.conversation"));
        const composer = page.getByRole("textbox", name("composer.input.label"));
        await composer.fill(profile.draft);
        const input = (await composer.elementHandle())!;
        const bar = conversation.getByRole("search", name("chatSearch.label"));
        const field = bar.locator("input");
        const status = bar.getByRole("status");
        const mounted = page.getByRole("search", {
          ...name("chatSearch.label"),
          includeHidden: true,
        });
        const query = "Runtime";

        const expectDraft = async () => {
          expect(await composer.evaluate((node, held) => node === held, input)).toBe(true);
          await expect(composer).toHaveValue(profile.draft);
        };
        const openFind = async () => {
          await page.keyboard.press("ControlOrMeta+f");
          await expect(field).toBeFocused();
        };
        const matchTotal = async () => {
          await expect(status).toHaveText(/^1 \/ [1-9]\d*$/);
          return Number((await status.textContent())!.split(" / ")[1]);
        };
        const activeMatchGeometry = () =>
          conversation.evaluate((region) => {
            const ranges = [...(CSS.highlights.get("search-match-active") ?? [])];
            const range = ranges[0];
            const parent = range?.startContainer.parentElement;
            const viewport = parent?.closest<HTMLElement>(".msg-scroll-viewport");
            if (ranges.length !== 1 || !(range instanceof Range) || !parent || !viewport)
              return null;
            const outside = viewport.getBoundingClientRect();
            const style = getComputedStyle(viewport);
            const frame = {
              left: Math.max(outside.left + viewport.clientLeft, 0),
              right: Math.min(
                outside.left + viewport.clientLeft + viewport.clientWidth,
                innerWidth,
              ),
              top: outside.top + parseFloat(style.scrollPaddingTop),
              bottom: outside.bottom - parseFloat(style.scrollPaddingBottom),
            };
            const pre = parent.closest("pre");
            if (pre) {
              const box = pre.getBoundingClientRect();
              frame.left = Math.max(frame.left, box.left + pre.clientLeft);
              frame.right = Math.min(frame.right, box.left + pre.clientLeft + pre.clientWidth);
              frame.top = Math.max(frame.top, box.top + pre.clientTop);
              frame.bottom = Math.min(frame.bottom, box.top + pre.clientTop + pre.clientHeight);
            }
            const boxes = [...range.getClientRects()].filter((box) => box.width && box.height);
            return {
              text: range.toString(),
              owned: region.contains(parent),
              frame,
              boxes: boxes.map((box) => box.toJSON()),
              visible:
                boxes.length > 0 &&
                boxes.every(
                  (box) =>
                    box.left >= frame.left - 0.5 &&
                    box.right <= frame.right + 0.5 &&
                    box.top >= frame.top - 0.5 &&
                    box.bottom <= frame.bottom + 0.5,
                ),
            };
          });
        const expectActiveMatch = async (label: string) => {
          try {
            await expect.poll(async () => (await activeMatchGeometry())?.visible).toBe(true);
            const current = await activeMatchGeometry();
            expect(current?.visible).toBe(true);
            expect(current?.owned).toBe(true);
            expect(current?.text.toLowerCase()).toBe(query.toLowerCase());
          } finally {
            await testInfo.attach(`chat-search-${label}-active-match`, {
              body: JSON.stringify(await activeMatchGeometry()),
              contentType: "application/json",
            });
          }
        };
        const expectBar = async (label: string) => {
          await expect(field).toBeFocused();
          const geometry = await conversation.evaluate(
            async (region, node) => {
              await new Promise(requestAnimationFrame);
              await new Promise(requestAnimationFrame);
              const bounds = node.getBoundingClientRect();
              const hit = (element: Element) => {
                const box = element.getBoundingClientRect();
                return element.contains(
                  document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2),
                );
              };
              const controls = [...node.querySelectorAll("input, button, [role='status']")].map(
                (control) => ({
                  label: control.getAttribute("aria-label") ?? control.getAttribute("role"),
                  bounds: control.getBoundingClientRect().toJSON(),
                  hit: hit(control),
                }),
              );
              return {
                viewport: { width: innerWidth, height: innerHeight },
                inside: region.contains(node),
                region: region.getBoundingClientRect().toJSON(),
                bounds: bounds.toJSON(),
                controls,
                overlaps: [
                  ...document.querySelectorAll<HTMLElement>(
                    "button, input, textarea, [role='separator']",
                  ),
                ]
                  .filter((other) => {
                    if (region.contains(other) || other.getClientRects().length === 0) return false;
                    if (getComputedStyle(other).visibility === "hidden") return false;
                    const box = other.getBoundingClientRect();
                    return controls.some(
                      ({ bounds: target }) =>
                        box.right > target.left &&
                        box.left < target.right &&
                        box.bottom > target.top &&
                        box.top < target.bottom,
                    );
                  })
                  .map((other) => ({
                    label: other.getAttribute("aria-label"),
                    bounds: other.getBoundingClientRect().toJSON(),
                  })),
              };
            },
            (await bar.elementHandle())!,
          );
          await testInfo.attach(`chat-search-${label}-geometry`, {
            body: JSON.stringify(geometry),
            contentType: "application/json",
          });
          await capture(label);
          expect(geometry.inside).toBe(true);
          const frame = {
            left: Math.max(geometry.region.left, 0),
            right: Math.min(geometry.region.right, geometry.viewport.width),
            top: Math.max(geometry.region.top, 0),
            bottom: Math.min(geometry.region.bottom, geometry.viewport.height),
          };
          for (const box of [
            geometry.bounds,
            ...geometry.controls.map((control) => control.bounds),
          ]) {
            expect(box.width).toBeGreaterThan(0);
            expect(box.left).toBeGreaterThanOrEqual(frame.left - 1);
            expect(box.right).toBeLessThanOrEqual(frame.right + 1);
            expect(box.top).toBeGreaterThanOrEqual(frame.top - 1);
            expect(box.bottom).toBeLessThanOrEqual(frame.bottom + 1);
          }
          for (const control of geometry.controls)
            expect(control.hit, String(control.label)).toBe(true);
          expect(geometry.overlaps).toEqual([]);
        };

        await openFind();
        await expect(field).toHaveValue("");
        await page.keyboard.type(query);
        await expect(field).toHaveValue(query);
        const total = await matchTotal();
        expect(total).toBeGreaterThan(1);
        await expectActiveMatch("first");
        await expectBar("open");
        await page.keyboard.press("Enter");
        await expect(status).toHaveText(`${(1 % total) + 1} / ${total}`);
        await expect(field).toBeFocused();
        await expectActiveMatch("next");
        await page.keyboard.press("Shift+Enter");
        await expect(status).toHaveText(`1 / ${total}`);
        await expectActiveMatch("previous");
        await page.keyboard.press("Shift+Enter");
        await expect(status).toHaveText(`${total} / ${total}`);
        await expect(field).toBeFocused();
        await expectActiveMatch("last");
        await page.keyboard.press("Enter");
        await expect(status).toHaveText(`1 / ${total}`);
        await expectActiveMatch("wrapped");
        await page.keyboard.press("End");
        const selection = () =>
          field.evaluate((node) => {
            const search = node as HTMLInputElement;
            return [search.selectionStart, search.selectionEnd];
          });
        expect(await selection()).toEqual([query.length, query.length]);
        await openFind();
        await expect.poll(selection).toEqual([0, query.length]);
        await expect(status).toHaveText(`1 / ${total}`);
        await page.keyboard.press("Escape");
        await expect(mounted).toHaveCount(0);
        await expect(composer).toBeFocused();
        await expectDraft();
        await capture("closed");

        await openFind();
        await page.keyboard.type(query);
        await matchTotal();
        await page.getByRole("button", name("dock.action.expand")).click();
        const material = page.locator('[data-slot="workspace-view-body"]:visible');
        await expect(material).toBeFocused();
        await expect(
          material.locator('[data-trajectory-record="item:item_code_reading"]'),
        ).toBeVisible();
        await expect(mounted).toHaveCount(0);
        await page.keyboard.press("ControlOrMeta+f");
        await page.evaluate(
          () =>
            new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))),
        );
        await expect(mounted).toHaveCount(0);
        await expect(material).toBeFocused();
        await capture("material");
        await page.setViewportSize({ width: 1280, height: 800 });
        await expect(mounted).toHaveCount(0);
        await expect(material).toBeFocused();
        await page.setViewportSize({ width: 1120, height: 755 });
        await expect(mounted).toHaveCount(0);
        await expect(material).toBeFocused();
        await page.getByRole("button", name("common.close")).click();
        await expect(conversation).toBeVisible();
        await expectDraft();

        await composer.click();
        await expect(composer).toBeFocused();
        await openFind();
        await expect(field).toHaveValue("");
        await expect(status).toHaveText("");
        await page.keyboard.type(query);
        expect(await matchTotal()).toBe(total);
        await expectBar("reopened");
        await page.keyboard.press("Enter");
        await expect(status).toHaveText(`2 / ${total}`);
        await expectActiveMatch("before-reflow");
        await page.setViewportSize({ width: 1280, height: 800 });
        await expect(field).toHaveValue(query);
        await expect(status).toHaveText(`2 / ${total}`);
        await expectBar("normal-larger-window");
        await expectActiveMatch("normal-larger-window");
        await page.setViewportSize({ width: 1120, height: 755 });
        await expect(field).toHaveValue(query);
        await expect(status).toHaveText(`2 / ${total}`);
        await expectBar("normal-restored-window");
        await expectActiveMatch("normal-restored-window");
        const code = conversation.locator("pre").first();
        const word = (await activeMatchGeometry())!.boxes[0]!;
        const beforeWheel = await code.evaluate((node) => node.scrollTop);
        await page.mouse.move(word.left + word.width / 2, word.top + word.height / 2);
        await page.mouse.wheel(0, 1200);
        await expect
          .poll(() => code.evaluate((node) => node.scrollTop))
          .toBeGreaterThan(beforeWheel);
        await expect.poll(async () => (await activeMatchGeometry())?.visible).toBe(false);
        await capture("wheel-exit");
        await page.setViewportSize({ width: 1280, height: 800 });
        await expectBar("wheel-reflow");
        await expect(status).toHaveText(`2 / ${total}`);
        await expect.poll(async () => (await activeMatchGeometry())?.visible).toBe(false);
        expect(await code.evaluate((node) => node.scrollTop)).toBeGreaterThan(0);
        await testInfo.attach("chat-search-wheel-retired-match", {
          body: JSON.stringify(await activeMatchGeometry()),
          contentType: "application/json",
        });
        await page.keyboard.press("Shift+Enter");
        await expect(status).toHaveText(`1 / ${total}`);
        await expectActiveMatch("explicit-resume");
        await page.setViewportSize({ width: 1120, height: 755 });
        await expect(field).toHaveValue(query);
        await expect(status).toHaveText(`1 / ${total}`);
        await expectActiveMatch("restored");
        await composer.click();
        await page.keyboard.press(process.platform === "darwin" ? "Meta+ArrowDown" : "Control+End");
        await page.setViewportSize({ width: 1280, height: 800 });
        await expect(composer).toBeFocused();
        await expectDraft();
        await page.setViewportSize({ width: 1120, height: 755 });
        await expect(composer).toBeFocused();
        await expectDraft();
        expect(
          await composer.evaluate((node) => {
            const draft = node as HTMLTextAreaElement;
            return [draft.selectionStart, draft.selectionEnd];
          }),
        ).toEqual([profile.draft.length, profile.draft.length]);
        await openFind();
        await expect.poll(selection).toEqual([0, query.length]);
        await expect(status).toHaveText(`1 / ${total}`);
        await expectBar("restored");

        await page.keyboard.press("Escape");
        await expect(mounted).toHaveCount(0);
        await expect(composer).toBeFocused();
        await expectDraft();
        await page.keyboard.press(process.platform === "darwin" ? "Meta+ArrowDown" : "Control+End");
        expect(
          await composer.evaluate((node) => {
            const draft = node as HTMLTextAreaElement;
            return [draft.selectionStart, draft.selectionEnd];
          }),
        ).toEqual([profile.draft.length, profile.draft.length]);
        await page.keyboard.type(profile.continuation);
        await expect(composer).toHaveValue(profile.draft + profile.continuation);
        await capture("continued-draft");
      },
    );
  }
}
