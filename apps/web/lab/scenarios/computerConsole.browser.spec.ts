import { expect, test, type Locator } from "./test";
import { COMPUTER_IMAGE_SAMPLES } from "./computerFixtureImages";
import { expectLabReady } from "../readiness";
import { LIMITS } from "../../../../plugins/computer/contracts/limits";

async function expectCapacity(
  panel: Locator,
  records: number,
  pending: number,
  locale: "en" | "zh" = "en",
) {
  const labels =
    locale === "zh"
      ? ["已保留记录 / 上限", "计入容量的字节 / 上限", "未确认结果"]
      : ["Retained records / limit", "Accounted bytes / limit", "Unconfirmed outcomes"];
  const value = (label: string) =>
    panel.getByText(label, { exact: true }).locator("..").locator("dd");
  const number = new Intl.NumberFormat(locale);
  await expect(value(labels[0]!)).toHaveText(
    `${number.format(records)} / ${number.format(LIMITS.maxRecords)}`,
  );
  await expect(value(labels[2]!)).toHaveText(number.format(pending));
  const [used, maximum] = (await value(labels[1]!).innerText()).split(" / ");
  expect(maximum).toBe(number.format(LIMITS.maxBytes));
  const bytes = Number(used!.replaceAll(",", ""));
  expect(Number.isSafeInteger(bytes)).toBe(true);
  if (records === 0) expect(bytes).toBe(0);
  else expect(bytes).toBeGreaterThan(0);
  return bytes;
}

