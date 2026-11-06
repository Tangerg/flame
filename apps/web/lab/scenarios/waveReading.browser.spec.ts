import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "./test";
import { en } from "@/lib/i18n/locales/en";
import zh from "../../../../plugins/locale-zh/package/io.github.tangerg.flame/messages.json" with { type: "json" };
import { catalog } from "../catalog";
import { previewURL, readSelection } from "../selection";

for (const environment of [
  { locale: "en", theme: "light", fontSize: 14, messages: en },
  { locale: "zh-CN", theme: "dark", fontSize: 18, messages: zh },
]) {
  test(`isolated reasoning remains readable and keyboard reachable in ${environment.locale}`, async ({
    page,
  }) => {
    const messages: Record<string, string> = environment.messages;
    const selection = readSelection(
      new URLSearchParams({
        panel: "business",
        story: "business-reasoning",
        variant: "running",
        locale: environment.locale,
        theme: environment.theme,
        "font-size": String(environment.fontSize),
      }).toString(),
      catalog,
    );
    await page.setViewportSize({ width: 320, height: 720 });
    await page.goto(previewURL(catalog.story(selection.storyId), selection, "isolated-reasoning"));
    await expect(page.locator("html")).toHaveAttribute("data-lab-ready", "true");
    const toggle = page.getByRole("button", { name: messages["reasoning.thinking"], exact: true });
    const glimpse = page.locator('[data-slot="reasoning-glimpse"]');
    await expect(glimpse).toBeVisible();
    await toggle.focus();
    await page.keyboard.press("Enter");
    const reader = page.getByRole("group", { name: messages["reasoning.text"], exact: true });
    await expect(reader).toBeVisible();
    await page.keyboard.press("Tab");
    await expect(reader).toBeFocused();
    await page.keyboard.press("Home");
    await expect.poll(() => reader.evaluate((element) => element.scrollTop)).toBe(0);
    await expect(reader).toBeInViewport({ ratio: 1 });
    const audit = await new AxeBuilder({ page })
      .withRules(["color-contrast", "label-content-name-mismatch"])
      .analyze();
    expect(audit.violations).toEqual([]);
    await page.keyboard.press("Shift+Tab");
    await expect(toggle).toBeFocused();
    await page.keyboard.press("Space");
    await expect(glimpse).toBeVisible();
    await page.keyboard.press("Enter");
    await expect(reader).toBeVisible();
    expect(await reader.evaluate((element) => element.scrollTop)).toBe(0);
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  });

  test(`live reasoning keeps a stable toggle and retained reading in ${environment.locale}`, async ({
    page,
  }, testInfo) => {
    const messages: Record<string, string> = environment.messages;
    const thinking = messages["reasoning.thinking"]!;
    const readerName = messages["reasoning.text"]!;
    const query = new URLSearchParams({
      panel: "business",
      story: "business-agent-scenarios",
      variant: "answer-opening",
      locale: environment.locale,
      theme: environment.theme,
      "font-size": String(environment.fontSize),
    });
    const selection = readSelection(query.toString(), catalog);
    await page.setViewportSize({ width: 1120, height: 900 });
    await page.goto(previewURL(catalog.story(selection.storyId), selection, "reasoning-reading"));
    await expect(page.locator("html")).toHaveAttribute("data-lab-ready", "true");
    const draft = page.getByRole("textbox");
    await draft.fill("Keep the draft while reading reasoning.");
    const toggle = page.getByRole("button", { name: thinking, exact: true });
    const glimpse = page.locator('[data-slot="reasoning-glimpse"]');
    await expect(glimpse).toBeVisible();
    await expect(toggle).toHaveText(thinking);
    const latestLine = await glimpse.textContent();
    expect(latestLine).toBeTruthy();

    const capture = async (phase: string) => {
      const audit = await new AxeBuilder({ page })
        .withRules(["color-contrast", "label-content-name-mismatch"])
        .analyze();
      await testInfo.attach(`reasoning-${phase}-audit`, {
        body: Buffer.from(JSON.stringify(audit, null, 2)),
        contentType: "application/json",
      });
      expect(audit.violations).toEqual([]);
      await testInfo.attach(`reasoning-${phase}`, {
        body: await page.screenshot({ animations: "disabled" }),
        contentType: "image/png",
      });
    };
    await capture("collapsed");

    await page.setViewportSize({ width: 320, height: 720 });
    await expect(glimpse).toBeVisible();
    await expect(toggle).toHaveText(thinking);
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
    await toggle.focus();
    await page.keyboard.press("Enter");
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    await expect(toggle).toBeFocused();
    await expect(glimpse).toHaveCount(0);
    const reader = page.getByRole("group", { name: readerName, exact: true });
    await expect(reader).toBeVisible();
    await page.keyboard.press("Tab");
    await expect(reader).toBeFocused();
    await page.keyboard.press("Home");
    await expect.poll(() => reader.evaluate((element) => element.scrollTop)).toBe(0);
    await expect(reader).toBeInViewport({ ratio: 1 });
    await capture("narrow-reading");

    await page.keyboard.press("Shift+Tab");
    await expect(toggle).toBeFocused();
    await page.keyboard.press("Space");
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expect(glimpse).toHaveText(latestLine!);
    await expect(toggle).toHaveText(thinking);
    await page.keyboard.press("Enter");
    await expect(reader).toBeVisible();
    expect(await reader.evaluate((element) => element.scrollTop)).toBe(0);
    expect(
      await reader.evaluate((element) => element.scrollHeight - element.clientHeight),
    ).toBeGreaterThan(0);
    await expect(toggle).toBeFocused();
    await expect(draft).toHaveValue("Keep the draft while reading reasoning.");
  });
}

