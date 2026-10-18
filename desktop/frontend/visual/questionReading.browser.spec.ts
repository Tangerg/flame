import { expect, test, type Locator } from "./test";

const lastOption =
  "Run all release checks for the selected workspace and include accessibility diagnostics";

async function expectFocusVisible(target: Locator, request: Locator) {
  await expect(target).toBeFocused();
  const bounds = (await request.boundingBox())!;
  const focused = (await target.boundingBox())!;
  const footer = (await request.getByRole("button", { name: /^(Next|Skip)$/ }).boundingBox())!;
  expect(focused.y).toBeGreaterThanOrEqual(bounds.y);
  expect(focused.y + focused.height).toBeLessThanOrEqual(footer.y);
}

for (const [width, height, fixture, theme] of [
  [1120, 577, "workspace", "dark"],
  [1120, 720, "workspace", "light"],
  [800, 500, "agent", "light"],
] as const) {
  test(`question requests stay within the conversation at ${width}×${height}`, async ({ page }) => {
    await page.setViewportSize({ width, height });
    const scene =
      fixture === "workspace"
        ? "fixture=workspace&state=dock-light&agent-state=question-multi"
        : "fixture=agent&state=question-multi";
    await page.goto(`/visual/?${scene}&theme=${theme}&font-size=18`);
    await page.locator("html[data-visual-ready]").waitFor();
    await page.evaluate(() => document.fonts.ready);
    const request = page.locator('[data-slot="question-request-surface"]');
    const measured = await request.evaluate((node) => {
      let overlay = node.parentElement;
      while (overlay && getComputedStyle(overlay).position !== "absolute")
        overlay = overlay.parentElement;
      if (!overlay?.parentElement) throw new Error("The request must use the conversation overlay");
      const host = overlay.parentElement.getBoundingClientRect();
      const card = node.getBoundingClientRect();
      const prompt = node.querySelector("h3")!.getBoundingClientRect();
      return {
        hostTop: host.top,
        hostBottom: host.bottom,
        cardTop: card.top,
        cardBottom: card.bottom,
        promptTop: prompt.top,
        promptBottom: prompt.bottom,
      };
    });
    expect(measured.cardTop).toBeGreaterThanOrEqual(measured.hostTop);
    expect(measured.cardBottom).toBeLessThanOrEqual(measured.hostBottom);
    expect(measured.promptTop).toBeGreaterThanOrEqual(measured.cardTop);
    expect(measured.promptBottom).toBeLessThan(measured.cardBottom);

    const options = request.getByRole("checkbox");
    await expect(options).toHaveCount(5);
    await request.focus();
    const stream = page.locator(".msg-scroll-viewport");
    const streamTop = await stream.evaluate((node) => node.scrollTop);
    await page.keyboard.press("Tab");
    await expectFocusVisible(options.first(), request);
    await page.keyboard.press("Space");
    for (let index = 1; index < 5; index++) {
      await page.keyboard.press("Tab");
      await expectFocusVisible(options.nth(index), request);
    }
    await page.keyboard.press("Space");
    const last = (await options.last().boundingBox())!;
    await page.mouse.move(last.x + last.width / 2, last.y + last.height / 2);
    await page.mouse.wheel(0, -1000);
    await expect
      .poll(async () => (await request.locator("h3").boundingBox())!.y)
      .toBeGreaterThanOrEqual(measured.cardTop);
    await page.keyboard.press("Tab");
    const custom = request.getByRole("textbox", {
      name: "Which gates should run before the release?",
      exact: true,
    });
    await expectFocusVisible(custom, request);
    const localScroll = await custom.evaluate((node) => {
      const card = node.closest('[data-slot="question-request-surface"]');
      for (
        let parent = node.parentElement;
        parent && parent !== card;
        parent = parent.parentElement
      )
        if (getComputedStyle(parent).overflowY === "auto")
          return { top: parent.scrollTop, range: parent.scrollHeight - parent.clientHeight };
      throw new Error("The answer must use a local reading viewport");
    });
    if (height < 720) {
      expect(localScroll.range).toBeGreaterThan(0);
      expect(localScroll.top).toBeGreaterThan(0);
    }
    expect(await stream.evaluate((node) => node.scrollTop)).toBe(streamTop);
    await custom.fill("Keep restored focus and source selection intact.");
    await page.setViewportSize({ width, height: height + 200 });
    await page.setViewportSize({ width, height });
    await expect(custom).toHaveValue("Keep restored focus and source selection intact.");
    await expect(options.first()).toBeChecked();
    await expect(request.getByRole("checkbox", { name: lastOption, exact: true })).toBeChecked();
    await page.keyboard.press("Tab");
    const submit = request.getByRole("button", { name: "Next", exact: true });
    await expect(submit).toBeFocused();
    const button = (await submit.boundingBox())!;
    const card = (await request.boundingBox())!;
    expect(button.y).toBeGreaterThanOrEqual(card.y);
    expect(button.y + button.height).toBeLessThanOrEqual(card.y + card.height);
    await page.keyboard.press("Enter");
    await expect(page.locator("html")).toHaveAttribute(
      "data-visual-resumed-response",
      JSON.stringify({
        type: "answer",
        answers: [
          ["Race detector", lastOption, "Keep restored focus and source selection intact."],
        ],
      }),
    );
    await expect(request).toHaveCount(0);
    await expect(page.locator('[data-slot="composer-root"]')).toBeVisible();
  });
}

for (const touch of [false, true]) {
  test.describe(`question reading with ${touch ? "touch" : "mouse"}`, () => {
    test.use({ hasTouch: touch });

    test("question jump stays below the session heading after the window shrinks", async ({
      page,
    }) => {
      await page.setViewportSize({ width: 1120, height: 800 });
      await page.goto(
        "/visual/?fixture=workspace&state=dock-light&agent-state=question-multi&theme=dark&font-size=18",
      );
      await page.locator("html[data-visual-ready]").waitFor();
      await page.evaluate(() => document.fonts.ready);
      const stream = page.locator(".msg-scroll-viewport");
      const request = page.locator('[data-slot="question-request-surface"]');
      const roomy = (await stream.boundingBox())!;
      await page.mouse.move(roomy.x + roomy.width / 2, roomy.y + 40);
      await page.mouse.wheel(0, -1000);
      const jump = page.getByRole("button", { name: "Jump to bottom", exact: true });
      await expect(jump).toHaveAttribute("tabindex", "0");

      await page.setViewportSize({ width: 1120, height: 577 });
      await request.focus();
      await page.keyboard.press("Shift+Tab");
      await expect(jump).toBeFocused();
      const viewport = (await stream.boundingBox())!;
      const card = (await request.boundingBox())!;
      const button = (await jump.boundingBox())!;
      const outline = await jump.evaluate((node) => {
        const style = getComputedStyle(node);
        return parseFloat(style.outlineWidth) + parseFloat(style.outlineOffset);
      });
      expect(await jump.evaluate((node) => node.matches(":focus-visible"))).toBe(true);
      expect(button.y - outline).toBeGreaterThanOrEqual(viewport.y);
      expect(button.y + button.height).toBeLessThanOrEqual(card.y);
      if (touch) expect(button.height).toBeGreaterThanOrEqual(44);
    });
  });
}
