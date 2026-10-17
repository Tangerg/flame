import { expect, test, type Locator, type Page } from "./test";
import { de } from "@/lib/i18n/locales/de";
import { en } from "@/lib/i18n/locales/en";

async function openShortcuts(page: Page, { width = 1120, theme = "light", locale = "en" } = {}) {
  await page.setViewportSize({ width, height: 900 });
  const query = new URLSearchParams({
    fixture: "workspace",
    state: "settings",
    pane: "shortcuts",
    "font-size": "18",
    theme,
    locale,
  });
  await page.goto(`/visual/?${query}`);
  await page.locator("html[data-visual-ready]").waitFor();
  const panel = page.getByRole("tabpanel");
  await panel.getByRole("searchbox").waitFor();
  return panel;
}

function changeButton(panel: Locator, action: string) {
  return panel.getByRole("button", {
    name: en["shortcuts.editAria"]!.replace("{{action}}", action),
    exact: true,
  });
}

async function overrides(page: Page) {
  return page.evaluate(() => {
    const stored = localStorage.getItem("flame.shortcut-overrides");
    if (!stored) return {};
    const persisted = JSON.parse(stored) as {
      state: { overrides: Record<string, string | null> };
    };
    return persisted.state.overrides;
  });
}

async function expectNoOverflow(page: Page, panel: Locator) {
  expect(await panel.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
}

for (const theme of ["light", "dark"]) {
  for (const width of [320, 1120]) {
    test(`German shortcut actions keep their complete words ${theme} ${width}`, async ({
      page,
    }) => {
      const panel = await openShortcuts(page, { width, theme, locale: "de" });
      const label = panel.getByText(de["shortcut.closeWorkspaceView"]!, { exact: true });
      await label.scrollIntoViewIfNeeded();
      const word = await label.evaluate((node) => {
        const text = "Arbeitsbereichsansicht";
        const walker = document.createTreeWalker(node, NodeFilter.SHOW_TEXT);
        while (walker.nextNode()) {
          const child = walker.currentNode;
          const start = child.textContent!.indexOf(text);
          if (start < 0) continue;
          const range = document.createRange();
          range.setStart(child, start);
          range.setEnd(child, start + text.length);
          return [...range.getClientRects()].map((rect) => ({
            left: rect.left,
            right: rect.right,
          }));
        }
        throw new Error("The complete shortcut action word is missing");
      });
      expect(word).toHaveLength(1);
      const bounds = (await label.boundingBox())!;
      expect(word[0]!.left).toBeGreaterThanOrEqual(bounds.x - 1);
      expect(word[0]!.right).toBeLessThanOrEqual(bounds.x + bounds.width + 1);
      await expect(panel.getByText("Esc", { exact: true })).toBeVisible();
      await expect(
        panel.getByRole("button", {
          name: de["shortcuts.editAria"]!.replace("{{action}}", de["settings.title"]!),
          exact: true,
        }),
      ).toBeVisible();
      await expectNoOverflow(page, panel);
    });
  }
}

test("recording keeps focus through Escape and a successful binding", async ({ page }) => {
  const panel = await openShortcuts(page);
  const change = changeButton(panel, en["command.showShortcuts"]!);
  const other = changeButton(panel, en["settings.title"]!);
  const filter = panel.getByRole("searchbox", { name: en["shortcuts.filterAria"]!, exact: true });
  const recorder = panel.getByRole("textbox", {
    name: en["shortcuts.editAria"]!.replace("{{action}}", en["command.showShortcuts"]!),
    exact: true,
  });
  const cancel = panel.getByRole("button", { name: en["common.cancel"]!, exact: true });
  await change.focus();
  await change.press("Enter");
  await expect(recorder).toBeFocused();
  await expect(recorder).toHaveJSProperty("readOnly", true);
  await expect(panel.getByRole("status")).toHaveText(en["shortcuts.recording"]!);
  await expect(filter).toBeDisabled();
  await expect(other).toBeDisabled();
  await page.keyboard.press("Escape");
  await expect(change).toBeFocused();
  await expect(filter).toBeEnabled();
  await expect(other).toBeEnabled();
  expect(await overrides(page)).toEqual({});
  await expect(page.getByTestId("active-main-view")).toHaveText("settings");

  for (const key of ["Escape", "Enter", "Space"]) {
    await change.press("Enter");
    await expect(recorder).toBeFocused();
    await recorder.press("Tab");
    await expect(cancel).toBeFocused();
    await cancel.press("Shift+Tab");
    await expect(recorder).toBeFocused();
    await recorder.press("Tab");
    await expect(cancel).toBeFocused();
    await cancel.press(key);
    await expect(change).toBeFocused();
    expect(await overrides(page)).toEqual({});
  }

  await change.press("Enter");
  await expect(recorder).toBeFocused();
  await page.keyboard.press("ControlOrMeta+j");
  await expect(change).toBeFocused();
  expect(await overrides(page)).toEqual({ "shortcuts.show": "mod+j" });
});

test("the recording field accepts bare Enter without activating its Cancel button", async ({
  page,
}) => {
  const panel = await openShortcuts(page);
  const change = changeButton(panel, en["command.showShortcuts"]!);
  await change.click();
  const recorder = panel.getByRole("textbox", {
    name: en["shortcuts.editAria"]!.replace("{{action}}", en["command.showShortcuts"]!),
    exact: true,
  });
  await expect(recorder).toBeFocused();
  await recorder.press("Enter");
  await expect(change).toBeFocused();
  await expect(recorder).toHaveCount(0);
  expect(await overrides(page)).toEqual({ "shortcuts.show": "enter" });
});

test("a fixed shortcut conflict cannot be replaced and cancellation preserves its binding", async ({
  page,
}) => {
  const panel = await openShortcuts(page, { width: 320, theme: "dark" });
  const change = changeButton(panel, en["command.showShortcuts"]!);
  await change.click();
  await page.keyboard.press("ControlOrMeta+n");
  await expect(panel.getByRole("alert")).toContainText(en["sidebar.action.newSession"]!);
  await expect(
    panel.getByRole("button", { name: en["shortcuts.replace"]!, exact: true }),
  ).toHaveCount(0);
  expect(await overrides(page)).toEqual({});
  await expectNoOverflow(page, panel);
  await panel.getByRole("button", { name: en["common.cancel"]!, exact: true }).click();
  await expect(change).toBeFocused();
  expect(await overrides(page)).toEqual({});
  await expect(page.getByTestId("active-main-view")).toHaveText("settings");
});

test("an editable shortcut conflict changes neither command until explicitly replaced", async ({
  page,
}) => {
  const panel = await openShortcuts(page, { width: 320 });
  const change = changeButton(panel, en["command.showShortcuts"]!);
  const replace = panel.getByRole("button", { name: en["shortcuts.replace"]!, exact: true });
  await change.click();
  await page.keyboard.press("ControlOrMeta+,");
  await expect(panel.getByRole("alert")).toContainText(en["settings.title"]!);
  await expect(replace).toBeVisible();
  expect(await overrides(page)).toEqual({});
  await panel.getByRole("button", { name: en["common.cancel"]!, exact: true }).click();
  await expect(change).toBeFocused();
  expect(await overrides(page)).toEqual({});

  await change.press("Enter");
  await page.keyboard.press("ControlOrMeta+,");
  await expect(replace).toBeVisible();
  expect(await overrides(page)).toEqual({});
  await replace.click();
  await expect(change).toBeFocused();
  expect(await overrides(page)).toEqual({ "settings.open": null, "shortcuts.show": "mod+," });
  await expect(panel.getByText(en["shortcuts.unbound"]!, { exact: true })).toBeVisible();
  await expectNoOverflow(page, panel);
});

test("single and all resets restore declared defaults and preserve persisted override semantics", async ({
  page,
}) => {
  const panel = await openShortcuts(page, { theme: "dark" });
  const filter = panel.getByRole("searchbox", { name: en["shortcuts.filterAria"]!, exact: true });
  const change = changeButton(panel, en["command.showShortcuts"]!);
  await filter.fill(en["command.showShortcuts"]!);
  await change.click();
  await page.keyboard.press("ControlOrMeta+j");
  await expect(change).toBeFocused();
  expect(await overrides(page)).toEqual({ "shortcuts.show": "mod+j" });
  await panel.getByRole("button", { name: en["shortcuts.reset"]!, exact: true }).click();
  await expect(change).toBeFocused();
  expect(await overrides(page)).toEqual({});
  await expect(panel.locator("kbd").last()).toHaveText("/");

  await change.press("Enter");
  await page.keyboard.press("ControlOrMeta+j");
  await expect(change).toBeFocused();
  await filter.fill(en["settings.title"]!);
  const settings = changeButton(panel, en["settings.title"]!);
  await settings.click();
  await page.keyboard.press("ControlOrMeta+k");
  await expect(settings).toBeFocused();
  expect(await overrides(page)).toEqual({ "shortcuts.show": "mod+j", "settings.open": "mod+k" });
  await panel.getByRole("button", { name: en["shortcuts.resetAll"]!, exact: true }).click();
  await expect(filter).toBeFocused();
  expect(await overrides(page)).toEqual({});

  await page.reload();
  await page.locator("html[data-visual-ready]").waitFor();
  expect(await overrides(page)).toEqual({});
  await filter.fill(en["command.showShortcuts"]!);
  await expect(panel.locator("kbd").last()).toHaveText("/");
  await filter.fill(en["settings.title"]!);
  await expect(panel.locator("kbd").last()).toHaveText(",");
  await expect(
    panel.getByRole("button", { name: en["shortcuts.resetAll"]!, exact: true }),
  ).toHaveCount(0);
});

test("shortcut changes return focus to the retained filter when their row leaves the results", async ({
  page,
}) => {
  const panel = await openShortcuts(page);
  const filter = panel.getByRole("searchbox", { name: en["shortcuts.filterAria"]!, exact: true });
  await filter.fill("Mod+/");
  await changeButton(panel, en["command.showShortcuts"]!).click();
  const recorder = panel.getByRole("textbox", {
    name: en["shortcuts.editAria"]!.replace("{{action}}", en["command.showShortcuts"]!),
    exact: true,
  });
  await expect(recorder).toBeFocused();
  await recorder.press("ControlOrMeta+j");
  await expect(filter).toHaveValue("Mod+/");
  await expect(panel.getByText(en["shortcuts.empty"]!, { exact: true })).toBeVisible();
  expect(await overrides(page)).toEqual({ "shortcuts.show": "mod+j" });
  await expect(filter).toBeFocused();

  await filter.fill("mod+j");
  await expect(changeButton(panel, en["command.showShortcuts"]!)).toBeVisible();
  await panel.getByRole("button", { name: en["shortcuts.reset"]!, exact: true }).click();
  await expect(filter).toHaveValue("mod+j");
  await expect(panel.getByText(en["shortcuts.empty"]!, { exact: true })).toBeVisible();
  expect(await overrides(page)).toEqual({});
  await expect(filter).toBeFocused();
});