for (const theme of ["light", "dark"] as const) {
  for (const width of [360, 1280]) {
    test(`published computer console supports permission and original results in ${theme} at ${width}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 960 });
      const remoteRequests: string[] = [];
      page.on("request", (request) => {
        if (!request.url().startsWith("http://127.0.0.1:")) remoteRequests.push(request.url());
      });
      await page.goto(`/lab/fixture.html?fixture=computer&theme=${theme}&font-size=18`);
      await expectLabReady(page);
      const panel = page.getByRole("region", { name: "Computer use", exact: true });
      await expectCapacity(panel, 0, 0);
      await expect(panel.getByText("isolated-desktop.example", { exact: true })).toBeVisible();
      await expect(
        panel.getByText(/operator console outside the controlled desktop/),
      ).toBeVisible();
      const opening = test.info().outputPath(`computer-${theme}-${width}-open.png`);
      await page.screenshot({ path: opening });
      await test.info().attach("published-computer-open-boundary", { path: opening });
      await panel.getByRole("textbox", { name: "Lease duration in seconds" }).fill("300");
      await panel.getByRole("button", { name: "Authorize and open control" }).click();
      const editor = panel.getByRole("textbox", { name: "Tool arguments (JSON)" });
      await expect(editor).toBeEnabled();
      await expectCapacity(panel, 1, 0);
      await editor.fill('{"code":"await fixture.inspect();"}');
      await panel.getByRole("button", { name: "Review operation", exact: true }).click();
      await expect(panel.getByRole("button", { name: "Back to editor" })).toBeFocused();
      await panel.getByRole("button", { name: "Execute reviewed operation" }).click();
      await expect(
        panel.getByText("Allow control of the isolated example editor?", { exact: true }),
      ).toBeVisible();
      await expect(panel.getByRole("button", { name: "Always allow" })).toHaveCount(0);
      await expect(panel.getByRole("button", { name: "Stop current operation" })).toBeEnabled();
      await expect(panel.getByRole("button", { name: "End control lease" })).toBeEnabled();
      const reservedBytes = await expectCapacity(panel, 2, 1);
      expect(reservedBytes).toBeGreaterThan(LIMITS.maxResultBytes);
      const operationId = (
        await panel
          .getByText("Operation identity", { exact: true })
          .locator("..")
          .locator("dd")
          .innerText()
      ).trim();
      expect(operationId).toMatch(/^[a-f0-9-]{36}$/);
      const capacity = panel
        .getByRole("heading", { name: "Original evidence capacity", exact: true })
        .locator("../../..");
      await capacity.scrollIntoViewIfNeeded();
      await expect(capacity).toBeInViewport({ ratio: 1 });
      await test.info().attach("computer-capacity-reserved", {
        body: await page.screenshot(),
        contentType: "image/png",
      });
      await panel.getByRole("button", { name: "Allow once", exact: true }).focus();
      const approval = test.info().outputPath(`computer-${theme}-${width}-approval.png`);
      const permissionHeading = panel.getByRole("heading", {
        name: "Permission request",
        exact: true,
      });
      await expect
        .poll(() =>
          permissionHeading.evaluate((node) => {
            const controls = node
              .closest('[data-slot="panel"]')
              ?.querySelector('[data-slot="panel-controls"]');
            if (!controls) throw new Error("Missing panel controls");
            return node.getBoundingClientRect().top - controls.getBoundingClientRect().bottom;
          }),
        )
        .toBeGreaterThanOrEqual(0);
      await expect(
        panel.getByText("Allow control of the isolated example editor?", { exact: true }),
      ).toBeInViewport({ ratio: 1 });
      await page.screenshot({ path: approval });
      await test.info().attach("published-computer-permission", { path: approval });
      await panel.getByRole("button", { name: "Allow for this session" }).click();
      await expect(
        panel.getByText("Allow control of the isolated example editor?", { exact: true }),
      ).toHaveCount(0);
      const settledBytes = await expectCapacity(panel, 3, 0);
      expect(settledBytes).toBeLessThan(reservedBytes);
      await capacity.scrollIntoViewIfNeeded();
      await expect(capacity).toBeInViewport({ ratio: 1 });
      await test.info().attach("computer-capacity-settled", {
        body: await page.screenshot(),
        contentType: "image/png",
      });
      const history = panel
        .getByRole("heading", { name: "Original outcomes", exact: true })
        .locator("../../..");
      await expect(history.getByRole("button", { name: /^[a-f0-9-]{36}$/ })).toHaveCount(3);
      const record = history.getByRole("button", { name: operationId, exact: true });
      await record.click();
      const original = panel.getByText("The original isolated fixture operation completed.", {
        exact: true,
      });
      await original.scrollIntoViewIfNeeded();
      await expect(original).toBeInViewport({ ratio: 1 });
      const image = panel.getByRole("img", { name: "Captured image 2" });
      await panel
        .getByRole("figure")
        .filter({ has: page.getByRole("img", { name: "Captured image 2" }) })
        .scrollIntoViewIfNeeded();
      await expect(image).toBeInViewport({ ratio: 1 });
      await expect(
        panel.getByText("Recorded image; this is not a live desktop.", { exact: true }),
      ).toBeInViewport({ ratio: 1 });
      const outcome = test.info().outputPath(`computer-${theme}-${width}-outcome.png`);
      await page.screenshot({ path: outcome });
      await test.info().attach("published-computer-original-outcome", { path: outcome });
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth - innerWidth),
      ).toBeLessThanOrEqual(1);
      expect(remoteRequests.filter((url) => !url.startsWith("data:"))).toEqual([]);
      await panel.getByRole("button", { name: "End control lease" }).click();
      await expect(panel.getByText("Closed", { exact: true })).toBeVisible();
      await expectCapacity(panel, 4, 0);
    });
  }
}

test("published console stops an outstanding permission without replaying its action", async ({
  page,
}) => {
  await page.goto("/lab/fixture.html?fixture=computer&theme=light");
  await expectLabReady(page);
  const panel = page.getByRole("region", { name: "Computer use", exact: true });
  await page.getByRole("button", { name: "Authorize and open control" }).click();
  await page
    .getByRole("textbox", { name: "Tool arguments (JSON)" })
    .fill('{"code":"await fixture.inspect();"}');
  await page.getByRole("button", { name: "Review operation", exact: true }).click();
  await page.getByRole("button", { name: "Execute reviewed operation" }).click();
  await expect(
    page.getByText("Allow control of the isolated example editor?", { exact: true }),
  ).toBeVisible();
  await expectCapacity(panel, 2, 1);
  await page.getByRole("button", { name: "Stop current operation" }).click();
  await expect(page.getByText("Closed", { exact: true })).toBeVisible();
  const retainedBytes = await expectCapacity(panel, 3, 1);
  expect(retainedBytes).toBeGreaterThan(LIMITS.maxResultBytes);
  await expect(
    panel.getByText("original execution outcome unconfirmed", { exact: true }),
  ).toBeVisible();
  await expect(panel.getByText(/invalid plugins\.call error/)).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Check original outcome" })).toBeVisible();
  await page.getByRole("button", { name: "Check original outcome" }).click();
  await expect(page.getByRole("button", { name: "Check original outcome" })).toBeVisible();
  expect(await expectCapacity(panel, 3, 1)).toBe(retainedBytes);
  await expect(
    panel.getByText("the original result is unconfirmed", { exact: true }),
  ).toBeVisible();
  await expect(panel.getByText(/invalid plugins\.call error/)).toHaveCount(0);
  await page.getByRole("button", { name: "Check original outcome" }).click();
  await expect(page.getByRole("button", { name: "Check original outcome" })).toBeVisible();
  expect(await expectCapacity(panel, 3, 1)).toBe(retainedBytes);
  await expect(
    panel.getByText("the original result is unconfirmed", { exact: true }),
  ).toBeVisible();
  await expect(panel.getByText(/invalid plugins\.call error/)).toHaveCount(0);
  await expect(
    page.getByText("Allow control of the isolated example editor?", { exact: true }),
  ).toHaveCount(0);
});

for (const profile of [
  {
    locale: "en",
    theme: "light",
    panel: "Computer use",
    open: "Authorize and open control",
    editor: "Tool arguments (JSON)",
    image: "Captured image 1",
    view: "View image",
    zoom: "Zoom in",
  },
  {
    locale: "zh",
    theme: "dark",
    panel: "电脑控制",
    open: "授权并打开控制",
    editor: "工具参数（JSON）",
    image: "捕获图像 1",
    view: "查看图片",
    zoom: "放大",
  },
] as const) {
  for (const width of [320, 1280]) {
    test(`recorded computer images support reading and return in ${profile.locale} at ${width}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 960 });
      await page.goto(
        `/lab/fixture.html?fixture=computer&state=images&locale=${profile.locale}&theme=${profile.theme}&font-size=18`,
      );
      await expectLabReady(page);
      const panel = page.getByRole("region", { name: profile.panel, exact: true });
      await expectCapacity(panel, COMPUTER_IMAGE_SAMPLES.length, 0, profile.locale);
      await panel.getByRole("button", { name: profile.open, exact: true }).click();
      const editor = panel.getByRole("textbox", { name: profile.editor, exact: true });
      const draft = '{"code":"keep the original image draft 保留"}';
      await editor.fill(draft);
      for (const sample of COMPUTER_IMAGE_SAMPLES) {
        const record = panel.getByRole("button", { name: sample.id, exact: true });
        await record.click();
        const image = panel.getByRole("img", { name: profile.image, exact: true });
        await image.evaluate((node: HTMLImageElement) => node.decode());
        const source = (await image.getAttribute("src"))!;
        expect(
          await image.evaluate((node: HTMLImageElement) => ({
            width: node.naturalWidth,
            height: node.naturalHeight,
          })),
        ).toEqual({ width: sample.width, height: sample.height });
        await panel
          .getByRole("figure")
          .filter({ has: page.getByRole("img", { name: profile.image, exact: true }) })
          .scrollIntoViewIfNeeded();
        await expect(image).toBeInViewport({ ratio: 1 });
        await expect(
          panel.getByText(
            profile.locale === "zh"
              ? "已记录图像，并非实时桌面。"
              : "Recorded image; this is not a live desktop.",
            { exact: true },
          ),
        ).toBeInViewport({ ratio: 1 });
        await test.info().attach(`${sample.id}-inline`, {
          body: await page.screenshot(),
          contentType: "image/png",
        });
        const trigger = panel.getByRole("button", {
          name: `${profile.view}: ${profile.image}`,
          exact: true,
        });
        await expect(trigger).toHaveCount(1);
        const inline = await image.boundingBox();
        expect(inline).not.toBeNull();
        expect(inline!.width / inline!.height).toBeCloseTo(sample.width / sample.height, 2);
        expect(inline!.width).toBeLessThanOrEqual(sample.width);
        expect(inline!.height).toBeLessThanOrEqual(sample.height);
        await trigger.focus();
        await page.keyboard.press("Enter");
        const dialog = page.getByRole("dialog", { name: profile.image, exact: true });
        await expect(dialog).toBeVisible();
        const expanded = dialog.getByRole("img", { name: profile.image, exact: true });
        await expanded.evaluate((node: HTMLImageElement) => node.decode());
        await expect(expanded).toHaveAttribute("src", source);
        const viewport = dialog.locator("[data-image-zoom]");
        await expect(viewport).toHaveAttribute("data-image-zoom", "100");
        await expect
          .poll(() =>
            expanded.evaluate((node) => {
              const image = node.getBoundingClientRect();
              const viewport = node.closest("[data-image-zoom]")!.getBoundingClientRect();
              return (
                image.width > 0 &&
                image.height > 0 &&
                image.left >= viewport.left - 1 &&
                image.right <= viewport.right + 1 &&
                image.top >= viewport.top - 1 &&
                image.bottom <= viewport.bottom + 1
              );
            }),
          )
          .toBe(true);
        await test.info().attach(`${sample.id}-fitted`, {
          body: await page.screenshot(),
          contentType: "image/png",
        });
        for (const _step of [125, 150, 200])
          await dialog.getByRole("button", { name: profile.zoom, exact: true }).click();
        await expect(viewport).toHaveAttribute("data-image-zoom", "200");
        const before = await viewport.evaluate((node) => ({
          top: node.scrollTop,
          height: node.scrollHeight,
          visible: node.clientHeight,
        }));
        if (before.height > before.visible + 1) {
          await viewport.focus();
          await page.keyboard.press("ArrowDown");
          await expect
            .poll(() => viewport.evaluate((node) => node.scrollTop))
            .toBeGreaterThan(before.top);
        }
        await viewport.focus();
        for (const key of [
          "Shift+ArrowUp",
          "Shift+ArrowUp",
          "Shift+ArrowLeft",
          "Shift+ArrowLeft",
        ]) {
          await page.keyboard.press(key);
        }
        await expect
          .poll(() => viewport.evaluate((node) => ({ left: node.scrollLeft, top: node.scrollTop })))
          .toEqual({ left: 0, top: 0 });
        await test.info().attach(`${sample.id}-expanded`, {
          body: await page.screenshot(),
          contentType: "image/png",
        });
        await page.keyboard.press("Escape");
        await expect(dialog).toHaveCount(0);
        await expect(trigger).toBeFocused();
        await expect(record).toHaveAttribute("aria-expanded", "true");
        await expect(editor).toHaveValue(draft);
        expect(
          await page.evaluate(() => document.documentElement.scrollWidth - innerWidth),
        ).toBeLessThanOrEqual(1);
      }
    });
  }

  test(`recorded computer decode failure preserves the next image in ${profile.locale}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 320, height: 960 });
    await page.goto(
      `/lab/fixture.html?fixture=computer&state=unreadable&locale=${profile.locale}&theme=${profile.theme}&font-size=18`,
    );
    await expectLabReady(page);
    const panel = page.getByRole("region", { name: profile.panel, exact: true });
    const unreadable = panel.getByRole("button", {
      name: "recorded-unreadable-image",
      exact: true,
    });
    await unreadable.click();
    const notice = panel.getByRole("img", { name: profile.image, exact: true });
    await expect(notice).toContainText(
      profile.locale === "zh"
        ? "无法显示此记录的图像。"
        : "This recorded image could not be displayed.",
    );
    await expect(
      panel.getByRole("button", { name: `${profile.view}: ${profile.image}`, exact: true }),
    ).toHaveCount(0);
    await expect(unreadable).toHaveAttribute("aria-expanded", "true");
    await notice.scrollIntoViewIfNeeded();
    await test.info().attach("recorded-image-unavailable", {
      body: await page.screenshot(),
      contentType: "image/png",
    });
    const valid = panel.getByRole("button", { name: "recorded-valid-image", exact: true });
    await valid.click();
    const trigger = panel.getByRole("button", {
      name: `${profile.view}: ${profile.image}`,
      exact: true,
    });
    await trigger.focus();
    await page.keyboard.press("Space");
    const dialog = page.getByRole("dialog", { name: profile.image, exact: true });
    await expect(dialog).toBeVisible();
    await dialog
      .getByRole("img", { name: profile.image, exact: true })
      .evaluate((node: HTMLImageElement) => node.decode());
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
    await expect(trigger).toBeFocused();
    await expect(valid).toHaveAttribute("aria-expanded", "true");
  });

  test(`recorded computer resource keeps its source beside image focus in ${profile.locale}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 320, height: 960 });
    await page.goto(
      `/lab/fixture.html?fixture=computer&state=resources&locale=${profile.locale}&theme=${profile.theme}&font-size=18`,
    );
    await expectLabReady(page);
    const panel = page.getByRole("region", { name: profile.panel, exact: true });
    await panel.getByRole("button", { name: "recorded-resources", exact: true }).click();
    const uri = panel.getByText(
      "memory://captures/2026-10-29/review/desktop-window-with-source-context-and-retained-original-identity.png",
      { exact: true },
    );
    const trigger = panel.getByRole("button", {
      name: `${profile.view}: ${profile.image}`,
      exact: true,
    });
    await trigger.focus();
    await expect(uri).toBeInViewport({ ratio: 1 });
    await expect(trigger).toBeInViewport({ ratio: 1 });
    await expect
      .poll(() =>
        uri.evaluate((node) => {
          const controls = node
            .closest('[data-slot="panel"]')!
            .querySelector('[data-slot="panel-controls"]')!;
          return node.getBoundingClientRect().top - controls.getBoundingClientRect().bottom;
        }),
      )
      .toBeGreaterThanOrEqual(0);
    await test.info().attach("recorded-resource-identity", {
      body: await page.screenshot(),
      contentType: "image/png",
    });
    await page.keyboard.press("Enter");
    const dialog = page.getByRole("dialog", { name: profile.image, exact: true });
    await expect(dialog).toBeVisible();
    await dialog
      .getByRole("button", {
        name: profile.locale === "zh" ? "关闭图片" : "Close image",
        exact: true,
      })
      .click();
    await expect(dialog).toHaveCount(0);
    await expect(trigger).toBeFocused();
    await expect(uri).toBeInViewport({ ratio: 1 });
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth - innerWidth),
    ).toBeLessThanOrEqual(1);
  });
}
