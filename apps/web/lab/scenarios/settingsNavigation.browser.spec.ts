import { expect, test, type Locator, type Page } from "./test";
import de from "../../../../plugins/locale-de/package/io.github.tangerg.flame/messages.json" with { type: "json" };
import zh from "../../../../plugins/locale-zh/package/io.github.tangerg.flame/messages.json" with { type: "json" };
import { en } from "@/lib/i18n/locales/en";
import providersDeclaration from "../../../../plugins/providers/package/io.github.tangerg.flame/declaration.json" with { type: "json" };
import { expectLabReady } from "../readiness";
const providerTitle = providersDeclaration.components[0]!.title;
async function openSettings(
  page: Page,
  { width = 1120, height = 720, theme = "light", locale = "en", fontSize = 18 } = {},
) {
  await page.setViewportSize({ width, height });
  const query = new URLSearchParams({
    fixture: "workspace",
    state: "settings",
    pane: "appearance",
    "font-size": String(fontSize),
    theme,
    locale,
  });
  await page.goto(`/lab/fixture.html?${query}`);
  await expectLabReady(page);
  await page.evaluate(() => document.fonts.ready);
  const catalog = locale === "de" ? de : locale === "zh" ? zh : en;
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
      listHeight: node.clientHeight,
      listTotal: node.scrollHeight,
      frameRows: getComputedStyle(node.parentElement!.parentElement!).gridTemplateRows,
      railRows: getComputedStyle(node.parentElement!).gridTemplateRows,
      railHeight: rail.height,
      railBottom: rail.bottom,
      railRight: rail.right,
      frameHeight: frame.height,
      contentHeight: content.height,
      contentTop: content.top,
      contentLeft: content.left,
      contentOverflow: panel.scrollWidth - panel.clientWidth,
      pageOverflow: document.documentElement.scrollWidth - innerWidth,
    };
  });
}

async function navigationLabels(list: Locator) {
  return list.evaluate((list) => {
    const bounds = list.getBoundingClientRect();
    const rtl = getComputedStyle(list).direction === "rtl";
    return [...list.querySelectorAll('[role="tab"]')].map((tab) => {
      const walker = document.createTreeWalker(tab, NodeFilter.SHOW_TEXT);
      let text = walker.nextNode();
      while (text && !text.textContent?.trim()) text = walker.nextNode();
      if (!text) throw new Error("Settings tab has no label");
      const range = document.createRange();
      range.selectNodeContents(text);
      const label = range.getBoundingClientRect();
      return {
        name: tab.textContent!.trim(),
        icon: tab.querySelector("svg") !== null,
        inset: rtl ? bounds.right - label.right : label.left - bounds.left,
        width: label.width,
      };
    });
  });
}

