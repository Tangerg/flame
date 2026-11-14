import { expect, test, type Locator } from "./test";
import { expectLabReady } from "../readiness";
import { ACTIVE_DIFF_FILE } from "./diffSnapshot";

async function selectText(line: Locator, start: number, end: number) {
  await line.evaluate(
    (node, [start, end]) => {
      const point = (offset: number): [Node, number] => {
        const walker = node.ownerDocument.createTreeWalker(node, NodeFilter.SHOW_TEXT);
        for (let text = walker.nextNode(); text; text = walker.nextNode()) {
          const length = text.textContent!.length;
          if (offset <= length) return [text, offset];
          offset -= length;
        }
        throw new Error("Diff text endpoint is outside the rendered code");
      };
      node.ownerDocument.getSelection()!.setBaseAndExtent(...point(start!), ...point(end!));
    },
    [start, end],
  );
}

async function copied(reader: Locator) {
  return reader.evaluate((node) => {
    const transfer = new DataTransfer();
    node.dispatchEvent(
      new ClipboardEvent("copy", {
        bubbles: true,
        composed: true,
        cancelable: true,
        clipboardData: transfer,
      }),
    );
    return transfer.getData("text/plain");
  });
}

for (const browserName of ["chromium", "webkit"] as const) {
  test.extend({ browserName })(
    `${browserName} keeps a native diff selection inside its reader`,
    async ({ page }) => {
      await page.goto("/lab/fixture.html?fixture=workspace&state=dock-review&theme=light");
      await expectLabReady(page);
      const reader = page.getByRole("region", {
        name: `Scrollable diff for ${ACTIVE_DIFF_FILE}`,
        exact: true,
      });
      await expect(reader).toBeVisible();
      const first = reader.getByText("const rail = railRef.current;", { exact: true });
      await expect(first).toBeVisible();
      await selectText(first, 3, 14);
      const partial = await copied(reader);
      expect(partial).toBe("st rail = r");
      await reader.focus();
      await reader.press("ControlOrMeta+a");
      const whole = await copied(reader);
      expect(whole).toMatch(/^const rail = railRef.current;\n/);
      expect(whole).toMatch(/return session;\n$/);
      expect(whole).not.toContain("Included patch");
      expect(whole).not.toContain("104const");
    },
  );

  for (const profile of [
    {
      theme: "light",
      locale: "en",
      region: `Scrollable diff for ${ACTIVE_DIFF_FILE}`,
      eof: "No newline at end of file",
      layout: "Diff layout",
      split: "Side by side",
      files: "Show changed files",
      back: "Back to changed files",
    },
    {
      theme: "dark",
      locale: "zh",
      region: `${ACTIVE_DIFF_FILE} 的可滚动 diff`,
      eof: "文件末尾没有换行符",
      layout: "Diff 布局",
      split: "并排",
      files: "显示已更改文件",
      back: "返回已更改文件",
    },
  ] as const) {
    test.extend({ browserName })(
      `${browserName} preserves native Unicode, CRLF and EOF copy in ${profile.locale}`,
      async ({ page }) => {
        await page.goto(
          `/lab/fixture.html?fixture=workspace&state=dock-diff-evidence&theme=${profile.theme}&locale=${profile.locale}`,
        );
        await expectLabReady(page);
        const reader = page.getByRole("region", { name: profile.region, exact: true });
        await expect(reader).toBeVisible();
        await expect(reader.getByText(profile.eof, { exact: true })).toHaveCount(2);
        if (profile.locale === "zh")
          await expect(reader.getByText("No newline at end of file", { exact: true })).toHaveCount(
            0,
          );
        const annotation = reader.getByText(profile.eof, { exact: true }).first();
        await annotation.evaluate((node) =>
          node.ownerDocument.getSelection()!.selectAllChildren(node),
        );
        const ordinaryCopy = await reader.evaluate((node) => {
          const transfer = new DataTransfer();
          transfer.setData("text/plain", "ordinary selection");
          const event = new ClipboardEvent("copy", {
            bubbles: true,
            composed: true,
            cancelable: true,
            clipboardData: transfer,
          });
          node.dispatchEvent(event);
          return { prevented: event.defaultPrevented, text: transfer.getData("text/plain") };
        });
        expect(ordinaryCopy).toEqual({ prevented: false, text: "ordinary selection" });
        const unicode = reader.getByText("alpha中🙂omega", { exact: true });
        await selectText(unicode, 13, 6);
        expect(await copied(reader)).toBe("🙂omega");
        const eof = reader.getByText("tail-after", { exact: true });
        await selectText(eof, 0, 10);
        expect(await copied(reader)).toBe("tail-after");
        await reader.focus();
        await reader.press("ControlOrMeta+a");
        expect(await copied(reader)).toBe(
          "alpha中🙂omega\r\nold\tvalue\r\nnew\tvalue\r\n\r\ntail-before\ntail-after",
        );
        const textColor = await unicode.evaluate((node) => getComputedStyle(node).color);
        const hostColor = await reader.evaluate((node) => getComputedStyle(node).color);
        expect(textColor).toBe(hostColor);
        await page.getByRole("combobox", { name: profile.layout, exact: true }).click();
        await page.getByRole("option", { name: profile.split, exact: true }).click();
        await selectText(reader.getByText("new\tvalue", { exact: true }), 1, 7);
        expect(await copied(reader)).toBe("ew\tval");
        await selectText(reader.getByText("tail-before", { exact: true }), 0, 11);
        expect(await copied(reader)).toBe("tail-before");
        await page.getByRole("button", { name: profile.files, exact: true }).click();
        await expect(reader).toHaveCount(0);
        await expect(page.locator("[data-diff-file]")).toHaveCount(4);
        await page.locator(`[data-diff-file="${ACTIVE_DIFF_FILE}"]`).click();
        await expect(reader).toBeVisible();
        await selectText(reader.getByText("tail-after", { exact: true }), 5, 10);
        expect(await copied(reader)).toBe("after");
        await page.getByRole("button", { name: profile.back, exact: true }).click();
        await page.locator('[data-diff-file="docs/新名字.md"]').click();
        await expect(reader).toHaveCount(0);
        await expect(page.getByText("docs/旧名字.md", { exact: false })).toBeVisible();
      },
    );
  }

  test.extend({ browserName })(
    `${browserName} copies the newly rendered source after a same-file replacement`,
    async ({ page }) => {
      await page.goto("/lab/fixture.html?fixture=workspace&state=dock-diff-evidence&theme=light");
      await expectLabReady(page);
      const reader = page.getByRole("region", {
        name: `Scrollable diff for ${ACTIVE_DIFF_FILE}`,
        exact: true,
      });
      await expect(reader.getByText("tail-after", { exact: true })).toBeVisible();
      await page.getByRole("button", { name: "Show changed files", exact: true }).click();
      await page.getByRole("combobox", { name: "Comparison", exact: true }).click();
      await page.getByRole("option", { name: "Branch changes", exact: true }).click();
      await expect(page.getByText("Showing branch changes.", { exact: true })).toBeVisible();
      await page.locator(`[data-diff-file="${ACTIVE_DIFF_FILE}"]`).click();
      await expect(reader.getByText("branch-after", { exact: true })).toBeVisible();
      await selectText(reader.getByText("branch-after", { exact: true }), 0, 6);
      expect(await copied(reader)).toBe("branch");
      await page.getByRole("button", { name: "Refresh", exact: true }).click();
      await expect(page.getByText("Showing branch changes.", { exact: true })).toBeVisible();
      await expect(reader.getByText("refreshed-after", { exact: true })).toBeVisible();
      await selectText(reader.getByText("refreshed-after", { exact: true }), 0, 9);
      expect(await copied(reader)).toBe("refreshed");
      const captured = await reader.elementHandle();
      await page.getByRole("button", { name: "Back to changed files", exact: true }).click();
      expect(
        await captured!.evaluate((node) => {
          const transfer = new DataTransfer();
          const event = new ClipboardEvent("copy", {
            bubbles: true,
            composed: true,
            cancelable: true,
            clipboardData: transfer,
          });
          node.dispatchEvent(event);
          return { prevented: event.defaultPrevented, text: transfer.getData("text/plain") };
        }),
      ).toEqual({ prevented: false, text: "" });
    },
  );

  test.extend({ browserName })(
    `${browserName} distinguishes an exhausted budget from a clean comparison`,
    async ({ page }) => {
      await page.goto("/lab/fixture.html?fixture=workspace&state=dock-diff-budget&theme=light");
      await expectLabReady(page);
      await expect(
        page.getByText("No complete file fits this read’s budget.", { exact: true }),
      ).toBeVisible();
      await expect(page.getByRole("note")).toContainText("Partial diff");
      await expect(page.getByText("No changes in this comparison.", { exact: true })).toHaveCount(
        0,
      );
      await expect(page.getByRole("region", { name: /^Scrollable diff for / })).toHaveCount(0);
    },
  );

  test.extend({ browserName })(
    `${browserName} reads and copies a complete file at the actual row budget`,
    async ({ page }) => {
      await page.goto("/lab/fixture.html?fixture=workspace&state=dock-diff-large&theme=dark");
      await expectLabReady(page);
      const reader = page.getByRole("region", {
        name: `Scrollable diff for ${ACTIVE_DIFF_FILE}`,
        exact: true,
      });
      await expect(
        reader.getByText("export const item4999 = 4999;", { exact: true }),
      ).toBeAttached();
      await reader.focus();
      await reader.press("ControlOrMeta+a");
      const source = await copied(reader);
      expect(source).toBe(
        Array.from(
          { length: 4999 },
          (_, index) => `export const item${index + 1} = ${index + 1};\n`,
        ).join(""),
      );
      await reader.press("End");
      await expect(
        reader.getByText("export const item4999 = 4999;", { exact: true }),
      ).toBeInViewport();
      await page.getByRole("button", { name: "Show changed files", exact: true }).click();
      await expect(page.locator(`[data-diff-file="${ACTIVE_DIFF_FILE}"]`)).toBeFocused();
    },
  );
}
