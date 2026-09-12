import { expect, test } from "./test";

const ROUTE = "/visual/?fixture=shell&state=populated&theme=light";

const OPAQUE = "rgb(0, 0, 0)";
const CLEAR = "rgba(0, 0, 0, 0)";

function fadedEdge(mask: string): "left" | "right" | "neither" {
  const clear = mask.indexOf(CLEAR);
  const opaque = mask.indexOf(OPAQUE);
  if (clear < 0 || opaque < 0) return "neither";
  return clear < opaque ? "left" : "right";
}

test("an overflowing label keeps a soft edge, on the side the text is crossing", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1120, height: 720 });
  await page.goto(ROUTE);
  await page.locator("html[data-visual-ready]").waitFor();
  await page.waitForTimeout(400);

  const labels = page.locator(".agent-overflow-label[data-overflowing]");
  const count = await labels.count();
  expect(count, "the route has to render labels that actually overflow").toBeGreaterThan(0);

  const wrong: string[] = [];
  for (let index = 0; index < count; index += 1) {
    const label = labels.nth(index);
    const rest = await label.evaluate((node) => ({
      mask: getComputedStyle(node).maskImage,
      overflow: getComputedStyle(node).overflow,
      text: (node.textContent ?? "").trim().slice(0, 24),
    }));

    if (rest.overflow !== "hidden") {
      wrong.push(`"${rest.text}" is not clipped (overflow: ${rest.overflow})`);
      continue;
    }
    if (fadedEdge(rest.mask) !== "right") {
      wrong.push(`"${rest.text}" at rest fades ${fadedEdge(rest.mask)}, want right`);
    }

    await label.hover();
    await page.waitForTimeout(150);
    const moving = await label.evaluate((node) => getComputedStyle(node).maskImage);
    if (fadedEdge(moving) !== "left") {
      wrong.push(
        `"${rest.text}" while revealing fades ${fadedEdge(moving)}, want left` +
          (moving === "none" ? " (mask removed — the box still clips)" : ""),
      );
    }
  }

  expect(wrong, "labels whose soft edge is on the wrong side, or gone").toEqual([]);
});
