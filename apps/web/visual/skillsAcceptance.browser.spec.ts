import { expect, test } from "./test";
import { VISUAL_SKILL_PROPOSALS } from "./skillSnapshots";
for (const [decision, width, theme] of [
  ["approve", 1120, "dark"],
  ["reject", 320, "light"],
] as const) {
  test(`Skills ${decision} keeps a keyboard destination when accepted proposals leave the owner list`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 720 });
    await page.goto(
      `/visual/?fixture=workspace&state=full-view&full-view=package:visual:skills&sidebar=collapsed&theme=${theme}&font-size=18&skills-commands=accept`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    const panel = page.getByRole("region", { name: "Skills", exact: true });
    await panel.getByRole("radio", { name: "Skill proposals", exact: true }).click();
    const search = panel.getByRole("searchbox", { name: "Search this catalog", exact: true });
    await expect(panel).toContainText("2 pending proposals");

    for (const [index, proposal] of VISUAL_SKILL_PROPOSALS.data.entries()) {
      const actions = panel.getByRole("group", {
        name: `Proposal actions for ${proposal.name}`,
        exact: true,
      });
      await actions
        .getByRole("button", { name: decision === "approve" ? "Approve" : "Reject", exact: true })
        .click();
      const confirmation = panel.getByRole("region", {
        name: "Confirm proposal decision",
        exact: true,
      });
      await expect(confirmation.getByRole("button", { name: "Cancel", exact: true })).toBeFocused();
      const submit = confirmation.getByRole("button", { name: `Confirm ${decision}`, exact: true });
      await submit.focus();
      await page.keyboard.press("Enter");
      await expect(confirmation).toHaveCount(0);
      await expect(
        panel.getByRole("status").filter({
          hasText: `${decision === "approve" ? "Approve" : "Reject"} ${proposal.name} completed.`,
        }),
      ).toBeVisible();
      await expect(actions).toHaveCount(0);
      await expect(panel).toContainText(`${1 - index} pending proposals`);
      await expect.soft(search).toBeFocused();
    }
    await expect(panel.getByText("No Skills in this catalog.", { exact: true })).toBeVisible();
  });
}
