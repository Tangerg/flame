import { expect, test, type Page } from "./test";
import { expectLabReady } from "../readiness";
import type { NotificationPresentation } from "@/features/notifications/public/notifications";

async function show(page: Page, message: Omit<NotificationPresentation, "action">) {
  await page.evaluate(async (message) => {
    const path = "/lab/scenarios/fixtureRuntime.tsx";
    const { visualClient } = (await import(path)) as typeof import("./fixtureRuntime");
    visualClient.messages.show(message);
  }, message);
}

for (const browserName of ["chromium", "webkit"] as const) {
  const engineTest = test.extend({ browserName });
  engineTest.describe(`window notifications in ${browserName}`, () => {
    engineTest.beforeEach(async ({ page }) => {
      await page.goto("/lab/fixture.html?fixture=workspace&state=dock-review&locale=zh");
      await expectLabReady(page);
    });

    engineTest(
      "keeps six complete messages in the reading band and preserves active reading",
      async ({ page }) => {
        for (let index = 0; index < 6; index++) {
          await show(page, {
            level: "error",
            message: `Failure ${index}`,
            description: "Complete recovery details remain readable. ".repeat(15),
            lifetime: "persistent",
          });
        }
        const list = page.getByRole("list", { name: "通知", exact: true });
        const messages = list.getByRole("dialog");
        await expect(messages).toHaveCount(6);
        await expect(list.getByRole("button", { name: "忽略", exact: true })).toHaveCount(6);
        const dimensions = await list.evaluate((node) => {
          const bounds = node.getBoundingClientRect();
          return {
            top: bounds.top,
            bottom: bounds.bottom,
            scroll: node.scrollHeight,
            height: node.clientHeight,
          };
        });
        expect(dimensions.top).toBeGreaterThanOrEqual(0);
        expect(dimensions.bottom).toBeLessThanOrEqual(720);
        expect(dimensions.scroll).toBeGreaterThan(dimensions.height);
        await list.focus();
        await list.evaluate((node) => {
          node.scrollTop = 100;
        });
        await show(page, {
          level: "info",
          message: "Arrived while reading",
          lifetime: "persistent",
        });
        await expect(list).toBeFocused();
        expect(await list.evaluate((node) => node.scrollTop)).toBeGreaterThan(0);
        await list.press("End");
        await expect(messages.last()).toContainText("Complete recovery details remain readable.");
        await messages.last().getByRole("button", { name: "忽略", exact: true }).click();
        await expect(messages).toHaveCount(6);
      },
    );

    engineTest(
      "supports F6, native Tab order, explicit actions and focus pause",
      async ({ page }) => {
        await page.clock.install();
        await page.evaluate(async () => {
          const path = "/lab/scenarios/fixtureRuntime.tsx";
          const { visualClient } = (await import(path)) as typeof import("./fixtureRuntime");
          visualClient.messages.show({
            level: "info",
            message: "Actionable result",
            action: {
              label: "Open result",
              run: () => {
                document.body.dataset.notificationAction = "opened";
              },
            },
          });
        });
        const message = page.getByRole("dialog", { name: "Actionable result" });
        await expect(message).toBeVisible();
        await page.keyboard.press("F6");
        await expect(page.getByRole("region", { name: "通知", exact: true })).toBeFocused();
        await page.keyboard.press("Tab");
        await expect(message).toBeFocused();
        await page.keyboard.press("Tab");
        await expect(message.getByRole("button", { name: "Open result" })).toBeFocused();
        await page.keyboard.press("Tab");
        await expect(message.getByRole("button", { name: "忽略" })).toBeFocused();
        await page.clock.runFor(4500);
        await expect(message).toBeVisible();
        await page.keyboard.press("Shift+Tab");
        await page.keyboard.press("Enter");
        await expect(page.locator("body")).toHaveAttribute("data-notification-action", "opened");
        await page.clock.runFor(200);
        await expect(message).toHaveCount(0);
      },
    );

    engineTest(
      "pauses the ordinary lifetime during pointer reading and window blur",
      async ({ page }) => {
        await page.clock.install();
        await show(page, { level: "info", message: "Read before expiry" });
        const message = page.getByRole("dialog", { name: "Read before expiry" });
        await message.hover();
        await page.clock.runFor(4500);
        await expect(message).toBeVisible();
        await page.mouse.move(0, 0);
        await page.evaluate(() => window.dispatchEvent(new Event("blur")));
        await page.clock.runFor(4500);
        await expect(message).toBeVisible();
        await page.evaluate(() => window.dispatchEvent(new Event("focus")));
        await page.clock.runFor(4500);
        await expect(message).toHaveCount(0);
      },
    );
  });
}
