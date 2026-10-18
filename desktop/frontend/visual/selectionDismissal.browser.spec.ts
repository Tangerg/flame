import { expect, test, type Page } from "./test";
import { CODE_READING_SOURCE } from "./agentSessionSnapshots";

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

test("a code reader selects its literal source without selecting the surrounding app", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1472, height: 900 });
  await page.goto("/visual/?fixture=agent&state=code-reading&theme=light");
  await page.locator("html[data-visual-ready]").waitFor();
  const source = page.getByRole("region", { name: "Code content", exact: true });
  await source.focus();
  await page.keyboard.press("ControlOrMeta+A");
  await expect
    .poll(() => page.evaluate(() => getSelection()?.toString()))
    .toBe(CODE_READING_SOURCE);
  await expect(source).toBeFocused();
});

test("clicking the conversation heading dismisses a text selection", async ({ page }) => {
  await openConversation(page);
  await page
    .getByText("The consumer owns persistence policy and transaction scope.", { exact: false })
    .click();
  await page.keyboard.press("ControlOrMeta+A");
  await expect.poll(() => hasSelectedText(page)).toBe(true);
  await page.getByRole("heading", { name: "Agent · long-content", exact: true }).click();
  await expect.poll(() => page.evaluate(() => getSelection()?.isCollapsed)).toBe(true);
});

test("a Dock code reader keeps select all inside its recorded evidence", async ({ page }) => {
  await page.setViewportSize({ width: 1120, height: 900 });
  await page.goto(
    "/visual/?fixture=workspace&state=dock-trajectory&agent-state=code-reading&theme=dark&font-size=18",
  );
  await page.locator("html[data-visual-ready]").waitFor();
  const record = page.locator('[data-trajectory-record="item:item_code_reading"]');
  await record.getByRole("button").first().click();
  await record.getByRole("button", { name: "Recorded evidence", exact: true }).click();
  const evidence = record.getByRole("region", { name: "Recorded evidence", exact: true });
  const source = await evidence.textContent();
  expect(JSON.parse(source!)).toMatchObject({ item: { id: "item_code_reading" } });
  await evidence.focus();
  await page.keyboard.press("ControlOrMeta+A");
  await expect.poll(() => page.evaluate(() => getSelection()?.toString())).toBe(source);
  await expect(evidence).toBeFocused();
  await record.getByRole("button", { name: "Recorded evidence", exact: true }).click();
  await expect(
    record.getByRole("button", { name: "Recorded evidence", exact: true }),
  ).toHaveAttribute("aria-expanded", "false");
});

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

test("a recorded subagent error can be selected and dismissed in Dock whitespace", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1120, height: 1000 });
  await page.goto(
    "/visual/?fixture=workspace&state=dock-subagents&agent-state=delegated&theme=dark&font-size=18",
  );
  await page.locator("html[data-visual-ready]").waitFor();
  const dock = page.locator('[data-dock-view-id="subagents"]');
  const detail = "The fixture generator exited 1.";
  await dock
    .locator('[data-slot="delegated-run-link"]')
    .filter({ hasText: detail })
    .getByRole("button")
    .first()
    .click();
  const body = dock.getByText(detail, { exact: true });
  await body.scrollIntoViewIfNeeded();
  const bounds = (await body.boundingBox())!;
  await page.mouse.move(bounds.x + 1, bounds.y + bounds.height / 2);
  await page.mouse.down();
  await page.mouse.move(bounds.x + bounds.width - 1, bounds.y + bounds.height / 2, { steps: 16 });
  await page.mouse.up();
  await expect.poll(() => page.evaluate(() => window.getSelection()?.toString())).toBe(detail);

  await dock.getByRole("region", { name: "Subagents", exact: true }).click({
    position: { x: 12, y: 400 },
  });

  await expect.poll(() => page.evaluate(() => window.getSelection()?.isCollapsed)).toBe(true);
});
