import { de as mcpDe } from "../../../plugins/mcp/component/locales/de";
import { expect, test } from "./test";
const SERVER_NAME = "workspace-tools-with-missing-configuration-".repeat(10);

const INPUT = JSON.stringify({ mcpServers: { [SERVER_NAME]: {} } });

const FAILURE = mcpDe["mcp.import.missingTransport"]!.replace("{{server}}", SERVER_NAME);

for (const [width, height] of [
  [1120, 720],
  [560, 360],
  [320, 640],
] as const) {
  for (const theme of ["light", "dark"]) {
    test(`JSON import reveals each failed attempt without losing its draft or focus ${width} ${theme}`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height });
      await page.goto(
        `/visual/?fixture=workspace&state=settings&pane=mcp-servers&theme=${theme}&font-size=18&locale=de`,
      );
      await page.locator("html[data-visual-ready]").waitFor();
      const trigger = page.getByRole("button", { name: "Aus JSON importieren", exact: true });
      await trigger.click();
      const dialog = page.getByRole("dialog", { name: "Aus JSON importieren", exact: true });
      const field = dialog.getByRole("textbox", { name: "JSON-Konfiguration", exact: true });
      const submit = dialog.getByRole("button", { name: "Importieren", exact: true });
      const alert = dialog.getByRole("alert");
      await expect(field).toBeFocused();
      const example = await field.getAttribute("placeholder");
      expect(example).toMatch(/\n {2}"mcpServers": \{\n {4}"my-server": \{/);
      expect(JSON.parse(example!)).toEqual({
        mcpServers: {
          "my-server": { type: "streamableHttp", url: "https://example.com/mcp" },
        },
      });
      await field.fill(INPUT);
      for (let attempt = 0; attempt < 2; attempt++) {
        await submit.click();
        await expect(alert).toHaveText(FAILURE);
        await expect(field).toHaveValue(INPUT);
        await expect(field).toHaveAttribute("aria-invalid", "true");
        await expect(field).toHaveAccessibleDescription(FAILURE);
        await expect(submit).toBeFocused();
        await expect
          .poll(() =>
            alert.evaluate((node) => {
              const body = node.closest('[data-slot="dialog-body"]')!;
              const walker = document.createTreeWalker(node, NodeFilter.SHOW_TEXT);
              let text = walker.nextNode();
              while (text && !text.textContent?.trim()) text = walker.nextNode();
              if (!text) throw new Error("The import failure has no text");
              const range = document.createRange();
              range.setStart(text, 0);
              range.setEnd(text, 1);
              const first = range.getBoundingClientRect();
              const view = body.getBoundingClientRect();
              return first.top >= view.top && first.bottom <= view.bottom;
            }),
          )
          .toBe(true);
        await dialog.locator('[data-slot="dialog-body"]').evaluate((node) => {
          node.scrollTop = 0;
        });
      }
      expect(await dialog.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
      await page.keyboard.press("Escape");
      await expect(dialog).toHaveCount(0);
      await expect(trigger).toBeFocused();
    });
  }
}
