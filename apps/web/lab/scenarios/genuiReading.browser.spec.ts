import { expect, test } from "./test";
test.setTimeout(30_000);

test("retains A2UI drafts and the captured action across real row remounts", async ({ page }) => {
  await page.goto("/lab/a2ui.html");
  await page.getByRole("button", { name: "Show interface", exact: true }).click();
  const field = page.getByRole("textbox", { name: "Review note", exact: true });
  await field.fill("local draft 9007199254740993");
  await page.getByRole("button", { name: "Show start", exact: true }).click();
  await expect(field).toHaveCount(0);
  await page.getByRole("button", { name: "Show interface", exact: true }).click();
  await expect(field).toHaveValue("local draft 9007199254740993");
  await page.getByRole("button", { name: "Send review", exact: true }).click();
  await expect(page.getByLabel("Captured submission")).toContainText(
    "local draft 9007199254740993",
  );
});

test("pins a composing lower row during reflow and prepend and retains portal ownership", async ({
  page,
}) => {
  await page.goto("/lab/a2ui.html");
  await page.getByRole("button", { name: "Show interface", exact: true }).click();
  const field = page.getByRole("textbox", { name: "Review note", exact: true });
  await field.fill("retained local draft");
  await field.focus();
  await field.evaluate((node) =>
    node.dispatchEvent(new CompositionEvent("compositionstart", { bubbles: true, data: "中" })),
  );
  await page
    .locator("nav button")
    .filter({ hasText: "Reflow upper row" })
    .evaluate((node: HTMLButtonElement) => node.click());
  await page
    .getByRole("button", { name: "Prepend", exact: true })
    .evaluate((node: HTMLButtonElement) => node.click());
  await expect(field).toBeFocused();
  await expect(field).toBeInViewport({ ratio: 1 });
  await expect(field).toHaveValue("retained local draft");
  await field.evaluate((node) =>
    node.dispatchEvent(new CompositionEvent("compositionend", { bubbles: true, data: "中" })),
  );
  await page.mouse.move(300, 400);
  await page.mouse.wheel(0, -900);
  await expect(field).not.toBeInViewport();
  await page
    .locator("nav button")
    .filter({ hasText: "Reflow upper row" })
    .evaluate((node: HTMLButtonElement) => node.click());
  await expect(field).toBeFocused();
  await expect(field).not.toBeInViewport();
  await page.getByRole("button", { name: "Show interface", exact: true }).click();
  await page.getByRole("button", { name: "Open captured dialog", exact: true }).click();
  const portal = page.getByRole("textbox", { name: "Portal note", exact: true });
  await portal.fill("portal draft");
  await page
    .locator("nav button")
    .filter({ hasText: "Reflow upper row" })
    .evaluate((node: HTMLButtonElement) => node.click());
  await expect(portal).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Open captured dialog", exact: true }),
  ).toBeFocused();
  await page.getByRole("button", { name: "Toggle activity", exact: true }).click();
  await page.getByRole("button", { name: "Toggle activity", exact: true }).click();
  await expect(field).toHaveValue("retained local draft");
});
