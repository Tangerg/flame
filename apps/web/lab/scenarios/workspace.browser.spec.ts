import { en as mcpEn } from "../../../../plugins/mcp/component/locales/en";
import { expect, test, type Locator, type Page } from "./test";
import AxeBuilder from "@axe-core/playwright";
import { type VisualAgentState } from "./agentSessionSnapshots";
import { en } from "@/lib/i18n/locales/en";
import { en as providersEnglish } from "../../../../plugins/providers/component/locales/en";
import { en as hooksEnglish } from "../../../../plugins/hooks/component/locales/en";
import { en as usageEnglish } from "../../../../plugins/usage/component/locales/en";
import { zh as usageChinese } from "../../../../plugins/usage/component/locales/zh";
import { en as reportsEnglish } from "../../../../plugins/usagereports/component/locales/en";
import { zh as reportsChinese } from "../../../../plugins/usagereports/component/locales/zh";
import { solidFill } from "@/features/theme/kit/scale";
import { type NotificationLevel } from "@/features/notifications/public/types";
import {
  DOCK_MIN_WIDTH_PX,
  CONVERSATION_READING_MIN_PX,
  defaultDockWidth,
} from "@/lib/shellGeometry";
import {
  VISUAL_DOCK_WIDTH_RATIO,
  DOCK_VIEW_BY_STATE,
  VISUAL_SETTINGS_PANES,
  VISUAL_WORKSPACE_STATES,
  VISUAL_WORKSPACE_VIEWPORT,
  type VisualSettingsPane,
  type VisualWorkspaceState,
  type VisualWorkspaceTheme,
} from "./workspaceFixtureStates";
import { expectLabReady } from "../readiness";
import { VISUAL_LONG_SKILL_DOCUMENTS } from "./skillSnapshots";
// The Browse panels trigger also carries the active view id; only the retained view hosts its body.
function dockView(page: Page, id: string) {
  return page
    .locator(`[data-dock-view-id="${id}"]`)
    .filter({ has: page.locator('[data-slot="workspace-view-body"]') });
}

function pluginView(page: Page, view = "trajectory") {
  return dockView(page, `package:visual:${view}`);
}

function dockCatalog(page: Page) {
  return page
    .locator(".agent-context-dock")
    .getByRole("listbox", { name: en["dock.catalog.title"]!, exact: true });
}

const SETTINGS_SEARCH = { name: en["settings.searchPlaceholder"]! };

const ACTIVE_FILE_PATH = "apps/web/src/features/shell/workbench/panel/DockResizer.tsx";

test.use({ viewport: VISUAL_WORKSPACE_VIEWPORT });

interface WorkspaceRoute {
  state: VisualWorkspaceState;
  theme?: VisualWorkspaceTheme;
  pane?: VisualSettingsPane;
  fullView?: string;
  agentState?: VisualAgentState;
}

async function openWorkspace(page: Page, route: WorkspaceRoute): Promise<void> {
  if (route.state === "settings") await page.setViewportSize({ width: 1120, height: 720 });
  const query = new URLSearchParams({
    fixture: "workspace",
    theme: route.theme ?? "light",
    state: route.state,
  });
  if (route.agentState) query.set("agent-state", route.agentState);
  if (route.pane) query.set("pane", route.pane);
  if (route.fullView) query.set("full-view", route.fullView);
  await page.goto(`/lab/fixture.html?${query}`);
  await expectLabReady(page);
  await expect(page.getByTestId("workspace-state")).toHaveAttribute("data-state", route.state);
}

test("a full view keeps its catalog title in prose and its file identity in mono", async ({
  page,
}) => {
  const readTitle = (title: Locator) =>
    title.evaluate((title) => {
      const head = (stack: string) =>
        stack
          .split(",")[0]!
          .trim()
          .replace(/^["']|["']$/g, "");
      const mono = head(getComputedStyle(document.documentElement).getPropertyValue("--font-mono"));
      const family = title ? getComputedStyle(title).fontFamily : "";
      return { text: title?.textContent?.trim() ?? "", isMono: head(family) === mono, family };
    });

  await openWorkspace(page, { state: "full-view", fullView: "package:visual:diff" });
  const heading = page
    .locator("main")
    .getByRole("heading", { name: "Workspace diff", exact: true });
  await expect(heading).toBeVisible();
  expect(await heading.evaluate((node) => getComputedStyle(node).fontFamily)).not.toMatch(
    /monospace/,
  );
  const header = page.locator('[data-slot="workspace-window-header"]');
  expect((await readTitle(header.getByText("Workspace diff", { exact: true }))).isMono).toBe(false);

  await openWorkspace(page, { state: "full-view", fullView: "file" });
  expect((await readTitle(header.getByText("Files", { exact: true }))).isMono).toBe(false);
  const identity = page
    .locator('[data-slot="workspace-view-header"]:visible')
    .getByTitle(ACTIVE_FILE_PATH, { exact: true });
  await expect(identity).toHaveText("DockResizer.tsx");
  const reader = page
    .getByRole("main")
    .getByRole("region", { name: ACTIVE_FILE_PATH, exact: true });
  const canonical = reader.locator('[data-slot="file-path"]');
  await expect(canonical).toHaveCount(1);
  const path = await readTitle(canonical);
  expect(path.text).toBe(ACTIVE_FILE_PATH);
  expect(path.isMono, `a PATH is mono, got ${path.family}`).toBe(true);
});

test("the icon gallery searches Flame glyphs by name and concept", async ({ page }) => {
  await openWorkspace(page, { state: "full-view", fullView: "package:visual:icon-gallery" });
  const view = page.locator("main");
  const search = view.getByRole("searchbox", { name: "Filter icons" });
  await expect(search).toBeVisible();
  await expect(view.getByRole("heading", { name: "Flame icon gallery" })).toBeVisible();
  await search.fill("file-diff");
  await expect(view.getByText("FileDiff", { exact: true })).toBeVisible();
  await expect(view.getByText("FileText", { exact: true })).toBeHidden();
  await search.fill("patch");
  await expect(view.getByText("FileDiff", { exact: true })).toBeVisible();
  await search.fill("no-such-flame-glyph");
  await expect(view.getByRole("status")).toContainText(/0 of \d+ icons/);
});

async function starveTheRow(page: Page): Promise<void> {
  const rail = page.getByRole("separator", { name: "Resize the workspace fixture sidebar" });
  await expect
    .poll(async () => {
      const landed = await rail.evaluate((node) => {
        (node as HTMLElement).focus();
        return document.activeElement === node;
      });
      if (!landed) return "the rail never took focus";
      await page.keyboard.press("End");
      const now = await rail.getAttribute("aria-valuenow");
      const max = await rail.getAttribute("aria-valuemax");
      return now === max ? "at its maximum" : `at ${now} of ${max}`;
    })
    .toBe("at its maximum");
}

async function waitForWorkspaceState(page: Page, state: VisualWorkspaceState): Promise<void> {
  if (state === "dock-light") {
    await expect(page.getByRole("tab", { name: "File" })).toHaveAttribute("data-active", "");
    return;
  }
  if (state === "dock-review") {
    const view = pluginView(page, "diff");
    const tab = page.getByRole("tab", { name: "Workspace diff", exact: true });
    await expect(tab).toBeVisible();
    await expect(tab).toHaveAttribute("aria-selected", "true");
    const identity = view.getByRole("heading", { name: "Workspace diff", exact: true });
    await expect(identity).toHaveCount(1);
    await expect(identity).toHaveClass(/(^|\s)sr-only(\s|$)/);
    await expect(
      view.getByRole("heading", { name: "DockResizer.tsx", exact: true }),
    ).toBeInViewport();
    const changes = view.getByRole("region", {
      name: `Changes in ${ACTIVE_FILE_PATH}`,
      exact: true,
    });
    await expect(changes.getByRole("table")).toBeVisible();
    await expect(
      changes.getByRole("cell", {
        name: "const rail = railRef.current;",
        exact: true,
      }),
    ).toBeVisible();
    return;
  }
  if (state === "dock-empty") {
    await expect(
      pluginView(page, "diff").getByText("No changes in this comparison.", { exact: true }),
    ).toBeVisible();
    return;
  }
  if (state === "dock-loading") {
    const view = pluginView(page, "diff");
    await expect(
      view.getByRole("status").filter({ hasText: "Reading uncommitted changes…" }),
    ).toBeVisible();
    await expect(view.getByRole("button", { name: "Refresh", exact: true })).toBeDisabled();
    return;
  }
  if (state === "dock-runs" || state === "dock-trajectory") {
    const view = pluginView(page);
    await expect(view.getByRole("heading", { name: "Session trajectory" })).toBeVisible();
    await expect(view.getByText(/Page 1 · \d+ records/)).toBeVisible();
    await expect(view.locator('[data-trajectory-kind="run"]')).toHaveCount(
      state === "dock-runs" ? 7 : 1,
    );
    await expect(
      view.locator('[data-trajectory-record="model:call_visual_recovered"]'),
    ).toContainText("unknown");
    return;
  }
  if (state === "dock-skills" || state === "dock-skills-catalog") {
    const count = state === "dock-skills" ? 2 : VISUAL_LONG_SKILL_DOCUMENTS.length;
    await expect(
      pluginView(page, "skills").getByRole("heading", {
        name: `${count} available Skills`,
        exact: true,
      }),
    ).toBeVisible();
    await expect(pluginView(page, "skills").getByRole("alert")).toHaveCount(0);
    return;
  }
  if (state === "dock-usage") {
    await expect(pluginView(page, "usage").getByRole("status")).toHaveText(
      "Recorded usage loaded. All time.",
    );
    return;
  }
  if (state === "dock-usage-reports") {
    await expect(
      pluginView(page, "usage-reports").getByRole("button", { name: /^View report:/ }),
    ).toHaveCount(2);
    return;
  }
  if (state === "dock-schedules" || state === "dock-schedules-models") {
    await expect(
      pluginView(page, "schedules").getByText("1 enabled · 1 disabled on this page"),
    ).toBeVisible();
    return;
  }
  if (state === "dock-agent-memory") {
    await expect(
      pluginView(page, "memory").getByText("1 pending · 1 active on this page"),
    ).toBeVisible();
    return;
  }
  if (state === "dock-feature-off") {
    await expect(
      pluginView(page, "skills")
        .getByRole("alert")
        .filter({ hasText: "skills.discovered.list requires skills" }),
    ).toBeVisible();
    return;
  }
  if (state === "dock-diagnostics" || state === "dock-diagnostics-recorded") {
    const view = pluginView(page, "diagnostics");
    await expect(view.getByRole("status")).toHaveText("Last buffer read");
    const recorded = state === "dock-diagnostics-recorded";
    for (const [signal, count] of [
      ["Traces", recorded ? 3 : 0],
      ["Metrics", recorded ? 2 : 0],
      ["Logs", recorded ? 2 : 0],
    ])
      await expect(
        view.getByRole("radio", { name: `${signal} · ${count}`, exact: true }),
      ).toBeVisible();
    return;
  }
  const CATALOGUE_READY: Partial<Record<VisualWorkspaceState, string>> = {
    "dock-subagents": "Sub-agent 4 of 4",
  };
  const catalogueReady = CATALOGUE_READY[state];
  if (catalogueReady !== undefined) {
    await expect(page.locator(".agent-workspace-view:visible")).toContainText(catalogueReady);
    return;
  }
  if (state === "dock-files") {
    const view = page.locator(".agent-workspace-view:visible");
    await expect(view).toContainText("go.mod");
    await expect(view).toContainText("README.md");
    return;
  }
  if (state === "dock-error") {
    await expect(
      page
        .getByRole("alert")
        .filter({ hasText: "Visual fixture could not load the workspace diff" }),
    ).toBeVisible();
    return;
  }
  if (state === "dock-file") {
    const view = page.locator(".agent-workspace-view:visible");
    await expect(view).toContainText("8 lines");
    await expect(view).toContainText("clampDockWidth(currentWidth + delta, row.clientWidth)");
    return;
  }
  if (state === "dock-file-error") {
    const reader = page.getByRole("region", { name: "README.md", exact: true });
    await expect(reader.getByText(en["file.error.title"]!, { exact: true })).toBeVisible();
    await expect(
      reader.getByRole("button", { name: en["common.retry"]!, exact: true }),
    ).toBeVisible();
    return;
  }
  if (state === "dock-file-unsupported") {
    const reader = page.getByRole("region", { name: "review-image.png", exact: true });
    await expect(reader.getByText(en["file.unsupported.image"]!, { exact: true })).toBeVisible();
    await expect(
      reader.getByRole("button", { name: en["common.retry"]!, exact: true }),
    ).toHaveCount(0);
    return;
  }
  if (state === "dock-catalog") {
    await expect(page.getByText(en["dock.catalog.title"]!, { exact: true })).toBeVisible();
    await expect(
      dockCatalog(page).getByRole("option", { name: "Files", exact: true }),
    ).toBeVisible();
    return;
  }
  if (state === "full-view") {
    const view = page.getByRole("main");
    await expect(view).toContainText(ACTIVE_FILE_PATH);
    await expect(view).toContainText("8 lines");
    await expect(view).toContainText("clampDockWidth(currentWidth + delta, row.clientWidth)");
    return;
  }
  if (state === "settings") {
    await expect(page.getByRole("tabpanel").getByRole("heading").first()).toBeVisible();
    await expect(page.locator('main section [aria-busy="true"]')).toHaveCount(0);
    return;
  }
  throw new Error(`No expectation declared for workspace state "${state}"`);
}

for (const state of VISUAL_WORKSPACE_STATES) {
  test(`production workspace renders ${state}`, async ({ page }) => {
    await openWorkspace(page, { state });
    await waitForWorkspaceState(page, state);
    await expect(page.getByTestId("requested-workspace-state")).toHaveText(state);
  });
}

test("collapse and reopen preserve the dock workspace", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });
  await expect(page.getByTestId("active-dock-view")).toHaveText("file");

  await page.getByRole("button", { name: "Collapse right workspace" }).click();
  await expect(page.getByTestId("dock-open")).toHaveText("false");
  await expect(page.getByTestId("active-dock-view")).toHaveText("");
  await expect(page.getByTestId("dock-view-ids")).toHaveText(
    "file,package:visual:diff,package:visual:trajectory",
  );
  await page.getByRole("button", { name: "Open right workspace" }).click();

  await expect(page.getByTestId("dock-open")).toHaveText("true");
  await expect(page.getByTestId("active-dock-view")).toHaveText("file");
  await expect(page.getByRole("tab", { name: "File" })).toHaveAttribute("data-active", "");
});

