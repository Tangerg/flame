import { expect, test, type Page } from "./test";
import { de } from "@/lib/i18n/locales/de";

const LONG_NAME = "Research model for detailed workspace analysis with extended context";
const LONG_ID = "workspace-analysis-2026-10-extended-context";

async function openRoles(page: Page, width: number, theme: string) {
  await page.setViewportSize({ width, height: 900 });
  await page.goto(
    `/visual/?fixture=workspace&state=settings&pane=providers&theme=${theme}&font-size=18&locale=de`,
  );
  await page.locator("html[data-visual-ready]").waitFor();
  const panel = page.getByRole("tabpanel");
  await expect(panel.locator("[aria-busy=true]")).toHaveCount(0);
  await panel.getByRole("button", { name: de["providers.utility.title"]!, exact: true }).waitFor();
  await page.evaluate(
    async ({ label, model }) => {
      const queryModule = "/src/lib/queryClient.ts";
      const modelsModule = "/src/plugins/builtin/providers/application/selectableModel.ts";
      const providersModule = "/src/plugins/builtin/providers/application/providerModels.ts";
      const snapshotModule = "/visual/runtimeSnapshots.ts";
      const { queryClient }: typeof import("../src/lib/queryClient") = await import(queryModule);
      const {
        SelectableModel,
      }: typeof import("../src/plugins/builtin/providers/application/selectableModel") =
        await import(modelsModule);
      const {
        ProviderConfiguration,
      }: typeof import("../src/plugins/builtin/providers/application/providerModels") =
        await import(providersModule);
      const { VISUAL_PROVIDERS }: typeof import("./runtimeSnapshots") = await import(
        snapshotModule
      );
      queryClient.setQueryData(
        ["providers"],
        VISUAL_PROVIDERS.map((provider) =>
          ProviderConfiguration.restore({
            ...provider,
            configured: true,
            embeddingCapable: true,
            defaultEmbeddingModel: `${provider.id}-embedding-default`,
          }),
        ),
      );
      queryClient.setQueryData(
        ["models"],
        [
          new SelectableModel({ provider: "openai", id: model, label }),
          new SelectableModel({ provider: "openai", id: "research-one", label: "Research model" }),
          new SelectableModel({
            provider: "anthropic",
            id: "research-two",
            label: "Research model",
          }),
        ],
      );
      queryClient.setQueryData(["utility-role"], { provider: "openai", model });
      queryClient.setQueryData(["embedding-role"], {
        provider: "openai",
        model: "custom-embedding",
      });
    },
    { label: LONG_NAME, model: LONG_ID },
  );
  await page.evaluate(() => document.fonts.ready);
  return panel;
}

for (const width of [320, 1120]) {
  for (const theme of ["light", "dark"]) {
    test(`model roles expose complete names, identities and selected state ${width} ${theme}`, async ({
      page,
    }) => {
      const panel = await openRoles(page, width, theme);
      const trigger = panel.getByRole("button", {
        name: de["providers.utility.title"]!,
        exact: true,
      });
      await expect(trigger).toHaveAccessibleDescription(new RegExp(`openai / ${LONG_ID}`));
      const name = trigger.getByText(LONG_NAME, { exact: true });
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
      const menu = page.getByRole("menu");
      const selected = menu.getByRole("menuitemradio", { name: new RegExp(LONG_ID) });
      await expect(selected).toHaveAttribute("aria-checked", "true");
      await expect(
        menu.getByRole("menuitemradio", { name: /openai \/ research-one/ }),
      ).toHaveAttribute("aria-checked", "false");
      await expect(
        menu.getByRole("menuitemradio", { name: /anthropic \/ research-two/ }),
      ).toHaveAttribute("aria-checked", "false");
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
  const trigger = panel.getByRole("button", {
    name: de["providers.embedding.title"]!,
    exact: true,
  });
  await expect(trigger).toHaveAccessibleDescription(/custom-embedding.*openai/);
  await trigger.focus();
  await trigger.press("Enter");
  const menu = page.getByRole("menu");
  const preset = menu.getByRole("menuitemradio", { name: /openai-embedding-default/ });
  await expect(preset).toHaveAttribute("aria-checked", "false");
  await expect(menu.getByRole("menuitemradio", { checked: true })).toHaveCount(0);
  await preset.focus();
  await preset.press("Enter");
  await expect(panel.getByRole("alert")).toBeVisible();
  await expect(trigger).toBeFocused();
  await expect(trigger).toHaveAccessibleDescription(/custom-embedding.*openai/);
  await trigger.press("Enter");
  await expect(page.getByRole("menu").getByRole("menuitemradio", { checked: true })).toHaveCount(0);
  await page.keyboard.press("Escape");
  await expect(trigger).toBeFocused();
});

test("typeahead and keyboard dismissal retain the Runtime selection without applying the highlighted model", async ({
  page,
}) => {
  const panel = await openRoles(page, 1120, "light");
  const trigger = panel.getByRole("button", { name: de["providers.utility.title"]!, exact: true });
  await trigger.focus();
  await trigger.press("ArrowDown");
  const menu = page.getByRole("menu");
  const current = menu.getByRole("menuitemradio", { name: new RegExp(LONG_ID) });
  await expect(current).toHaveAttribute("aria-checked", "true");
  await page.keyboard.press("r");
  await expect(current).toBeFocused();
  await current.press("ArrowDown");
  await expect(menu.getByRole("menuitemradio", { name: /research-one/ })).toBeFocused();
  await expect(current).toHaveAttribute("aria-checked", "true");
  await page.keyboard.press("Escape");
  await expect(trigger).toBeFocused();
  await expect(trigger).toHaveAccessibleDescription(new RegExp(LONG_ID));
  await expect(panel.getByRole("alert")).toHaveCount(0);
});
