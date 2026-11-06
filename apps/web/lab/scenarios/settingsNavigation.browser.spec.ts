import { expect, test, type Locator, type Page } from "./test";
import de from "../../../../plugins/locale-de/package/io.github.tangerg.flame/messages.json" with { type: "json" };
import zh from "../../../../plugins/locale-zh/package/io.github.tangerg.flame/messages.json" with { type: "json" };
import { en } from "@/lib/i18n/locales/en";
import providersDeclaration from "../../../../plugins/providers/package/io.github.tangerg.flame/declaration.json" with { type: "json" };
const providerTitle = providersDeclaration.components[0]!.title;
async function openSettings(
  page: Page,
  { width = 1120, height = 720, theme = "light", locale = "en" } = {},
) {
  await page.setViewportSize({ width, height });
  const query = new URLSearchParams({
    fixture: "workspace",
    state: "settings",
    pane: "appearance",
    "font-size": "18",
    theme,
    locale,
  });
  await page.goto(`/lab/fixture.html?${query}`);
  await page.locator("html[data-visual-ready]").waitFor();
  await page.evaluate(() => document.fonts.ready);
  const catalog = locale === "de" ? de : en;
  return {
    catalog,
    list: page.getByRole("tablist", { name: catalog["settings.title"]!, exact: true }),
    search: page.getByRole("searchbox", {
      name: catalog["settings.searchPlaceholder"]!,
      exact: true,
    }),
  };
}

async function geometry(list: Locator) {
  return list.evaluate((node) => {
    const rail = node.parentElement!.getBoundingClientRect();
    const frame = node.parentElement!.parentElement!.getBoundingClientRect();
    const panel = document.querySelector('[role="tabpanel"]:not([aria-hidden="true"])')!;
    const content = panel.getBoundingClientRect();
    return {
      railHeight: rail.height,
      railRight: rail.right,
      frameHeight: frame.height,
      contentHeight: content.height,
      contentLeft: content.left,
      contentOverflow: panel.scrollWidth - panel.clientWidth,
      pageOverflow: document.documentElement.scrollWidth - innerWidth,
    };
  });
}

