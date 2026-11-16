import { expect, test } from "./test";
import { en } from "@/lib/i18n/locales/en";
import { expectLabReady } from "../readiness";

for (const browserName of ["chromium", "webkit"] as const) {
  test.extend({ browserName })(
    `${browserName} persistence Retry returns focus only when its presentation really exits`,
    async ({ page }) => {
      await page.goto("/lab/fixture.html?fixture=agent&state=idle&theme=light");
      await expectLabReady(page);
      await page.evaluate(async () => {
        const path = "/lab/scenarios/fixtureRuntime.tsx";
        const { visualClient } = await import(path);
        visualClient.composer.start();
      });
      await expect
        .poll(() =>
          page.evaluate(async () => {
            const path = "/lab/scenarios/fixtureRuntime.tsx";
            const { visualClient } = await import(path);
            return visualClient.composer.store.getState().persistenceState.status;
          }),
        )
        .toBe("saved");
      await page.evaluate(() => {
        const transaction = IDBDatabase.prototype.transaction;
        IDBDatabase.prototype.transaction = function (...args: Parameters<typeof transaction>) {
          if (this.name === "flame.composer" && args[1] === "readwrite")
            throw new DOMException("fixture quota refusal", "QuotaExceededError");
          return transaction.apply(this, args);
        };
        document.addEventListener(
          "restore-fixture-draft-storage",
          () => {
            IDBDatabase.prototype.transaction = transaction;
          },
          { once: true },
        );
      });
      const input = page.getByRole("textbox", { name: "Message composer", exact: true });
      await input.fill("Keep the exact recovery draft");
      await input.evaluate((node: HTMLTextAreaElement) => node.setSelectionRange(5, 5));
      const retry = page.getByRole("button", { name: "Retry", exact: true });
      await retry.focus();
      await retry.press("Enter");
      await expect(page.getByRole("alert")).toContainText(en["composer.persistence.saveFailed"]!);
      await expect(retry).toBeFocused();
      await page.evaluate(async () => {
        const path = "/lab/scenarios/fixtureRuntime.tsx";
        const { visualClient } = await import(path);
        visualClient.theme.store.setState({ fontSize: 16 });
      });
      await expect(retry).toBeFocused();
      await page.evaluate(() => document.dispatchEvent(new Event("restore-fixture-draft-storage")));
      await retry.press("Enter");
      await expect(retry).toHaveCount(0);
      await expect(input).toBeFocused();
      await expect(input).toHaveValue("Keep the exact recovery draft");
      expect(
        await input.evaluate((node: HTMLTextAreaElement) => [
          node.selectionStart,
          node.selectionEnd,
        ]),
      ).toEqual([5, 5]);
    },
  );
}
