import { expect, test } from "./test";

// The two ends and the middle of the slider, plus the point the floor stops binding.
const CONTRAST = [0, 12, 25, 50, 100] as const;

// Measured in 8-bit channels against the surface the row sits on. Around 4 is where a flat
// fill of this size stops being visible at all; the floor holds the low end at 8.1 and 16.2.
const MIN_HOVER = 6;
const MIN_SELECTED = 12;

// Codex and zcode both put the open row at twice the ink of the hovered one.
const MIN_RATIO = 1.7;

const READ = `() => {
  // A fresh element per token, painted through cssText: re-assigning \`style.backgroundColor\`
  // on one element and reading it back returns the colour it already had.
  const swatch = (token) => {
    const node = document.createElement("div");
    node.style.cssText = "position:fixed;left:-9999px;top:0;width:4px;height:4px;background:" + token;
    document.body.append(node);
    const css = getComputedStyle(node).backgroundColor;
    node.remove();
    const n = (css.match(/[\\d.]+(?:e[+-]?\\d+)?/g) ?? []).map(Number);
    const rgb = css.startsWith("color(") ? n.slice(0, 3).map((v) => v * 255) : n.slice(0, 3);
    return { rgb, a: n.length > 3 ? n[3] : 1 };
  };
  const base = swatch("var(--app-drawer-surface)");
  const shift = (token) => {
    const wash = swatch(token);
    return Math.max(...wash.rgb.map((c, i) => Math.abs((c - base.rgb[i]) * wash.a)));
  };
  return { hover: shift("var(--wash-hover)"), selected: shift("var(--wash-selected)") };
}`;

for (const theme of ["light", "dark"] as const) {
  test(`a row still answers the pointer at every contrast setting ${theme}`, async ({ page }) => {
    const faint: string[] = [];

    for (const contrast of CONTRAST) {
      await page.goto(`/visual/?fixture=shell&state=populated&theme=${theme}&contrast=${contrast}`);
      await page.waitForSelector("html[data-visual-ready]");
      const measured = await page.evaluate<{ hover: number; selected: number }, string>(
        (source) => new Function(`return (${source})()`)() as { hover: number; selected: number },
        READ,
      );
      if (measured.hover < MIN_HOVER) {
        faint.push(`${theme} contrast=${contrast} hover ${measured.hover.toFixed(1)}/255`);
      }
      if (measured.selected < MIN_SELECTED) {
        faint.push(`${theme} contrast=${contrast} selected ${measured.selected.toFixed(1)}/255`);
      }
      const ratio = measured.selected / measured.hover;
      if (ratio < MIN_RATIO) {
        faint.push(`${theme} contrast=${contrast} selected is only ${ratio.toFixed(2)}x hover`);
      }
    }

    expect(
      faint,
      "the contrast preference flattens SURFACES — it may not take the only signal a row has " +
        "for hovered or open down with them",
    ).toEqual([]);
  });
}
