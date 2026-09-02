import { test as base, expect } from "@playwright/test";

export { expect };
export type { Browser, Locator, Page } from "@playwright/test";

/** Every spec's `test`, extended to fail on a console complaint from its `page`. Pages a spec
 *  opens itself through `browser.newPage()` are not covered. */
export const test = base.extend<{ quietConsole: void }>({
  quietConsole: [
    async ({ page }, use) => {
      const complaints: string[] = [];
      page.on("console", (message) => {
        if (message.type() !== "error" && message.type() !== "warning") return;
        complaints.push(`${message.type()}: ${message.text()}`);
      });
      page.on("pageerror", (error) => complaints.push(`pageerror: ${error.message}`));

      await use();

      expect(complaints).toEqual([]);
    },
    { auto: true },
  ],
});
