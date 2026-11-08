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
    openFull:
      "Open material full width — the window is too narrow to show it beside the conversation",
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
    openFull: "全宽打开材料——窗口太窄，无法与对话并排显示",
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
}
