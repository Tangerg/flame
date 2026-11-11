import { expect, test, type Locator } from "./test";
import { expectLabReady } from "../readiness";

async function controlGeometry(control: Locator) {
  return control.evaluate((node) => {
    const box = node.getBoundingClientRect();
    return {
      box: box.toJSON(),
      reachable: node.contains(
        document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2),
      ),
    };
  });
}

for (const theme of ["light", "dark"]) {
  test(`Hosted panels keep one visible identity and complete context at both densities in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1472, height: 900 });
    await page.goto(
      `/lab/fixture.html?fixture=workspace&state=dock-review&theme=${theme}&font-size=18`,
    );
    await expectLabReady(page);
    const view = page
      .locator('[data-dock-view-id="package:visual:diff"]')
      .filter({ has: page.locator('[data-slot="workspace-view-body"]') });
    const viewport = view.locator('[data-slot="workspace-view-body"]');
    const heading = view.getByRole("heading", { name: "Workspace diff", exact: true });
    await expect(heading).toHaveCount(1);
    const bounds = (await heading.boundingBox())!;
    expect(bounds.width).toBeLessThanOrEqual(1);
    expect(bounds.height).toBeLessThanOrEqual(1);

    await view.getByRole("button", { name: "Show changed files", exact: true }).click();
    for (const narrow of [false, true]) {
      if (narrow) {
        const resizer = page.locator(".agent-pane-resizer");
        await resizer.focus();
        await resizer.press("Home");
        expect((await view.boundingBox())!.width).toBeCloseTo(320, 0);
      }
      await viewport.evaluate((node) => node.scrollTo(0, 0));
      const context = view.locator('header [dir="ltr"]');
      await expect(context).toHaveText("/Users/visual/scope");
      await expect(context).toBeInViewport({ ratio: 1 });
      const filter = view.getByRole("searchbox", { name: "Filter files", exact: true });
      for (const control of [
        filter,
        view.getByRole("button", { name: "Refresh", exact: true }),
        view.getByRole("combobox", { name: "Comparison", exact: true }),
      ]) {
        await expect(control).toBeInViewport({ ratio: 1 });
      }
      await filter.fill("session.go");
      await expect(view.locator("[data-diff-file]")).toHaveCount(1);
      await expect(view.locator("[data-diff-file]").first()).toHaveAttribute(
        "title",
        "packages/go-sdk/protocol/session.go",
      );
      await filter.fill("");
      await expect(view.locator("[data-diff-file]")).toHaveCount(2);
    }

    await page.setViewportSize({ width: 320, height: 900 });
    await page.goto(
      `/lab/fixture.html?fixture=workspace&state=full-view&full-view=package:visual:diff&sidebar=collapsed&theme=${theme}&font-size=18`,
    );
    await expectLabReady(page);
    const full = page.getByRole("region", { name: "Workspace diff", exact: true });
    const pageTitle = full.getByRole("heading", { name: "Workspace diff", exact: true });
    expect((await pageTitle.boundingBox())!.height).toBeLessThanOrEqual(1);
    await expect(
      page
        .locator('[data-slot="workspace-window-header"]')
        .getByText("Workspace diff", { exact: true }),
    ).toBeVisible();
    await full.locator('header [dir="ltr"]').scrollIntoViewIfNeeded();
    await expect(full.locator('header [dir="ltr"]')).toHaveText("/Users/visual/scope");
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  });
}

for (const profile of [
  {
    locale: "en",
    theme: "light",
    fontSize: 14,
    panel: "Workspace diff",
    composer: "Message composer",
    close: "Close",
    conversation: "Conversation",
    openFull: "Open material full width",
    showFiles: "Show changed files",
    refresh: "Refresh",
    filter: "Filter files",
    comparison: "Comparison",
    failedShort: "Read failed",
    empty: "No changes in this comparison.",
    error: "Could not read uncommitted changes: Visual fixture could not load the workspace diff",
    draft: "Keep the diff review draft.",
    suffix: " Continue reviewing.",
  },
  {
    locale: "zh",
    theme: "dark",
    fontSize: 18,
    panel: "工作区 diff",
    composer: "消息输入框",
    close: "关闭",
    conversation: "对话",
    openFull: "全宽打开材料",
    showFiles: "显示已更改文件",
    refresh: "刷新",
    filter: "筛选文件",
    comparison: "比较",
    failedShort: "读取失败",
    empty: "此比较中没有更改。",
    error: "无法读取未提交的更改：Visual fixture could not load the workspace diff",
    draft: "保留这份差异检查草稿。",
    suffix: "继续核对。",
  },
] as const) {
  test(`Workspace diff keeps keyboard focus on Refresh after a failed file read in ${profile.locale}`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width: 1472, height: 1000 });
    await page.goto(
      `/lab/fixture.html?fixture=workspace&state=dock-error&locale=${profile.locale}&theme=${profile.theme}&font-size=${profile.fontSize}&motion=off`,
    );
    await expectLabReady(page);
    await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
    const composer = page.getByRole("textbox", { name: profile.composer, exact: true });
    await composer.fill(profile.draft);
    const panel = page
      .locator('[data-slot="workspace-view-body"]:visible')
      .getByRole("region", { name: profile.panel, exact: true });
    const alert = panel.getByRole("alert");
    const refresh = panel.getByRole("button", { name: profile.refresh, exact: true });
    const filter = panel.getByRole("searchbox", { name: profile.filter, exact: true });
    const comparison = panel.getByRole("combobox", { name: profile.comparison, exact: true });
    await expect(alert).toHaveText(profile.error);

    const files = panel.getByRole("button", { name: profile.showFiles, exact: true });
    await files.focus();
    await files.press("Enter");
    await expect(files).toHaveCount(0);
    await expect(refresh).toBeFocused();
    expect(await refresh.evaluate((node) => node.matches(":focus-visible"))).toBe(true);
    await expect(alert).toHaveText(profile.error);
    await expect(filter).toBeDisabled();
    await expect(panel.getByText(profile.failedShort, { exact: true })).toBeVisible();
    await expect(panel.getByText(profile.empty, { exact: true })).toHaveCount(0);
    await expect(panel.locator("[data-diff-file]")).toHaveCount(0);
    const focus = await controlGeometry(refresh);
    await expect(refresh).toBeInViewport({ ratio: 1 });
    expect(focus.reachable).toBe(true);
    await testInfo.attach("diff-read-failure-focus.png", {
      body: await page.screenshot({ animations: "disabled", caret: "hide" }),
      contentType: "image/png",
    });

    for (let attempt = 0; attempt < 2; attempt++) {
      await refresh.press("Enter");
      await expect(refresh).toBeFocused();
      await expect(refresh).not.toHaveAttribute("aria-disabled", "true");
      await expect(alert).toHaveText(profile.error);
      await expect(filter).toBeDisabled();
    }

    await page.setViewportSize({ width: 640, height: 400 });
    const openFull = page.getByRole("button", { name: profile.openFull, exact: true });
    await expect(openFull).toBeVisible();
    await openFull.focus();
    await openFull.press("Enter");
    const close = page.getByRole("button", { name: profile.close, exact: true });
    await expect(close).toBeVisible();
    await expect(alert).toHaveText(profile.error);
    await expect(filter).toBeDisabled();
    await expect(panel.getByText(profile.empty, { exact: true })).toHaveCount(0);
    await expect(panel.locator("[data-diff-file]")).toHaveCount(0);
    await alert.scrollIntoViewIfNeeded();
    await expect(alert).toBeInViewport({ ratio: 1 });
    for (const control of [refresh, comparison, filter]) {
      await expect(control).toBeInViewport({ ratio: 1 });
    }
    const message = await alert.evaluate((node) => {
      const body = node.closest('[data-slot="workspace-view-body"]')!;
      const frame = body.getBoundingClientRect();
      const controls = node
        .closest('[data-slot="panel"]')!
        .querySelector('[data-slot="panel-controls"]')!
        .getBoundingClientRect();
      const box = node.getBoundingClientRect();
      const range = document.createRange();
      range.selectNodeContents(node);
      return {
        box: box.toJSON(),
        lines: [...range.getClientRects()].map((line) => line.toJSON()),
        frame: frame.toJSON(),
        controlsBottom: controls.bottom,
        reachable: node.contains(
          document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2),
        ),
        bodyOverflow: body.scrollWidth - body.clientWidth,
        documentOverflow: document.documentElement.scrollWidth - innerWidth,
      };
    });
    expect(message.reachable).toBe(true);
    expect(message.box.top).toBeGreaterThanOrEqual(message.controlsBottom);
    expect(message.box.bottom).toBeLessThanOrEqual(Math.min(400, message.frame.bottom));
    expect(message.lines.length).toBeGreaterThan(0);
    for (const line of message.lines) {
      expect(line.left).toBeGreaterThanOrEqual(message.frame.left);
      expect(line.right).toBeLessThanOrEqual(message.frame.right);
    }
    expect(message.bodyOverflow).toBe(0);
    expect(message.documentOverflow).toBe(0);
    const controls = {
      refresh: await controlGeometry(refresh),
      comparison: await controlGeometry(comparison),
      filter: await controlGeometry(filter),
    };
    expect(controls.refresh.reachable).toBe(true);
    expect(controls.comparison.reachable).toBe(true);
    await testInfo.attach("diff-read-failure.png", {
      body: await page.screenshot({ animations: "disabled", caret: "hide" }),
      contentType: "image/png",
    });

    await close.focus();
    await close.press("Enter");
    await expect(
      page.getByRole("region", { name: profile.conversation, exact: true }),
    ).toBeFocused();
    await expect(composer).toHaveValue(profile.draft);
    await composer.focus();
    await composer.press("End");
    await page.keyboard.type(profile.suffix);
    await expect(composer).toHaveValue(profile.draft + profile.suffix);
    await testInfo.attach("diff-read-failure-return.png", {
      body: await page.screenshot({ animations: "disabled", caret: "hide" }),
      contentType: "image/png",
    });
    await testInfo.attach("diff-read-failure.json", {
      body: JSON.stringify({
        locale: profile.locale,
        theme: profile.theme,
        viewport: page.viewportSize(),
        refreshAttempts: 2,
        focus,
        message,
        controls,
        draft: await composer.inputValue(),
      }),
      contentType: "application/json",
    });
  });

  test(`Long Composer draft stays readable across narrow material return in ${profile.locale}`, async ({
    page,
  }, testInfo) => {
    const lines =
      profile.locale === "en"
        ? [
            "Draft start.",
            "Keep the accepted source.",
            "Compare the current file.",
            "Read the complete response.",
            "Keep the review context.",
            "Check the changed scope.",
            "Retain the unsent message.",
            "Return from the material.",
            "Continue the same review.",
            "Draft end.",
          ]
        : [
            "草稿开头。",
            "保留已接收的来源。",
            "比较当前文件。",
            "阅读完整回复。",
            "保留检查上下文。",
            "确认更改范围。",
            "保留未发送消息。",
            "从材料返回。",
            "继续同一份检查。",
            "草稿末尾。",
          ];
    const draft = lines.join("\n");
    const continued = draft + profile.suffix;
    const startKey = process.platform === "darwin" ? "Meta+ArrowUp" : "Control+Home";
    const endKey = process.platform === "darwin" ? "Meta+ArrowDown" : "Control+End";
    const evidence: Record<string, unknown>[] = [];
    try {
      await page.setViewportSize({ width: 1472, height: 1000 });
      await page.goto(
        `/lab/fixture.html?fixture=workspace&state=dock-error&locale=${profile.locale}&theme=${profile.theme}&font-size=18&motion=off`,
      );
      await expectLabReady(page);
      await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
      const composer = page.getByRole("textbox", { name: profile.composer, exact: true });
      await composer.fill(draft);
      const handle = (await composer.elementHandle())!;
      const send = page.getByRole("button", {
        name: profile.locale === "en" ? "Send" : "发送",
        exact: true,
      });

      // Measures the em box of the known first or last line, clipped by every box that can hide it.
      const measure = (step: string, line: "first" | "last") =>
        composer
          .evaluate(
            (node: HTMLTextAreaElement, { line, handle }) => {
              const style = getComputedStyle(node);
              const fontSize = parseFloat(style.fontSize);
              const lineHeight = parseFloat(style.lineHeight);
              const padTop = parseFloat(style.paddingTop);
              const padBottom = parseFloat(style.paddingBottom);
              const padLeft = parseFloat(style.paddingLeft);
              const padRight = parseFloat(style.paddingRight);
              const box = node.getBoundingClientRect();
              const clientLeft = box.left + node.clientLeft;
              const clientTop = box.top + node.clientTop;
              const lineTop =
                line === "first"
                  ? clientTop + padTop - node.scrollTop
                  : clientTop + node.scrollHeight - padBottom - node.scrollTop - lineHeight;
              const em = {
                top: lineTop + (lineHeight - fontSize) / 2,
                bottom: lineTop + (lineHeight + fontSize) / 2,
                left: clientLeft + padLeft,
                right: clientLeft + node.clientWidth - padRight,
              };
              const clips: {
                name: string;
                top: number;
                bottom: number;
                left: number;
                right: number;
              }[] = [
                {
                  name: "textarea",
                  top: clientTop,
                  bottom: clientTop + node.clientHeight,
                  left: clientLeft,
                  right: clientLeft + node.clientWidth,
                },
                { name: "viewport", top: 0, bottom: innerHeight, left: 0, right: innerWidth },
              ];
              const root = node.closest<HTMLElement>('[data-slot="composer-root"]')!;
              const footerBox = root
                .querySelector(':scope > [data-slot="composer-footer"]')!
                .getBoundingClientRect();
              clips.push({
                name: "footer",
                top: 0,
                bottom: footerBox.top,
                left: 0,
                right: innerWidth,
              });
              for (let parent = node.parentElement; parent; parent = parent.parentElement) {
                const overflow = getComputedStyle(parent);
                if (overflow.overflowX !== "visible" || overflow.overflowY !== "visible") {
                  const frame = parent.getBoundingClientRect();
                  clips.push({
                    name: `${parent.tagName.toLowerCase()}.${parent.getAttribute("data-slot") ?? ""}`,
                    top: overflow.overflowY === "visible" ? 0 : frame.top + parent.clientTop,
                    bottom:
                      overflow.overflowY === "visible"
                        ? innerHeight
                        : frame.top + parent.clientTop + parent.clientHeight,
                    left: overflow.overflowX === "visible" ? 0 : frame.left + parent.clientLeft,
                    right:
                      overflow.overflowX === "visible"
                        ? innerWidth
                        : frame.left + parent.clientLeft + parent.clientWidth,
                  });
                }
                if (parent === root) break;
              }
              const clip = (target: { top: number; bottom: number; left: number; right: number }) =>
                clips.reduce(
                  (area, frame) => ({
                    top: Math.max(area.top, frame.top),
                    bottom: Math.min(area.bottom, frame.bottom),
                    left: Math.max(area.left, frame.left),
                    right: Math.min(area.right, frame.right),
                  }),
                  target,
                );
              const visible = clip(em);
              const textarea = clip({
                top: box.top,
                bottom: box.bottom,
                left: box.left,
                right: box.right,
              });
              const hit = (area: typeof visible) =>
                area.bottom > area.top && area.right > area.left
                  ? document.elementFromPoint(
                      (area.left + area.right) / 2,
                      (area.top + area.bottom) / 2,
                    ) === node
                  : false;
              return {
                line,
                value: node.value,
                sameNode: node === handle,
                focused: document.activeElement === node,
                selection: { start: node.selectionStart, end: node.selectionEnd },
                style: { fontSize, lineHeight, padTop, padBottom, padLeft, padRight },
                box: box.toJSON(),
                client: { top: node.clientTop, width: node.clientWidth, height: node.clientHeight },
                scroll: {
                  height: node.scrollHeight,
                  top: node.scrollTop,
                  horizontalOverflow: node.scrollWidth - node.clientWidth,
                },
                em,
                clips,
                visible,
                textarea,
                metricsValid:
                  Number.isFinite(lineHeight) &&
                  Number.isFinite(fontSize) &&
                  lineHeight > 0 &&
                  fontSize > 0,
                lineVisible:
                  visible.bottom > visible.top &&
                  visible.right > visible.left &&
                  visible.top <= em.top + 0.5 &&
                  visible.bottom >= em.bottom - 0.5 &&
                  visible.left <= em.left + 0.5 &&
                  visible.right >= em.right - 0.5 &&
                  hit(visible),
                textareaHit: hit(textarea),
                documentOverflow: document.documentElement.scrollWidth - innerWidth,
              };
            },
            { line, handle },
          )
          .then((sample) => {
            evidence.push({ step, viewport: page.viewportSize(), ...sample });
            return sample;
          });
      const expectSend = async () => {
        await expect(send).toBeInViewport({ ratio: 1 });
        expect((await controlGeometry(send)).reachable).toBe(true);
      };
      const capture = async (name: string) => {
        await testInfo.attach(name, {
          body: await page.screenshot({ animations: "disabled", caret: "hide" }),
          contentType: "image/png",
        });
      };
      const settled = (
        sample: Awaited<ReturnType<typeof measure>>,
        value: string,
        selection: number,
      ) => ({
        lineVisible: sample.lineVisible,
        metricsValid: sample.metricsValid,
        horizontalOverflow: sample.scroll.horizontalOverflow,
        sameNode: sample.sameNode,
        focused: sample.focused,
        value: sample.value === value,
        selection: sample.selection.start === selection && sample.selection.end === selection,
        documentOverflow: sample.documentOverflow,
      });
      const expected = {
        lineVisible: true,
        metricsValid: true,
        horizontalOverflow: 0,
        sameNode: true,
        focused: true,
        value: true,
        selection: true,
        documentOverflow: 0,
      };

      await composer.focus();
      await page.keyboard.press(startKey);
      await page.setViewportSize({ width: 373, height: 243 });
      await expect
        .poll(async () => settled(await measure("narrow-start", "first"), draft, 0))
        .toEqual(expected);

      await page.keyboard.press(endKey);
      await expect
        .poll(async () => settled(await measure("narrow-end", "last"), draft, draft.length))
        .toEqual(expected);
      await expectSend();
      await capture("composer-long-draft-narrow-end.png");

      const reading = await measure("narrow-wheel-target", "last");
      expect(reading.textarea.bottom - reading.textarea.top).toBeGreaterThan(0);
      expect(reading.textarea.right - reading.textarea.left).toBeGreaterThan(0);
      expect(reading.textareaHit).toBe(true);
      expect(reading.scroll.height - reading.client.height).toBeGreaterThan(0);
      await page.mouse.move(
        (reading.textarea.left + reading.textarea.right) / 2,
        (reading.textarea.top + reading.textarea.bottom) / 2,
      );
      await page.mouse.wheel(0, -2000);
      await expect
        .poll(async () => settled(await measure("narrow-wheel", "first"), draft, draft.length))
        .toEqual(expected);

      await page.setViewportSize({ width: 560, height: 364 });
      await expect
        .poll(async () => settled(await measure("manual-reading", "first"), draft, draft.length))
        .toEqual(expected);
      expect((await measure("manual-reading-end", "last")).lineVisible).toBe(false);
      await expectSend();
      await capture("composer-long-draft-manual-reading.png");

      const openFull = page.getByRole("button", { name: profile.openFull, exact: true });
      await expect(openFull).toBeVisible();
      await openFull.focus();
      await openFull.press("Enter");
      const panel = page
        .locator('[data-slot="workspace-view-body"]:visible')
        .getByRole("region", { name: profile.panel, exact: true });
      await expect(panel.getByRole("alert")).toHaveText(profile.error);
      const close = page.getByRole("button", { name: profile.close, exact: true });
      await expect(close).toBeVisible();
      await close.focus();
      await close.press("Enter");
      await expect(
        page.getByRole("region", { name: profile.conversation, exact: true }),
      ).toBeFocused();
      await expect(composer).toHaveValue(draft);

      await composer.focus();
      await expect
        .poll(async () => settled(await measure("return-end", "last"), draft, draft.length))
        .toEqual(expected);
      await page.keyboard.type(profile.suffix);
      await page.keyboard.press("Backspace");
      await page.keyboard.type([...profile.suffix].at(-1)!);
      await expect
        .poll(async () =>
          settled(await measure("continued-end", "last"), continued, continued.length),
        )
        .toEqual(expected);
      await expectSend();
      await capture("composer-long-draft-continued.png");

      await page.keyboard.press(startKey);
      await page.setViewportSize({ width: 1472, height: 1000 });
      await expect
        .poll(async () => settled(await measure("restored-start", "first"), continued, 0))
        .toEqual(expected);
      await expectSend();
      await capture("composer-long-draft-restored.png");
    } finally {
      await testInfo.attach("composer-long-draft.json", {
        body: JSON.stringify({
          locale: profile.locale,
          theme: profile.theme,
          fontSize: 18,
          draft,
          evidence,
        }),
        contentType: "application/json",
      });
    }
  });
}
