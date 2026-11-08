import { expect, test, type Locator } from "./test";
import { expectLabReady } from "../readiness";

// Focus and in-viewport checks cannot see sticky controls painted over the name.
async function expectProposalNameReadable(panel: Locator, attachment: string) {
  const geometry = await panel.getByText("risk-first-review", { exact: true }).evaluate((name) => {
    const panel = name.closest('[data-slot="panel"]');
    if (!panel) throw new Error("skills panel is not rendered");
    const range = document.createRange();
    range.selectNodeContents(name);
    const rects = [...range.getClientRects()]
      .filter((rect) => rect.width > 0 && rect.height > 0)
      .map(({ top, right, bottom, left }) => ({ top, right, bottom, left }));
    const controls = panel.querySelector('[data-slot="panel-controls"]');
    if (!controls) throw new Error("skills panel controls are not rendered");
    const frame = {
      top: Math.max(0, controls.getBoundingClientRect().bottom),
      right: window.innerWidth,
      bottom: window.innerHeight,
      left: 0,
    };
    for (let parent: Element | null = name; parent; parent = parent.parentElement) {
      const style = getComputedStyle(parent);
      const bounds = parent.getBoundingClientRect();
      if (style.overflowY !== "visible") {
        frame.top = Math.max(frame.top, bounds.top + parent.clientTop);
        frame.bottom = Math.min(frame.bottom, bounds.top + parent.clientTop + parent.clientHeight);
      }
      if (style.overflowX !== "visible") {
        frame.left = Math.max(frame.left, bounds.left + parent.clientLeft);
        frame.right = Math.min(frame.right, bounds.left + parent.clientLeft + parent.clientWidth);
      }
    }
    return { frame, rects };
  });
  await test.info().attach(attachment, {
    body: JSON.stringify(geometry, null, 2),
    contentType: "application/json",
  });
  expect(geometry.rects.length).toBeGreaterThan(0);
  for (const rect of geometry.rects) {
    expect(rect.top).toBeGreaterThanOrEqual(geometry.frame.top - 1);
    expect(rect.bottom).toBeLessThanOrEqual(geometry.frame.bottom + 1);
    expect(rect.left).toBeGreaterThanOrEqual(geometry.frame.left - 1);
    expect(rect.right).toBeLessThanOrEqual(geometry.frame.right + 1);
  }
}

for (const width of [320, 1120]) {
  for (const decision of ["approve", "reject"] as const) {
    test(`Skills ${decision} returns focus after a refused confirmation ${width}`, async ({
      page,
    }, testInfo) => {
      await page.setViewportSize({ width, height: 720 });
      await page.goto(
        `/lab/fixture.html?fixture=workspace&state=full-view&full-view=package:visual:skills&sidebar=collapsed&theme=${width === 320 ? "light" : "dark"}&font-size=18`,
      );
      await expectLabReady(page);
      const panel = page
        .getByRole("region", { name: "Skills", exact: true })
        .and(page.locator('[data-slot="panel"]'));
      await panel.getByRole("radio", { name: "Skill proposals", exact: true }).click();
      const opener = panel
        .getByRole("button", {
          name: decision === "approve" ? "Approve" : "Reject",
          exact: true,
        })
        .first();
      await opener.click();
      const confirmation = panel.getByRole("region", {
        name: "Confirm proposal decision",
        exact: true,
      });
      await expect(confirmation.getByRole("button", { name: "Cancel", exact: true })).toBeFocused();
      const submit = confirmation.getByRole("button", {
        name: `Confirm ${decision}`,
        exact: true,
      });
      await submit.focus();
      await page.keyboard.press("Enter");
      await expect(confirmation).toHaveCount(0);
      await expect(
        panel.getByRole("status").filter({
          hasText: `Visual Runtime has no response for "skills.proposals.${decision}"`,
        }),
      ).toBeVisible();
      await expect(opener).toBeEnabled();
      await expect(opener).toBeFocused();
      await expect(panel).toContainText("2 pending proposals");
      await testInfo.attach(`skills-${decision}-refused-${width}`, {
        body: await page.screenshot({ animations: "disabled" }),
        contentType: "image/png",
      });
      await expectProposalNameReadable(panel, `skills-${decision}-refused-${width}-name-geometry`);

      // The keyboard reopens the focused opener without Playwright scrolling it.
      await page.keyboard.press("Enter");
      await expect(confirmation.getByRole("button", { name: "Cancel", exact: true })).toBeFocused();
      await page.keyboard.press("Escape");
      await expect(confirmation).toHaveCount(0);
      await expect(opener).toBeFocused();
      await expect(panel).toContainText("2 pending proposals");
      await testInfo.attach(`skills-${decision}-cancelled-${width}`, {
        body: await page.screenshot({ animations: "disabled" }),
        contentType: "image/png",
      });
      await expectProposalNameReadable(
        panel,
        `skills-${decision}-cancelled-${width}-name-geometry`,
      );
    });
  }
}