test("an unsafe narrow row folds the dock without forgetting its tabs", async ({ page }) => {
  await page.setViewportSize({ width: 1120, height: 720 });
  await openWorkspace(page, { state: "dock-light" });
  const location = page.url();
  const historyLength = await page.evaluate(() => history.length);
  await starveTheRow(page);

  await expect(page.getByTestId("dock-open")).toHaveText("true");
  await expect
    .poll(() =>
      page
        .locator(".agent-dock-row .agent-context-dock")
        .evaluate((dock) => getComputedStyle(dock).visibility),
    )
    .toBe("hidden");

  const geometry = await page.locator(".agent-dock-row").evaluate((row) => {
    const conversation = row.firstElementChild;
    const dock = row.querySelector(".agent-context-dock");
    return {
      rowWidth: row.getBoundingClientRect().width,
      conversationWidth: conversation?.getBoundingClientRect().width ?? 0,
      dockVisible: dock ? getComputedStyle(dock).visibility !== "hidden" : false,
    };
  });
  expect(geometry.rowWidth).toBeLessThan(CONVERSATION_READING_MIN_PX + DOCK_MIN_WIDTH_PX);
  expect(geometry.dockVisible).toBe(false);
  expect(geometry.conversationWidth).toBe(geometry.rowWidth);
  await expect(page.getByRole("button", { name: /^Open material full width/ })).toBeEnabled();
  await expect(page.getByTestId("dock-view-ids")).toHaveText(
    "file,package:visual:diff,package:visual:trajectory",
  );
  expect(page.url()).toBe(location);
  expect(await page.evaluate(() => history.length)).toBe(historyLength);
  await page.setViewportSize({ width: 1520, height: 900 });
  await expect(page.locator(".agent-dock-row")).toHaveAttribute("data-dock", "open");
  await expect(page.getByTestId("active-dock-view")).toHaveText("file");
  await expect(page.getByRole("tab", { name: "File" })).toBeVisible();
});

test("the model remains readable when the dock narrows the composer", async ({ page }) => {
  await openWorkspace(page, { state: "dock-review" });
  const model = page.getByRole("button", { name: /Switch model$/ });
  for (const width of [1800, 1120]) {
    await page.setViewportSize({ width, height: 800 });
    await expect(model.locator('[data-slot="composer-chip-label"]')).toBeVisible();
    await expect(model).toHaveAttribute("title", /GPT/);
  }
});

