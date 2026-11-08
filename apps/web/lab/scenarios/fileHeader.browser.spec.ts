import { expect, test, type Locator } from "./test";
import { expectLabReady } from "../readiness";
async function expectReadingHierarchy(header: Locator) {
  const geometry = await header.evaluate((node) => {
    const back = node.querySelector("button")!.getBoundingClientRect();
    const identity = node.querySelector("[title]")!.getBoundingClientRect();
    const controls = node
      .querySelector('[data-slot="workspace-view-controls"]')!
      .getBoundingClientRect();
    return { back: back.toJSON(), identity: identity.toJSON(), controls: controls.toJSON() };
  });
  expect(geometry.back.right).toBeLessThanOrEqual(geometry.identity.x);
  expect(
    geometry.identity.bottom <= geometry.controls.y ||
      geometry.identity.right <= geometry.controls.x,
  ).toBe(true);
}

for (const [fontSize, theme] of [
  [14, "light"],
  [18, "dark"],
] as const) {
  test(`file identity leads its reading controls within the narrow Dock ${fontSize}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1472, height: 900 });
    await page.goto(
      `/lab/fixture.html?fixture=workspace&state=dock-light&agent-state=idle&theme=${theme}&font-size=${fontSize}&locale=en`,
    );
    await expectLabReady(page);
    const view = page
      .locator('[data-dock-view-id="file"]')
      .filter({ has: page.locator('[data-slot="workspace-view-header"]') });
    const header = view.locator('[data-slot="workspace-view-header"]');
    await expectReadingHierarchy(header);
    const openingIdentity = header.locator("[title]").first();
    const openedFile = {
      name: await openingIdentity.innerText(),
      path: (await openingIdentity.getAttribute("title"))!,
    };
    await header.getByRole("button", { name: "Back to files", exact: true }).click();
    const returned = page.getByRole("treeitem", { name: openedFile.name, exact: true });
    await expect(returned).toHaveAttribute("aria-current", "true");
    await expect(returned).toHaveAttribute("title", openedFile.path);
    await page.getByRole("treeitem", { name: "README.md", exact: true }).click();
    await expect(header.getByTitle("README.md", { exact: true })).toBeVisible();
    await expectReadingHierarchy(header);

    const resizer = page.locator(".agent-pane-resizer");
    await resizer.focus();
    await resizer.press("Home");
    expect((await view.boundingBox())!.width).toBeCloseTo(320, 0);
    const identity = header.getByTitle("README.md", { exact: true });
    expect(
      await identity.evaluate((node) => {
        const range = document.createRange();
        range.selectNodeContents(node);
        const host = node.closest('[data-slot="workspace-view-header"]')!.getBoundingClientRect();
        return [...range.getClientRects()].every(
          (box) => box.width > 0 && box.left >= host.left && box.right <= host.right,
        );
      }),
    ).toBe(true);
    for (const control of await header.locator('button,[role="tab"]').all()) {
      await expect(control).toBeInViewport({ ratio: 1 });
      expect(
        await control.evaluate((node) => {
          const box = node.getBoundingClientRect();
          const host = node.closest('[data-slot="workspace-view-header"]')!.getBoundingClientRect();
          return (
            box.left >= host.left &&
            box.right <= host.right &&
            box.top >= host.top &&
            box.bottom <= host.bottom &&
            node.contains(document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2))
          );
        }),
      ).toBe(true);
    }
    expect(await view.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
  });
}

test("file copy feedback follows clipboard acceptance and the current file", async ({ page }) => {
  let accept!: () => void;
  const clipboardReady = new Promise<void>((resolve) => {
    accept = resolve;
  });
  const writes: string[] = [];
  await page.exposeFunction("__writeFilePath", (text: string) => {
    writes.push(text);
    return clipboardReady;
  });
  await page.addInitScript(() => {
    const write = (window as unknown as { __writeFilePath: (text: string) => Promise<void> })
      .__writeFilePath;
    Object.defineProperty(navigator, "clipboard", { value: { writeText: write } });
  });
  await page.goto(
    "/lab/fixture.html?fixture=workspace&state=dock-light&agent-state=idle&locale=en",
  );
  await expectLabReady(page);
  const view = page
    .locator('[data-dock-view-id="file"]')
    .filter({ has: page.locator('[data-slot="workspace-view-header"]') });
  const header = view.locator('[data-slot="workspace-view-header"]');
  const path = await header.locator("[title]").first().getAttribute("title");
  try {
    await header.getByRole("button", { name: "Copy path", exact: true }).click();
    await expect.poll(() => writes).toEqual([path]);
    await expect(header.getByRole("button", { name: "Path copied", exact: true })).toHaveCount(0);
    accept();
    await expect(header.getByRole("button", { name: "Path copied", exact: true })).toBeVisible();
    await header.getByRole("button", { name: "Back to files", exact: true }).click();
    await page.getByRole("treeitem", { name: "README.md", exact: true }).click();
    await expect(header.getByTitle("README.md", { exact: true })).toBeVisible();
    await expect(header.getByRole("button", { name: "Path copied", exact: true })).toHaveCount(0);
    await header.getByRole("button", { name: "Copy path", exact: true }).click();
    await expect.poll(() => writes).toEqual([path, "README.md"]);
    await expect(header.getByRole("button", { name: "Path copied", exact: true })).toBeVisible();
  } finally {
    accept();
  }
});

async function expectFileActionsVisible(view: Locator) {
  const header = view.locator('[data-slot="workspace-view-header"]');
  for (const button of await header.getByRole("button").all()) {
    await expect(button).toBeInViewport({ ratio: 1 });
  }
  const geometry = await header.evaluate((node) => {
    const bounds = node.getBoundingClientRect();
    const buttons = [...node.querySelectorAll("button")].map((button) => {
      const box = button.getBoundingClientRect();
      return {
        name: button.getAttribute("aria-label"),
        fits:
          box.left >= bounds.left - 1 &&
          box.right <= bounds.right + 1 &&
          box.top >= bounds.top - 1 &&
          box.bottom <= bounds.bottom + 1,
        reachable: button.contains(
          document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2),
        ),
      };
    });
    return { buttons, documentOverflow: document.documentElement.scrollWidth - innerWidth };
  });
  expect(geometry.buttons.length).toBeGreaterThan(0);
  expect(geometry.buttons.every((button) => button.fits && button.reachable)).toBe(true);
  expect(geometry.documentOverflow).toBe(0);
  return geometry;
}

for (const profile of [
  {
    locale: "en",
    theme: "light",
    fontSize: 14,
    composer: "Message composer",
    back: "Back to conversation",
    close: "Close",
    conversation: "Conversation",
    openFull:
      "Open material full width — the window is too narrow to show it beside the conversation",
    backToFiles: "Back to files",
    retry: "Retry",
    goToLine: "Go to line",
    open: "Open",
    readError: "Couldn't open file",
    unsupported: "No image preview here yet",
    draft: "Keep the file review draft.",
    suffix: " Continue reviewing.",
  },
  {
    locale: "zh",
    theme: "dark",
    fontSize: 18,
    composer: "消息输入框",
    back: "返回对话",
    close: "关闭",
    conversation: "对话",
    openFull: "全宽打开材料——窗口太窄，无法与对话并排显示",
    backToFiles: "返回文件列表",
    retry: "重试",
    goToLine: "跳转到行",
    open: "打开",
    readError: "无法打开文件",
    unsupported: "这里暂时无法预览图片",
    draft: "保留这份文件检查草稿。",
    suffix: "继续核对。",
  },
] as const) {
  test(`file read failure retains retry and reference return in ${profile.locale}`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width: 1472, height: 1000 });
    await page.goto(
      `/lab/fixture.html?fixture=workspace&state=dock-file-error&agent-state=prose-blocks&locale=${profile.locale}&theme=${profile.theme}&font-size=${profile.fontSize}&motion=off`,
    );
    await expectLabReady(page);
    await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
    const composer = page.getByRole("textbox", { name: profile.composer, exact: true });
    await composer.fill(profile.draft);
    const view = page.locator('[data-slot="workspace-view-body"]:visible');
    await expect(view.getByText(profile.readError, { exact: true })).toBeVisible();
    await view.getByRole("button", { name: profile.backToFiles, exact: true }).click();
    const row = view.getByRole("treeitem", { name: "README.md", exact: true });
    await expect(view.getByRole("treeitem").first()).toBeFocused();
    await expect(row).toHaveAttribute("aria-current", "true");

    const reference = page
      .locator(".md")
      .filter({ hasText: "Source references" })
      .getByRole("button", { name: "README.md:15:2", exact: true });
    await reference.focus();
    await reference.press("Enter");
    await page.setViewportSize({ width: 720, height: 450 });
    await expect(page.getByRole("button", { name: profile.back, exact: true })).toBeVisible();
    const reader = view.getByRole("region", { name: "README.md", exact: true });
    await expect(reader.locator('[data-slot="file-path"]')).toHaveText("README.md");
    await expect(
      view.locator('[data-slot="workspace-view-header"]').getByTitle("README.md", { exact: true }),
    ).toBeInViewport({ ratio: 1 });
    await expect(view.locator('[data-quote-source="file"]')).toHaveCount(0);
    const retry = reader.getByRole("button", { name: profile.retry, exact: true });
    for (let attempt = 0; attempt < 2; attempt++) {
      await retry.focus();
      await expect(retry).toBeInViewport({ ratio: 1 });
      await retry.press("Enter");
      await expect(reader).toBeFocused();
      await expect(reader.getByText(profile.readError, { exact: true })).toBeInViewport({
        ratio: 1,
      });
      await expect(retry).toBeVisible();
      await expect(reader).toBeFocused();
    }
    const geometry = await expectFileActionsVisible(view);
    await testInfo.attach("file-read-failure.png", {
      body: await page.screenshot({ animations: "disabled", caret: "hide" }),
      contentType: "image/png",
    });
    await reader.press("Escape");
    await expect(reference).toBeFocused();
    await expect(reference).toBeInViewport();
    await expect(composer).toHaveValue(profile.draft);
    await composer.focus();
    await composer.press("End");
    await page.keyboard.type(profile.suffix);
    await expect(composer).toHaveValue(profile.draft + profile.suffix);
    await testInfo.attach("file-read-failure-return.png", {
      body: await page.screenshot({ animations: "disabled", caret: "hide" }),
      contentType: "image/png",
    });
    await testInfo.attach("file-read-failure.json", {
      body: JSON.stringify({
        locale: profile.locale,
        attempts: 2,
        geometry,
        draft: await composer.inputValue(),
      }),
      contentType: "application/json",
    });
  });

  test(`unsupported file retains its tree return and draft in ${profile.locale}`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width: 1472, height: 1000 });
    await page.goto(
      `/lab/fixture.html?fixture=workspace&state=dock-file-unsupported&agent-state=prose-blocks&locale=${profile.locale}&theme=${profile.theme}&font-size=${profile.fontSize}&motion=off`,
    );
    await expectLabReady(page);
    await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
    const composer = page.getByRole("textbox", { name: profile.composer, exact: true });
    await composer.fill(profile.draft);
    const view = page.locator('[data-slot="workspace-view-body"]:visible');
    await expect(view.getByText(profile.unsupported, { exact: true })).toBeVisible();
    await view.getByRole("button", { name: profile.backToFiles, exact: true }).click();
    const row = view.getByRole("treeitem", { name: "review-image.png", exact: true });
    await expect(view.getByRole("treeitem").first()).toBeFocused();
    await expect(row).toHaveAttribute("aria-current", "true");
    await row.focus();
    await row.press("Enter");
    const reader = view.getByRole("region", { name: "review-image.png", exact: true });
    await expect(reader).toBeVisible();
    await expect(
      view.getByRole("button", { name: profile.backToFiles, exact: true }),
    ).toBeFocused();
    await page.setViewportSize({ width: 720, height: 450 });
    const openFull = page.getByRole("button", { name: profile.openFull, exact: true });
    await expect(openFull).toBeVisible();
    await openFull.focus();
    await openFull.press("Enter");
    await expect(reader.locator('[data-slot="file-path"]')).toHaveText("review-image.png");
    await expect(
      view
        .locator('[data-slot="workspace-view-header"]')
        .getByTitle("review-image.png", { exact: true }),
    ).toBeInViewport({ ratio: 1 });
    await expect(reader.getByText(profile.unsupported, { exact: true })).toBeInViewport({
      ratio: 1,
    });
    for (const name of [profile.retry, profile.goToLine, profile.open]) {
      await expect(view.getByRole("button", { name, exact: true })).toHaveCount(0);
    }
    await expect(view.locator('[data-quote-source="file"]')).toHaveCount(0);
    const geometry = await expectFileActionsVisible(view);
    await testInfo.attach("file-unsupported.png", {
      body: await page.screenshot({ animations: "disabled", caret: "hide" }),
      contentType: "image/png",
    });
    const backToFiles = view.getByRole("button", { name: profile.backToFiles, exact: true });
    await backToFiles.focus();
    await backToFiles.press("Enter");
    await expect(row).toBeFocused();
    await expect(row).toHaveAttribute("aria-current", "true");
    await page.getByRole("button", { name: profile.close, exact: true }).click();
    await expect(
      page.getByRole("region", { name: profile.conversation, exact: true }),
    ).toBeFocused();
    await expect(composer).toHaveValue(profile.draft);
    await composer.focus();
    await composer.press("End");
    await page.keyboard.type(profile.suffix);
    await expect(composer).toHaveValue(profile.draft + profile.suffix);
    await testInfo.attach("file-unsupported-return.png", {
      body: await page.screenshot({ animations: "disabled", caret: "hide" }),
      contentType: "image/png",
    });
    await testInfo.attach("file-unsupported.json", {
      body: JSON.stringify({
        locale: profile.locale,
        geometry,
        returnedTo: "review-image.png",
        draft: await composer.inputValue(),
      }),
      contentType: "application/json",
    });
  });
}
