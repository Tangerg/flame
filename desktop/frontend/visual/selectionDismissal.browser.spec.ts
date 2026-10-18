import { expect, test, type Page } from "./test";

async function openConversation(page: Page) {
  await page.setViewportSize({ width: 1472, height: 900 });
  await page.goto("/visual/?fixture=agent&state=long-content&theme=light&font-size=14");
  await page.locator("html[data-visual-ready]").waitFor();
  return page.locator(".msg-scroll-viewport");
}

async function hasSelectedText(page: Page) {
  return page.evaluate(() => {
    const selection = window.getSelection();
    return Boolean(selection && !selection.isCollapsed && selection.toString().trim());
  });
}

test("clicking conversation whitespace dismisses a keyboard text selection", async ({ page }) => {
  const viewport = await openConversation(page);
  await page
    .getByText("The consumer owns persistence policy and transaction scope.", { exact: false })
    .click();
  await page.keyboard.press("ControlOrMeta+A");
  await expect.poll(() => hasSelectedText(page)).toBe(true);

  await viewport.click({ position: { x: 12, y: 230 } });

  await expect.poll(() => page.evaluate(() => window.getSelection()?.isCollapsed)).toBe(true);
});

test("clicking conversation whitespace dismisses an inline code selection", async ({ page }) => {
  const viewport = await openConversation(page);
  await page.locator(".msg-content .md p > code").first().dblclick();
  await expect.poll(() => hasSelectedText(page)).toBe(true);

  await viewport.click({ position: { x: 12, y: 230 } });

  await expect.poll(() => page.evaluate(() => window.getSelection()?.isCollapsed)).toBe(true);
});
