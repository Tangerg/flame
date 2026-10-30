import { expect, test, type Page } from "./test";
import { de } from "../../../plugins/providers/component/locales/de";
import { LONG_MODEL_NAME as LONG_NAME, LONG_MODEL_ID as LONG_ID } from "./runtimeSnapshots";
async function openRoles(page: Page, width: number, theme: string) {
  await page.setViewportSize({ width, height: 900 });
  await page.goto(
    `/visual/?fixture=workspace&state=settings&pane=providers&theme=${theme}&font-size=18&locale=de&long-models=1`,
  );
  await page.locator("html[data-visual-ready]").waitFor();
  const panel = page.getByRole("tabpanel");
  await expect(panel.locator("[aria-busy=true]")).toHaveCount(0);
  await panel
    .getByRole("combobox", { name: de["providers.utility.title"]!, exact: true })
    .waitFor();
  await page.evaluate(() => document.fonts.ready);
  return panel;
}

for (const width of [320, 1120]) {
  for (const theme of ["light", "dark"]) {
    test(`model roles expose complete names, identities and selected state ${width} ${theme}`, async ({
      page,
    }) => {
      const panel = await openRoles(page, width, theme);
      const trigger = panel.getByRole("combobox", {
        name: de["providers.utility.title"]!,
        exact: true,
      });
      await expect(trigger).toHaveAccessibleDescription(new RegExp(`openai / ${LONG_ID}`));
      const name = trigger.getByText(LONG_NAME, { exact: false });
      expect(await name.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
      const control = await trigger.evaluate((node) => ({
        width: node.getBoundingClientRect().width,
        trackWidth: node.parentElement!.getBoundingClientRect().width,
      }));
      expect(control.trackWidth, "the control track must not reserve unused intrinsic width").toBe(
        control.width,
      );
      await trigger.focus();
      await trigger.press("Enter");
      const menu = page.getByRole("listbox");
      const selected = menu.getByRole("option", { name: new RegExp(LONG_ID) });
      await expect(selected).toHaveAttribute("aria-selected", "true");
      await expect(menu.getByRole("option", { name: /openai \/ research-one/ })).toHaveAttribute(
        "aria-selected",
        "false",
      );
      await expect(menu.getByRole("option", { name: /anthropic \/ research-two/ })).toHaveAttribute(
        "aria-selected",
        "false",
      );
      const layout = await menu.evaluate((node) => {
        const box = node.getBoundingClientRect();
        return {
          left: box.left,
          right: box.right,
          overflow: node.scrollWidth - node.clientWidth,
          clipped: [...node.querySelectorAll("span")]
            .filter(
              (child) =>
                child.textContent?.trim() &&
                child.clientWidth > 0 &&
                child.scrollWidth > child.clientWidth,
            )
            .map((child) => child.textContent),
        };
      });
      expect(layout.left).toBeGreaterThanOrEqual(0);
      expect(layout.right).toBeLessThanOrEqual(width);
      expect(layout.overflow).toBe(0);
      expect(layout.clipped).toEqual([]);
      await page.keyboard.press("Escape");
      await expect(trigger).toBeFocused();
      expect(await panel.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
    });
  }
}

test("an embedding role keeps its custom model and selection when a default change fails", async ({
  page,
}) => {
  const panel = await openRoles(page, 320, "dark");
  const trigger = panel.getByRole("combobox", {
    name: de["providers.embedding.title"]!,
    exact: true,
  });
  await expect(trigger).toHaveAccessibleDescription(/custom-embedding.*openai/);
  await trigger.focus();
  await trigger.press("Enter");
  const menu = page.getByRole("listbox");
  const preset = menu.getByRole("option", { name: /openai-embedding-default/ });
  await expect(preset).toHaveAttribute("aria-selected", "false");
  await expect(menu.getByRole("option", { selected: true })).toHaveCount(1);
  await expect(menu.getByRole("option", { selected: true })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  await preset.focus();
  await preset.press("Enter");
  await expect(panel.getByRole("alert")).toBeVisible();
  await expect(trigger).toBeFocused();
  await expect(trigger).toHaveAccessibleDescription(/custom-embedding.*openai/);
  await trigger.press("Enter");
  await expect(page.getByRole("listbox").getByRole("option", { selected: true })).toHaveCount(1);
  await page.keyboard.press("Escape");
  await expect(trigger).toBeFocused();
});

test("typeahead and keyboard dismissal retain the Runtime selection without applying the highlighted model", async ({
  page,
}) => {
  const panel = await openRoles(page, 1120, "light");
  const trigger = panel.getByRole("combobox", {
    name: de["providers.utility.title"]!,
    exact: true,
  });
  await trigger.focus();
  await trigger.press("ArrowDown");
  const menu = page.getByRole("listbox");
  const current = menu.getByRole("option", { name: new RegExp(LONG_ID) });
  await expect(current).toHaveAttribute("aria-selected", "true");
  await page.keyboard.press("r");
  await expect(menu.getByRole("option", { name: /research-one/ })).toBeFocused();
  await page.keyboard.press("ArrowDown");
  await expect(menu.getByRole("option", { name: /research-two/ })).toBeFocused();
  await expect(current).toHaveAttribute("aria-selected", "true");
  await page.keyboard.press("Escape");
  await expect(trigger).toBeFocused();
  await expect(trigger).toHaveAccessibleDescription(new RegExp(LONG_ID));
  await expect(panel.getByRole("alert")).toHaveCount(0);
});
