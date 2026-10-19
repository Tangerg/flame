import { expect, test, type Locator } from "./test";

test.use({ hasTouch: true });

async function expectReadableTask(root: Locator) {
  const complete = root.getByRole("checkbox", {
    name: "Preserve the durable fact",
    exact: true,
    checked: true,
  });
  const pending = root.getByRole("checkbox", {
    name: "Verify the projected view",
    exact: true,
    checked: false,
  });
  await expect(complete).toBeDisabled();
  await expect(pending).toBeDisabled();
  await complete.scrollIntoViewIfNeeded();

  const geometry = await complete.evaluate((marker) => {
    const item = marker.closest("li")!;
    const paragraphs = item.querySelectorAll(":scope > p");
    const title = paragraphs[0]!;
    const followUp = paragraphs[1]!;
    const style = getComputedStyle(item);
    return {
      marker: marker.getBoundingClientRect().toJSON(),
      title: title.getBoundingClientRect().toJSON(),
      followUp: followUp.getBoundingClientRect().toJSON(),
      fontSize: Number.parseFloat(style.fontSize),
      lineHeight: Number.parseFloat(style.lineHeight),
      text: item.textContent,
    };
  });
  expect(geometry.fontSize).toBe(21);
  expect(Math.abs(geometry.marker.width - geometry.fontSize)).toBeLessThanOrEqual(1);
  expect(Math.abs(geometry.marker.height - geometry.fontSize)).toBeLessThanOrEqual(1);
  expect(geometry.marker.right).toBeLessThan(geometry.title.x);
  expect(geometry.marker.y).toBeGreaterThanOrEqual(geometry.title.y);
  expect(geometry.marker.bottom).toBeLessThanOrEqual(geometry.title.y + geometry.lineHeight + 1);
  expect(Math.abs(geometry.title.x - geometry.followUp.x)).toBeLessThanOrEqual(1);
  expect(geometry.followUp.y).toBeGreaterThanOrEqual(geometry.title.bottom);
  expect(geometry.text).not.toContain("[x]");
  expect(geometry.text).toContain(
    "Keep the follow-up explanation aligned with the task body, not beneath the checkbox.",
  );
}

test("recorded tasks keep their reading size and body alignment in narrow touch views", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1472, height: 900 });
  await page.goto("/visual/?fixture=workspace&state=dock-files&theme=light&font-size=18");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.getByRole("treeitem", { name: "reading-guide.md", exact: true }).click();
  const resizer = page.locator(".agent-pane-resizer");
  await resizer.focus();
  await resizer.press("Home");

  const view = page.locator('[data-dock-view-id="file"]');
  await expectReadableTask(view.locator('[data-slot="document-body"]'));
  const previewTab = (await view.getByRole("tab", { name: "Preview", exact: true }).boundingBox())!;
  expect(previewTab.width).toBeGreaterThanOrEqual(44);
  expect(previewTab.height).toBeGreaterThanOrEqual(44);

  await page.setViewportSize({ width: 320, height: 900 });
  await page.goto("/visual/?fixture=agent&state=long-content&theme=light&font-size=18");
  await page.locator("html[data-visual-ready]").waitFor();
  await expectReadableTask(page.locator(".md").filter({ hasText: "Structural primitives" }));
  expect(await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)).toBe(
    0,
  );
});
