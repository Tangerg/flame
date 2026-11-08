import { expect, test } from "./test";
import { VISUAL_NOW } from "./agentFixtureFacts";
import { expectLabReady } from "../readiness";

for (const theme of ["light", "dark"]) {
  test(`Diff accepts reads on the fixture's advancing calendar in ${theme}`, async ({ page }) => {
    await page.goto(
      `/lab/fixture.html?fixture=workspace&state=dock-review&theme=${theme}&font-size=${theme === "light" ? 14 : 18}`,
    );
    await expectLabReady(page);
    const view = page
      .locator('[data-dock-view-id="package:visual:diff"]')
      .filter({ has: page.locator('[data-slot="workspace-view-body"]') });
    const controls = view.locator('[data-slot="panel-controls"]');
    const time = controls.locator("time");
    await expect(time).toHaveCount(1);
    const accepted = (await time.getAttribute("datetime"))!;
    const clock = await page.evaluate(() => {
      const before = Date.now();
      const constructed = new Date().getTime();
      const called = Date.parse(Date());
      const after = Date.now();
      return { before, constructed, called, after, elapsed: performance.now() };
    });
    expect(clock.before).toBeGreaterThanOrEqual(VISUAL_NOW);
    expect(clock.after - VISUAL_NOW).toBeLessThanOrEqual(clock.elapsed);
    expect(clock.constructed).toBeGreaterThanOrEqual(clock.before);
    expect(clock.constructed).toBeLessThanOrEqual(clock.after);
    expect(clock.called).toBeGreaterThanOrEqual(Math.floor(clock.before / 1000) * 1000);
    expect(clock.called).toBeLessThanOrEqual(Math.floor(clock.after / 1000) * 1000);
    expect(Date.parse(accepted)).toBeGreaterThanOrEqual(VISUAL_NOW);
    expect(Date.parse(accepted)).toBeLessThanOrEqual(clock.after);

    const composer = page.getByRole("textbox", { name: "Message composer", exact: true });
    const draft = "Keep the fixture calendar reading draft.";
    await composer.fill(draft);
    await expect.poll(() => page.evaluate(() => Date.now())).toBeGreaterThan(Date.parse(accepted));
    await controls.getByRole("button", { name: "Refresh", exact: true }).click();
    await expect(time).not.toHaveAttribute("datetime", accepted);
    const refreshed = (await time.getAttribute("datetime"))!;
    expect(Date.parse(refreshed)).toBeGreaterThan(Date.parse(accepted));
    expect(Date.parse(refreshed)).toBeLessThanOrEqual(await page.evaluate(() => Date.now()));
    await expect(composer).toHaveValue(draft);
    await test.info().attach(`fixture-calendar-${theme}`, {
      body: await page.screenshot({ animations: "disabled" }),
      contentType: "image/png",
    });
  });
}
