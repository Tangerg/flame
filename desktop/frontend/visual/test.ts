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

      // A third channel, because two were not enough. Some errors reach `window` as an
      // `ErrorEvent` and Playwright reports them as NEITHER a console message nor a
      // `pageerror` — `ResizeObserver loop completed with undelivered notifications` is one,
      // and the product was raising it on both dock routes whenever the row was resized. It
      // showed up only in the dev server's relayed client log, which nothing asserts on, so a
      // real unhandled error sat in a suite of 738 green tests. Verified by measurement: an
      // in-page listener saw it while `page.on("console")` and `page.on("pageerror")` saw
      // nothing at all.
      //
      // Relayed through a binding rather than collected on `window`, because each navigation
      // gets a fresh `window` and a spec navigates many times; the binding outlives them.
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
