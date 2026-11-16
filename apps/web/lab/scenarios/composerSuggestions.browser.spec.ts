import { expect, test } from "./test";
import { expectLabReady } from "../readiness";
import { en } from "@/lib/i18n/locales/en";
import AxeBuilder from "@axe-core/playwright";

for (const browserName of ["chromium", "webkit"] as const) {
  test.extend({ browserName })(
    `${browserName} routes native editing, IME and candidate navigation through one Composer`,
    async ({ page }) => {
      await page.goto("/lab/fixture.html?fixture=agent&state=idle&sidebar=collapsed");
      await expectLabReady(page);
      const input = page.getByLabel(en["composer.input.label"]!, { exact: true });
      const original = await input.elementHandle();
      await input.fill("@store");
      const list = page.getByRole("listbox", {
        name: en["composer.mention.heading"]!,
        exact: true,
      });
      const first = list.getByRole("option", { name: "runtime/session/store.go", exact: true });
      const last = list.getByRole("option", { name: "runtime/session/store_test.go", exact: true });
      await expect(list.getByRole("option")).toHaveCount(2);
      await expect(first).toHaveAttribute("aria-selected", "true");
      await input.press("Escape");
      await expect(list).toBeHidden();
      await input.press("Home");
      const homeCaret = await input.evaluate((node: HTMLTextAreaElement) => node.selectionStart);
      await input.press("End");
      const endCaret = await input.evaluate((node: HTMLTextAreaElement) => node.selectionStart);

      const draft = "Keep the first line\n@store\nTail";
      const tail = "\nTail";
      const caret = draft.length - tail.length;
      const selection = () =>
        input.evaluate((node: HTMLTextAreaElement) => ({
          start: node.selectionStart,
          end: node.selectionEnd,
          direction: node.selectionDirection,
        }));
      const openMultilineMention = async () => {
        await input.fill("");
        await input.fill(draft);
        for (let remaining = tail.length; remaining > 0; remaining -= 1) {
          await input.press("ArrowLeft");
        }
        await expect(input).toHaveJSProperty("selectionStart", caret);
        await expect(input).toHaveJSProperty("selectionEnd", caret);
        await expect(list.getByRole("option")).toHaveCount(2);
        await expect(first).toHaveAttribute("aria-selected", "true");
      };
      for (const key of ["Shift+ArrowUp", "Shift+ArrowDown"]) {
        await openMultilineMention();
        await input.press("Escape");
        await expect(list).toBeHidden();
        await input.press(key);
        const native = await selection();
        expect(native.end).toBeGreaterThan(native.start);
        const continued = `${draft.slice(0, native.start)}continued${draft.slice(native.end)}`;
        await page.keyboard.type("continued");
        await expect(input).toHaveValue(continued);

        await openMultilineMention();
        await input.press(key);
        expect(await selection()).toEqual(native);
        await page.keyboard.type("continued");
        await expect(input).toHaveValue(continued);
        await expect(input).toBeFocused();
      }

      await input.fill("");
      await input.fill("@store");
      await expect(first).toHaveAttribute("aria-selected", "true");

      await input.press("Home");
      await expect(input).toHaveJSProperty("selectionStart", homeCaret);
      await input.press("End");
      await expect(input).toHaveJSProperty("selectionStart", endCaret);
      await expect(list).toBeVisible();
      await input.press("ArrowUp");
      await expect(last).toHaveAttribute("aria-selected", "true");
      await expect(input).toHaveAttribute(
        "aria-activedescendant",
        (await last.getAttribute("id")) as string,
      );
      await input.dispatchEvent("compositionstart");
      await input.dispatchEvent("keydown", { key: "ArrowDown", isComposing: true });
      await input.dispatchEvent("keydown", { key: "Enter", isComposing: true });
      await expect(last).toHaveAttribute("aria-selected", "true");
      await expect(input).toHaveValue("@store");
      await input.dispatchEvent("compositionend");
      await input.dispatchEvent("keydown", { key: "Enter", keyCode: 229 });
      await expect(input).toHaveValue("@store");
      await input.dispatchEvent("keyup", { key: "Enter" });
      await input.fill("@store_test");
      await expect(list.getByRole("option")).toHaveCount(1);
      await expect(last).toHaveAttribute("aria-selected", "true");
      await page.setViewportSize({ width: 373, height: 251 });
      await input.press("ArrowDown");
      await expect
        .poll(() =>
          last.evaluate((row) => {
            const list = row.closest('[role="listbox"]')!;
            const bounds = list.getBoundingClientRect();
            const target = row.getBoundingClientRect();
            return target.top >= bounds.top - 1 && target.bottom <= bounds.bottom + 1;
          }),
        )
        .toBe(true);
      const accessibility = await new AxeBuilder({ page })
        .include('[data-slot="composer-root"] textarea')
        .include('[role="listbox"]')
        .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
        .analyze();
      expect(accessibility.violations).toEqual([]);
      await last.hover();
      await expect(input).toHaveAttribute(
        "aria-activedescendant",
        (await last.getAttribute("id")) as string,
      );
      await last.click();
      await expect(input).toHaveValue("@runtime/session/store_test.go ");
      await expect(input).toBeFocused();
      await expect(list).toBeHidden();
      expect(await input.evaluate((node, previous) => node === previous, original)).toBe(true);
      expect(await page.locator("html").getAttribute("data-visual-sent-input")).toBeNull();
    },
  );
}
