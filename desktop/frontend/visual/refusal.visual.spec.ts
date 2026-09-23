import { expect, test } from "./test";

const ROUTES = [
  "fixture=shell&state=populated",
  "fixture=workspace&state=settings",
  "fixture=workspace&state=dock-light",
  "fixture=agent&state=narrative",
];

const SETTLE_MS = 400;

const PROBE = "[data-refusal-probe]";

test("a control that refuses a click says so, however it was refused", async ({ page }) => {
  const mismatched: string[] = [];
  let compared = 0;

  for (const route of ROUTES) {
    await page.goto(`/visual/?${route}&theme=light`);
    await page.waitForSelector("html[data-visual-ready]");
    await page.waitForTimeout(200);

    await page
      .locator('button[data-control="button"]:not(:disabled):not([aria-disabled="true"])')
      .evaluateAll((nodes) => {
        nodes.forEach((node, index) => node.setAttribute("data-refusal-probe", String(index)));
      });

    const wear = async (mode: "disabled" | "pending") => {
      await page.locator(PROBE).evaluateAll((nodes, mode) => {
        for (const node of nodes) {
          const button = node as HTMLButtonElement;
          button.disabled = mode === "disabled";
          if (mode === "pending") button.setAttribute("aria-disabled", "true");
          else button.removeAttribute("aria-disabled");
        }
      }, mode);
      await page.waitForTimeout(SETTLE_MS);
      return page.locator(PROBE).evaluateAll((nodes) => {
        const out: Record<string, { label: string; look: string; refused: boolean }> = {};
        for (const node of nodes) {
          const style = getComputedStyle(node);
          out[node.getAttribute("data-refusal-probe") ?? "?"] = {
            label: (node.getAttribute("aria-label") ?? node.textContent ?? "")
              .trim()
              .replace(/\s+/g, " ")
              .slice(0, 28),
            look: `opacity=${style.opacity} cursor=${style.cursor} fill=${style.backgroundColor} ink=${style.color}`,
            refused:
              (node as HTMLButtonElement).disabled || node.getAttribute("aria-disabled") === "true",
          };
        }
        return out;
      });
    };

    const unavailable = await wear("disabled");
    const inFlight = await wear("pending");

    for (const [probe, entry] of Object.entries(unavailable)) {
      const flight = inFlight[probe];
      if (!flight || !entry.refused || !flight.refused) continue;
      compared += 1;
      if (entry.look === flight.look) continue;
      mismatched.push(
        `${route} "${entry.label}"\n    disabled: ${entry.look}\n    pending:  ${flight.look}`,
      );
    }
  }

  expect(compared, "the sweep has to reach real controls").toBeGreaterThan(40);
  expect(
    [...new Set(mismatched)],
    "`pending` is `disabled` that keeps its focus — it may not also keep its live look",
  ).toEqual([]);
});
