import { expect, test, type Page } from "./test";
import { freezeVisualClock } from "./frozenClock";
import { en } from "@/lib/i18n/locales/en";
import { TOOL_ICON_BY_NAME } from "@/lib/toolFamilies";
import { DOCK_MIN_WIDTH_PX, DOCK_SAFE_AREA_PX } from "@/lib/shellGeometry";
import {
  VISUAL_DOCK_WIDTH_RATIO,
  VISUAL_REVIEW_VIEWPORT,
  VISUAL_WORKSPACE_STATES,
  VISUAL_WORKSPACE_VIEWPORT,
  type VisualSettingsPane,
  type VisualWorkspaceState,
  type VisualWorkspaceTheme,
} from "./workspaceFixtureStates";

const SETTINGS_SEARCH = { name: en["settings.searchPlaceholder"]! };
const ACTIVE_FILE_PATH = "desktop/frontend/src/plugins/builtin/shell/kernel/panel/DockResizer.tsx";

test.use({ viewport: VISUAL_WORKSPACE_VIEWPORT });

interface WorkspaceRoute {
  state: VisualWorkspaceState;
  theme?: VisualWorkspaceTheme;
  pane?: VisualSettingsPane;
  fullView?: string;
}

async function openWorkspace(page: Page, route: WorkspaceRoute): Promise<void> {
  if (route.state === "settings") await page.setViewportSize({ width: 1120, height: 720 });
  const query = new URLSearchParams({
    fixture: "workspace",
    theme: route.theme ?? "light",
    state: route.state,
  });
  if (route.pane) query.set("pane", route.pane);
  if (route.fullView) query.set("full-view", route.fullView);
  await page.goto(`/visual/?${query}`);
  await page.locator("html[data-visual-ready]").waitFor();
  await expect(page.getByTestId("workspace-state")).toHaveAttribute("data-state", route.state);
}

