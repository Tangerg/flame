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

async function growAbove(page: Page): Promise<boolean> {
  return page.evaluate((height) => {
    const content = document.querySelector(".msg-scroll-viewport")?.firstElementChild;
    if (!content) return false;
    const block = document.createElement("div");
    block.style.height = `${height}px`;
    block.dataset.grownByTest = "";
    content.prepend(block);
    return true;
  }, GROWTH_PX * 3);
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

test("a running turn says so where the transcript cannot take it away", async ({ page }) => {
  await page.setViewportSize({ width: 1120, height: 720 });
  await page.goto("/visual/?fixture=agent&state=waves&theme=light");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.locator(".msg-scroll-viewport").waitFor();
  await page.waitForTimeout(500);

  const status = page.locator('[data-slot="agent-status"]');
  await expect(status, "the run is running, so exactly one thing says so").toHaveCount(1);
  await expect(status, "and says how long the wait has been").toHaveText(/\d/);

  const onScreen = () =>
    status.evaluate((node) => {
      const box = node.getBoundingClientRect();
      return box.top >= 0 && box.bottom <= window.innerHeight;
    });

  expect(await growAbove(page), "the insert has to find the content element").toBe(true);
  await page.waitForTimeout(700);
  expect(await onScreen(), "pinned to the tail, the run status is on screen").toBe(true);

  await page.locator(".msg-scroll-viewport").evaluate((viewport) => {
    viewport.scrollTop = 0;
  });
  await page.waitForTimeout(400);
  const parked = await read(page);
  expect(parked.top, "the reader has left the tail").toBe(0);
  expect(
    parked.fromBottom,
    "this proves nothing unless the tail is far enough away to be off screen",
  ).toBeGreaterThan(GROWTH_PX);

  expect(await onScreen(), "reading earlier in the transcript does not hide it").toBe(true);
});

test("a thought that is still being written follows its own tail", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto("/visual/?fixture=agent&state=answer-opening&theme=light");
  await page.locator("html[data-visual-ready]").waitFor();
  await page
    .locator('[data-slot="agent-activity-disclosure"]')
    .filter({ hasText: "Thinking" })
    .first()
    .getByRole("button", { expanded: false })
    .first()
    .click();
  const scroller = page.locator('[data-slot="reasoning-scroller"]');
  await scroller.waitFor();
  await page.waitForTimeout(500);

  const write = (lines: number) =>
    scroller.evaluate(async (el, count) => {
      const content = el.firstElementChild;
      for (let i = 0; i < count; i += 1) {
        const line = document.createElement("p");
        line.textContent = "Streamed reasoning line: tracing the ownership boundary.";
        line.style.cssText = "margin:0;height:20px";
        content?.appendChild(line);
        await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
      }
      return {
        scrollTop: Math.round(el.scrollTop),
        fromBottom: Math.round(el.scrollHeight - el.scrollTop - el.clientHeight),
      };
    }, lines);

  const following = await write(12);
  expect(
    following.fromBottom,
    "reasoning arrives at the end, so the end is what a thinking block shows",
  ).toBeLessThanOrEqual(2);
  expect(following.scrollTop, "this proves nothing unless the box actually moved").toBeGreaterThan(
    100,
  );

  await scroller.hover();
  await page.mouse.wheel(0, -2000);
  await page.waitForTimeout(300);
  const parked = await write(6);
  expect(parked.scrollTop, "a reader who went back must not be dragged forward again").toBe(0);
  expect(parked.fromBottom, "and the text keeps arriving below them").toBeGreaterThan(100);

  await page.mouse.wheel(0, 4000);
  await page.waitForTimeout(300);
  const resumed = await write(6);
  expect(resumed.fromBottom, "returning to the end takes the follow back up").toBeLessThanOrEqual(
    2,
  );
});
