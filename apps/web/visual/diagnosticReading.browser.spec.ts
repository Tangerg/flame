import { expect, test } from "./test";
test("a log has a concise identity and preserves its complete multiline reading body", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1120, height: 1000 });
  await page.goto("/visual/?fixture=workspace&state=dock-diagnostics&theme=dark&font-size=18");
  await page.locator("html[data-visual-ready]").waitFor();
  const headline = "Workspace preview could not be refreshed.";
  const body = `\n${headline}\n\nError: the file changed after the accepted request.\n    at refresh (/Users/visual/scope/apps/web/src/features/workspace/ui/files/FileWorkspace.tsx:89:7)\nThe last accepted preview remains available.`;
  await page.evaluate(async (body) => {
    const modulePath = "/src/lib/observability/stores.ts";
    const { useTelemetryStore } = await import(modulePath);
    useTelemetryStore.getState().ingestLogs([
      {
        id: "reading-log",
        timeMs: 1785484801000,
        severity: "ERROR",
        body,
        attrs: { source: "workspace.preview" },
      },
    ]);
  }, body);
  const panel = page.getByRole("region", { name: "Client diagnostics", exact: true });
  await panel.getByRole("button", { name: "Refresh", exact: true }).click();
  await panel.getByRole("radio", { name: /^Logs ·/ }).click();
  const observations = panel.getByRole("region", { name: "Local observations", exact: true });
  const summary = observations.getByRole("button", { name: new RegExp(headline) });
  await expect(summary).toContainText(headline);
  await expect(summary).not.toContainText("at refresh");
  await expect(summary).not.toContainText("The last accepted preview");
  await summary.click();
  const paragraph = observations.locator("p").filter({ hasText: "at refresh" });
  await expect(paragraph).toBeVisible();
  expect(await paragraph.textContent()).toBe(body);
  const rendered = await paragraph.evaluate((node) => {
    const selection = window.getSelection()!;
    const range = document.createRange();
    range.selectNodeContents(node);
    selection.removeAllRanges();
    selection.addRange(range);
    const text = selection.toString();
    selection.removeAllRanges();
    return { text, overflow: node.scrollWidth - node.clientWidth };
  });
  expect(rendered.text).toBe(body);
  expect(rendered.overflow).toBe(0);
  await observations.getByRole("button", { name: "Raw observation", exact: true }).click();
  await expect(
    observations.getByRole("region", { name: "Local observation details" }),
  ).toContainText(JSON.stringify(body));
  expect(await panel.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
});