test("a full view sets its title in mono only when the title is a path", async ({ page }) => {
  const readTitle = () =>
    page.evaluate(() => {
      const head = (stack: string) =>
        stack
          .split(",")[0]!
          .trim()
          .replace(/^["']|["']$/g, "");
      const title = document.querySelector("main .agent-surface-header span");
      const mono = head(getComputedStyle(document.documentElement).getPropertyValue("--font-mono"));
      const family = title ? getComputedStyle(title).fontFamily : "";
      return { text: title?.textContent?.trim() ?? "", isMono: head(family) === mono, family };
    });

  await openWorkspace(page, { state: "full-view" });
  const prose = await readTitle();
  expect(prose.text).toBe(en["search.title"]);
  expect(prose.isMono, `a view's NAME is prose, got ${prose.family}`).toBe(false);

  await openWorkspace(page, { state: "full-view", fullView: "file" });
  const path = await readTitle();
  expect(path.text).toBe(ACTIVE_FILE_PATH);
  expect(path.isMono, `a PATH is mono, got ${path.family}`).toBe(true);
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
    await expect(page.getByRole("tab", { name: "Plan" })).toHaveAttribute("data-active", "");
    await expect(page.getByText("Task plan", { exact: true })).toBeVisible();
    return;
  }
  if (state === "dock-review") {
    await expect(page.locator("[data-diff-file]")).toHaveCount(2);
    await page.locator('[data-diff-file] span[style*="color"]').first().waitFor();
    return;
  }
  if (state === "dock-inbox") {
    await expect(page.getByText("Which database should the migration target?")).toBeVisible();
    await expect(page.getByText("+2", { exact: true })).toBeVisible();
    return;
  }
  if (state === "dock-tools") {
    const view = page.locator(".agent-workspace-view:visible");
    const listing = await view.innerText();
    expect(listing.indexOf("Shell")).toBeLessThan(listing.indexOf("Files"));
    expect(listing.indexOf("Files")).toBeLessThan(listing.indexOf("Search"));
    await expect(view).toContainText("acme_deploy");
    expect(listing.indexOf("Other")).toBeGreaterThan(listing.indexOf("Recall"));
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
  if (state === "dock-runs") {
    const view = page.locator(".agent-workspace-view:visible");
    await expect(view).toContainText("7 runs");
    await expect(view).toContainText("parent run_child");
    await expect(view).toContainText("Tool started");
    for (const status of ["Canceled", "Error", "Limit reached", "Finished"]) {
      await expect(view.getByText(status, { exact: true }).first()).toBeVisible();
    }
    return;
  }
  if (state === "dock-timeline") {
    const view = page.locator(".agent-workspace-view:visible");
    await expect(view).toContainText("8 events");
    await expect(view.getByRole("img", { name: "err" })).toBeVisible();
    return;
  }
  const CATALOGUE_READY: Partial<Record<VisualWorkspaceState, string>> = {
    "dock-recipes": "2 available",
    "dock-agent-docs": "3 found",
    "dock-skills": "2 available",
    "dock-knowledge": "2 scopes",
    "dock-agent-memory": "1 pending",
    "dock-feature-off": "Skills are off",
    "dock-notifications": "No notifications",
  };
  const catalogueReady = CATALOGUE_READY[state];
  if (catalogueReady !== undefined) {
    await expect(page.locator(".agent-workspace-view:visible")).toContainText(catalogueReady);
    return;
  }
  if (state === "dock-search") {
    await expect(page.locator(".agent-workspace-view:visible")).toContainText(
      "regex over the session workspace",
    );
    return;
  }
  if (state === "dock-explorer") {
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
      page.locator(".agent-context-dock").getByRole("button", { name: "Explorer" }),
    ).toBeVisible();
    return;
  }
  if (state === "full-view") {
    await expect(
      page.getByRole("main").getByRole("searchbox", { name: en["search.aria"]! }),
    ).toBeVisible();
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
  await expect(page.getByTestId("active-dock-view")).toHaveText("plan");

  await page.getByRole("button", { name: "Collapse right workspace" }).click();
  await expect(page.getByTestId("dock-open")).toHaveText("false");
  await expect(page.getByTestId("active-dock-view")).toHaveText("");
  await expect(page.getByTestId("dock-view-ids")).toHaveText(
    "explorer,file,diff,search,plan,timeline",
  );
  await page.getByRole("button", { name: "Open right workspace" }).click();

  await expect(page.getByTestId("dock-open")).toHaveText("true");
  await expect(page.getByTestId("active-dock-view")).toHaveText("plan");
  await expect(page.getByRole("tab", { name: "Plan" })).toHaveAttribute("data-active", "");
});

test("an unsafe narrow row folds the dock without forgetting its tabs", async ({ page }) => {
  await page.setViewportSize({ width: 1120, height: 720 });
  await openWorkspace(page, { state: "dock-light" });
  await starveTheRow(page);

  await expect(page.getByTestId("dock-open")).toHaveText("false");
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
  expect(geometry.rowWidth).toBeLessThan(DOCK_SAFE_AREA_PX + DOCK_MIN_WIDTH_PX);
  expect(geometry.dockVisible).toBe(false);
  expect(geometry.conversationWidth).toBe(geometry.rowWidth);
  await expect(
    page.getByRole("button", { name: "Widen the window to open the right workspace" }),
  ).toBeDisabled();
  await expect(page.getByTestId("dock-view-ids")).toHaveText(
    "explorer,file,diff,search,plan,timeline",
  );
});

test("the composer's chips drop their labels whole rather than ellipse them", async ({ page }) => {
  await openWorkspace(page, { state: "dock-review" });

  const footer = page.locator(".agent-composer-footer");
  const labels = footer.locator('[data-slot="composer-chip-label"]');
  const model = page.getByRole("button", { name: "Switch model" });

  await page.setViewportSize({ width: 1800, height: 1000 });
  await expect(footer).toHaveAttribute("data-labelled", "");
  await expect(labels.first()).toBeVisible();
  const clipped = await labels.evaluateAll((nodes) =>
    nodes.filter((node) => node.scrollWidth > Math.ceil(node.getBoundingClientRect().width)),
  );
  expect(clipped).toHaveLength(0);

  await page.setViewportSize({ width: 1120, height: 720 });
  await expect(footer).not.toHaveAttribute("data-labelled", "");
  await expect(labels.first()).toBeHidden();
  await expect(model).toHaveAttribute("title", /GPT/);
  await expect(model).toBeVisible();
});

test("closing tabs selects a neighbor without collapsing the workspace", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  await page.getByRole("tab", { name: "Plan" }).hover();
  await page.getByRole("button", { name: "Close Plan" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("timeline");
  await expect(page.getByTestId("dock-open")).toHaveText("true");

  await page.getByRole("tab", { name: "Timeline" }).hover();
  await page.getByRole("button", { name: "Close Timeline" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("search");
  await expect(page.getByTestId("dock-view-ids")).toHaveText("explorer,file,diff,search");
});

test("add-panel menu restores a closed singleton and focuses it", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  await page.getByRole("tab", { name: "Search" }).hover();
  await page.getByRole("button", { name: "Close Search" }).click();
  await expect(page.getByTestId("dock-view-ids")).not.toContainText("search");

  await page.getByRole("button", { name: "Browse panels" }).click();

  const onTop = await page.locator("[role=combobox]").evaluate((input) => {
    const panel = input.closest("[role=dialog], div[class*='z-50']") ?? input.parentElement!;
    const box = panel.getBoundingClientRect();
    const hit = document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2);
    return panel.contains(hit);
  });
  expect(onTop).toBe(true);

  await page.getByRole("combobox").fill("Search");
  await page.getByRole("option", { name: "Search" }).waitFor();
  await page.keyboard.press("Enter");

  await expect(page.getByTestId("active-dock-view")).toHaveText("search");
  await expect(page.getByTestId("dock-view-ids")).toHaveText(
    "explorer,file,diff,plan,timeline,search",
  );
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

  const plan = page.getByRole("tab", { name: "Plan" });
  await plan.focus();
  await plan.press("ArrowLeft");

  await expect(page.getByTestId("active-dock-view")).toHaveText("search");
  await expect(page.getByRole("tab", { name: "Search" })).toBeFocused();
  await expect(page.getByRole("tab", { name: "Search" })).toHaveAttribute("data-active", "");
});

test("the active overflow tab stays visible and both hidden edges remain signposted", async ({
  page,
}) => {
  await openWorkspace(page, { state: "dock-light" });

  const strip = page.locator(".agent-dock-tabs");
  await expect(strip).toHaveAttribute("data-overflow-start", "");
  await expect(strip).toHaveAttribute("data-overflow-end", "");
  const [stripBox, activeBox] = await Promise.all([
    strip.boundingBox(),
    page.getByRole("tab", { name: "Plan" }).boundingBox(),
  ]);
  expect(stripBox).not.toBeNull();
  expect(activeBox).not.toBeNull();
  expect(activeBox!.x).toBeGreaterThanOrEqual(stripBox!.x);
  expect(activeBox!.x + activeBox!.width).toBeLessThanOrEqual(stripBox!.x + stripBox!.width);
});

test("file and timeline tabs render through their production view plugins", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  await page.getByRole("tab", { name: "File" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("file");
  const fileView = page.locator('[data-dock-view-id="file"]');
  await expect(fileView.getByTitle(ACTIVE_FILE_PATH)).toBeVisible();
  await expect(fileView.getByText(/const currentWidth = readDockWidth/)).toBeVisible();

  await page.getByRole("tab", { name: "Timeline" }).click();
  await expect(page.getByTestId("active-dock-view")).toHaveText("timeline");
  await expect(fileView.getByText(/const currentWidth = readDockWidth/)).toBeHidden();
  await expect(page.getByText("Root run", { exact: true })).toBeVisible();
  await expect(page.getByText("run_root", { exact: true })).toBeVisible();
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

  await page.getByRole("tab", { name: "Plan" }).click();
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
  await expect(page.getByTestId("dock-open")).toHaveText("false");
  await expect(separator).toHaveCount(0);
  await expect(persistedRatio).toHaveText(String(VISUAL_DOCK_WIDTH_RATIO));
});

test("settings filtering and menu dismissal stay inside production semantics", async ({ page }) => {
  await openWorkspace(page, { state: "settings" });
  await waitForWorkspaceState(page, "settings");

  const search = page.getByRole("searchbox", SETTINGS_SEARCH);
  await search.fill("missing pane");
  await expect(page.getByRole("heading", { name: "Appearance" })).toHaveCount(0);
  await search.fill("Appearance");
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

  const smoothing = page.getByRole("checkbox", { name: en["settings.font.smoothing"]! });
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

  const plan = page.getByRole("tab", { name: "Plan" });
  await expect(plan).toBeVisible();
  await plan.click({ button: "middle" });

  await expect(page.getByTestId("dock-view-ids")).not.toContainText("plan");
});

test("a dock tab closes from the keyboard", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  const plan = page.getByRole("tab", { name: "Plan" });
  await plan.focus();
  await expect(plan).toBeFocused();
  await plan.press("Delete");
  await expect(page.getByTestId("dock-view-ids")).not.toContainText("plan");

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

for (const pane of ["plugins", "providers", "brand-icons"] as const) {
  test(`workspace golden settings pane ${pane}`, async ({ page }) => {
    await openWorkspace(page, { state: "settings", pane });
    await waitForWorkspaceState(page, "settings");
    await expect(page.locator('main section [aria-busy="true"]')).toHaveCount(0);
    await expect(page).toHaveScreenshot(`workspace-light-settings-${pane}.png`);
  });
}

test("a chosen cron preset stays chosen while the pointer crosses the others", async ({ page }) => {
  await openWorkspace(page, { state: "settings", pane: "schedules" });
  await waitForWorkspaceState(page, "settings");
  await page.getByRole("button", { name: /New schedule/ }).click();

  const presets = page
    .locator("[aria-pressed]")
    .filter({ hasText: /Hourly|Daily|Weekdays|Weekly/ });
  await expect(presets).toHaveCount(4);
  expect(
    await presets.evaluateAll(
      (els) => els.filter((element) => element.getAttribute("aria-pressed") === "true").length,
    ),
  ).toBe(1);

  const paint = () =>
    presets.evaluateAll((els) =>
      els.map((element) => ({
        chosen: element.getAttribute("aria-pressed") === "true",
        edge: getComputedStyle(element).borderTopColor,
      })),
    );

  const resting = await paint();
  await presets.first().hover();
  const hovered = await paint();

  for (const paints of [resting, hovered]) {
    const chosen = paints.filter((option) => option.chosen);
    expect(chosen).toHaveLength(1);
    for (const other of paints.filter((option) => !option.chosen)) {
      expect(other.edge).not.toBe(chosen[0]!.edge);
    }
  }
});

test("deleting a schedule asks first, and a declined ask changes nothing", async ({ page }) => {
  await openWorkspace(page, { state: "settings", pane: "schedules" });
  await waitForWorkspaceState(page, "settings");

  const rows = page.getByRole("button", { name: "Delete schedule" });
  await expect(rows).toHaveCount(2);
  await rows.first().click();

  const dialog = page.getByRole("alertdialog");
  await expect(dialog).toContainText("Nightly dependency audit");
  await expect(dialog).toContainText("cannot be undone");
  await dialog.getByRole("button", { name: "Cancel" }).click();

  await expect(rows).toHaveCount(2);
});

test("the timeline names tools the way the transcript does, never by wire name", async ({
  page,
}) => {
  const wireNames = Object.keys(TOOL_ICON_BY_NAME);
  expect(wireNames.length).toBeGreaterThan(20);

  for (const state of ["dock-timeline", "dock-runs"] as const) {
    await openWorkspace(page, { state });
    await waitForWorkspaceState(page, state);
    const subjects = await page.locator("[data-timeline-subject]").allInnerTexts();
    expect(subjects.length).toBeGreaterThan(3);
    expect(subjects.filter((subject) => wireNames.includes(subject.trim()))).toEqual([]);
  }
});

for (const answer of [
  { button: "Allow once", mark: "approved" },
  { button: "Deny", mark: "declined" },
] as const) {
  test(`answering an approval with ${answer.button} writes a settled entry`, async ({ page }) => {
    await openWorkspace(page, { state: "dock-runs" });
    await waitForWorkspaceState(page, "dock-runs");

    const timeline = page.locator("[data-dock-view-id='timeline']");
    await expect(timeline.getByText("Approval requested")).toBeVisible();
    await expect(timeline.getByRole("img", { name: answer.mark })).toHaveCount(0);

    await page.getByRole("button", { name: answer.button, exact: true }).click();

    await expect(timeline.getByRole("img", { name: answer.mark })).toBeVisible();
    const settled = timeline.getByText("Approval settled").locator("xpath=ancestor::*[2]");
    await expect(settled).toContainText("go list -deps ./...");
  });
}
