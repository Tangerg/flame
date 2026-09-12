import { test as base, expect } from "@playwright/test";

export { expect };
export type { Browser, Locator, Page } from "@playwright/test";

export const test = base.extend<{ quietConsole: void }>({
  quietConsole: [
    async ({ page }, use) => {
      const complaints: string[] = [];
      page.on("console", (message) => {
        if (message.type() !== "error" && message.type() !== "warning") return;
        complaints.push(`${message.type()}: ${message.text()}`);
      });
      page.on("pageerror", (error) => complaints.push(`pageerror: ${error.message}`));

      await page.exposeFunction("__visualWindowError", (message: string) => {
        complaints.push(`window.error: ${message}`);
      });
      await page.addInitScript(() => {
        const relay = (message: string) => {
          const send = (window as unknown as Record<string, unknown>).__visualWindowError;
          if (typeof send === "function") (send as (value: string) => void)(message);
        };
        window.addEventListener("error", (event) => relay(event.message));
        window.addEventListener("unhandledrejection", (event) =>
          relay(`unhandledrejection: ${String(event.reason)}`),
        );
      });

      await use();

      expect(complaints).toEqual([]);
    },
    { auto: true },
  ],
});
