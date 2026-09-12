import { expect, test, type Page } from "./test";

const ROUTE = "/visual/?fixture=agent&state=long-content&theme=light";
const JUMP = "Jump to bottom";

const GROWTH_PX = 400;

async function land(page: Page) {
  await page.goto(ROUTE);
  await page.locator("html[data-visual-ready]").waitFor();
  await page.locator(".msg-scroll-viewport").waitFor();
  await page.waitForTimeout(500);
}

async function grow(page: Page): Promise<boolean> {
  return page.evaluate((height) => {
    const content = document.querySelector(".msg-scroll-viewport")?.firstElementChild;
    if (!content) return false;
    const block = document.createElement("div");
    block.style.height = `${height}px`;
    block.dataset.grownByTest = "";
    content.appendChild(block);
    return true;
  }, GROWTH_PX);
}

async function read(page: Page) {
  const reading = await page.evaluate(() => {
    const v = document.querySelector(".msg-scroll-viewport") as HTMLElement | null;
    if (!v) return null;
    return {
      top: Math.round(v.scrollTop),
      fromBottom: Math.round(v.scrollHeight - v.clientHeight - v.scrollTop),
      scrollable: v.scrollHeight - v.clientHeight,
    };
  });
  expect(reading, "the transcript viewport has to be on the page").not.toBeNull();
  return reading!;
}

test("a growing transcript follows the tail, but only from the bottom", async ({ page }) => {
  test.setTimeout(120_000);
  await page.setViewportSize({ width: 1120, height: 720 });

  await land(page);
  const room = (await read(page)).scrollable;
  expect(room, "the route has to render more transcript than fits").toBeGreaterThan(GROWTH_PX * 2);

  await page.locator(".msg-scroll-viewport").hover();
  await page.mouse.wheel(0, room + 1000);
  await page.waitForTimeout(600);
  expect((await read(page)).fromBottom, "the wheel has to reach the bottom").toBeLessThanOrEqual(2);
  await expect(page.getByRole("button", { name: JUMP })).toHaveAttribute("tabindex", "-1");

  expect(await grow(page), "the append has to find the content element").toBe(true);
  await page.waitForTimeout(700);
  const followed = await read(page);
  expect(followed.fromBottom, "content arrived and the tail left the viewport").toBeLessThanOrEqual(
    2,
  );

  await land(page);
  await page.locator(".msg-scroll-viewport").hover();
  await page.mouse.wheel(0, room + 1000);
  await page.waitForTimeout(500);
  await page.mouse.wheel(0, -600);
  await page.waitForTimeout(600);
  const parked = await read(page);
  expect(parked.fromBottom, "the reader has to actually be away from the bottom").toBeGreaterThan(
    100,
  );

  expect(await grow(page), "the append has to find the content element").toBe(true);
  await page.waitForTimeout(700);
  const held = await read(page);
  expect(held.top, "content arrived and the view moved under the reader").toBe(parked.top);
  expect(
    held.fromBottom - parked.fromBottom,
    "the new material has to be below the reader, not around them",
  ).toBe(GROWTH_PX);

  const jump = page.getByRole("button", { name: JUMP });
  await expect(jump).toHaveAttribute("tabindex", "0");
  await jump.click();
  await expect
    .poll(async () => (await read(page)).fromBottom, { timeout: 5_000 })
    .toBeLessThanOrEqual(2);
});
