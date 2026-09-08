import { test } from "./test";
const VISUAL_URL = "http://127.0.0.1:4174/visual/";
test("tray box breakdown", async ({ page }) => {
  await page.goto(`${VISUAL_URL}?fixture=agent&theme=light&state=empty`);
  await page.locator("html[data-visual-ready]").waitFor();
  const r = await page.evaluate(() => {
    const t = document.querySelector('[data-slot="composer-top-tray-surface"]') as HTMLElement;
    const cs = getComputedStyle(t);
    const inner = t.firstElementChild as HTMLElement | null;
    return {
      height: t.getBoundingClientRect().height,
      paddingTop: cs.paddingTop,
      paddingBottom: cs.paddingBottom,
      borderTop: cs.borderTopWidth,
      borderBottom: cs.borderBottomWidth,
      marginBottom: cs.marginBottom,
      marginTop: cs.marginTop,
      innerHeight: inner ? inner.getBoundingClientRect().height : null,
      innerSlot: inner?.getAttribute("data-slot"),
      innerPadding: inner ? getComputedStyle(inner).padding : null,
    };
  });
  console.log("TRAY " + JSON.stringify(r));
});