test("closing tabs selects a neighbor without collapsing the workspace", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  await page.getByRole("tab", { name: "Session trajectory" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("package:visual:trajectory");
  await page.getByRole("tab", { name: "Session trajectory" }).hover();
  await page.getByRole("button", { name: "Close Session trajectory" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("package:visual:diff");
  await expect(page.getByTestId("dock-open")).toHaveText("true");

  await page.getByRole("tab", { name: "Workspace diff" }).hover();
  await page.getByRole("button", { name: "Close Workspace diff" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("file");
  await expect(page.getByTestId("dock-view-ids")).toHaveText("file");
});

test("add-panel menu restores a closed singleton and focuses it", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  await page.getByRole("tab", { name: "Session trajectory" }).hover();
  await page.getByRole("button", { name: "Close Session trajectory" }).click();
  await expect(page.getByTestId("dock-view-ids")).not.toContainText("package:visual:trajectory");

  await page.getByRole("button", { name: "Browse panels" }).click();

  const onTop = await page.getByRole("combobox", { name: "Find a panel…" }).evaluate((input) => {
    const panel = input.closest("[role=dialog], div[class*='z-50']") ?? input.parentElement!;
    const box = panel.getBoundingClientRect();
    const hit = document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2);
    return panel.contains(hit);
  });
  expect(onTop).toBe(true);

  await page.getByRole("combobox", { name: "Find a panel…" }).fill("Session trajectory");
  await page.getByRole("option", { name: "Session trajectory" }).waitFor();
  await page.keyboard.press("Enter");

  await expect(page.getByTestId("active-dock-view")).toHaveText("package:visual:trajectory");
  await expect(page.getByTestId("dock-view-ids")).toHaveText(
    "file,package:visual:diff,package:visual:trajectory",
  );
});

test("files browse and preview share one dock tab", async ({ page }) => {
  await openWorkspace(page, { state: "dock-files" });
  const dock = page.locator(".agent-context-dock");
  const fileName = ACTIVE_FILE_PATH.split("/").at(-1)!;
  await dock.getByRole("treeitem", { name: "apps", exact: true }).click();
  await dock.getByRole("treeitem", { name: fileName, exact: true }).click();
  await expect(dock).toContainText("clampDockWidth(currentWidth + delta, row.clientWidth)");
  await expect(dock.getByRole("tab", { name: "Files", exact: true })).toHaveCount(1);
  const back = dock.getByRole("button", { name: "Back to files" });
  await expect(back).toBeFocused();
  await page.keyboard.press("Tab");
  await page.keyboard.press("Shift+Tab");
  await expect(back).toBeFocused();
  // Header chrome paints keyboard focus as a fill; tree rows draw the shared outline.
  expect(await back.evaluate((node) => node.matches(":focus-visible"))).toBe(true);
  await expect(back).not.toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await page.keyboard.press("Enter");
  await expect(dock.getByRole("treeitem", { name: "apps", exact: true })).toHaveAttribute(
    "aria-expanded",
    "true",
  );
  const file = dock.getByRole("treeitem", { name: fileName, exact: true });
  await expect(file).toBeFocused();
  const rowFocus = await file.evaluate((node) => {
    const style = getComputedStyle(node);
    return {
      visible: node.matches(":focus-visible"),
      style: style.outlineStyle,
      width: Number.parseFloat(style.outlineWidth),
    };
  });
  expect(rowFocus.visible).toBe(true);
  expect(rowFocus.style).not.toBe("none");
  expect(rowFocus.width).toBeGreaterThan(0);
  await expect(page.getByTestId("active-dock-view")).toHaveText("file");
});

for (const [browserName, skillsTest] of [
  ["chromium", test],
  ["webkit", test.extend({ browserName: "webkit" })],
] as const) {
  for (const profile of [
    {
      locale: "en",
      theme: "light",
      fontSize: 14,
      panel: "Skills",
      available: "Available Skills",
      search: "Search this catalog",
      clear: "Clear search",
      total: "80 available Skills",
      matching: "40 of 80 available Skills",
      instructions: "Instructions for runtime-review-035",
      source: "Source and revision",
      project: "Project",
      back: "Back to available Skills",
      refresh: "Refresh",
      documentRefresh: "Refresh document",
      composer: "Message composer",
      conversation: "Conversation",
      openFull:
        "Open material full width — the window is too narrow to show it beside the conversation",
      close: "Close",
      draft: "Keep this Skills catalog draft.",
      suffix: " Continue after checking the catalog.",
    },
    {
      locale: "zh",
      theme: "dark",
      fontSize: 18,
      panel: "技能",
      available: "可用技能",
      search: "搜索此目录",
      clear: "清除搜索",
      total: "80 个可用技能",
      matching: "80 个可用技能中的 40 个",
      instructions: "runtime-review-035 的指令",
      source: "来源与修订",
      project: "项目",
      back: "返回可用技能",
      refresh: "刷新",
      documentRefresh: "刷新文档",
      composer: "消息输入框",
      conversation: "对话",
      openFull: "全宽打开材料——窗口太窄，无法与对话并排显示",
      close: "关闭",
      draft: "核对技能目录后继续。",
      suffix: " 已核对，继续。",
    },
  ] as const) {
    skillsTest(
      `Skills long catalog preserves search, reading and draft return in ${profile.locale} ${browserName}`,
      async ({ page }, testInfo) => {
        await page.setViewportSize({ width: 1472, height: 1000 });
        const query = new URLSearchParams({
          fixture: "workspace",
          state: "dock-skills-catalog",
          locale: profile.locale,
          theme: profile.theme,
          "font-size": String(profile.fontSize),
          motion: "off",
        });
        await page.goto(`/lab/fixture.html?${query}`);
        await expectLabReady(page);
        await page
          .getByRole("button", { name: "Collapse the workspace fixture sidebar", exact: true })
          .click();
        const composer = page.getByRole("textbox", { name: profile.composer, exact: true });
        await composer.fill(profile.draft);
        if (profile.locale === "zh") {
          await page.setViewportSize({ width: 640, height: 400 });
          await page.getByRole("button", { name: profile.openFull, exact: true }).click();
        }
        const viewport = page.locator('[data-slot="workspace-view-body"]:visible');
        const panel = viewport
          .getByRole("region", { name: profile.panel, exact: true })
          .and(page.locator('[data-slot="panel"]'));
        const catalog = panel.getByRole("region", { name: profile.available, exact: true });
        const search = panel.getByRole("searchbox", { name: profile.search, exact: true });
        const row = (name: string) =>
          catalog.getByRole("button").filter({
            has: page.getByText(name, { exact: true }),
          });
        const first = row(VISUAL_LONG_SKILL_DOCUMENTS[0]!.name);
        const selected = VISUAL_LONG_SKILL_DOCUMENTS.find(
          (item) => item.name === "runtime-review-035",
        )!;
        const opener = row(selected.name);
        const scrollTop = () => viewport.evaluate((node) => node.scrollTop);
        const evidence: unknown[] = [];
        const capture = async (name: string) => {
          evidence.push({
            name,
            geometry: await viewport.evaluate((node) => ({
              viewport: node.getBoundingClientRect().toJSON(),
              scrollTop: node.scrollTop,
              documentOverflow: document.documentElement.scrollWidth - innerWidth,
              focused: document.activeElement?.outerHTML.slice(0, 500),
            })),
          });
          await testInfo.attach(`skills-catalog-${profile.locale}-${name}`, {
            body: await page.screenshot({ animations: "disabled" }),
            contentType: "image/png",
          });
        };
        const reachable = (control: Locator) =>
          control.evaluate((node) => {
            const box = node.getBoundingClientRect();
            return node.contains(
              document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2),
            );
          });
        try {
          await expect(
            panel.getByRole("heading", { name: profile.total, exact: true }),
          ).toBeVisible();
          await expect(catalog.getByRole("button")).toHaveCount(VISUAL_LONG_SKILL_DOCUMENTS.length);
          evidence.push({
            name: "entry-geometry",
            first: await first.boundingBox(),
            controls: await panel.locator('[data-slot="panel-controls"]').boundingBox(),
          });
          await capture("catalog-entry");
          await expect(first).toBeInViewport();
          await first.hover();
          await page.mouse.wheel(0, 1200);
          await expect.poll(scrollTop).toBeGreaterThan(300);
          await expect(first).not.toBeInViewport();

          await search.fill("runtime");
          await expect(search).toBeFocused();
          await expect(
            panel.getByRole("heading", { name: profile.matching, exact: true }),
          ).toBeVisible();
          await expect(catalog.getByRole("button")).toHaveCount(40);
          await expect.poll(scrollTop).toBe(0);
          await expect(first).toBeInViewport({ ratio: 1 });
          await capture("first-match");

          const controls = panel.locator('[data-slot="panel-controls"]');
          const firstBox = (await first.boundingBox())!;
          const targetBox = (await opener.boundingBox())!;
          const viewportBox = (await viewport.boundingBox())!;
          const controlsBox = (await controls.boundingBox())!;
          const frameBottom = viewportBox.y + viewportBox.height;
          const room = frameBottom - (controlsBox.y + controlsBox.height) - targetBox.height;
          const wheelDelta = Math.round(targetBox.y + targetBox.height - frameBottom + room / 4);
          evidence.push({
            name: "wheel-geometry",
            first: firstBox,
            target: targetBox,
            viewport: viewportBox,
            controls: controlsBox,
            room,
            wheelDelta,
          });
          expect(room).toBeGreaterThan(0);
          expect(wheelDelta).toBeGreaterThan(0);
          await page.mouse.move(firstBox.x + firstBox.width / 2, firstBox.y + firstBox.height / 2);
          await page.mouse.wheel(0, wheelDelta);
          await expect
            .poll(
              async () => {
                const positions = await viewport.evaluate(async (node) => {
                  const frames = [node.scrollTop];
                  for (let frame = 0; frame < 8; frame += 1) {
                    await new Promise(requestAnimationFrame);
                    frames.push(node.scrollTop);
                  }
                  return frames;
                });
                evidence.push({ name: "wheel-settle", positions });
                return positions[0]! > 0 && positions.every((top) => top === positions[0]);
              },
              { message: "the catalog wheel must settle before entering the selected row" },
            )
            .toBe(true);
          await expect(opener).toBeInViewport({ ratio: 1 });
          if (profile.locale === "zh") await opener.focus();
          const readingTop = await scrollTop();
          expect(readingTop).toBeGreaterThan(300);
          const title = (await opener.getByText(selected.name, { exact: true }).boundingBox())!;
          const description = (await opener
            .getByText(selected.description!, { exact: true })
            .boundingBox())!;
          const scope = (await opener.getByText(profile.project, { exact: true }).boundingBox())!;
          evidence.push({ name: "row-alignment", title, description, scope, readingTop });
          expect(Math.abs(title.x - description.x)).toBeLessThanOrEqual(1);
          expect(description.y).toBeGreaterThanOrEqual(title.y + title.height - 1);
          expect(
            Math.min(title.x + title.width, scope.x + scope.width) > Math.max(title.x, scope.x) &&
              Math.min(title.y + title.height, scope.y + scope.height) > Math.max(title.y, scope.y),
          ).toBe(false);
          await capture("middle-catalog");
          if (profile.locale === "en") await opener.click();
          else await opener.press("Enter");
          const heading = panel.getByRole("heading", { name: selected.name, exact: true });
          const instructions = panel.getByRole("region", {
            name: profile.instructions,
            exact: true,
          });
          await expect(heading).toBeFocused();
          await expect.poll(() => instructions.textContent()).toBe(selected.instructions);
          await heading.press("ControlOrMeta+A");
          const selectedInstructions = () =>
            instructions.evaluate((element) => {
              const selection = getSelection();
              const range = selection?.rangeCount === 1 ? selection.getRangeAt(0) : null;
              return {
                text: range?.toString(),
                startsAtInstructions: range?.startContainer === element && range.startOffset === 0,
                endsAtInstructions:
                  range?.endContainer === element && range.endOffset === element.childNodes.length,
              };
            });
          await expect.poll(selectedInstructions).toEqual({
            text: selected.instructions,
            startsAtInstructions: true,
            endsAtInstructions: true,
          });
          evidence.push({
            name: "instruction-selection",
            selection: await selectedInstructions(),
            rendered: await page.evaluate(() => getSelection()?.toString()),
          });
          await capture("instructions");

          const source = panel.getByRole("button", { name: profile.source, exact: true });
          await source.click();
          const path = panel.getByText(selected.path, { exact: true });
          await path.scrollIntoViewIfNeeded();
          await expect(path).toBeInViewport();
          await expect(panel.getByText(selected.revision, { exact: true })).toBeVisible();
          const facts = await panel.locator("dl").evaluate((node) => ({
            box: node.getBoundingClientRect().toJSON(),
            overflow: node.scrollWidth - node.clientWidth,
            pairs: Array.from(node.children, (pair) => ({
              box: pair.getBoundingClientRect().toJSON(),
              label: pair.querySelector("dt")!.getBoundingClientRect().toJSON(),
              value: pair.querySelector("dd")!.getBoundingClientRect().toJSON(),
              text: pair.querySelector("dd")!.textContent,
            })),
          }));
          evidence.push({ name: "source-alignment", facts });
          expect(facts.overflow).toBeLessThanOrEqual(1);
          expect(facts.pairs.map((pair) => pair.text)).toEqual([selected.path, selected.revision]);
          for (const pair of facts.pairs) {
            expect(pair.value.x).toBeGreaterThanOrEqual(facts.box.x - 1);
            expect(pair.value.right).toBeLessThanOrEqual(facts.box.right + 1);
            expect(pair.label.x).toBeGreaterThanOrEqual(pair.box.x - 1);
            expect(pair.value.right).toBeLessThanOrEqual(pair.box.right + 1);
            expect(Math.abs(pair.label.x - pair.value.x)).toBeLessThanOrEqual(1);
          }
          await capture("source");
          const back = panel.getByRole("button", { name: profile.back, exact: true });
          for (const control of [
            back,
            panel.getByRole("button", { name: profile.documentRefresh, exact: true }),
          ]) {
            await control.scrollIntoViewIfNeeded();
            expect(await reachable(control)).toBe(true);
          }
          if (profile.locale === "en") await page.keyboard.press("Escape");
          else {
            await back.focus();
            await back.press("Enter");
          }
          await expect(search).toHaveValue("runtime");
          await expect(opener).toBeFocused();
          await expect
            .poll(async () => Math.abs((await scrollTop()) - readingTop))
            .toBeLessThanOrEqual(1);
          const returnedFrames = await viewport.evaluate(
            async (node, { row, bar }) => {
              const frames = [];
              for (let frame = 0; frame < 8; frame += 1) {
                await new Promise(requestAnimationFrame);
                frames.push({
                  scrollTop: node.scrollTop,
                  viewport: node.getBoundingClientRect().toJSON(),
                  controls: bar.getBoundingClientRect().toJSON(),
                  row: row.getBoundingClientRect().toJSON(),
                  focused: document.activeElement === row,
                });
              }
              return frames;
            },
            { row: await opener.elementHandle(), bar: await controls.elementHandle() },
          );
          evidence.push({ name: "returned-frames", readingTop, frames: returnedFrames });
          for (const frame of returnedFrames) {
            expect(Math.abs(frame.scrollTop - readingTop)).toBeLessThanOrEqual(1);
            expect(frame.focused).toBe(true);
            expect(frame.row.top).toBeGreaterThanOrEqual(
              Math.max(frame.viewport.top, frame.controls.bottom) - 1,
            );
            expect(frame.row.bottom).toBeLessThanOrEqual(frame.viewport.bottom + 1);
            expect(frame.row.left).toBeGreaterThanOrEqual(frame.viewport.left - 1);
            expect(frame.row.right).toBeLessThanOrEqual(frame.viewport.right + 1);
          }
          await capture("returned-catalog");

          const refresh = panel.getByRole("button", { name: profile.refresh, exact: true });
          await refresh.focus();
          const refreshTop = await scrollTop();
          await refresh.press("Enter");
          await expect(refresh).toBeEnabled();
          await expect(search).toHaveValue("runtime");
          await expect
            .poll(async () => Math.abs((await scrollTop()) - refreshTop))
            .toBeLessThanOrEqual(1);
          await search.focus();
          if (profile.locale === "en") await search.press("Escape");
          else {
            const clear = panel.getByRole("button", { name: profile.clear, exact: true });
            await clear.focus();
            await clear.press("Enter");
          }
          await expect(search).toHaveValue("");
          await expect(
            panel.getByRole("heading", { name: profile.total, exact: true }),
          ).toBeVisible();
          await expect.poll(scrollTop).toBe(0);
          await expect(first).toBeInViewport({ ratio: 1 });
          expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(
            0,
          );

          if (profile.locale === "en") {
            await page.setViewportSize({ width: 640, height: 400 });
            await page.getByRole("button", { name: profile.openFull, exact: true }).click();
          }
          const close = page.getByRole("button", { name: profile.close, exact: true });
          await close.focus();
          await close.press("Enter");
          await expect(
            page.getByRole("region", { name: profile.conversation, exact: true }),
          ).toBeFocused();
          await expect(composer).toHaveValue(profile.draft);
          await composer.focus();
          await composer.press(process.platform === "darwin" ? "Meta+ArrowDown" : "Control+End");
          await page.keyboard.insertText(profile.suffix);
          await expect(composer).toHaveValue(profile.draft + profile.suffix);
          evidence.push({
            name: "continued-draft",
            value: await composer.inputValue(),
            focused: await composer.evaluate((node) => node === document.activeElement),
          });
          await testInfo.attach(`skills-catalog-${profile.locale}-continued-draft`, {
            body: await page.screenshot({ animations: "disabled" }),
            contentType: "image/png",
          });
        } finally {
          await testInfo.attach("skills-catalog-reading.json", {
            body: JSON.stringify({ locale: profile.locale, browserName, evidence }),
            contentType: "application/json",
          });
        }
      },
    );

    skillsTest(
      `Skills long catalog keeps its reading row across Dock collapse, gradual fold and full-width material in ${profile.locale} ${browserName}`,
      async ({ page }, testInfo) => {
        await page.setViewportSize({ width: 1440, height: 900 });
        const query = new URLSearchParams({
          fixture: "workspace",
          state: "dock-skills-catalog",
          locale: profile.locale,
          theme: profile.theme,
          "font-size": String(profile.fontSize),
          motion: "off",
        });
        await page.goto(`/lab/fixture.html?${query}`);
        await expectLabReady(page);
        await page
          .getByRole("button", { name: "Collapse the workspace fixture sidebar", exact: true })
          .click();
        const composer = page.getByRole("textbox", { name: profile.composer, exact: true });
        await composer.fill(profile.draft);
        const dockRow = page.locator(".agent-dock-row");
        // The toggle's name is localized; its host and expanded state are not.
        const toggle = page.locator(".agent-dock-control").getByRole("button");
        const viewport = page.locator('[data-slot="workspace-view-body"]:visible');
        const panel = viewport
          .getByRole("region", { name: profile.panel, exact: true })
          .and(page.locator('[data-slot="panel"]'));
        const catalog = panel.getByRole("region", { name: profile.available, exact: true });
        const search = panel.getByRole("searchbox", { name: profile.search, exact: true });
        const controls = panel.locator('[data-slot="panel-controls"]');
        const row = (name: string) =>
          catalog.getByRole("button").filter({
            has: page.getByText(name, { exact: true }),
          });
        const first = row(VISUAL_LONG_SKILL_DOCUMENTS[0]!.name);
        const selected = VISUAL_LONG_SKILL_DOCUMENTS.find(
          (item) => item.name === "runtime-review-035",
        )!;
        const opener = row(selected.name);
        const scrollTop = () => viewport.evaluate((node) => node.scrollTop);
        await expect(
          panel.getByRole("heading", { name: profile.total, exact: true }),
        ).toBeVisible();
        await expect(catalog.getByRole("button")).toHaveCount(VISUAL_LONG_SKILL_DOCUMENTS.length);
        const scroller = await viewport.elementHandle();
        const searchInput = await search.elementHandle();
        const draftInput = await composer.elementHandle();
        const evidence: unknown[] = [];
        // Read-only geometry of the retained scrollport, sampled across frames.
        const observe = () =>
          page.evaluate(
            async ({ scroller, names, target }) => {
              const known = new Set(names);
              const frames = [scroller.scrollTop];
              for (let frame = 0; frame < 8; frame += 1) {
                await new Promise(requestAnimationFrame);
                frames.push(scroller.scrollTop);
              }
              const box = scroller.getBoundingClientRect();
              const frameTop = box.top + scroller.clientTop;
              const frameLeft = box.left + scroller.clientLeft;
              const frame = {
                top: frameTop,
                bottom: frameTop + scroller.clientHeight,
                left: frameLeft,
                right: frameLeft + scroller.clientWidth,
              };
              const bar = scroller
                .querySelector('[data-slot="panel-controls"]')
                ?.getBoundingClientRect();
              const readingEdge = Math.max(frame.top, bar?.bottom ?? frame.top);
              const rows = Array.from(scroller.querySelectorAll('button, [role="button"]')).flatMap(
                (button) => {
                  const title = Array.from(button.querySelectorAll("*")).find(
                    (node) => node.childElementCount === 0 && known.has(node.textContent ?? ""),
                  );
                  if (!title) return [];
                  const { top, bottom, left, right } = button.getBoundingClientRect();
                  return [{ name: title.textContent!, top, bottom, left, right }];
                },
              );
              const index = rows.findIndex(
                (candidate) =>
                  candidate.bottom > candidate.top &&
                  candidate.top >= readingEdge - 1 &&
                  candidate.bottom <= frame.bottom + 1,
              );
              const reading = rows.find((candidate) => candidate.name === target) ?? null;
              const shell = scroller.closest(".agent-dock-row");
              const active = document.activeElement;
              return {
                frames,
                scrollTop: scroller.scrollTop,
                scrollHeight: scroller.scrollHeight,
                clientWidth: scroller.clientWidth,
                clientHeight: scroller.clientHeight,
                frame,
                readingEdge,
                dock: {
                  state: shell?.getAttribute("data-dock") ?? null,
                  presentation: shell?.getAttribute("data-presentation") ?? null,
                  measure: shell
                    ? getComputedStyle(shell).getPropertyValue("--dock-measure")
                    : null,
                  width: scroller.closest(".agent-context-dock")?.getBoundingClientRect().width,
                },
                first:
                  index < 0
                    ? null
                    : {
                        name: rows[index]!.name,
                        index,
                        previous: rows[index - 1]?.name ?? null,
                        next: rows[index + 1]?.name ?? null,
                      },
                target: reading,
                targetFits:
                  reading !== null &&
                  reading.top >= readingEdge - 1 &&
                  reading.bottom <= frame.bottom + 1 &&
                  reading.left >= frame.left - 1 &&
                  reading.right <= frame.right + 1,
                rows,
                focused: active
                  ? {
                      tag: active.tagName,
                      label: active.getAttribute("aria-label"),
                      html: active.outerHTML.slice(0, 300),
                    }
                  : null,
                documentOverflow: document.documentElement.scrollWidth - innerWidth,
              };
            },
            {
              scroller,
              names: VISUAL_LONG_SKILL_DOCUMENTS.map((item) => item.name),
              target: selected.name,
            },
          );
        type Observation = Awaited<ReturnType<typeof observe>>;
        const settle = async (name: string, ready: (seen: Observation) => boolean = () => true) => {
          let observation: Observation | undefined;
          try {
            await expect
              .poll(
                async () => {
                  observation = await observe();
                  const seen = observation;
                  return seen.frames.every((top) => top === seen.frames[0]) && ready(seen);
                },
                { message: `the retained catalog scrollport must settle (${name})` },
              )
              .toBe(true);
          } finally {
            evidence.push({ name, observation });
          }
          return observation!;
        };
        const capture = async (name: string) => {
          evidence.push({ name: `${name}-capture`, observation: await observe() });
          await testInfo.attach(`skills-reflow-${profile.locale}-${name}`, {
            body: await page.screenshot({ animations: "disabled" }),
            contentType: "image/png",
          });
        };
        try {
          await search.fill("runtime");
          await expect(
            panel.getByRole("heading", { name: profile.matching, exact: true }),
          ).toBeVisible();
          await expect(catalog.getByRole("button")).toHaveCount(40);
          await expect.poll(scrollTop).toBe(0);
          await expect(first).toBeInViewport({ ratio: 1 });
          await capture("first-match");

          const firstBox = (await first.boundingBox())!;
          const targetBox = (await opener.boundingBox())!;
          const viewportBox = (await viewport.boundingBox())!;
          const controlsBox = (await controls.boundingBox())!;
          const frameBottom = viewportBox.y + viewportBox.height;
          const room = frameBottom - (controlsBox.y + controlsBox.height) - targetBox.height;
          const wheelDelta = Math.round(targetBox.y + targetBox.height - frameBottom + room / 4);
          evidence.push({
            name: "wheel-geometry",
            first: firstBox,
            target: targetBox,
            viewport: viewportBox,
            controls: controlsBox,
            room,
            wheelDelta,
          });
          expect(room).toBeGreaterThan(0);
          expect(wheelDelta).toBeGreaterThan(0);
          await page.mouse.move(firstBox.x + firstBox.width / 2, firstBox.y + firstBox.height / 2);
          await page.mouse.wheel(0, wheelDelta);
          const reading = await settle("reading", (seen) => seen.scrollTop > 0);
          const readingTop = reading.scrollTop;
          expect(readingTop).toBeGreaterThan(300);
          expect(reading.targetFits, `${selected.name} must be fully readable`).toBe(true);
          await capture("before-collapse");

          await expect(toggle).toHaveAttribute("aria-expanded", "true");
          if (profile.locale === "en") {
            await expect(toggle).toHaveAccessibleName(en["dock.action.hide"]!);
          }
          const hideLabel = (await toggle.getAttribute("aria-label"))!;
          await toggle.focus();
          await page.keyboard.press("Enter");
          await expect(dockRow).toHaveAttribute("data-dock", "collapsed");
          await expect(toggle).toHaveAttribute("aria-expanded", "false");
          await expect(toggle).not.toHaveAttribute("aria-label", hideLabel);
          if (profile.locale === "en") {
            await expect(toggle).toHaveAccessibleName(en["dock.action.show"]!);
          }
          await expect(toggle).toBeFocused();
          await settle("collapsed", (seen) => seen.dock.state === "collapsed");
          await capture("collapsed");
          await expect(composer).toHaveValue(profile.draft);

          await expect(toggle).toBeFocused();
          await page.keyboard.press("Enter");
          await expect(dockRow).toHaveAttribute("data-dock", "open");
          await expect(toggle).toHaveAttribute("aria-expanded", "true");
          await expect(toggle).toHaveAttribute("aria-label", hideLabel);
          await expect(toggle).toBeFocused();
          await expect(search).toHaveValue("runtime");
          await expect(
            panel.getByRole("heading", { name: profile.matching, exact: true }),
          ).toBeVisible();
          await expect
            .poll(async () => Math.abs((await scrollTop()) - readingTop), {
              message: "reopening the Dock at the same viewport must keep the reading offset",
            })
            .toBeLessThanOrEqual(1);
          const reopened = await settle("reopened", (seen) => seen.dock.state === "open");
          await capture("reopened");
          for (const top of reopened.frames) {
            expect(Math.abs(top - readingTop)).toBeLessThanOrEqual(1);
          }
          expect(reopened.targetFits, `${selected.name} must stay fully readable`).toBe(true);
          await expect(toggle).toBeFocused();
          await expect(composer).toHaveValue(profile.draft);
          expect(await search.evaluate((node, before) => node === before, searchInput)).toBe(true);
          expect(await viewport.evaluate((node, before) => node === before, scroller)).toBe(true);

          await opener.click();
          await expect(
            panel.getByRole("region", { name: profile.instructions, exact: true }),
          ).toBeVisible();
          await page.keyboard.press("Escape");
          await expect(opener).toBeFocused();
          const reflowSearchInput = await search.elementHandle();
          const returned = await settle("pre-reflow-reading");
          expect(returned.targetFits, "the returned reading row must be fully visible").toBe(true);
          for (let step = 1; step <= 20; step += 1) {
            const scale = 1 + step / 20;
            await page.setViewportSize({
              width: Math.floor(1440 / scale),
              height: Math.floor(900 / scale),
            });
            const narrowed = await settle(`narrow-${step}`);
            if (narrowed.dock.state === "open") {
              await expect(opener).toBeFocused();
              expect(narrowed.targetFits, "the focused reading row must survive each reflow").toBe(
                true,
              );
            }
          }
          await expect(dockRow).toHaveAttribute("data-dock", "collapsed");
          await expect(
            page.getByRole("region", { name: profile.conversation, exact: true }),
          ).toBeFocused();
          await expect(
            page.getByRole("searchbox", { name: profile.search, exact: true }),
          ).toHaveCount(0);
          const openFull = page.getByRole("button", { name: profile.openFull, exact: true });
          await expect(openFull).toBeEnabled();
          await settle("folded", (seen) => seen.dock.state === "collapsed");
          await capture("folded");
          await openFull.click();
          await expect(dockRow).toHaveAttribute("data-presentation", "main");
          await expect(search).toHaveValue("runtime");
          await expect(
            panel.getByRole("heading", { name: profile.matching, exact: true }),
          ).toBeVisible();
          await expect(catalog.getByRole("button")).toHaveCount(40);
          expect(await search.evaluate((node, before) => node === before, reflowSearchInput)).toBe(
            true,
          );
          expect(await viewport.evaluate((node, before) => node === before, scroller)).toBe(true);
          const material = await settle("full-width", (seen) => seen.dock.presentation === "main");
          await capture("full-width");
          expect(material.targetFits, "the original reading target must remain readable").toBe(
            true,
          );
          expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(
            0,
          );

          const close = page.getByRole("button", { name: profile.close, exact: true });
          await close.focus();
          await close.press("Enter");
          await expect(
            page.getByRole("region", { name: profile.conversation, exact: true }),
          ).toBeFocused();
          await expect(composer).toHaveValue(profile.draft);
          expect(await composer.evaluate((node, before) => node === before, draftInput)).toBe(true);
          await composer.focus();
          await composer.press(process.platform === "darwin" ? "Meta+ArrowDown" : "Control+End");
          await page.keyboard.insertText(profile.suffix);
          await expect(composer).toHaveValue(profile.draft + profile.suffix);
          await capture("continued-draft");
        } catch (error) {
          await capture("failure").catch((reason: unknown) => {
            evidence.push({ name: "failure-capture", error: String(reason) });
          });
          throw error;
        } finally {
          await testInfo.attach("skills-catalog-reflow.json", {
            body: JSON.stringify({ locale: profile.locale, browserName, evidence }),
            contentType: "application/json",
          });
        }
      },
    );
  }
}

test("portable Skills captures discovery, review and personal targets in one dock tab", async ({
  page,
}) => {
  await openWorkspace(page, { state: "dock-skills" });
  const view = pluginView(page, "skills");
  const catalog = view.getByRole("radiogroup", { name: "Catalog" });
  await expect(view).toContainText("2 available Skills");
  await catalog.getByRole("radio", { name: "Skill proposals", exact: true }).click();
  await expect(view).toContainText("2 pending proposals");
  const proposal = view.getByRole("button", {
    name: "risk-first-review Proposal Proposed review instructions.",
    exact: true,
  });
  await expect(proposal).toHaveAttribute("aria-expanded", "false");
  await proposal.click();
  await expect(proposal).toHaveAttribute("aria-expanded", "true");
  await expect(view).toContainText("Start from the riskiest hunk.");
  await expect(view.getByRole("button", { name: "Confirm approve", exact: true })).toHaveCount(0);
  await view.getByRole("button", { name: "Approve", exact: true }).first().click();
  const confirmation = view.getByRole("region", { name: "Confirm proposal decision" });
  await expect(confirmation).toContainText("risk-first-review");
  await confirmation.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(confirmation).toHaveCount(0);
  await expect(view).toContainText("2 pending proposals");
  await catalog.getByRole("radio", { name: "Personal Skill library", exact: true }).click();
  await expect(view).toContainText("2 user-managed Skills");
  await expect(page.getByTestId("active-dock-view")).toHaveText("package:visual:skills");
  await catalog.getByRole("radio", { name: "Available Skills", exact: true }).click();
  await expect(view).toContainText("2 available Skills");
});

for (const theme of ["light", "dark"] as const) {
  for (const section of ["Review", "Personal"] as const) {
    test(`skills ${section.toLowerCase()} catalog ${theme}`, async ({ page }) => {
      await openWorkspace(page, { state: "dock-skills", theme });
      const view = pluginView(page, "skills");
      await view
        .getByRole("radiogroup", { name: "Catalog" })
        .getByRole("radio", {
          name: section === "Review" ? "Skill proposals" : "Personal Skill library",
          exact: true,
        })
        .click();
      await expect(pluginView(page, "skills")).toContainText(
        section === "Review" ? "2 pending proposals" : "2 user-managed Skills",
      );
    });
  }
}

test("dock tabs use roving focus and arrow-key activation", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  const file = page.getByRole("tab", { name: "File" });
  await file.focus();
  await file.press("ArrowRight");

  await expect(page.getByTestId("active-dock-view")).toHaveText("package:visual:diff");
  await expect(page.getByRole("tab", { name: "Workspace diff" })).toBeFocused();
  await expect(page.getByRole("tab", { name: "Workspace diff" })).toHaveAttribute(
    "data-active",
    "",
  );
});

test("the active overflow tab stays visible and both hidden edges remain signposted", async ({
  page,
}) => {
  await openWorkspace(page, { state: "dock-agent-memory" });
  await page.getByRole("tab", { name: "Workspace diff" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("package:visual:diff");

  const separator = page.getByRole("separator", { name: "Resize right workspace" });
  await separator.focus();
  await separator.press("Home");

  const strip = page.locator(".agent-dock-tabs");
  await expect(strip).toHaveAttribute("data-overflow-start", "");
  await expect(strip).toHaveAttribute("data-overflow-end", "");
  const [stripBox, activeBox] = await Promise.all([
    strip.boundingBox(),
    page.getByRole("tab", { name: "Workspace diff" }).boundingBox(),
  ]);
  expect(stripBox).not.toBeNull();
  expect(activeBox).not.toBeNull();
  expect(activeBox!.x).toBeGreaterThanOrEqual(stripBox!.x);
  expect(activeBox!.x + activeBox!.width).toBeLessThanOrEqual(stripBox!.x + stripBox!.width);
  await page.mouse.move(activeBox!.x + activeBox!.width / 2, activeBox!.y + activeBox!.height / 2);
  const close = page.getByRole("button", { name: "Close Workspace diff", exact: true });
  await expect(close).toBeVisible();
  const closeBox = await close.boundingBox();
  expect(closeBox).not.toBeNull();
  const edge = await strip.evaluate((node) =>
    Number.parseFloat(getComputedStyle(node).scrollPaddingInlineStart),
  );
  expect(closeBox!.x).toBeGreaterThanOrEqual(stripBox!.x + edge);
  expect(closeBox!.x + closeBox!.width).toBeLessThanOrEqual(stripBox!.x + stripBox!.width - edge);
});

test("file and portable trajectory tabs render through their production views", async ({
  page,
}) => {
  await openWorkspace(page, { state: "dock-light" });

  await page.getByRole("tab", { name: "File" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("file");
  const fileView = dockView(page, "file");
  await expect(fileView.getByTitle(ACTIVE_FILE_PATH)).toBeVisible();
  await expect(fileView.getByText(/const currentWidth = readDockWidth/)).toBeVisible();

  await page.getByRole("tab", { name: "Session trajectory" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("package:visual:trajectory");
  await expect(fileView.getByText(/const currentWidth = readDockWidth/)).toBeHidden();
  const run = pluginView(page).locator('[data-trajectory-record="run:run_root"]');
  await expect(run).toBeVisible();
  await expect(run.getByRole("button", { expanded: false })).toHaveAccessibleName(
    "Run · openai/gpt-5.6-sol Running run_root 8:00:00 AM",
  );
});

test("automatic dock sizing never records a user preference", async ({ page }) => {
  await page.setViewportSize({ width: 1120, height: 800 });
  await openWorkspace(page, { state: "dock-catalog" });
  const preference = page.getByTestId("persisted-dock-ratio");
  const dock = page.locator(".agent-context-dock");
  for (const width of [1120, 1800, 1280]) {
    await page.setViewportSize({ width, height: 800 });
    await expect(preference).toHaveText("");
    await expect
      .poll(async () => {
        const { row, width } = await dock.evaluate((element) => ({
          row: element.closest(".agent-dock-row")!.getBoundingClientRect().width,
          width: element.getBoundingClientRect().width,
        }));
        return Math.round(width) - defaultDockWidth(row);
      })
      .toBe(0);
  }
  await page.getByRole("separator", { name: "Resize right workspace" }).focus();
  await page.keyboard.press("ArrowLeft");
  await expect(preference).not.toHaveText("");
});

test("all dock views share one stable user-owned width", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  const separator = page.getByRole("separator", { name: "Resize right workspace" });
  const persistedRatio = page.getByTestId("persisted-dock-ratio");
  await separator.focus();
  const liveNow = Number(await separator.getAttribute("aria-valuenow"));
  await separator.press("ArrowRight");
  const settledWidth = String(liveNow - 8);
  await expect(separator).toHaveAttribute("aria-valuenow", settledWidth);
  const settledRatio = await persistedRatio.textContent();

  await page.getByRole("tab", { name: "Workspace diff" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("package:visual:diff");
  await expect(separator).toHaveAttribute("aria-valuenow", settledWidth);
  await expect(persistedRatio).toHaveText(String(settledRatio));

  await page.getByRole("tab", { name: "File" }).click();
  await expect(separator).toHaveAttribute("aria-valuenow", settledWidth);
  await expect(persistedRatio).toHaveText(String(settledRatio));
});

test("dock separator exposes its real range and commits a pointer drag once", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });
  await waitForWorkspaceState(page, "dock-light");

  const separator = page.getByRole("separator", { name: "Resize right workspace" });
  const persistedRatio = page.getByTestId("persisted-dock-ratio");
  await expect(separator).toHaveAttribute("aria-valuemin", String(DOCK_MIN_WIDTH_PX));
  const max = Number(await separator.getAttribute("aria-valuemax"));
  const now = Number(await separator.getAttribute("aria-valuenow"));
  expect(now).toBeGreaterThan(DOCK_MIN_WIDTH_PX);
  expect(now).toBeLessThanOrEqual(max);
  await expect(persistedRatio).toHaveText(String(VISUAL_DOCK_WIDTH_RATIO));

  const dock = page.locator(".agent-context-dock");
  const dockBefore = (await dock.boundingBox())?.width;
  const box = await separator.boundingBox();
  if (!box || dockBefore === undefined) throw new Error("Dock separator has no layout box");
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 2 + 48, box.y + box.height / 2);

  await expect(persistedRatio).toHaveText(String(VISUAL_DOCK_WIDTH_RATIO));
  await expect(page.locator("html")).toHaveAttribute("data-visual-dock-width-commits", "0");
  await expect.poll(async () => (await dock.boundingBox())?.width ?? 0).toBeLessThan(dockBefore);

  await page.mouse.up();
  await expect(page.locator("html")).toHaveAttribute("data-visual-dock-width-commits", "1");
  await expect(persistedRatio).not.toHaveText(String(VISUAL_DOCK_WIDTH_RATIO));
  const settledRatio = await persistedRatio.textContent();
  if (!settledRatio) throw new Error("Persisted dock ratio is missing");
});

test("window clamping does not overwrite the dock preference", async ({ page }) => {
  await page.setViewportSize({ width: 1520, height: 900 });
  await openWorkspace(page, { state: "dock-light" });
  await waitForWorkspaceState(page, "dock-light");

  const separator = page.getByRole("separator", { name: "Resize right workspace" });
  const persistedRatio = page.getByTestId("persisted-dock-ratio");
  const wideMax = Number(await separator.getAttribute("aria-valuemax"));
  const wideNow = Number(await separator.getAttribute("aria-valuenow"));
  expect(wideNow).toBeLessThanOrEqual(wideMax);
  await expect(persistedRatio).toHaveText(String(VISUAL_DOCK_WIDTH_RATIO));

  await page.setViewportSize({ width: 1120, height: 720 });
  await starveTheRow(page);
  await expect(page.getByTestId("dock-open")).toHaveText("true");
  await expect(separator).toHaveCount(0);
  await expect(persistedRatio).toHaveText(String(VISUAL_DOCK_WIDTH_RATIO));
});

test("settings filtering and menu dismissal stay inside production semantics", async ({ page }) => {
  await openWorkspace(page, { state: "settings" });
  await waitForWorkspaceState(page, "settings");

  const search = page.getByRole("searchbox", SETTINGS_SEARCH);
  await search.fill("missing pane");
  await expect(page.getByText("No settings match “missing pane”.")).toBeVisible();
  await search.fill("Appearance");
  await search.press("Enter");
  await expect(page.getByRole("heading", { name: "Appearance" })).toBeVisible();

  const theme = page.getByRole("button", { name: "Theme" });
  await theme.click();
  await expect(page.getByRole("menuitemradio", { name: "Light" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("menuitemradio", { name: "Light" })).toHaveCount(0);
  await expect(theme).toBeFocused();
  await page
    .locator('[data-slot="workspace-window-header"]')
    .getByRole("button", { name: en["common.close"]!, exact: true })
    .click();
  await expect(page.getByTestId("active-main-view")).toHaveText("");
  await expect(search).toHaveCount(0);
});

test("font smoothing updates the browser rendering preference and survives reload", async ({
  page,
}) => {
  await openWorkspace(page, { state: "settings" });
  await waitForWorkspaceState(page, "settings");

  const smoothing = page.getByRole("switch", { name: en["settings.font.smoothing"]! });
  await smoothing.uncheck();
  await expect(page.locator("html")).toHaveCSS("-webkit-font-smoothing", "auto");

  await page.reload();
  await expectLabReady(page);
  await expect(smoothing).not.toBeChecked();
  await expect(page.locator("html")).toHaveCSS("-webkit-font-smoothing", "auto");

  await smoothing.check();
  await expect(page.locator("html")).toHaveCSS("-webkit-font-smoothing", "antialiased");
});

test("accent selection gives an immediate, durable visual acknowledgement", async ({ page }) => {
  await openWorkspace(page, { state: "settings" });
  await waitForWorkspaceState(page, "settings");

  const purple = page.getByRole("button", { name: "Accent: Purple" });
  await purple.click();

  await expect(purple).toHaveAttribute("aria-pressed", "true");
  await expect
    .poll(() =>
      page.evaluate(() => document.documentElement.style.getPropertyValue("--color-accent")),
    )
    .toBe(solidFill("#7f52ff"));
  await expect
    .poll(() =>
      page.evaluate(() => {
        const persisted = JSON.parse(localStorage.getItem("flame.appearance") ?? "null") as {
          state?: { accent?: string };
        } | null;
        return persisted?.state?.accent;
      }),
    )
    .toBe("#7f52ff");

  await expect(purple.locator('[data-slot="accent-selection-mark"]')).toBeVisible();
  expect((await purple.boundingBox())?.width).toBeGreaterThanOrEqual(28);

  await page.reload();
  await expectLabReady(page);
  await waitForWorkspaceState(page, "settings");
  await expect(page.getByRole("button", { name: "Accent: Purple" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
});

test("settings hosts shortcut contributions without a second page frame", async ({ page }) => {
  await openWorkspace(page, { state: "settings" });

  await page.getByRole("searchbox", SETTINGS_SEARCH).fill("Keyboard shortcuts");
  await page.getByRole("searchbox", SETTINGS_SEARCH).press("Enter");
  await expect(page.getByRole("heading", { name: "Keyboard shortcuts" })).toHaveCount(1);
  await expect(page.getByText("New session", { exact: true })).toBeVisible();

  await page.getByRole("searchbox", { name: "Filter shortcuts" }).fill("Escape");
  await expect(page.getByText("Close workspace view", { exact: true })).toBeVisible();
  await expect(page.getByText("New session", { exact: true })).toHaveCount(0);
  await expect(page.getByText("Esc", { exact: true })).toBeVisible();
});

test("provider and model settings keep validation local to their form", async ({ page }) => {
  await openWorkspace(page, { state: "settings" });

  await page.getByRole("searchbox", SETTINGS_SEARCH).fill("Providers");
  await page.getByRole("searchbox", SETTINGS_SEARCH).press("Enter");
  await expect(page.getByRole("heading", { name: "Providers" })).toBeVisible();
  await expect(page.getByText("Utility model", { exact: true })).toBeVisible();
  await expect(page.getByText("Embedding model", { exact: true })).toBeVisible();

  const utilityModel = page.getByRole("combobox", { name: "Utility model" });
  await utilityModel.click();
  await expect(page.getByRole("option", { name: /GPT-5.6/ })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(utilityModel).toBeFocused();

  const anthropicKey = page.getByLabel("anthropic API key");
  const saveButtons = page.getByRole("button", { name: "Save" });
  await expect(saveButtons.last()).toBeDisabled();
  await anthropicKey.fill("sk-ant-visual");
  await expect(saveButtons.last()).toBeEnabled();
});

test("dock add-panel control names itself and dismisses on Escape", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  const add = page.getByRole("button", { name: "Browse panels" });
  await expect(add).toHaveAccessibleName("Browse panels");
  await expect(add).toHaveAttribute("aria-expanded", "false");

  await add.click();
  await expect(add).toHaveAttribute("aria-expanded", "true");
  await expect(page.getByRole("listbox")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("listbox")).toHaveCount(0);
  await expect(add).toHaveAttribute("aria-expanded", "false");
  await expect(add).toBeFocused();
});

test("dock close control reveals its contextual glyph on hover and focus", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  const hide = page.getByRole("button", { name: "Collapse right workspace" });
  const rest = hide.locator('.t-icon-swap .t-icon[data-glyph="rest"]');
  const hover = hide.locator('.t-icon-swap .t-icon[data-glyph="hover"]');
  const opacityOf = (target: typeof rest) =>
    target.evaluate((node) => getComputedStyle(node).opacity);

  await expect.poll(() => opacityOf(rest)).toBe("1");
  await expect.poll(() => opacityOf(hover)).toBe("0");

  await hide.hover();
  await expect.poll(() => opacityOf(hover)).toBe("1");
  await expect.poll(() => opacityOf(rest)).toBe("0");

  await page.mouse.move(0, 0);
  await expect.poll(() => opacityOf(rest)).toBe("1");

  await hide.focus();
  await expect.poll(() => opacityOf(hover)).toBe("1");
});

test("a dock tab closes on a middle click", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  const timeline = page.getByRole("tab", { name: "Session trajectory" });
  await expect(timeline).toBeVisible();
  await timeline.click({ button: "middle" });

  await expect(page.getByTestId("dock-view-ids")).not.toContainText("package:visual:trajectory");
});

test("a dock tab closes from the keyboard", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  const timeline = page.getByRole("tab", { name: "Session trajectory" });
  await timeline.focus();
  await expect(timeline).toBeFocused();
  await timeline.press("Delete");
  await expect(page.getByTestId("dock-view-ids")).not.toContainText("package:visual:trajectory");

  const reachable = await page.evaluate(() => {
    const strip = document.querySelector('[aria-label="Right workspace panels"]');
    return [...(strip?.querySelectorAll("button") ?? [])].some(
      (node) =>
        getComputedStyle(node).visibility !== "hidden" && node.getAttribute("role") !== "tab",
    );
  });
  expect(reachable).toBe(false);
});

for (const theme of ["light", "dark"] as const) {
  test(`plugin notifications follow the application theme and dismiss automatically ${theme}`, async ({
    page,
  }) => {
    await page.emulateMedia({ colorScheme: theme === "light" ? "dark" : "light" });
    await openWorkspace(page, { state: "settings", theme });
    await page
      .locator("html")
      .evaluate((root) => root.style.setProperty("--font-sans", "monospace"));

    await page.evaluate(() => {
      const host = window as unknown as {
        flameVisualNotify?: (message: string, level: NotificationLevel) => void;
      };
      if (!host.flameVisualNotify) throw new Error("the fixture notifier plugin did not install");
      host.flameVisualNotify("Provider credentials were rejected", "error");
    });

    const toast = page.locator("[data-sonner-toast]");
    await expect(toast).toContainText("Provider credentials were rejected");
    await expect(toast).toHaveAttribute("data-type", "error");
    await expect(toast).toHaveCSS("font-family", "monospace");
    await expect(page.locator("[data-sonner-toaster]")).toHaveAttribute("data-sonner-theme", theme);
    await expect(toast).toHaveCSS("border-top-width", "0px");

    await expect.poll(() => toast.count(), { timeout: 6_000 }).toBe(0);
  });
}

test("workspace surfaces do not create page-level horizontal overflow", async ({ page }) => {
  await openWorkspace(page, { state: "dock-review" });
  await waitForWorkspaceState(page, "dock-review");

  const overflow = await page.locator("html").evaluate((element) => {
    return element.scrollWidth - element.clientWidth;
  });
  expect(overflow).toBeLessThanOrEqual(0);
});

for (const pane of VISUAL_SETTINGS_PANES) {
  test(`settings pane availability ${pane}`, async ({ page }) => {
    await openWorkspace(page, { state: "settings", pane });
    await waitForWorkspaceState(page, "settings");
    if (pane === "providers") {
      await expect(
        page.getByRole("combobox", { name: providersEnglish["providers.utility.title"]! }),
      ).toHaveText("GPT-5.6 Sol · openai / gpt-5.6-sol");
      await expect(
        page.getByRole("combobox", { name: providersEnglish["providers.embedding.title"]! }),
      ).toHaveText(providersEnglish["providers.embedding.off"]!);
    }
    const emptyLabel = {
      "mcp-servers": mcpEn["mcp.empty"],
      hooks: hooksEnglish.empty,
      approvals: en["approvals.rules.empty"],
    };
    if (pane in emptyLabel) {
      await expect(
        page.getByText(emptyLabel[pane as keyof typeof emptyLabel]!, { exact: true }),
      ).toBeVisible();
    }
    if (pane === "plugins") {
      await expect(page.getByRole("heading", { name: "flame.memory", exact: true })).toBeVisible();
      await expect(
        page.getByRole("heading", { name: "flame.schedules", exact: true }),
      ).toBeVisible();
      for (const [name, contribution] of [
        ["flame.memory", "memory-provider"],
        ["flame.schedules", "schedules-provider"],
      ]) {
        await expect(
          page.getByText(
            en["packages.providers.current"]!.replace("{{name}}", name!).replace(
              "{{contribution}}",
              contribution!,
            ),
            { exact: true },
          ),
        ).toBeVisible();
      }
    }
  });
}

for (const browserName of ["chromium", "webkit"] as const) {
  const reportsTest = test.extend({ browserName });
  for (const profile of [
    {
      locale: "en",
      theme: "light",
      fontSize: 14,
      width: 1120,
      height: 720,
      labels: reportsEnglish,
      composer: "Message composer",
      expand: "Open full width",
      draft: "Keep the saved report draft.",
      suffix: " Continue after reading.",
    },
    {
      locale: "zh",
      theme: "dark",
      fontSize: 18,
      width: 640,
      height: 400,
      labels: reportsChinese,
      composer: "消息输入框",
      expand: "全宽打开材料——窗口太窄，无法与对话并排显示",
      draft: "保留归档阅读前的草稿。",
      suffix: "核对后继续。",
    },
  ] as const) {
    reportsTest(
      `Saved reports preserve exact evidence and task return in ${browserName}/${profile.locale}`,
      async ({ page }, testInfo) => {
        const { labels, width, height } = profile;
        const named = (key: string) => ({ name: labels[key]!, exact: true });
        const capture = async (name: string) =>
          testInfo.attach(`saved-reports-${name}-${browserName}-${profile.locale}`, {
            body: await page.screenshot({ animations: "disabled" }),
            contentType: "image/png",
          });
        await page.setViewportSize({ width, height });
        await page.goto(
          `/lab/fixture.html?fixture=workspace&state=dock-usage-reports&locale=${profile.locale}&theme=${profile.theme}&font-size=${profile.fontSize}&motion=off`,
        );
        await expectLabReady(page);
        await expect(page.getByTestId("workspace-state")).toHaveAttribute(
          "data-state",
          "dock-usage-reports",
        );
        await page
          .getByRole("button", { name: "Collapse the workspace fixture sidebar", exact: true })
          .click();
        const composer = page.getByRole("textbox", { name: profile.composer, exact: true });
        await composer.fill(profile.draft);
        await page.getByRole("button", { name: profile.expand, exact: true }).click();
        const panel = page
          .getByRole("region", named("panel.title"))
          .and(page.locator('[data-slot="panel"]'));
        const reportButtons = panel.getByRole("button", {
          name: new RegExp(`^${labels["report.view.aria"]!.split("{{title}}")[0]}`),
        });
        await expect(reportButtons).toHaveCount(2);
        await capture("archive");
        const daily = reportButtons.nth(1);
        await daily.press("Enter");
        const title = panel.getByRole("heading", {
          name: new RegExp(`^${labels["period.daily.label"]}`),
        });
        await expect(title).toBeFocused();
        const totals = panel
          .locator("section")
          .filter({ has: page.getByRole("heading", named("report.totals.title")) });
        await expect(totals.locator("dd")).toHaveText([
          "6",
          "9,007,199,254,740,993",
          "0",
          "0",
          labels["cost.notReported"]!,
          "6/6",
          "5/6",
          "0",
        ]);
        await totals.locator("dl").scrollIntoViewIfNeeded();
        await capture("daily-totals");
        await panel.getByRole("button", named("report.breakdown.title")).click();
        await expect(panel.getByRole("heading", { name: "2026-10-19", exact: true })).toBeVisible();
        await capture("daily-breakdown");
        const identity = panel.getByRole("button", named("report.identity.title"));
        await identity.click();
        const reportId = panel.getByText("a".repeat(64), { exact: true });
        await expect(reportId).toBeVisible();
        await expect(panel.getByText("2026-10-20T08:00:00Z", { exact: true })).toBeVisible();
        await expect(panel.getByText("2026-10-19T08:00:00Z", { exact: true })).toBeVisible();
        await reportId.scrollIntoViewIfNeeded();
        await capture("exact-identity");
        await page.setViewportSize({ width: 320, height: 600 });
        await totals.locator("dl").scrollIntoViewIfNeeded();
        const geometry = await totals.locator("dl").evaluate((node) => ({
          overflow: node.scrollWidth - node.clientWidth,
          pairs: Array.from(node.children, (pair) => {
            const box = pair.getBoundingClientRect();
            const value = pair.querySelector("dd")!.getBoundingClientRect();
            return {
              left: box.left,
              right: box.right,
              valueLeft: value.left,
              valueRight: value.right,
              overflow: pair.scrollWidth - pair.clientWidth,
            };
          }),
        }));
        await testInfo.attach("saved-reports-facts-geometry.json", {
          body: JSON.stringify(geometry),
          contentType: "application/json",
        });
        expect(geometry.overflow).toBeLessThanOrEqual(1);
        for (const pair of geometry.pairs) {
          expect(pair.left).toBeGreaterThanOrEqual(0);
          expect(pair.right).toBeLessThanOrEqual(321);
          expect(pair.overflow).toBeLessThanOrEqual(1);
          expect(Math.abs(pair.valueLeft - pair.left)).toBeLessThanOrEqual(1);
          expect(Math.abs(pair.valueRight - pair.right)).toBeLessThanOrEqual(1);
        }
        expect(await panel.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
        await totals.getByText("9,007,199,254,740,993", { exact: true }).scrollIntoViewIfNeeded();
        await capture("daily-320");
        await totals.locator("dd").last().scrollIntoViewIfNeeded();
        await capture("daily-coverage-320");
        await reportId.scrollIntoViewIfNeeded();
        expect(await reportId.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
        await capture("identity-320");
        await page.setViewportSize({ width, height });
        const refresh = panel.getByRole("button", named("report.refresh"));
        await refresh.click();
        await expect(refresh).toBeFocused();
        await expect(totals.locator("dd")).toHaveText([
          "6",
          "9,007,199,254,740,993",
          "0",
          "0",
          labels["cost.notReported"]!,
          "6/6",
          "5/6",
          "0",
        ]);
        await expect(identity).toHaveAttribute("aria-expanded", "true");
        const remove = panel.getByRole("button", named("report.remove"));
        await remove.press("Enter");
        const confirmation = page.getByRole("alertdialog");
        await expect(confirmation.getByRole("button", named("removal.cancel"))).toBeFocused();
        await capture("removal-cancel");
        await page.keyboard.press("Escape");
        await expect(confirmation).toBeHidden();
        await expect(remove).toBeFocused();
        await panel.getByRole("button", named("report.back")).press("Enter");
        await expect(daily).toBeFocused();
        await expect(reportButtons).toHaveCount(2);
        const archiveRefresh = panel.getByRole("button", named("archive.refresh"));
        await archiveRefresh.click();
        await expect(archiveRefresh).toBeFocused();
        await expect(reportButtons).toHaveCount(2);
        await reportButtons.first().press("Enter");
        await expect(totals.locator("dd")).toHaveText([
          "24",
          "128,400",
          "41,900",
          "4.12",
          "4.12",
          "24/24",
          "24/24",
          "0",
        ]);
        await totals.locator("dl").scrollIntoViewIfNeeded();
        await capture("weekly-totals");
        await panel.getByRole("button", named("report.back")).press("Enter");
        await expect(reportButtons.first()).toBeFocused();
        await page.keyboard.press("Escape");
        await expect(composer).toHaveValue(profile.draft);
        await composer.focus();
        await page.keyboard.press("ControlOrMeta+A");
        await page.keyboard.press("ArrowRight");
        await page.keyboard.type(profile.suffix);
        await expect(composer).toHaveValue(profile.draft + profile.suffix);
        await capture("continued-conversation");
      },
    );
  }
}

for (const browserName of ["chromium", "webkit"] as const) {
  const usageTest = test.extend({ browserName });
  for (const profile of [
    {
      locale: "en",
      theme: "light",
      fontSize: 14,
      width: 1120,
      height: 720,
      labels: usageEnglish,
      composer: "Message composer",
      expand: "Open full width",
      draft: "Keep the Usage reading draft.",
      suffix: " Continue after checking all sources.",
    },
    {
      locale: "zh",
      theme: "dark",
      fontSize: 18,
      width: 640,
      height: 400,
      labels: usageChinese,
      composer: "消息输入框",
      expand: "全宽打开材料——窗口太窄，无法与对话并排显示",
      draft: "保留用量阅读前的草稿。",
      suffix: "核对来源后继续。",
    },
  ] as const) {
    usageTest(
      `Usage groupings preserve accounting and task return in ${browserName}/${profile.locale}`,
      async ({ page }, testInfo) => {
        const { labels, width, height } = profile;
        const named = (key: string) => ({ name: labels[key]!, exact: true });
        const capture = async (name: string) =>
          testInfo.attach(`usage-${name}-${browserName}-${profile.locale}`, {
            body: await page.screenshot({ animations: "disabled" }),
            contentType: "image/png",
          });
        await page.setViewportSize({ width, height });
        await page.goto(
          `/lab/fixture.html?fixture=workspace&state=dock-usage&locale=${profile.locale}&theme=${profile.theme}&font-size=${profile.fontSize}&motion=off`,
        );
        await expectLabReady(page);
        await page
          .getByRole("button", { name: "Collapse the workspace fixture sidebar", exact: true })
          .click();
        const composer = page.getByRole("textbox", { name: profile.composer, exact: true });
        await composer.fill(profile.draft);
        await page.getByRole("button", { name: profile.expand, exact: true }).click();
        const panel = page
          .getByRole("region", named("panel.title"))
          .and(page.locator('[data-slot="panel"]'));
        await expect(panel.getByRole("status")).toHaveText(
          labels["status.loaded"]!.replace("{{period}}", labels["period.all"]!),
        );
        const totals = panel.locator("#totals");
        const originalTotals = (await totals.textContent())!;
        for (const value of ["4.12", "128,400", "41,900"]) expect(originalTotals).toContain(value);
        const sources = panel.getByRole("region", named("breakdown.group"));
        const filter = panel.getByRole("searchbox", named("filter.label"));
        const group = panel.getByRole("combobox", named("group.label"));
        const chooseGroup = async (index: number, key: string) => {
          const selected = (await group.textContent())!;
          await group.focus();
          await page.keyboard.press("Enter");
          await expect(page.getByRole("option", { name: selected, exact: true })).toBeFocused();
          await page.keyboard.press("Home");
          await expect(page.getByRole("option").first()).toBeFocused();
          for (let step = 0; step < index; step++) {
            await page.keyboard.press("ArrowDown");
            await expect(page.getByRole("option").nth(step + 1)).toBeFocused();
          }
          await page.keyboard.press("Enter");
          await expect(group).toBeFocused();
          await expect(group).toHaveText(labels[`group.${key}`]!);
          await expect(totals).toHaveText(originalTotals);
        };
        const details = (row: Locator) => row.getByRole("button", named("bucket.details"));
        await expect(sources.getByRole("heading")).toHaveText(["openai", "anthropic"]);
        await filter.fill("anthropic");
        await expect(sources.getByRole("heading")).toHaveText(["anthropic"]);
        await expect(totals).toHaveText(originalTotals);
        await filter.fill("");
        await chooseGroup(1, "byModel");
        await expect(sources.getByRole("heading")).toHaveText([
          "openai/gpt-5.6-sol",
          "anthropic/claude-opus-5",
        ]);
        await chooseGroup(2, "byDay");
        await expect(sources.getByRole("heading")).toHaveText(["2026-10-18", "2026-10-19"]);
        const firstDay = sources.getByRole("article").first();
        await details(firstDay).click();
        await expect(firstDay.locator("dd")).toHaveText([
          "3.04",
          "18/18",
          "18/18",
          "96,300",
          "31,200",
          "0",
          "0",
          "0",
        ]);
        await firstDay.locator("dl").scrollIntoViewIfNeeded();
        await capture("day-facts");
        await chooseGroup(3, "byOrigin");
        await expect(sources.getByRole("heading")).toHaveText([
          labels["bucket.byOrigin.automatic"]!,
          labels["bucket.byOrigin.human"]!,
        ]);
        await filter.fill(labels["bucket.byOrigin.human"]!);
        await expect(sources.getByRole("heading")).toHaveText([labels["bucket.byOrigin.human"]!]);
        await panel.getByRole("button", named("filter.clear")).press("Enter");
        await expect(filter).toBeFocused();
        await filter.fill("AUTO");
        await expect(sources.getByRole("heading")).toHaveText([
          labels["bucket.byOrigin.automatic"]!,
        ]);
        const automatic = sources.getByRole("article");
        await details(automatic).click();
        await expect(automatic.locator("dd")).toHaveText([
          "1.08",
          "6/6",
          "6/6",
          "32,100",
          "10,700",
          "0",
          "0",
          "0",
        ]);
        await automatic.locator("dl").scrollIntoViewIfNeeded();
        await capture("origin-facts");
        await page.setViewportSize({ width: 320, height: 600 });
        await automatic.locator("dl").scrollIntoViewIfNeeded();
        const geometry = await automatic.locator("dl").evaluate((node) => ({
          overflow: node.scrollWidth - node.clientWidth,
          pairs: Array.from(node.children, (pair) => {
            const box = pair.getBoundingClientRect();
            const value = pair.querySelector("dd")!.getBoundingClientRect();
            return {
              left: box.left,
              right: box.right,
              valueLeft: value.left,
              valueRight: value.right,
              overflow: pair.scrollWidth - pair.clientWidth,
            };
          }),
        }));
        await testInfo.attach("usage-facts-geometry.json", {
          body: JSON.stringify(geometry),
          contentType: "application/json",
        });
        expect(geometry.overflow).toBeLessThanOrEqual(1);
        for (const pair of geometry.pairs) {
          expect(pair.left).toBeGreaterThanOrEqual(0);
          expect(pair.right).toBeLessThanOrEqual(321);
          expect(pair.overflow).toBeLessThanOrEqual(1);
          expect(Math.abs(pair.valueLeft - pair.left)).toBeLessThanOrEqual(1);
          expect(Math.abs(pair.valueRight - pair.right)).toBeLessThanOrEqual(1);
        }
        expect(await panel.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
        expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(
          0,
        );
        await capture("origin-320");
        await page.setViewportSize({ width, height });
        const refresh = panel.getByRole("button", named("refresh"));
        await refresh.click();
        await expect(refresh).toBeFocused();
        await expect(filter).toHaveValue("AUTO");
        await expect(group).toHaveText(labels["group.byOrigin"]!);
        await expect(details(automatic)).toHaveAttribute("aria-expanded", "true");
        await panel.getByRole("combobox", named("period.label")).click();
        await page.getByRole("option", named("period.7")).click();
        await expect(panel.getByRole("status")).toHaveText(
          labels["status.loaded"]!.replace("{{period}}", labels["period.7"]!),
        );
        await expect(filter).toHaveValue("AUTO");
        await expect(details(automatic)).toHaveAttribute("aria-expanded", "true");
        await capture("retained-reading");
        await chooseGroup(4, "byScope");
        await expect(
          sources.getByText(labels["breakdown.empty.title"]!, { exact: true }),
        ).toBeVisible();
        await panel.getByRole("button", named("filter.clear")).press("Enter");
        await expect(filter).toBeFocused();
        await expect(sources.getByRole("heading")).toHaveText([labels["bucket.byScope.run"]!]);
        const run = sources.getByRole("article");
        await details(run).click();
        await expect(run.locator("dd")).toHaveText([
          "4.12",
          "24/24",
          "24/24",
          "128,400",
          "41,900",
          "0",
          "0",
          "0",
        ]);
        await run.locator("dl").scrollIntoViewIfNeeded();
        await expect(totals).toHaveText(originalTotals);
        await capture("association-facts");
        await page.keyboard.press("Escape");
        await expect(composer).toHaveValue(profile.draft);
        await composer.focus();
        await page.keyboard.press("ControlOrMeta+A");
        await page.keyboard.press("ArrowRight");
        await page.keyboard.type(profile.suffix);
        await expect(composer).toHaveValue(profile.draft + profile.suffix);
        await capture("continued-draft");
      },
    );
  }
}

test("a package template fills a draft without creating a schedule", async ({ page }) => {
  await openWorkspace(page, { state: "dock-schedules" });
  await waitForWorkspaceState(page, "dock-schedules");
  await page.getByRole("button", { name: "Schedule templates" }).click();
  await page.getByRole("menuitem", { name: "Weekly maintenance review", exact: true }).click();
  await expect(page.getByRole("textbox", { name: "Title", exact: true })).toHaveValue(
    "Weekly maintenance review",
  );
  await expect(page.getByRole("textbox", { name: "Cron expression" })).toHaveValue("0 9 * * 1");
  await expect(pluginView(page, "schedules").locator("article")).toHaveCount(2);
});

test("deleting a schedule asks first, and a declined ask changes nothing", async ({ page }) => {
  await openWorkspace(page, { state: "dock-schedules" });
  await waitForWorkspaceState(page, "dock-schedules");

  const item = pluginView(page, "schedules")
    .getByRole("article")
    .filter({ has: page.getByRole("heading", { name: "Nightly dependency audit", exact: true }) });
  await item.getByRole("button", { name: "Schedule details", exact: true }).click();
  await expect(item.getByText("0 2 * * *", { exact: true })).toBeVisible();
  const trigger = item.getByRole("button", {
    name: "More actions for Nightly dependency audit",
    exact: true,
  });
  await trigger.click();
  await page.getByRole("menuitem", { name: "Delete", exact: true }).click();

  const dialog = page.getByRole("alertdialog", { name: "Delete Nightly dependency audit?" });
  await expect(dialog).toContainText("Nightly dependency audit");
  await expect(dialog).toContainText("Captured work and accepted Runs are retained.");
  await expect(dialog.getByRole("button", { name: "Confirm deletion", exact: true })).toBeVisible();
  const cancel = dialog.getByRole("button", { name: "Cancel" });
  await expect(cancel).toBeFocused();
  await cancel.press("Enter");
  await expect(trigger).toBeFocused();

  await expect(pluginView(page, "schedules").locator("article")).toHaveCount(2);
});

test("the portable trajectory preserves canonical Tool evidence for inspection", async ({
  page,
}) => {
  await openWorkspace(page, { state: "dock-runs" });
  await waitForWorkspaceState(page, "dock-runs");
  const view = pluginView(page);
  await expect(view.locator('[data-trajectory-record="item:item_child_approval"]')).toHaveCount(0);
  const tool = view.locator('[data-trajectory-record="item:item_nested_delegate"]');
  await expect(tool.getByRole("button", { expanded: false })).toContainText("delegate_task");
  await tool.getByRole("button", { expanded: false }).click();
  await expect(tool).toContainText("Verify package dependencies");
  await expect(view.getByRole("button", { name: /^(Cancel|Allow once|Deny)$/ })).toHaveCount(0);
});

test("choosing a subagent swaps the panel's name for the way back", async ({ page }) => {
  await openWorkspace(page, { state: "dock-subagents" });
  await waitForWorkspaceState(page, "dock-subagents");

  const panel = dockView(page, "subagents");
  await expect(panel.getByRole("button", { name: "All subagents", exact: true })).toHaveCount(0);

  await panel.locator('[data-slot="delegated-run-link"] button').first().click();
  await expect(panel.getByRole("button", { name: "All subagents", exact: true })).toBeVisible();

  const placement = await panel.locator("header").evaluate((root) => {
    const name = root.querySelector('[data-slot="subagent-identity"]');
    const status = [...root.querySelectorAll("span")].find(
      (node) => node.textContent?.trim() === "Needs input",
    );
    if (!name || !status) return null;
    const identity = name.getBoundingClientRect();
    const state = status.getBoundingClientRect();
    return { inset: Math.abs(state.left - identity.left), gap: state.top - identity.bottom };
  });
  expect(placement, "the selected task keeps its status directly beneath its name").not.toBeNull();
  expect(placement!.inset).toBeLessThanOrEqual(1);
  expect(placement!.gap).toBeGreaterThanOrEqual(0);
  expect(placement!.gap).toBeLessThanOrEqual(8);
});

test("every dock view the app offers is a view some state opens", async ({ page }) => {
  await openWorkspace(page, { state: "dock-catalog" });
  await waitForWorkspaceState(page, "dock-catalog");

  const offered = await dockCatalog(page)
    .getByRole("option")
    .evaluateAll((nodes) => nodes.map((node) => (node.textContent ?? "").trim()).filter(Boolean));
  const opened = new Set(
    Object.entries(DOCK_VIEW_BY_STATE)
      .filter(([state]) => state !== "dock-catalog")
      .map(([, id]) => id),
  );

  expect(
    offered.length,
    "a view the catalogue offers and no state opens is a view no screenshot has ever taken",
  ).toBe(opened.size);
  const visited = new Set<string>();
  for (const label of offered) {
    await openWorkspace(page, { state: "dock-catalog" });
    await waitForWorkspaceState(page, "dock-catalog");
    await dockCatalog(page).getByRole("option", { name: label, exact: true }).click();
    const id = await page.getByTestId("active-dock-view").textContent();
    expect(id).not.toBeNull();
    visited.add(id!);
  }
  expect(visited).toEqual(opened);
});

test("every dock state keeps its visible view within the dock canvas", async ({ page }) => {
  for (const [state] of Object.entries(DOCK_VIEW_BY_STATE)) {
    if (state === "dock-catalog") continue;
    await openWorkspace(page, { state: state as VisualWorkspaceState });
    await waitForWorkspaceState(page, state as VisualWorkspaceState);
    const dock = page.locator(".agent-context-dock");
    const view = dock
      .locator("[data-dock-view-id]:visible")
      .filter({ has: page.locator('[data-slot="workspace-view-body"]') });
    await expect(view).toHaveCount(1);
    const [canvas, content] = await Promise.all([dock.boundingBox(), view.boundingBox()]);
    expect(canvas, state).not.toBeNull();
    expect(content, state).not.toBeNull();
    expect(content!.width, state).toBeGreaterThan(0);
    expect(content!.height, state).toBeGreaterThan(0);
    expect(content!.x, state).toBeGreaterThanOrEqual(canvas!.x);
    expect(content!.y, state).toBeGreaterThanOrEqual(canvas!.y);
    expect(content!.x + content!.width, state).toBeLessThanOrEqual(canvas!.x + canvas!.width);
    expect(content!.y + content!.height, state).toBeLessThanOrEqual(canvas!.y + canvas!.height);
  }
});

for (const answer of [
  { button: "Allow once", decision: "approve", label: "Approved" },
  { button: "Deny", decision: "deny", label: "Denied" },
] as const) {
  test(`answering an approval with ${answer.button} exposes its retained tool decision`, async ({
    page,
  }) => {
    await openWorkspace(page, { state: "dock-trajectory", agentState: "waiting" });
    await waitForWorkspaceState(page, "dock-trajectory");

    const view = pluginView(page);
    await expect(view.getByRole("button", { name: /^(Allow once|Deny)$/ })).toHaveCount(0);

    const workbench = page.locator("main");
    await workbench.getByRole("button", { name: answer.button, exact: true }).click();
    await expect(workbench.getByRole("button", { name: answer.button, exact: true })).toHaveCount(
      0,
    );

    await view.getByRole("button", { name: "Refresh" }).click();
    const settled = view.locator('[data-trajectory-record="item:item_approval"]');
    await expect(settled.getByRole("button", { expanded: false })).toContainText(
      `Approval: ${answer.label}`,
    );
    await settled.getByRole("button", { expanded: false }).click();
    await expect(settled).toContainText("go test -race ./...");
    await expect(settled.getByText("Approval", { exact: true })).toBeVisible();
    await expect(settled.getByText(answer.label, { exact: true })).toBeVisible();
    await expect(
      settled.getByRole("region", { name: "Recorded evidence", exact: true }),
    ).toContainText(`"approvalDecision": "${answer.decision}"`);
  });
}

test("memory cards keep state compact and preserve full record evidence behind disclosure", async ({
  page,
}) => {
  await openWorkspace(page, { state: "dock-agent-memory" });
  const view = pluginView(page, "memory");
  const card = view.getByRole("article").first();
  const badge = card.getByText("Pending review", { exact: true });
  await expect(badge).toBeVisible();
  const cardWidth = await card.evaluate((node) => node.getBoundingClientRect().width);
  const badgeWidth = await badge.evaluate((node) => node.getBoundingClientRect().width);
  expect(badgeWidth).toBeLessThan(cardWidth / 2);
  const id = card.getByText("mem_00000000000000000000000000000002", { exact: true });
  await expect(id).toBeHidden();
  await card.getByRole("button", { name: "Record details Pending review", exact: true }).click();
  await expect(id).toBeVisible();
  await expect(card.getByText("ses_visual", { exact: true })).toBeVisible();
  await expect(view.getByRole("article")).toHaveCount(2);
});

for (const theme of ["light", "dark"] as const) {
  test(`Schedule menus keep actions aligned with identity and details independent ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1120, height: 720 });
    await openWorkspace(page, { state: "dock-schedules", theme });
    await waitForWorkspaceState(page, "dock-schedules");
    const card = pluginView(page, "schedules").getByRole("article").first();
    const details = card.getByRole("button", { name: "Schedule details", exact: true });
    const title = card.getByRole("heading", { name: "Nightly dependency audit", exact: true });
    const menu = card.getByRole("button", {
      name: "More actions for Nightly dependency audit",
      exact: true,
    });
    const summary = (await title.boundingBox())!;
    const action = (await menu.boundingBox())!;
    expect(
      Math.min(summary.y + summary.height, action.y + action.height) -
        Math.max(summary.y, action.y),
    ).toBeGreaterThan(0);
    expect(action.x).toBeGreaterThanOrEqual(summary.x + summary.width);
    await expect(details).toHaveAttribute("aria-expanded", "false");
    await menu.press("Enter");
    await expect(page.getByRole("menuitem", { name: "Run now", exact: true })).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(menu).toBeFocused();
    await expect(details).toHaveAttribute("aria-expanded", "false");
    await details.click();
    await expect(details).toHaveAttribute("aria-expanded", "true");
    await expect(card.getByText("sch_nightly", { exact: true })).toBeVisible();
  });

  test(`portable editors keep visible field labels and one save action ${theme}`, async ({
    page,
  }) => {
    await openWorkspace(page, { state: "dock-schedules", theme });
    await waitForWorkspaceState(page, "dock-schedules");
    await page.getByRole("button", { name: "Add schedule", exact: true }).click();
    const schedule = page.getByRole("dialog", { name: "New schedule", exact: true });
    const save = schedule.getByRole("button", { name: "Save schedule", exact: true });
    const cancel = schedule.getByRole("button", { name: "Cancel", exact: true });
    for (const label of [
      "Title",
      "Instructions",
      "Cron expression",
      "Time zone",
      "Workspace path",
      "Model",
      "Reasoning effort",
    ]) {
      const field = schedule.locator("label").getByText(label, { exact: true });
      await field.scrollIntoViewIfNeeded();
      await expect(field).toBeVisible();
      await expect(save).toBeVisible();
      await expect(cancel).toBeVisible();
    }
    await expect(
      page.getByRole("button", { name: "Add schedule", exact: true, includeHidden: true }),
    ).toBeDisabled();
    await expect(save).toHaveCount(1);

    await openWorkspace(page, { state: "dock-agent-memory", theme });
    await waitForWorkspaceState(page, "dock-agent-memory");
    await page.getByRole("button", { name: "Add memory", exact: true }).click();
    const memory = page.getByRole("region", { name: "Add memory", exact: true });
    await expect(
      memory.locator("label").getByText("Memory content", { exact: true }),
    ).toBeVisible();
    await expect(memory.getByRole("textbox", { name: "Memory content", exact: true })).toHaveValue(
      "",
    );
    await expect(page.getByRole("button", { name: "Add memory", exact: true })).toBeDisabled();
    await expect(memory.getByRole("button", { name: "Save", exact: true })).toHaveCount(1);
  });

  test(`portable Markdown keeps its reading hierarchy and captured content ${theme}`, async ({
    page,
  }) => {
    await openWorkspace(page, { state: "dock-files", theme });
    await page.getByRole("treeitem", { name: "README.md", exact: true }).click();
    const document = page.locator('[data-slot="document-body"]');
    const view = dockView(page, "file");
    await expect(view.getByRole("heading", { name: "README.md", exact: true })).toHaveCount(0);
    await expect(view.getByText("Captured file content", { exact: true })).toHaveCount(0);
    const header = view.locator('[data-slot="workspace-view-header"]');
    await expect(header).toContainText("24 lines");
    await expect(view.locator('[data-slot="file-path"]')).toHaveText("README.md");
    await expect(
      document.getByRole("heading", { name: "Workspace notes", exact: true }),
    ).toBeVisible();
    const introduction = await document.evaluate((node) => {
      const host = node.closest('[data-dock-view-id="file"]')!;
      const path = host.querySelector('[data-slot="file-path"]')!;
      const content = node.closest('[data-slot="panel"]')!.firstElementChild!;
      return {
        gap:
          node.querySelector("h1")!.getBoundingClientRect().top -
          path.getBoundingClientRect().bottom,
        inset: Number.parseFloat(getComputedStyle(content).paddingBlockStart),
      };
    });
    expect(Math.abs(introduction.gap - introduction.inset)).toBeLessThanOrEqual(1);
    await expect(document.getByRole("listitem")).toHaveCount(3);
    await expect(document.getByRole("table")).toContainText("Memory");
    const sizes = await document.evaluate((node) =>
      ["h1", "h2", "h3"].map((tag) =>
        parseFloat(getComputedStyle(node.querySelector(tag)!).fontSize),
      ),
    );
    expect(sizes[0]).toBeGreaterThan(sizes[1]!);
    expect(sizes[1]).toBeGreaterThanOrEqual(sizes[2]!);
    const source = document.getByRole("region", { name: /Code block/ });
    await expect(document.getByText("ts", { exact: true })).toBeVisible();
    await expect(source).toHaveText('const outcome = "recorded";');
    await source.focus();
    await expect(source).toBeFocused();
    const accessibility = await new AxeBuilder({ page })
      .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
      .analyze();
    expect(accessibility.violations).toEqual([]);
  });
}

test("bounded schedule fields and submission remain reachable by keyboard", async ({ page }) => {
  await page.setViewportSize({ width: 1120, height: 560 });
  await openWorkspace(page, { state: "dock-schedules" });
  await waitForWorkspaceState(page, "dock-schedules");
  await page.getByRole("button", { name: "Add schedule", exact: true }).click();
  const editor = page.getByRole("dialog", { name: "New schedule", exact: true });
  const viewport = editor.locator('[data-slot="dialog-body"]');
  const title = editor.getByRole("textbox", { name: "Title", exact: true });
  const instructions = editor.getByRole("textbox", { name: "Instructions", exact: true });
  const cron = editor.getByRole("textbox", { name: "Cron expression", exact: true });
  const timeZone = editor.getByRole("textbox", { name: "Time zone", exact: true });
  const workspace = editor.getByRole("textbox", { name: "Workspace path", exact: true });
  const model = editor.getByRole("combobox", { name: "Model", exact: true });
  const save = editor.getByRole("button", { name: "Save schedule", exact: true });
  const cancel = editor.getByRole("button", { name: "Cancel", exact: true });
  await expect
    .poll(() => viewport.evaluate((node) => node.scrollHeight - node.clientHeight))
    .toBeGreaterThan(0);
  await expect(title).toBeFocused();
  await title.press("Tab");
  await expect(instructions).toBeFocused();
  await instructions.fill("Review dependencies and summarize actionable changes.");
  await instructions.press("Tab");
  await expect(cron).toBeFocused();
  await cron.fill("0 9 * * 1");
  await cron.press("Tab");
  await expect(timeZone).toBeFocused();
  await expect(timeZone).toHaveValue("UTC");
  await timeZone.press("Tab");
  await expect(workspace).toBeFocused();
  await workspace.press("Tab");
  await expect(editor.getByRole("checkbox", { name: "Enabled" })).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(model).toBeFocused();
  await expect.poll(() => viewport.evaluate((node) => node.scrollTop)).toBeGreaterThan(0);
  await expect(save).toBeVisible();
  await expect(cancel).toBeVisible();
  await model.press("Tab");
  await expect(editor.getByRole("button", { name: "Refresh model choices" })).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(cancel).toBeFocused();
  await cancel.press("Tab");
  await expect(save).toBeFocused();
});
