import { expect, test, type Page } from "./test";
import { freezeVisualClock } from "./frozenClock";
import type { VisualAgentState } from "./agentSessionSnapshots";
import { en } from "@/lib/i18n/locales/en";
import {
  DOCK_MIN_WIDTH_PX,
  CONVERSATION_READING_MIN_PX,
  defaultDockWidth,
} from "@/lib/shellGeometry";
import {
  VISUAL_DOCK_WIDTH_RATIO,
  DOCK_VIEW_BY_STATE,
  VISUAL_REVIEW_VIEWPORT,
  VISUAL_SETTINGS_PANES,
  VISUAL_WORKSPACE_STATES,
  VISUAL_WORKSPACE_VIEWPORT,
  type VisualSettingsPane,
  type VisualWorkspaceState,
  type VisualWorkspaceTheme,
} from "./workspaceFixtureStates";

function pluginViewFrame(page: Page, view = "trajectory") {
  return page
    .locator(`[data-dock-view-id="package:visual:${view}"]`)
    .frameLocator("iframe")
    .frameLocator("iframe");
}

const SETTINGS_SEARCH = { name: en["settings.searchPlaceholder"]! };
const ACTIVE_FILE_PATH =
  "desktop/frontend/src/plugins/builtin/shell/workbench/panel/DockResizer.tsx";

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
  await page.goto(`/visual/?${query}`);
  await page.locator("html[data-visual-ready]").waitFor();
  await expect(page.getByTestId("workspace-state")).toHaveAttribute("data-state", route.state);
}

