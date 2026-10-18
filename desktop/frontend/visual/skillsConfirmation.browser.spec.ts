import { expect, test } from "./test";

for (const width of [320, 1120]) {
  for (const decision of ["approve", "reject"] as const) {
    test(`Skills ${decision} returns focus after a refused confirmation ${width}`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 720 });
      await page.goto(
        `/visual/?fixture=workspace&state=full-view&full-view=package:visual:skills&sidebar=collapsed&theme=${width === 320 ? "light" : "dark"}&font-size=18`,
      );
      await page.locator("html[data-visual-ready]").waitFor();
      const panel = page.getByRole("region", { name: "Skills", exact: true });
      await panel.getByRole("tab", { name: "Skill proposals", exact: true }).click();
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
    });
  }
}