for (const environment of [
  { locale: "en", theme: "light", messages: en },
  { locale: "zh", theme: "dark", messages: zh },
]) {
  test(`appearance choices keep visible names through keyboard selection in ${environment.locale}`, async ({
    page,
  }, testInfo) => {
    const messages: Record<string, string> = environment.messages;
    await openSettings(page, { width: 1440, height: 900, ...environment });
    const pane = page.getByRole("tabpanel");
    const theme = pane.getByRole("button", {
      name: new RegExp(`, ${messages["settings.theme"]}$`),
    });
    await expect(theme).toHaveAccessibleName(
      `${await theme.innerText()}, ${messages["settings.theme"]}`,
    );
    await theme.focus();
    await theme.press("Enter");
    await page.keyboard.press("Home");
    await expect(
      page.getByRole("menuitemradio", {
        name: messages["settings.theme.system"],
        exact: true,
      }),
    ).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(theme).toHaveAccessibleName(
      `${messages["settings.theme.system"]}, ${messages["settings.theme"]}`,
    );
    await expect(theme).toBeFocused();

    for (const key of ["settings.font.ui", "settings.font.code"]) {
      const purpose = messages[key]!;
      const trigger = pane.getByRole("button", { name: new RegExp(`, ${purpose}$`) });
      await expect(trigger).toHaveAccessibleName(`${await trigger.innerText()}, ${purpose}`);
      await trigger.focus();
      await trigger.press("Enter");
      const options = page.getByRole("menuitemradio");
      await expect(options.first()).toBeFocused();
      expect(await options.count(), "a real system font must be available").toBeGreaterThan(1);
      const choice = await options.last().innerText();
      await page.keyboard.press("End");
      await expect(options.last()).toBeFocused();
      await page.keyboard.press("Enter");
      await expect(trigger).toHaveAccessibleName(`${choice}, ${purpose}`);
      await expect(trigger).toHaveText(choice);
      await expect(trigger).toBeFocused();
    }

    const language = pane.getByRole("button", {
      name: new RegExp(`, ${messages["settings.language.label"]}$`),
    });
    await expect(language).toHaveAccessibleName(
      `${await language.innerText()}, ${messages["settings.language.label"]}`,
    );
    await language.focus();
    await language.press("Enter");
    await page.keyboard.press("Home");
    await expect(page.getByRole("menuitemradio", { name: "English", exact: true })).toBeFocused();
    await page.keyboard.press("Enter");
    const english = pane.getByRole("button", { name: "English, Language", exact: true });
    await expect(english).toBeFocused();
    await page.setViewportSize({ width: 560, height: 900 });
    await expect(english).toHaveText("English");
    expect(await pane.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
    await testInfo.attach("appearance-current-choices", {
      body: await page.screenshot({ animations: "disabled" }),
      contentType: "image/png",
    });
  });
}

for (const theme of ["light", "dark"]) {
  for (const width of [320, 640]) {
    test(`filtered navigation gives unused height back to the open pane ${theme} ${width}`, async ({
      page,
    }) => {
      const { catalog, list, search } = await openSettings(page, {
        width,
        height: 900,
        theme,
        locale: "de",
      });
      const before = await geometry(list);
      const tabs = await list.getByRole("tab").count();
      for (const query of [providerTitle, "ZZZ-unmatched"]) {
        await search.fill(query);
        expect(await list.getByRole("tab").count()).toBeLessThan(tabs);
        const after = await geometry(list);
        expect(after.railHeight).toBeLessThan(before.railHeight);
        expect(after.contentHeight).toBeGreaterThan(before.contentHeight);
        expect(after.contentHeight).toBeGreaterThan(after.railHeight);
        expect(after.contentOverflow).toBe(0);
        expect(after.pageOverflow).toBe(0);
        await expect(
          page.getByRole("tabpanel", { name: catalog["settings.pane.appearance"]!, exact: true }),
        ).toBeVisible();
      }
    });
  }

  test(`desktop minimum size keeps navigation beside the named open pane ${theme}`, async ({
    page,
  }) => {
    const { catalog, list, search } = await openSettings(page, { theme, locale: "de" });
    const before = await geometry(list);
    await search.fill(providerTitle);
    const after = await geometry(list);
    expect(after.railRight).toBeLessThanOrEqual(after.contentLeft);
    expect(after.railHeight).toBe(before.railHeight);
    expect(after.contentHeight).toBe(before.contentHeight);
    expect(after.contentOverflow).toBe(0);
    expect(after.pageOverflow).toBe(0);
    await expect(
      page.getByRole("tabpanel", { name: catalog["settings.pane.appearance"]!, exact: true }),
    ).toBeVisible();
  });
}

test("a short zoomed viewport scrolls the rail with keyboard navigation and keeps the pane usable", async ({
  page,
}) => {
  const { catalog, list } = await openSettings(page, { width: 560, height: 360, theme: "dark" });
  const first = list.getByRole("tab").first();
  const last = list.getByRole("tab").last();
  expect(await list.evaluate((node) => node.scrollHeight > node.clientHeight)).toBe(true);
  const bounds = await geometry(list);
  expect(bounds.contentHeight).toBeGreaterThan(bounds.railHeight);
  await first.focus();
  await first.press("End");
  await expect(last).toBeFocused();
  await expect(last).toHaveAttribute("aria-selected", "true");
  await expect(
    page.getByRole("tabpanel", { name: await last.innerText(), exact: true }),
  ).toBeVisible();
  const position = await last.evaluate((node) => {
    const tab = node.getBoundingClientRect();
    const list = node.closest('[role="tablist"]')!.getBoundingClientRect();
    return { top: tab.top, bottom: tab.bottom, listTop: list.top, listBottom: list.bottom };
  });
  expect(position.top).toBeGreaterThanOrEqual(position.listTop);
  expect(position.bottom).toBeLessThanOrEqual(position.listBottom);
  await last.press("Home");
  await expect(first).toBeFocused();
  await expect(first).toHaveAttribute("aria-selected", "true");
  await expect(
    page.getByRole("tabpanel", { name: catalog["settings.pane.appearance"]!, exact: true }),
  ).toBeVisible();
  expect((await geometry(list)).pageOverflow).toBe(0);
});

test("both clear actions return focus to search without changing the open pane", async ({
  page,
}) => {
  const { catalog, list, search } = await openSettings(page, {
    width: 320,
    height: 900,
    theme: "dark",
  });
  const tabs = await list.getByRole("tab").count();
  await search.fill("Providers");
  await search.press("Tab");
  const clear = page.getByRole("button", { name: catalog["common.clearSearch"]!, exact: true });
  await expect(clear).toBeFocused();
  await clear.press("Enter");
  await expect(search).toBeFocused();
  await expect(search).toHaveValue("");
  await expect(list.getByRole("tab")).toHaveCount(tabs);

  await search.fill("ZZZ-unmatched");
  const emptyClear = page.getByRole("status").getByRole("button", {
    name: catalog["common.clearSearch"]!,
    exact: true,
  });
  await emptyClear.click();
  await expect(search).toBeFocused();
  await expect(search).toHaveValue("");
  await expect(list.getByRole("tab")).toHaveCount(tabs);
  await expect(
    page.getByRole("tabpanel", { name: catalog["settings.pane.appearance"]!, exact: true }),
  ).toBeVisible();
});

test("Enter opens a search result and Escape clears the filter while preserving that selection", async ({
  page,
}) => {
  const { list, search } = await openSettings(page);
  await search.fill("Providers");
  await search.press("Enter");
  await expect(page.getByRole("tabpanel", { name: providerTitle, exact: true })).toBeVisible();
  await expect(search).toBeFocused();
  await search.press("Escape");
  await expect(search).toHaveValue("");
  await expect(search).toBeFocused();
  await expect(list.getByRole("tab", { name: providerTitle, exact: true })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(page.getByTestId("active-main-view")).toHaveText("settings");
});

test("opening Settings without a pane keeps its default content until a result is chosen", async ({
  page,
}) => {
  await page.goto("/lab/fixture.html?fixture=workspace&state=dock-light&theme=light&font-size=18");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.evaluate(async () => {
    const moduleUrl = "/src/features/workspace/public/deeplinks.ts";
    const { openSettingsView }: typeof import("@/features/workspace/public/deeplinks") =
      await import(moduleUrl);
    const fixturePath = "/lab/scenarios/fixtureRuntime.tsx";
    const { visualClient } = (await import(fixturePath)) as typeof import("./fixtureRuntime");
    openSettingsView(visualClient.workspace.access);
  });
  const search = page.getByRole("searchbox", {
    name: en["settings.searchPlaceholder"]!,
    exact: true,
  });
  const appearance = page.getByRole("tabpanel", {
    name: en["settings.pane.appearance"]!,
    exact: true,
  });
  await expect(appearance).toBeVisible();
  await search.fill(providerTitle);
  await expect(appearance).toBeVisible();
  await search.press("Enter");
  await expect(page.getByRole("tabpanel", { name: providerTitle, exact: true })).toBeVisible();
  await search.press("Escape");
  await expect(search).toHaveValue("");
  await expect(page.getByRole("tabpanel", { name: providerTitle, exact: true })).toBeVisible();
});
