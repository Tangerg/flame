import { expect, test } from "./test";

const instructionLines = [
  "Inspect dependencies.",
  "Run accepted checks.",
  "",
  "Report:",
  "- State concrete risks.",
  "- Preserve exact output.",
  "",
  `/Users/visual/${"retained-evidence-".repeat(8)}request.json`,
];
const instructions = instructionLines.join("\n");

for (const theme of ["light", "dark"]) {
  for (const width of [1120, 320]) {
    test(`schedule instructions retain authored lines and complete content ${theme} ${width}`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 720 });
      const query = new URLSearchParams({
        fixture: "workspace",
        state: width === 1120 ? "dock-schedules" : "full-view",
        "font-size": "18",
        theme,
      });
      if (width === 320) query.set("full-view", "package:visual:schedules");
      await page.goto(`/visual/?${query}`);
      await page.locator("html[data-visual-ready]").waitFor();
      if (width === 320)
        await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();

      const panel = page.getByRole("region", { name: "Schedules", exact: true });
      await panel.getByRole("heading", { name: "Nightly dependency audit", exact: true }).waitFor();
      await page.evaluate(async (value) => {
        const modulePath = "/visual/componentSnapshots.ts";
        const { VISUAL_SCHEDULES_PAGE } = await import(modulePath);
        VISUAL_SCHEDULES_PAGE.Rows[0].Instructions = value;
      }, instructions);
      await panel.getByRole("button", { name: "Refresh", exact: true }).click();

      const card = panel.getByRole("article").filter({
        has: page.getByRole("heading", { name: "Nightly dependency audit", exact: true }),
      });
      const body = card.getByText(instructions, { exact: true });
      await expect(body).toBeVisible();
      expect(await body.textContent()).toBe(instructions);
      const reading = await body.evaluate((node, markers) => {
        const text = node.textContent!;
        const walker = document.createTreeWalker(node, NodeFilter.SHOW_TEXT);
        const nodes: Node[] = [];
        while (walker.nextNode()) nodes.push(walker.currentNode);
        const position = (offset: number): [Node, number] => {
          for (const child of nodes) {
            const length = child.textContent!.length;
            if (offset <= length) return [child, offset];
            offset -= length;
          }
          throw new Error("Instruction text position is unavailable");
        };
        const range = document.createRange();
        const bounds = node.getBoundingClientRect();
        const lines = markers.map((marker) => {
          const start = text.indexOf(marker);
          range.setStart(...position(start));
          range.setEnd(...position(start + marker.length));
          return [...range.getClientRects()].map((rect) => ({
            top: rect.top,
            bottom: rect.bottom,
            left: rect.left,
            right: rect.right,
          }));
        });
        return {
          whiteSpace: getComputedStyle(node).whiteSpace,
          left: bounds.left,
          right: bounds.right,
          lines,
        };
      }, instructionLines.filter(Boolean));

      expect(reading.whiteSpace).toBe("pre-wrap");
      for (const [index, lines] of reading.lines.entries()) {
        expect(lines.length).toBeGreaterThan(0);
        if (index > 0)
          expect(lines[0]!.top).toBeGreaterThanOrEqual(reading.lines[index - 1]!.at(-1)!.bottom);
        for (const line of lines) {
          expect(line.left).toBeGreaterThanOrEqual(reading.left - 1);
          expect(line.right).toBeLessThanOrEqual(reading.right + 1);
        }
      }
      const ordinaryAdvance = reading.lines[1]![0]!.top - reading.lines[0]![0]!.top;
      const paragraphAdvance = reading.lines[2]![0]!.top - reading.lines[1]!.at(-1)!.top;
      expect(paragraphAdvance).toBeGreaterThan(ordinaryAdvance);
      expect(await card.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
      expect(await panel.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
      expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
    });
  }
}
