import { expect, test } from "./test";

export function registerUiOwnershipContracts() {
  test("autosize follows content, width, font and reset after a hidden mount", async ({ page }) => {
    await page.goto("/visual/?fixture=atoms&state=input-contracts&theme=light");
    await page.getByRole("button", { name: "Show notes" }).click();
    const note = page.getByRole("textbox", { name: "Controlled note", exact: true });
    const height = () => note.evaluate((element) => element.getBoundingClientRect().height);
    const initial = await height();
    expect(initial).toBeGreaterThan(10);
    await note.fill("one\ntwo\nthree\nfour");
    await expect.poll(height).toBeGreaterThan(initial * 2);
    await note.fill(Array.from({ length: 30 }, (_, index) => `Line ${index}`).join("\n"));
    await expect.poll(height).toBe(160);
    expect(await note.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(
      true,
    );
    await note.fill("Words that wrap into multiple lines when the note becomes narrow.");
    const wide = await height();
    await page.getByRole("button", { name: "Toggle note width" }).click();
    await expect.poll(height).toBeGreaterThan(wide);
    await note.fill("Short");
    const smallFont = await height();
    await page.getByRole("button", { name: "Toggle note font" }).click();
    await expect.poll(height).toBeGreaterThan(smallFont);
    const uncontrolled = page.getByRole("textbox", { name: "Uncontrolled note", exact: true });
    await uncontrolled.fill("one\ntwo\nthree\nfour\nfive");
    await page.getByRole("button", { name: "Reset notes" }).click();
    await expect(note).toHaveValue("A short note");
    await expect(uncontrolled).toHaveValue("A short note");
    await expect
      .poll(() => uncontrolled.evaluate((element) => element.getBoundingClientRect().height))
      .toBeLessThan(initial * 2);
  });

  test("search mutation paths obey the field state with real keyboard activation", async ({
    page,
  }) => {
    await page.goto("/visual/?fixture=atoms&state=input-contracts&theme=light");
    for (const state of ["disabled", "readOnly", "pending"]) {
      const clear = page.getByRole("button", { name: `Clear ${state} search`, exact: true });
      await expect(clear).toBeDisabled();
      await clear.dispatchEvent("click");
      await expect(
        page.getByRole("searchbox", { name: `${state} search`, exact: true }),
      ).toHaveValue("fixed");
    }
    const clear = page.getByRole("button", { name: "Clear editable search", exact: true });
    await clear.focus();
    await page.keyboard.press("Enter");
    await expect(page.getByRole("searchbox", { name: "Editable search" })).toHaveValue("");
    await expect(clear).toHaveCount(0);
  });

  test("Dock menu movement and closing restore focus without iterator helpers", async ({
    page,
  }) => {
    await page.addInitScript(() => {
      const prototype = Object.getPrototypeOf(Object.getPrototypeOf([][Symbol.iterator]()));
      Object.defineProperty(prototype, "find", { value: undefined, configurable: true });
    });
    await page.goto("/visual/?fixture=workspace&state=dock-light&theme=light");
    await page.locator("html[data-visual-ready]").waitFor();
    const trajectory = page.getByRole("tab", { name: "Session trajectory", exact: true });
    await trajectory.focus();
    await page.keyboard.press("Shift+F10");
    await expect(page.getByRole("menuitem", { name: "Move panel right" })).toBeDisabled();
    await page.getByRole("menuitem", { name: "Move panel left" }).click();
    await expect(page.getByTestId("dock-view-ids")).toHaveText(
      "file,package:visual:trajectory,package:visual:diff",
    );
    await expect(trajectory).toBeFocused();
    await page.keyboard.press("Delete");
    const diff = page.getByRole("tab", { name: "Workspace diff", exact: true });
    await expect(diff).toBeFocused();
    await page.keyboard.press("Shift+F10");
    await page.getByRole("menuitem", { name: "Close all panels" }).click();
    await expect(page.getByRole("button", { name: "Browse panels" })).toBeFocused();
    await expect(page.getByRole("tab")).toHaveCount(0);
  });

  test("a canceled Dock resize restores preview and leaves the preference untouched", async ({
    page,
  }) => {
    await page.goto("/visual/?fixture=workspace&state=dock-light&theme=light");
    await page.locator("html[data-visual-ready]").waitFor();
    const ratio = page.getByTestId("persisted-dock-ratio");
    const before = await ratio.textContent();
    const row = page.locator(".agent-dock-row");
    const property = await row.evaluate((element) =>
      (element as HTMLElement).style.getPropertyValue("--dock-measure"),
    );
    const separator = page.getByRole("separator", { name: "Resize right workspace" });
    const box = await separator.boundingBox();
    await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2);
    await page.mouse.down();
    await page.mouse.move(box!.x - 80, box!.y + box!.height / 2, { steps: 4 });
    await page.keyboard.press("Escape");
    await page.mouse.up();
    await expect(ratio).toHaveText(before!);
    expect(
      await row.evaluate((element) =>
        (element as HTMLElement).style.getPropertyValue("--dock-measure"),
      ),
    ).toBe(property);
  });

  test("image zoom anchors a pixel and pan is independent of event segmentation", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 640, height: 360 });
    await page.goto("/visual/?fixture=agent&state=long-content&theme=light");
    await page.locator("html[data-visual-ready]").waitFor();
    const trigger = page.getByRole("button", { name: "Inline architecture", exact: true });
    await trigger.scrollIntoViewIfNeeded();
    await trigger.click();
    const dialog = page.getByRole("dialog", { name: "Inline architecture", exact: true });
    const pan = dialog.getByRole("region", { name: "Inline architecture" });
    const image = dialog.locator("img");
    for (let step = 0; step < 4; step++)
      await page.getByRole("button", { name: "Zoom in image" }).click();
    await expect(pan).toHaveAttribute("data-image-zoom", "300");
    const box = await pan.boundingBox();
    const point = { x: box!.x + box!.width / 2 + 8, y: box!.y + box!.height / 2 };
    const before = await image.boundingBox();
    const coordinate = {
      x: (point.x - before!.x) / before!.width,
      y: (point.y - before!.y) / before!.height,
    };
    await page.mouse.move(point.x, point.y);
    await page.mouse.wheel(0, -100);
    await expect(pan).toHaveAttribute("data-image-zoom", "400");
    const after = await image.boundingBox();
    expect(Math.abs(after!.x + coordinate.x * after!.width - point.x)).toBeLessThan(1.5);
    expect(Math.abs(after!.y + coordinate.y * after!.height - point.y)).toBeLessThan(1.5);
    const drag = async (steps: number) => {
      await pan.evaluate((element) => {
        element.scrollLeft = 100;
        element.scrollTop = 30;
      });
      await page.mouse.move(point.x, point.y);
      await page.mouse.down();
      await page.mouse.move(point.x - 30, point.y - 20, { steps });
      await page.mouse.up();
      return pan.evaluate((element) => ({ x: element.scrollLeft, y: element.scrollTop }));
    };
    const single = await drag(1);
    const segmented = await drag(3);
    expect(segmented).toEqual(single);
    expect(single.x).toBeGreaterThan(100);
    await pan.focus();
    await page.keyboard.press("ArrowRight");
    await expect(dialog).toBeVisible();
    await expect
      .poll(() => pan.evaluate((element) => element.scrollLeft))
      .toBeGreaterThan(single.x);
    await page.setViewportSize({ width: 700, height: 400 });
    await expect(pan).toHaveAttribute("data-image-zoom", "100");
    const next = page.getByRole("button", { name: "Next image", exact: true });
    await next.click();
    await expect(page.getByRole("dialog", { name: "Inline detail", exact: true })).toBeVisible();
    await expect(next).toBeFocused();
    await page.keyboard.press("ArrowLeft");
    await expect(dialog).toBeVisible();
    await expect(pan).toHaveAttribute("data-image-zoom", "100");
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
    await expect(trigger).toBeFocused();
  });
}
