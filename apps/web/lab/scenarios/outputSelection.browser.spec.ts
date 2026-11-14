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
      await command.getByRole("button").first().click();
      await command.getByRole("button", { name: /earlier lines · 50000 total/ }).click();
      const output = command.getByRole("region", { name: "Tool output", exact: true });
      await expect(output).toContainText("build output line 49999");
      await output.evaluate((node) => {
        node.scrollTop = 0;
        node.dispatchEvent(new Event("scroll"));
      });
      const anchor = output.locator('[data-output-line="2"]');
      await expect(anchor).toBeVisible();
      await anchor.evaluate((node) => {
        const text = node.firstChild!.firstChild!;
        node.ownerDocument.getSelection()!.setBaseAndExtent(text, 6, text, 6);
        node.ownerDocument.dispatchEvent(new Event("selectionchange"));
      });
      await output.evaluate((node) => {
        node.scrollTop = 31_400 * parseFloat(getComputedStyle(node).lineHeight);
        node.dispatchEvent(new Event("scroll"));
      });
      const end = output.locator('[data-output-line="31415"]');
      await expect(end).toBeVisible();
      await expect(anchor).toHaveCount(1);
      await end.evaluate((node) => {
        const selection = node.ownerDocument.getSelection()!;
        selection.extend(node.children[1]!.firstChild!, 4);
        node.ownerDocument.dispatchEvent(new Event("selectionchange"));
      });
      await output.dispatchEvent("wheel", { deltaY: -1 });
      await output.evaluate((node) => {
        node.scrollTop = 400 * parseFloat(getComputedStyle(node).lineHeight);
        node.dispatchEvent(new Event("scroll"));
      });
      await expect(output.locator('[data-output-line="400"]')).toBeVisible();
      await expect(anchor).toHaveCount(1);
      await expect(end).toHaveCount(1);
      expect(await output.locator("[data-output-line]").count()).toBeLessThan(100);
      await expect(output.locator('[data-output-line="25000"]')).toHaveCount(0);
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
        text: lines.slice(2, 31_415).join("\n").slice(6) + "\n" + lines[31_415]!.slice(0, 19),
      });
      await output.focus();
      await output.press("ControlOrMeta+a");
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
      await expect(output.locator('[data-output-line="400"]')).toBeInViewport();
      await command.getByRole("button").first().click();
      await expect(output).toBeHidden();
      await command.getByRole("button").first().click();
      await expect(output.locator('[data-output-line="400"]')).toBeInViewport();
      await output.press("ControlOrMeta+f");
      const find = command.getByRole("searchbox", { name: "Find in output" });
      await find.fill("line 31415");
      await expect(output.locator("[data-current-match]")).toBeInViewport();
      await output.dispatchEvent("wheel", { deltaY: -1 });
      await output.evaluate((node) => {
        node.scrollTop = 400 * parseFloat(getComputedStyle(node).lineHeight);
        node.dispatchEvent(new Event("scroll"));
      });
      await expect(output.locator('[data-output-line="400"]')).toBeInViewport();
      await command.evaluate((node) =>
        (node as HTMLElement).style.setProperty("--fs-code", "18px"),
      );
      await expect(output.locator('[data-output-line="400"]')).toBeInViewport();
      await expect(output.locator("[data-current-match]")).toHaveCount(0);
      await expect(find).toBeFocused();
    },
  );
}
