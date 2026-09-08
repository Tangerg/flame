import { test } from "@playwright/test";
test("probe", async ({ page }) => {
  await page.setViewportSize({ width: 1120, height: 720 });
  await page.goto("http://127.0.0.1:4174/visual/?fixture=agent&state=empty&theme=light");
  await page.locator("html[data-visual-ready]").waitFor();
  console.log(
    "P " +
      JSON.stringify(
        await page.evaluate(() => {
          const col = document.querySelector(".panel-scroll");
          if (!col) return ["无列"];
          return Array.from(col.children).map((n, i) => {
            const r = n.getBoundingClientRect();
            const cs = getComputedStyle(n);
            return `${i} h=${Math.round(r.height)} pt=${cs.paddingTop} pb=${cs.paddingBottom} mt=${cs.marginTop} ${n.tagName}.${(n.className || "").slice(0, 20)}`;
          });
        }),
        null,
        1,
      ),
  );
});
