import { test } from "@playwright/test";
test("probe", async ({ page }) => {
  await page.setViewportSize({ width: 1120, height: 720 });
  await page.goto("http://127.0.0.1:4174/visual/?fixture=agent&theme=light&state=tool-shells");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.getByRole("button", { name: /steps/ }).first().click();
  await page
    .locator('[data-tool="apply_patch"] button[aria-expanded]')
    .filter({ hasText: "specialisedPreviewProjections.ts" })
    .click();
  await page.waitForTimeout(400);
  console.log(
    "N " +
      JSON.stringify(
        await page.evaluate(() => {
          const n = Array.from(document.querySelectorAll("*"))[337];
          if (!n) return ["无"];
          const cs = getComputedStyle(n);
          return [
            `${n.tagName} class=${(n.className || "").slice(0, 60)}`,
            `h=${Math.round(n.getBoundingClientRect().height)} pad=${cs.padding} lh=${cs.lineHeight} fs=${cs.fontSize} display=${cs.display} gtc=${cs.gridTemplateColumns}`,
            `outer=${n.outerHTML.slice(0, 200)}`,
          ];
        }),
        null,
        1,
      ),
  );
});
