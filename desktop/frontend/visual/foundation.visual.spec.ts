import { expect, test } from "./test";

const THEMES = ["light", "dark"] as const;
const SIDEBAR_STATES = ["expanded", "collapsed"] as const;

for (const theme of THEMES) {
  for (const sidebar of SIDEBAR_STATES) {
    test(`foundation ${theme} ${sidebar}`, async ({ page }) => {
      await page.goto(`/visual/?theme=${theme}&sidebar=${sidebar}`);
      await page.locator("html[data-visual-ready]").waitFor();

      const root = page.locator(":root");
      const contentCard = page.getByTestId("content-card");
      const composer = page.getByTestId("composer");

      await expect(root).toHaveClass(new RegExp(`theme-${theme}`));
      await expect(page.getByTestId("sidebar-state")).toHaveText(sidebar);
      // Square in BOTH states, which is the shell's answer rather than an accident of one.
      //
      // This used to resolve `--app-content-card-radius` on a throwaway element and compare
      // the expanded state against it. That name has never been defined — not here, not in
      // any commit, which `git log -S` says plainly — so it resolved to `0px` and the
      // assertion compared `0px` against `0px` in both branches, under a comment describing a
      // corner the visual style declares. A probe reading a name nothing owns cannot fail.
      await expect(contentCard).toHaveCSS("border-top-left-radius", "0px");
      // Its seam is a SHADOW and not a border — the "one edge, never two" rule the whole
      // boundary model rests on, which nothing else asserts for this surface.
      await expect(contentCard).toHaveCSS("border-top-width", "0px");
      await expect(contentCard).not.toHaveCSS("box-shadow", "none");
      // No border: the composer's edge is the ring in its own box-shadow, asserted
      // in full where the rest of its material is. What belongs here is only that
      // the foundation fixture builds the real surface and not a stand-in.
      await expect(composer).toHaveCSS("border-top-width", "0px");
      await expect(composer).not.toHaveCSS("box-shadow", "none");
      await expect(page).toHaveScreenshot(`foundation-${theme}-${sidebar}.png`);
    });
  }
}