for (const [theme, fontSize, width] of [
  ["light", 14, 1472],
  ["dark", 18, 800],
] as const) {
  test(`completed work opens directly into aligned activity rows in ${theme}`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      `/lab/fixture.html?fixture=agent&state=waves&theme=${theme}&font-size=${fontSize}`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    const summary = page.getByRole("button", { name: /3 steps/ });
    await summary.click();
    const wave = page
      .locator('[data-slot="agent-activity-disclosure"]')
      .filter({ has: summary })
      .first();
    await expect(wave.getByText("1 read · 1 search", { exact: true })).toHaveCount(1);
    await expect(wave.getByRole("button", { name: /calls/ })).toHaveCount(0);
    const thought = wave.getByRole("button", { name: "Thought", exact: true });
    const read = wave.locator('[data-tool="read"]');
    const search = wave.locator('[data-tool="grep"]');
    await expect(read).toBeVisible();
    await expect(search).toBeVisible();
    await page.evaluate(() => document.fonts.ready);
    const starts = await Promise.all([
      thought
        .locator('[data-slot="agent-activity-label"]')
        .evaluate((node) => node.getBoundingClientRect().left),
      read.getByText("Read", { exact: true }).evaluate((node) => node.getBoundingClientRect().left),
      search
        .getByText("Searched", { exact: true })
        .evaluate((node) => node.getBoundingClientRect().left),
    ]);
    expect(Math.max(...starts) - Math.min(...starts)).toBeLessThanOrEqual(1);

    await read.click();
    const result = wave.getByRole("region", { name: "Result", exact: true });
    await expect(result.locator("pre")).toHaveCount(6);
    await expect(result).toContainText("flushLocked");
    await expect(result).toBeInViewport();
    await summary.click();
    await expect(summary).toHaveAttribute("aria-expanded", "false");
    await expect(result).not.toBeInViewport();
    await result.focus();
    await expect(result).not.toBeFocused();
    await summary.click();
    await expect(read).toHaveAttribute("aria-expanded", "true");
    await expect(result.locator("pre")).toHaveCount(6);
    await expect(result).toContainText("flushLocked");
    await expect(result).toBeInViewport();
    expect(await wave.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  });
}