for (const environment of [
  { locale: "en", theme: "light", fontSize: 14 },
  { locale: "zh", theme: "dark", fontSize: 18 },
]) {
  for (const width of [320, 1120]) {
    test(`settings navigation keeps one label axis with optional icons in ${environment.locale} at ${width}px`, async ({
      page,
    }) => {
      const { list, search } = await openSettings(page, {
        ...environment,
        width,
        height: 900,
      });
      const labels = await navigationLabels(list);
      expect(labels.some((label) => label.icon)).toBe(true);
      expect(labels.some((label) => !label.icon)).toBe(true);
      expect(labels.every((label) => label.width > 0)).toBe(true);
      const axis = labels.find((label) => label.icon)!.inset;
      for (const label of labels) expect(Math.abs(label.inset - axis)).toBeLessThanOrEqual(0.5);
      await test.info().attach("settings-navigation-label-axis", {
        body: await page.screenshot({ animations: "disabled" }),
        contentType: "image/png",
      });

      const last = list.getByRole("tab").last();
      await list.getByRole("tab").first().focus();
      await page.keyboard.press("End");
      await expect(last).toBeFocused();
      await expect(last).toHaveAttribute("aria-selected", "true");
      await expect(last).toBeInViewport({ ratio: 1 });

      const plain = labels.find((label) => !label.icon)!.name;
      await search.fill(plain);
      await search.press("Enter");
      await expect(list.getByRole("tab", { name: plain, exact: true })).toHaveAttribute(
        "aria-selected",
        "true",
      );
      const filtered = await navigationLabels(list);
      for (const label of filtered) expect(Math.abs(label.inset - axis)).toBeLessThanOrEqual(0.5);
      await expect(search).toBeFocused();
      await test.info().attach("settings-filtered-label-axis", {
        body: await page.screenshot({ animations: "disabled" }),
        contentType: "image/png",
      });
      await search.press("Escape");
      await expect(search).toHaveValue("");
      await expect(list.getByRole("tab", { name: plain, exact: true })).toHaveAttribute(
        "aria-selected",
        "true",
      );

      await page.evaluate(() => {
        document.documentElement.dir = "rtl";
      });
      await expect(list).toHaveCSS("direction", "rtl");
      const mirrored = await navigationLabels(list);
      const mirroredAxis = mirrored.find((label) => label.icon)!.inset;
      for (const label of mirrored)
        expect(Math.abs(label.inset - mirroredAxis)).toBeLessThanOrEqual(0.5);
      await test.info().attach("settings-navigation-label-measurements", {
        body: JSON.stringify({ labels, filtered, mirrored }, null, 2),
        contentType: "application/json",
      });
      await test.info().attach("settings-mirrored-label-axis", {
        body: await page.screenshot({ animations: "disabled" }),
        contentType: "image/png",
      });
    });
  }
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

for (const browserName of ["chromium", "webkit"] as const) {
  const shortTest = test.extend({ browserName });
  for (const environment of [
    { locale: "en", theme: "light", fontSize: 14 },
    { locale: "zh", theme: "dark", fontSize: 18 },
  ]) {
    shortTest(
      `a short zoomed viewport scrolls the rail with keyboard navigation and keeps the pane usable in ${environment.locale} ${browserName}`,
      async ({ page, browser }, testInfo) => {
        expect(browser.browserType().name()).toBe(browserName);
        const { catalog, list } = await openSettings(page, {
          width: 373,
          height: 360,
          ...environment,
        });
        const first = list.getByRole("tab").first();
        const last = list.getByRole("tab").last();
        for (const height of [360, 240, 360]) {
          await page.setViewportSize({ width: 373, height });
          const bounds = await geometry(list);
          await testInfo.attach(`settings-short-rail-${environment.locale}-${height}-frame`, {
            body: JSON.stringify(bounds),
            contentType: "application/json",
          });
          expect(bounds.listTotal).toBeGreaterThan(bounds.listHeight);
          expect(bounds.contentTop).toBeGreaterThanOrEqual(bounds.railBottom);
          if (height === 360) expect(bounds.contentHeight).toBeGreaterThan(bounds.railHeight);
          expect(bounds.contentHeight).toBeGreaterThanOrEqual((await last.boundingBox())!.height);
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
            const rail = node.closest('[role="tablist"]')!.parentElement!.getBoundingClientRect();
            const hit = document.elementFromPoint(tab.x + tab.width / 2, tab.y + tab.height / 2);
            let top = Math.max(list.top, rail.top, 0);
            let bottom = Math.min(list.bottom, rail.bottom, innerHeight);
            for (let ancestor = node.parentElement; ancestor; ancestor = ancestor.parentElement) {
              if (!/(auto|scroll|hidden|clip)/.test(getComputedStyle(ancestor).overflowY)) continue;
              const bounds = ancestor.getBoundingClientRect();
              top = Math.max(top, bounds.top + ancestor.clientTop);
              bottom = Math.min(bottom, bounds.top + ancestor.clientTop + ancestor.clientHeight);
            }
            return {
              top: tab.top,
              bottom: tab.bottom,
              listTop: top,
              listBottom: bottom,
              reachable: hit !== null && node.contains(hit),
            };
          });
          await testInfo.attach(`settings-short-rail-${environment.locale}-${height}-geometry`, {
            body: JSON.stringify({ bounds, position }),
            contentType: "application/json",
          });
          expect(position.top).toBeGreaterThanOrEqual(position.listTop);
          expect(position.bottom).toBeLessThanOrEqual(position.listBottom);
          expect(position.reachable).toBe(true);
          await testInfo.attach(`settings-short-rail-${environment.locale}-${height}`, {
            body: await page.screenshot({ animations: "disabled" }),
            contentType: "image/png",
          });
          await last.press("Home");
          await expect(first).toBeFocused();
          await expect(first).toHaveAttribute("aria-selected", "true");
          await expect(first).toBeInViewport({ ratio: 1 });
          await expect(
            page.getByRole("tabpanel", { name: catalog["settings.pane.appearance"]!, exact: true }),
          ).toBeVisible();
          expect((await geometry(list)).pageOverflow).toBe(0);
        }
      },
    );
  }
}

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
  await expectLabReady(page);
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
