import { expect, test } from "./test";
import { expectLabReady } from "../readiness";

for (const browserName of ["chromium", "webkit"] as const) {
  test.extend({ browserName })(
    `${browserName} serializes a native partial selection across virtual output windows`,
    async ({ page }) => {
      await page.goto(
        "/lab/fixture.html?fixture=agent&theme=light&state=tool-shells&long-output=1",
      );
      await expectLabReady(page);
      await page.getByRole("button", { name: /steps/ }).first().click();
      const command = page.locator('[data-tool="shell"]').first();
      const disclosure = command.getByRole("button").first();
      if ((await disclosure.getAttribute("aria-expanded")) !== "true") await disclosure.click();
      await command.getByRole("button", { name: /earlier lines · 50000 total/ }).click();
      const output = command.getByRole("group", { name: "Tool output", exact: true });
      await expect(output).toContainText("build output line 49999");
      await output.evaluate((node) => {
        node.scrollTop = 0;
        node.dispatchEvent(new Event("scroll"));
      });
      const visibleLine = () =>
        output.locator("[data-output-line]").evaluateAll((rows) => {
          const viewport = rows[0]?.closest('[role="group"]')?.getBoundingClientRect();
          if (!viewport) return null;
          const row = rows.find((row) => {
            const bounds = row.getBoundingClientRect();
            return (
              bounds.top >= viewport.top + viewport.height / 3 && bounds.bottom <= viewport.bottom
            );
          });
          return row?.getAttribute("data-output-line") ?? null;
        });
      const anchor = output.locator('[data-output-line="2"]');
      await expect(anchor).toBeVisible();
      await anchor.evaluate((node) => {
        const text = node.firstChild!.firstChild!;
        node.ownerDocument.getSelection()!.setBaseAndExtent(text, 6, text, 6);
        node.ownerDocument.dispatchEvent(new Event("selectionchange"));
      });
      await output.evaluate((node) => {
        node.scrollTop = 35 * parseFloat(getComputedStyle(node).lineHeight);
        node.dispatchEvent(new Event("scroll"));
      });
      const end = output.locator('[data-output-line="42"]');
      await expect(end).toBeVisible();
      await expect(anchor).toHaveCount(1);
      await end.evaluate((node) => {
        const selection = node.ownerDocument.getSelection()!;
        const text = node.ownerDocument.createTreeWalker(node, NodeFilter.SHOW_TEXT).nextNode()!;
        selection.extend(text, 5);
        node.ownerDocument.dispatchEvent(new Event("selectionchange"));
      });
      await output.dispatchEvent("wheel", { deltaY: -1 });
      await output.evaluate((node) => {
        node.scrollTop = 400 * parseFloat(getComputedStyle(node).lineHeight);
        node.dispatchEvent(new Event("scroll"));
      });
      await expect.poll(visibleLine).not.toBeNull();
      let readingLine = (await visibleLine())!;
      expect(Number(readingLine)).toBeGreaterThan(42);
      await expect(output.locator(`[data-output-line="${readingLine}"]`)).toBeInViewport();
      await expect(anchor).toHaveCount(1);
      await expect(end).toHaveCount(1);
      expect(await output.locator("[data-output-line]").count()).toBeLessThan(200);
      await expect(output.locator('[data-output-line="25"]')).toHaveCount(1);
      const copy = () =>
        output.evaluate((node) => {
          const data = new DataTransfer();
          const event = new ClipboardEvent("copy", {
            bubbles: true,
            cancelable: true,
            clipboardData: data,
          });
          node.dispatchEvent(event);
          return { prevented: event.defaultPrevented, text: data.getData("text/plain") };
        });
      const lines = Array.from({ length: 50_000 }, (_, index) => `build output line ${index}`);
      expect(await copy()).toEqual({
        prevented: true,
        text: lines.slice(2, 42).join("\n").slice(6) + "\n" + lines[42]!.slice(0, 5),
      });
      await output.focus();
      await output.press("ControlOrMeta+a");
      expect(await output.locator("[data-output-line]").count()).toBeLessThan(200);
      expect(await copy()).toEqual({ prevented: true, text: lines.join("\n") });
      await output.evaluate((node) => {
        node.scrollTop = 0;
        node.dispatchEvent(new Event("scroll"));
      });
      await expect(output.locator('[data-output-line="0"]')).toHaveCount(1);
      expect(await copy()).toEqual({ prevented: true, text: lines.join("\n") });
      await output.dispatchEvent("wheel", { deltaY: -1 });
      await output.evaluate((node) => {
        node.scrollTop = 400 * parseFloat(getComputedStyle(node).lineHeight);
        node.dispatchEvent(new Event("scroll"));
      });
      await expect.poll(visibleLine).not.toBeNull();
      readingLine = (await visibleLine())!;
      await expect(output.locator(`[data-output-line="${readingLine}"]`)).toBeInViewport();
      const readingOffset = await output.evaluate((node) => node.scrollTop);
      await command.getByRole("button").first().click();
      await expect(output).toBeHidden();
      await command.getByRole("button").first().click();
      await expect.poll(() => output.evaluate((node) => node.scrollTop)).toBe(readingOffset);
      await output.scrollIntoViewIfNeeded();
      await expect(output.locator(`[data-output-line="${readingLine}"]`)).toBeInViewport();
      await output.press("ControlOrMeta+f");
      const find = command.getByRole("searchbox", { name: "Find in output" });
      await find.fill("line 42");
      await expect(output.locator("[data-current-match]")).toBeInViewport();
      await output.dispatchEvent("wheel", { deltaY: -1 });
      await output.evaluate((node) => {
        node.scrollTop = 400 * parseFloat(getComputedStyle(node).lineHeight);
        node.dispatchEvent(new Event("scroll"));
      });
      await expect.poll(visibleLine).not.toBeNull();
      readingLine = (await visibleLine())!;
      await expect(output.locator(`[data-output-line="${readingLine}"]`)).toBeInViewport();
      await command.evaluate((node) =>
        (node as HTMLElement).style.setProperty("--fs-code", "18px"),
      );
      await expect(output.locator(`[data-output-line="${readingLine}"]`)).toBeInViewport();
      await expect(output.locator("[data-current-match]")).toHaveCount(0);
      await expect(find).toBeFocused();
    },
  );
}
