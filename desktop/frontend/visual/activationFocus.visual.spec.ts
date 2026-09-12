import { test as base, expect } from "@playwright/test";

const EXPECTED_NOISE = /RpcConnectionError|Failed to fetch|net::ERR_CONNECTION_REFUSED/;

const ROUTES = [
  "fixture=agent&state=narrative",
  "fixture=workspace&state=dock-agent-memory",
  "fixture=shell&state=populated",
] as const;

const PER_ROUTE = 45;

base("activating a control never leaves focus on nothing", async ({ page }) => {
  base.setTimeout(ROUTES.length * 90_000 + 30_000);
  const unexpected: string[] = [];
  page.on("console", (message) => {
    if (message.type() !== "error" && message.type() !== "warning") return;
    if (EXPECTED_NOISE.test(message.text())) return;
    unexpected.push(`${message.type()}: ${message.text()}`);
  });

  await page.setViewportSize({ width: 1472, height: 900 });
  const orphaned: string[] = [];
  const unfocusable: string[] = [];
  let activated = 0;

  for (const route of ROUTES) {
    await page.goto(`/visual/?${route}&theme=light`);
    await page.locator("html[data-visual-ready]").waitFor();
    await page.waitForTimeout(300);

    const total = await page.locator('[data-slot="button"]:not([data-fixture-chrome] *)').count();
    for (let index = 0; index < Math.min(total, PER_ROUTE); index += 1) {
      const control = page.locator('[data-slot="button"]:not([data-fixture-chrome] *)').nth(index);
      if ((await control.count()) === 0) continue;
      const name = await control.evaluate(
        (node) =>
          `${(node.textContent ?? "").trim().slice(0, 18)}|${node.getAttribute("aria-label") ?? ""}`,
      );

      const landed = await control.evaluate((node) => {
        (node as HTMLElement).focus();
        return document.activeElement === node;
      });
      if (!landed) {
        unfocusable.push(`${route} #${index} "${name}"`);
        continue;
      }

      await page.keyboard.press("Enter");
      await page.waitForTimeout(200);
      activated += 1;
      const landedOn = await page.evaluate(() => {
        const active = document.activeElement as HTMLElement | null;
        return !active || active === document.body ? "BODY" : "somewhere";
      });
      if (landedOn === "BODY") orphaned.push(`${route} #${index} "${name}"`);

      await page.keyboard.press("Escape");
      await page.waitForTimeout(60);
    }
  }

  expect(activated, "the sweep has to actually activate controls").toBeGreaterThan(30);
  expect(orphaned, "controls that left focus on `<body>`").toEqual([]);
  expect(unexpected, "console complaints other than the fixture's missing runtime").toEqual([]);
  if (unfocusable.length > 0) console.log(`not focusable (by design): ${unfocusable.length}`);
});
