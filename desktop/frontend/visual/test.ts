import { test as base, expect } from "@playwright/test";

export { expect };
export type { Browser, Locator, Page } from "@playwright/test";

/**
 * A duplicate React key, a controlled-to-uncontrolled switch, an invalid DOM nesting or a
 * thrown effect all report themselves only to the console — the screenshot of a tree that
 * logged one is pixel-identical to the screenshot of a clean one, so the suite that renders
 * every surface in the product is exactly the suite that should be reading it.
 *
 * Pages this file cannot see are the ones a spec opens itself through `browser.newPage()`.
 */
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