test("a full view sets its title in mono only when the title is a path", async ({ page }) => {
  const readTitle = () =>
    page
      .locator("main .agent-surface-header:visible span")
      .first()
      .evaluate((title) => {
        const head = (stack: string) =>
          stack
            .split(",")[0]!
            .trim()
            .replace(/^["']|["']$/g, "");
        const mono = head(
          getComputedStyle(document.documentElement).getPropertyValue("--font-mono"),
        );
        const family = title ? getComputedStyle(title).fontFamily : "";
        return { text: title?.textContent?.trim() ?? "", isMono: head(family) === mono, family };
      });

  await openWorkspace(page, { state: "full-view", fullView: "diff" });
  const prose = await readTitle();
  expect(prose.text).toBe(en["diff.workingTree"]);
  expect(prose.isMono, `a view's NAME is prose, got ${prose.family}`).toBe(false);

  await openWorkspace(page, { state: "full-view", fullView: "file" });
  const path = await readTitle();
  expect(path.text).toBe(ACTIVE_FILE_PATH);
  expect(path.isMono, `a PATH is mono, got ${path.family}`).toBe(true);
});

test("the icon gallery searches Flame glyphs by name and concept", async ({ page }) => {
  await openWorkspace(page, { state: "full-view", fullView: "icon-gallery" });
  const search = page.getByRole("searchbox", { name: en["iconGallery.filterLabel"]! });
  await expect(search).toBeVisible();
  await expect(page.getByText(en["iconGallery.title"]!, { exact: true })).toBeVisible();
  await search.fill("file-diff");
  await expect(page.getByText("FileDiff", { exact: true })).toBeVisible();
  await expect(page.getByText("FileText", { exact: true })).toHaveCount(0);
  await search.fill("patch");
  await expect(page.getByText("FileDiff", { exact: true })).toBeVisible();
  await search.fill("no-such-flame-glyph");
  await expect(
    page.getByText(en["iconGallery.empty"]!.replace("{{q}}", "no-such-flame-glyph"), {
      exact: true,
    }),
  ).toBeVisible();
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
    await expect(page.locator("[data-diff-file]")).toHaveCount(2);
    await page.locator('[data-diff-file] span[style*="color"]').first().waitFor();
    return;
  }
  if (state === "dock-empty") {
    await expect(page.getByText("Nothing to compare", { exact: true })).toBeVisible();
    return;
  }
  if (state === "dock-loading") {
    await expect(
      page.locator(".agent-context-dock [data-dock-view-id]:visible output[aria-busy=true]"),
    ).toBeVisible();
    return;
  }
  if (state === "dock-runs" || state === "dock-trajectory") {
    const view = pluginViewFrame(page);
    await expect(view.getByRole("heading", { name: "Session trajectory" })).toBeVisible();
    await expect(view.getByText(/recorded observations loaded on this page/)).toBeVisible();
    await expect(view.locator('[data-trajectory-kind="run"]')).toHaveCount(
      state === "dock-runs" ? 7 : 1,
    );
    await expect(
      view.locator('[data-trajectory-record="model:call_visual_recovered"]'),
    ).toContainText("unknown");
    return;
  }
  if (state === "dock-usage") {
    await expect(pluginViewFrame(page, "usage").locator("#status")).toHaveText(
      "Recorded usage loaded.",
    );
    return;
  }
  if (state === "dock-schedules") {
    await expect(
      pluginViewFrame(page, "schedules").getByText("1 enabled · 1 disabled on this page"),
    ).toBeVisible();
    return;
  }
  if (state === "dock-agent-memory") {
    await expect(
      pluginViewFrame(page, "memory").getByText("1 pending · 1 active on this page"),
    ).toBeVisible();
    return;
  }
  const CATALOGUE_READY: Partial<Record<VisualWorkspaceState, string>> = {
    "dock-subagents": "Sub-agent 4 of 4",
    "dock-diagnostics": "run + RPC spans appear here",
    "dock-skills": "2 available",
    "dock-feature-off": "Skills are off",
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
    await expect(page.getByText("Couldn't load the diff", { exact: true })).toBeVisible();
    return;
  }
  if (state === "dock-file") {
    const view = page.locator(".agent-workspace-view:visible");
    await expect(view).toContainText("8 lines");
    await expect(view).toContainText("clampDockWidth(currentWidth + delta, row.clientWidth)");
    return;
  }
  if (state === "dock-catalog") {
    await expect(page.getByText(en["dock.catalog.title"]!, { exact: true })).toBeVisible();
    await expect(
      page.locator(".agent-context-dock").getByRole("button", { name: "Files" }),
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
    await expect(page.getByRole("heading").first()).toBeVisible();
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
  await expect(page.getByTestId("dock-view-ids")).toHaveText("file,diff,package:visual:trajectory");
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
  await expect(page.getByTestId("dock-view-ids")).toHaveText("file,diff,package:visual:trajectory");
  expect(page.url()).toBe(location);
  expect(await page.evaluate(() => history.length)).toBe(historyLength);
  await page.setViewportSize({ width: 1520, height: 900 });
  await expect(page.locator(".agent-dock-row")).toHaveAttribute("data-dock", "open");
  await expect(page.getByTestId("active-dock-view")).toHaveText("file");
  await expect(page.getByRole("tab", { name: "File" })).toBeVisible();
});

test("the model remains readable when the dock narrows the composer", async ({ page }) => {
  await openWorkspace(page, { state: "dock-review" });
  const model = page.getByRole("button", { name: "Switch model" });
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
  await expect(page.getByTestId("active-dock-view")).toHaveText("diff");
  await expect(page.getByTestId("dock-open")).toHaveText("true");

  await page.getByRole("tab", { name: "Diff" }).hover();
  await page.getByRole("button", { name: "Close Diff" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("file");
  await expect(page.getByTestId("dock-view-ids")).toHaveText("file");
});

test("add-panel menu restores a closed singleton and focuses it", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  await page.getByRole("tab", { name: "Session trajectory" }).hover();
  await page.getByRole("button", { name: "Close Session trajectory" }).click();
  await expect(page.getByTestId("dock-view-ids")).not.toContainText("package:visual:trajectory");

  await page.getByRole("button", { name: "Browse panels" }).click();

  const onTop = await page.locator("[role=combobox]").evaluate((input) => {
    const panel = input.closest("[role=dialog], div[class*='z-50']") ?? input.parentElement!;
    const box = panel.getBoundingClientRect();
    const hit = document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2);
    return panel.contains(hit);
  });
  expect(onTop).toBe(true);

  await page.getByRole("combobox").fill("Session trajectory");
  await page.getByRole("option", { name: "Session trajectory" }).waitFor();
  await page.keyboard.press("Enter");

  await expect(page.getByTestId("active-dock-view")).toHaveText("package:visual:trajectory");
  await expect(page.getByTestId("dock-view-ids")).toHaveText("file,diff,package:visual:trajectory");
});

test("files browse and preview share one dock tab", async ({ page }) => {
  await openWorkspace(page, { state: "dock-files" });
  const dock = page.locator(".agent-context-dock");
  await dock.getByRole("treeitem", { name: "app", exact: true }).click();
  await dock.getByRole("treeitem", { name: "resizer.ts", exact: true }).click();
  await expect(dock).toContainText("clampDockWidth(currentWidth + delta, row.clientWidth)");
  await expect(dock.getByRole("tab", { name: "Files", exact: true })).toHaveCount(1);
  await dock.getByRole("button", { name: "Back to files" }).click();
  await expect(dock.getByRole("treeitem", { name: "app", exact: true })).toHaveAttribute(
    "aria-expanded",
    "true",
  );
  await expect(dock.getByRole("treeitem", { name: "resizer.ts", exact: true })).toBeVisible();
  await expect(page.getByTestId("active-dock-view")).toHaveText("file");
});

test("skills keeps discovery, review, and personal curation in one dock tab", async ({ page }) => {
  await openWorkspace(page, { state: "dock-skills" });
  const view = page.locator(".agent-workspace-view:visible");
  const sections = view.getByRole("tablist", { name: "Skills" });
  await expect(view).toContainText("2 available");

  await sections.getByRole("tab", { name: "Available" }).focus();
  await page.keyboard.press("ArrowRight");
  await expect(sections.getByRole("tab", { name: "Review" })).toBeFocused();
  await expect(view).toContainText("2 awaiting review");
  await view.getByRole("button", { name: "Read instructions" }).first().click();
  await expect(view).toContainText("Start from the riskiest hunk.");
  await expect(view.getByRole("button", { name: "Approve", exact: true })).toHaveCount(2);
  await expect(view.getByRole("button", { name: "Reject", exact: true })).toHaveCount(2);

  await sections.getByRole("tab", { name: "Personal" }).click();
  await expect(view).toContainText("1 active · 1 archived");
  await expect(view.getByRole("button", { name: "Archive", exact: true })).toBeVisible();
  await expect(view.getByRole("button", { name: "Restore", exact: true })).toBeVisible();
  await expect(page.getByTestId("active-dock-view")).toHaveText("skills");

  await sections.getByRole("tab", { name: "Available" }).click();
  await expect(view).toContainText("2 available");
  await expect(view.getByRole("button", { name: "Approve", exact: true })).toHaveCount(0);
});

for (const theme of ["light", "dark"] as const) {
  for (const section of ["Review", "Personal"] as const) {
    test(`skills ${section.toLowerCase()} golden ${theme}`, async ({ page }) => {
      await openWorkspace(page, { state: "dock-skills", theme });
      const view = page.locator(".agent-workspace-view:visible");
      await view.getByRole("tab", { name: section, exact: true }).click();
      await expect(view).toContainText(section === "Review" ? "2 awaiting review" : "1 active");
      await expect(page).toHaveScreenshot(`skills-${section.toLowerCase()}-${theme}.png`);
    });
  }
}

test("dock tabs use roving focus and arrow-key activation", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  const file = page.getByRole("tab", { name: "File" });
  await file.focus();
  await file.press("ArrowRight");

  await expect(page.getByTestId("active-dock-view")).toHaveText("diff");
  await expect(page.getByRole("tab", { name: "Diff" })).toBeFocused();
  await expect(page.getByRole("tab", { name: "Diff" })).toHaveAttribute("data-active", "");
});

test("the active overflow tab stays visible and both hidden edges remain signposted", async ({
  page,
}) => {
  await openWorkspace(page, { state: "dock-agent-memory" });
  await page.getByRole("tab", { name: "Diff" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("diff");

  const separator = page.getByRole("separator", { name: "Resize right workspace" });
  await separator.focus();
  await separator.press("Home");

  const strip = page.locator(".agent-dock-tabs");
  await expect(strip).toHaveAttribute("data-overflow-start", "");
  await expect(strip).toHaveAttribute("data-overflow-end", "");
  const [stripBox, activeBox] = await Promise.all([
    strip.boundingBox(),
    page.getByRole("tab", { name: "Diff" }).boundingBox(),
  ]);
  expect(stripBox).not.toBeNull();
  expect(activeBox).not.toBeNull();
  expect(activeBox!.x).toBeGreaterThanOrEqual(stripBox!.x);
  expect(activeBox!.x + activeBox!.width).toBeLessThanOrEqual(stripBox!.x + stripBox!.width);
});

test("file and portable trajectory tabs render through their production views", async ({
  page,
}) => {
  await openWorkspace(page, { state: "dock-light" });

  await page.getByRole("tab", { name: "File" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("file");
  const fileView = page.locator('[data-dock-view-id="file"]');
  await expect(fileView.getByTitle(ACTIVE_FILE_PATH)).toBeVisible();
  await expect(fileView.getByText(/const currentWidth = readDockWidth/)).toBeVisible();

  await page.getByRole("tab", { name: "Session trajectory" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("package:visual:trajectory");
  await expect(fileView.getByText(/const currentWidth = readDockWidth/)).toBeHidden();
  await expect(
    pluginViewFrame(page).locator('[data-trajectory-record="run:run_root"]'),
  ).toBeVisible();
  await expect(
    pluginViewFrame(page).locator("summary").filter({ hasText: "run_root" }).first(),
  ).toBeVisible();
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

  await page.getByRole("tab", { name: "Diff" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("diff");
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
  await expect(page.getByRole("menuitem", { name: "Light" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("menuitem", { name: "Light" })).toHaveCount(0);
  await expect(theme).toBeFocused();
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
  await page.locator("html[data-visual-ready]").waitFor();
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
    .toBe("#6d3ff0");
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
  await page.locator("html[data-visual-ready]").waitFor();
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

  const utilityModel = page.getByRole("button", { name: "Utility model" });
  await utilityModel.click();
  await expect(page.getByRole("menuitem", { name: /GPT-5.6/ })).toBeVisible();
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
  await expect(add).toHaveAttribute("title", "Browse panels");

  await add.click();
  await expect(page.getByRole("listbox")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("listbox")).toHaveCount(0);
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

test("plugin notifications use the production toast and dismiss automatically", async ({
  page,
}) => {
  await openWorkspace(page, { state: "settings" });

  await page.evaluate(() => {
    const host = window as unknown as {
      flameVisualNotify?: (message: string, level?: "info" | "warn" | "error") => void;
    };
    if (!host.flameVisualNotify) throw new Error("the fixture notifier plugin did not install");
    host.flameVisualNotify("Provider credentials were rejected", "error");
  });

  const toast = page.locator("[data-sonner-toast]");
  await expect(toast).toContainText("Provider credentials were rejected");
  await expect(toast).toHaveAttribute("data-type", "error");

  await expect(toast).toHaveScreenshot("toast-error.png");

  await expect.poll(() => toast.count(), { timeout: 6_000 }).toBe(0);
});

test("workspace surfaces do not create page-level horizontal overflow", async ({ page }) => {
  await openWorkspace(page, { state: "dock-review" });
  await waitForWorkspaceState(page, "dock-review");

  const overflow = await page.locator("html").evaluate((element) => {
    return element.scrollWidth - element.clientWidth;
  });
  expect(overflow).toBeLessThanOrEqual(0);
});

for (const theme of ["light", "dark"] as const) {
  for (const state of VISUAL_WORKSPACE_STATES) {
    test(`workspace golden ${theme} ${state}`, async ({ page }) => {
      if (state === "dock-review") {
        await page.setViewportSize(VISUAL_REVIEW_VIEWPORT);
      }
      await openWorkspace(page, { state, theme });
      await waitForWorkspaceState(page, state);
      await page.waitForFunction(() => {
        const scroller = document.querySelector(".msg-scroll-viewport");
        if (!scroller) return true;
        scroller.scrollTop = scroller.scrollHeight;
        const probe = window as unknown as { settle?: { top: number; frames: number } };
        const settle = (probe.settle ??= { top: -1, frames: 0 });
        if (scroller.scrollTop === settle.top) settle.frames += 1;
        else {
          settle.top = scroller.scrollTop;
          settle.frames = 0;
        }
        return settle.frames >= 5;
      });
      await freezeVisualClock(page);
      await expect(page).toHaveScreenshot(`workspace-${theme}-${state}.png`);
    });
  }
}

for (const pane of VISUAL_SETTINGS_PANES) {
  test(`workspace golden settings pane ${pane}`, async ({ page }) => {
    await openWorkspace(page, { state: "settings", pane });
    await waitForWorkspaceState(page, "settings");
    if (pane === "providers") {
      await expect(page.getByRole("button", { name: en["providers.utility.title"]! })).toHaveText(
        "GPT-5.6 Sol",
      );
      await expect(page.getByRole("button", { name: en["providers.embedding.title"]! })).toHaveText(
        en["providers.embedding.off"]!,
      );
    }
    const emptyLabel = {
      "mcp-servers": en["mcp.empty"],
      hooks: en["hooks.empty"],
      plugins: en["packages.empty"],
    };
    if (pane in emptyLabel) {
      await expect(
        page.getByText(emptyLabel[pane as keyof typeof emptyLabel]!, { exact: true }),
      ).toBeVisible();
    }
    await expect(page).toHaveScreenshot(`workspace-light-settings-${pane}.png`);
  });
}

test("the portable Usage page filters recorded buckets without changing totals", async ({
  page,
}) => {
  await openWorkspace(page, { state: "dock-usage" });
  await waitForWorkspaceState(page, "dock-usage");
  const view = pluginViewFrame(page, "usage");
  await expect(view.locator("tbody tr")).toHaveCount(2);
  const totals = await view.locator("#totals").textContent();
  await view.getByRole("searchbox", { name: "Filter" }).fill("anthropic");
  await expect(view.locator("tbody tr")).toHaveCount(1);
  await expect(view.locator("#totals")).toHaveText(totals!);
  await view.getByRole("searchbox", { name: "Filter" }).fill("");
  await view.getByRole("combobox", { name: "Group by" }).selectOption("byModel");
  await expect(view.locator("tbody")).toContainText("openai/gpt-5.6-sol");
  await page.getByRole("tab", { name: "7d", exact: true }).click();
  await waitForWorkspaceState(page, "dock-usage");
  await expect(view.locator("#totals")).toHaveText(totals!);
});

test("a package template fills a draft without creating a schedule", async ({ page }) => {
  await openWorkspace(page, { state: "dock-schedules" });
  await waitForWorkspaceState(page, "dock-schedules");
  await page.getByRole("button", { name: "Use a template" }).click();
  await page.getByRole("menuitem", { name: /Weekly maintenance review/ }).click();
  await expect(page.getByRole("textbox", { name: "Title (optional)" })).toHaveValue(
    "Weekly maintenance review",
  );
  await expect(page.getByRole("textbox", { name: "Cron expression" })).toHaveValue("0 9 * * 1");
  await expect(pluginViewFrame(page, "schedules").locator("article")).toHaveCount(2);
});

test("deleting a schedule asks first, and a declined ask changes nothing", async ({ page }) => {
  await openWorkspace(page, { state: "dock-schedules" });
  await waitForWorkspaceState(page, "dock-schedules");

  await page.getByRole("button", { name: "Select a schedule" }).click();
  await page.getByRole("menuitem", { name: /sch_nightly/ }).click();
  await page.getByRole("button", { name: "Delete schedule" }).click();

  const dialog = page.getByRole("alertdialog");
  await expect(dialog).toContainText("Nightly dependency audit");
  await expect(dialog).toContainText("cannot be undone");
  await dialog.getByRole("button", { name: "Cancel" }).click();

  await expect(pluginViewFrame(page, "schedules").locator("article")).toHaveCount(2);
});

test("the portable trajectory preserves canonical Tool evidence for inspection", async ({
  page,
}) => {
  await openWorkspace(page, { state: "dock-runs" });
  await waitForWorkspaceState(page, "dock-runs");
  const view = pluginViewFrame(page);
  await expect(view.locator('[data-trajectory-record="item:item_child_approval"]')).toHaveCount(0);
  const tool = view.locator('[data-trajectory-record="item:item_nested_delegate"]');
  await expect(tool.locator("summary")).toContainText("delegate_task");
  await tool.locator("summary").click();
  await expect(tool).toContainText("Verify package dependencies");
  await expect(view.getByRole("button", { name: /Cancel|Allow once|Deny/ })).toHaveCount(0);
});

test("choosing a subagent swaps the panel's name for the way back", async ({ page }) => {
  await openWorkspace(page, { state: "dock-subagents" });
  await waitForWorkspaceState(page, "dock-subagents");

  const panel = page.locator('[data-dock-view-id="subagents"]');
  await expect(panel.getByRole("button", { name: "Subagents", exact: true })).toHaveCount(0);

  await panel.locator('[data-slot="delegated-run-link"] button').first().click();
  await expect(panel.getByRole("button", { name: "Subagents", exact: true })).toBeVisible();

  const gap = await panel
    .locator('[role="region"]')
    .first()
    .evaluate((root) => {
      const name = root.querySelector("span[title]");
      const status = [...root.querySelectorAll("span")].find(
        (node) => node.textContent?.trim() === "Needs input",
      );
      if (!name || !status) return null;
      const written = document.createRange();
      written.selectNodeContents(name);
      return Math.round(
        status.getBoundingClientRect().left - written.getBoundingClientRect().right,
      );
    });
  expect(
    gap,
    "a run's status reads with the run it names, not at the pane's far edge",
  ).toBeLessThan(40);
});

test("every dock view the app offers is a view some state opens", async ({ page }) => {
  await openWorkspace(page, { state: "dock-catalog" });
  await waitForWorkspaceState(page, "dock-catalog");

  const offered = await page
    .locator(".agent-context-dock")
    .getByRole("button")
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
});

test("every dock state renders its view inside the frame that paints one", async ({ page }) => {
  const unframed: string[] = [];
  for (const [state] of Object.entries(DOCK_VIEW_BY_STATE)) {
    if (state === "dock-catalog") continue;
    await openWorkspace(page, { state: state as VisualWorkspaceState });
    await waitForWorkspaceState(page, state as VisualWorkspaceState);
    const framed = await page.locator(".agent-context-dock .agent-workspace-view").count();
    if (framed === 0) unframed.push(state);
  }
  expect(
    unframed,
    "`.agent-workspace-view` names the container a view measures against and paints its canvas",
  ).toEqual([]);
});

for (const answer of [
  { button: "Allow once", decision: "approve" },
  { button: "Deny", decision: "deny" },
] as const) {
  test(`answering an approval with ${answer.button} exposes its retained tool decision`, async ({
    page,
  }) => {
    await openWorkspace(page, { state: "dock-trajectory", agentState: "waiting" });
    await waitForWorkspaceState(page, "dock-trajectory");

    const view = pluginViewFrame(page);
    await expect(view.getByRole("button", { name: /Allow once|Deny/ })).toHaveCount(0);

    const workbench = page.locator("main");
    await workbench.getByRole("button", { name: answer.button, exact: true }).click();
    await expect(workbench.getByRole("button", { name: answer.button, exact: true })).toHaveCount(
      0,
    );

    await view.getByRole("button", { name: "Refresh" }).click();
    const settled = view.locator('[data-trajectory-record="item:item_approval"]');
    await expect(settled.locator("summary")).toContainText(answer.decision);
    await settled.locator("summary").click();
    await expect(settled).toContainText("go test -race ./...");
    await expect(settled.getByText("Approval", { exact: true })).toBeVisible();
    await expect(settled.getByText(answer.decision, { exact: true }).last()).toBeVisible();
  });
}
