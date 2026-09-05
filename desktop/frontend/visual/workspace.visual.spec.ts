import { expect, test, type Page } from "./test";
import { freezeVisualClock } from "./frozenClock";
import { en } from "@/lib/i18n/locales/en";
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

// Named from the catalogue, not copied out of it. Three literals of this string lived
// here, so changing the copy broke a test that has nothing to do with the copy.
const SETTINGS_SEARCH = { name: en["settings.searchPlaceholder"]! };
const ACTIVE_FILE_PATH = "desktop/frontend/src/plugins/builtin/shell/kernel/panel/DockResizer.tsx";

test.use({ viewport: VISUAL_WORKSPACE_VIEWPORT });

interface WorkspaceRoute {
  state: VisualWorkspaceState;
  theme?: VisualWorkspaceTheme;
  pane?: VisualSettingsPane;
}

async function openWorkspace(page: Page, route: WorkspaceRoute): Promise<void> {
  if (route.state === "settings") await page.setViewportSize({ width: 1120, height: 720 });
  const query = new URLSearchParams({
    fixture: "workspace",
    theme: route.theme ?? "light",
    state: route.state,
  });
  if (route.pane) query.set("pane", route.pane);
  await page.goto(`/visual/?${query}`);
  await page.locator("html[data-visual-ready]").waitFor();
  await expect(page.getByTestId("workspace-state")).toHaveAttribute("data-state", route.state);
}

/**
 * The flank folds on the ROW, and the row is what the drawer leaves behind. At the
 * shell's minimum window the drawer has to be at its widest before the row is narrow
 * enough to make the fold observable.
 */
async function starveTheRow(page: Page): Promise<void> {
  const rail = page.getByRole("separator", { name: "Resize the workspace fixture sidebar" });
  await rail.focus();
  await rail.press("End");
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
    // Both rows, and the batch count that says one of them holds three asks —
    // the queue is only useful if it distinguishes what is waiting and how much.
    await expect(page.getByText("Which database should the migration target?")).toBeVisible();
    await expect(page.getByText("+2", { exact: true })).toBeVisible();
    return;
  }
  if (state === "dock-stats") {
    // Every dock view stays MOUNTED, so `.first()` here once matched a hidden
    // tool-stats pane while the diff was the one on screen — and the assertions
    // passed against a view nobody could see. Scope to what is visible.
    const view = page.locator(".agent-workspace-view:visible");
    // Six, since the shells state gained the write whose empty card body started this. The
    // total moved when every settled call in that fixture finally carried the duration the
    // Runtime always sends with one: the patch had been counting as instant.
    await expect(view).toContainText("6 calls · 8.7s");
    // The two ways a call fails to deliver, counted apart.
    await expect(view).toContainText("1 failed");
    await expect(view).toContainText("1 denied");
    // Ordered by time SPENT, not by call count: the one 8.4s command has to
    // outrank the faster reads, which is the whole reason this is not a counter.
    const listing = await view.innerText();
    expect(listing.indexOf("shell")).toBeLessThan(listing.indexOf("read"));
    return;
  }
  if (state === "dock-tools") {
    const view = page.locator(".agent-workspace-view:visible");
    // The families, in the table's order and not the runtime's listing order — the
    // fixture reports `shell` first and `search_memory` last, and grouping is the
    // whole feature.
    const listing = await view.innerText();
    expect(listing.indexOf("Shell")).toBeLessThan(listing.indexOf("Files"));
    expect(listing.indexOf("Files")).toBeLessThan(listing.indexOf("Search"));
    // A tool the local family table has never heard of still lists, under the
    // trailing family — the alternative is a call the agent can make that the
    // catalog denies exists.
    await expect(view).toContainText("acme_deploy");
    expect(listing.indexOf("Other")).toBeGreaterThan(listing.indexOf("Recall"));
    return;
  }
  if (state === "dock-empty") {
    await expect(page.getByText("Nothing to compare", { exact: true })).toBeVisible();
    return;
  }
  if (state === "dock-loading") {
    // Scoped to the tab on screen. Every open tab stays mounted, and a tab that is
    // hidden has its effects torn down — so its query never subscribes and it renders
    // its own busy state indefinitely. Unscoped, this matched three spinners and
    // asserted on the first, which is whichever tab happens to be leftmost.
    await expect(
      page.locator(".agent-context-dock [data-dock-view-id]:visible output[aria-busy=true]"),
    ).toBeVisible();
    return;
  }
  if (state === "dock-runs") {
    // The other half of the same view. Ready is the deepest node — a run whose parent is
    // itself delegated — because the tree paints outside-in and a two-level lineage is the
    // last thing to arrive.
    const view = page.locator(".agent-workspace-view:visible");
    await expect(view).toContainText("7 runs");
    await expect(view).toContainText("parent run_child");
    // Every state a run can end in, which is why this state exists.
    for (const status of ["Canceled", "Error", "Limit reached", "Finished"]) {
      await expect(view.getByText(status, { exact: true }).first()).toBeVisible();
    }
    return;
  }
  if (state === "dock-timeline") {
    // The view reads the session's run tree, which resolves a query — so the header count is
    // not ready, it is the first thing painted. Ready is a row that only the resolved data can
    // produce: the failed patch, which is also the entry whose status mark this state exists to
    // photograph.
    const view = page.locator(".agent-workspace-view:visible");
    await expect(view).toContainText("8 events");
    await expect(view.getByRole("img", { name: "err" })).toBeVisible();
    return;
  }
  // Four catalogues that had no provider until this round; ready is the count each header
  // states, which only the seeded data can produce.
  const CATALOGUE_READY: Partial<Record<VisualWorkspaceState, string>> = {
    "dock-skill-proposals": "2 awaiting review",
    "dock-skill-library": "1 active",
    "dock-recipes": "2 available",
    "dock-agent-docs": "3 found",
    "dock-skills": "2 available",
    "dock-knowledge": "2 scopes",
    "dock-agent-memory": "1 pending",
    // The other side of the same view: a Runtime that does not advertise the feature.
    "dock-feature-off": "Skills are off",
    "dock-run-summary": "run_root",
    "dock-notifications": "No notifications",
  };
  const catalogueReady = CATALOGUE_READY[state];
  if (catalogueReady !== undefined) {
    await expect(page.locator(".agent-workspace-view:visible")).toContainText(catalogueReady);
    return;
  }
  if (state === "dock-search") {
    // Nothing has been searched, so ready is the view explaining what it searches — the empty
    // state is the whole surface here, and the only one a fixture can photograph honestly.
    await expect(page.locator(".agent-workspace-view:visible")).toContainText(
      "regex over the session workspace",
    );
    return;
  }
  if (state === "dock-files") {
    const view = page.locator(".agent-workspace-view:visible");
    await expect(view).toContainText("2 files changed");
    await expect(view).toContainText("DockResizer.tsx");
    return;
  }
  if (state === "dock-explorer") {
    // The tree is seeded, not fetched — ready is the root listing it renders from that seed.
    const view = page.locator(".agent-workspace-view:visible");
    await expect(view).toContainText("go.mod");
    await expect(view).toContainText("README.md");
    return;
  }
  if (state === "dock-terminal") {
    // The failing line, in the tone its escape codes ask for: this state exists because that
    // pane used to print the codes instead of reading them.
    const view = page.locator(".agent-workspace-view:visible");
    await expect(view).toContainText("exit 1");
    await expect(view.locator(".text-negative", { hasText: "FAIL:" }).first()).toBeVisible();
    return;
  }
  if (state === "dock-error") {
    await expect(page.getByText("Couldn't load the diff", { exact: true })).toBeVisible();
    return;
  }
  if (state === "dock-file") {
    const view = page.locator(".agent-workspace-view:visible");
    await expect(view).toContainText("8 lines");
    // The tail of the file's longest line. Whether it can be READ is the clipping
    // check's job; this only pins that the viewer renders the whole line.
    await expect(view).toContainText("clampDockWidth(currentWidth + delta, row.clientWidth)");
    return;
  }
  if (state === "dock-catalog") {
    // A dock holding nothing shows what it could hold. Ready is the catalogue's own heading
    // plus one destination row — the tab strip is empty here, so there is no tab to wait on.
    await expect(page.getByText(en["dock.catalog.title"]!, { exact: true })).toBeVisible();
    await expect(
      page.locator(".agent-context-dock").getByRole("button", { name: "Explorer" }),
    ).toBeVisible();
    return;
  }
  if (state === "settings") {
    // The heading is owned by the settings host and renders before the lazy pane, so it
    // says nothing about whether the chunk resolved. The Suspense fallback marks itself
    // `aria-busy`, and its absence from the pane's own section is the ready boundary every
    // pane shares — a control Appearance owns was one only for the pane that was hard-coded.
    await expect(page.getByRole("heading").first()).toBeVisible();
    await expect(page.locator('main section [aria-busy="true"]')).toHaveCount(0);
    return;
  }
  // Exhaustiveness belongs here: an added state must declare its own ready boundary
  // instead of being diagnosed against an unrelated surface.
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
  // Collapsed means there is no destination, rather than a hidden one: the dock
  // is open exactly when the location names a view. What survives is the tab set
  // and the memory of which tab you were on — asserted by the round trip below.
  await expect(page.getByTestId("dock-open")).toHaveText("false");
  await expect(page.getByTestId("active-dock-view")).toHaveText("");
  await expect(page.getByTestId("dock-view-ids")).toHaveText(
    "explorer,file,diff,terminal,plan,timeline",
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

  // Two settles before any synchronous read. The fold is a store round-trip away from the
  // resize, and `visibility` is then transitioned with a delay equal to the slide-out, so the
  // dock stays visible for the whole fold BY DESIGN. Reading a frame instead of the end state
  // is what made this flaky.
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
    "explorer,file,diff,terminal,plan,timeline",
  );
});

test("the composer's chips drop their labels whole rather than ellipse them", async ({ page }) => {
  await openWorkspace(page, { state: "dock-review" });

  const footer = page.locator(".agent-composer-footer");
  const labels = footer.locator('[data-slot="composer-chip-label"]');
  const model = page.getByRole("button", { name: "Switch model" });

  // Wide enough for all three: every label reads in full, none clipped.
  await page.setViewportSize({ width: 1800, height: 1000 });
  await expect(footer).toHaveAttribute("data-labelled", "");
  await expect(labels.first()).toBeVisible();
  const clipped = await labels.evaluateAll((nodes) =>
    nodes.filter((node) => node.scrollWidth > Math.ceil(node.getBoundingClientRect().width)),
  );
  expect(clipped).toHaveLength(0);

  // Narrow: the labels go, and nothing is left ellipsed in their place.
  await page.setViewportSize({ width: 1120, height: 720 });
  await expect(footer).not.toHaveAttribute("data-labelled", "");
  await expect(labels.first()).toBeHidden();
  // The value is still readable, which is the whole reason the label may go.
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
  await expect(page.getByTestId("active-dock-view")).toHaveText("terminal");
  await expect(page.getByTestId("dock-view-ids")).toHaveText("explorer,file,diff,terminal");
});

test("add-panel menu restores a closed singleton and focuses it", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  await page.getByRole("tab", { name: "Terminal" }).hover();
  await page.getByRole("button", { name: "Close Terminal" }).click();
  await expect(page.getByTestId("dock-view-ids")).not.toContainText("terminal");

  await page.getByRole("button", { name: "Browse panels" }).click();

  // The panel has to be ON TOP of the dock, not merely mounted. Base UI positions
  // the portaled node with a `transform`, which makes it a stacking context — so
  // the panel's own z-index settles nothing outside it, and with the positioner
  // left at `auto` the whole popup lost to the dock's `z-15` backing and painted
  // entirely behind the panel it was opened from. Every assertion below passed
  // through all of that: the DOM was right and not one pixel was drawn.
  const onTop = await page.locator("[role=combobox]").evaluate((input) => {
    const panel = input.closest("[role=dialog], div[class*='z-50']") ?? input.parentElement!;
    const box = panel.getBoundingClientRect();
    const hit = document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2);
    return panel.contains(hit);
  });
  expect(onTop).toBe(true);

  // The catalog is a searchable combobox, not a menu. Filtering and committing
  // from the keyboard is also the path the control is shaped for: the input takes
  // focus on open and `autoHighlight` puts the first match under Enter.
  await page.getByRole("combobox").fill("Terminal");
  await page.getByRole("option", { name: "Terminal" }).waitFor();
  await page.keyboard.press("Enter");

  await expect(page.getByTestId("active-dock-view")).toHaveText("terminal");
  await expect(page.getByTestId("dock-view-ids")).toHaveText(
    "explorer,file,diff,plan,timeline,terminal",
  );
});

test("dock tabs use roving focus and arrow-key activation", async ({ page }) => {
  await openWorkspace(page, { state: "dock-light" });

  const plan = page.getByRole("tab", { name: "Plan" });
  await plan.focus();
  await plan.press("ArrowLeft");

  await expect(page.getByTestId("active-dock-view")).toHaveText("terminal");
  await expect(page.getByRole("tab", { name: "Terminal" })).toBeFocused();
  await expect(page.getByRole("tab", { name: "Terminal" })).toHaveAttribute("data-active", "");
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

// Deliberately NOT the review state: that one is seeded wide enough to exercise
// the diff's split, and this test is about the rail at the general persisted width.
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

test("accent selection gives an immediate, durable visual acknowledgement", async ({ page }) => {
  await openWorkspace(page, { state: "settings" });
  await waitForWorkspaceState(page, "settings");

  const purple = page.getByRole("button", { name: "Accent: Purple" });
  await purple.click();

  // One click has to cross the complete production topology: preference,
  // dynamic custom-theme contribution, document painter, React projection and
  // persistence. A duplicate contribution used to abort that listener chain,
  // making the swatch feel as though it ignored the click.
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

  // The trigger is an icon with no label beside it, so its own accessible name
  // and native title are the only thing that says what it does.
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
  // Asserted on what is SEEN rather than on a state attribute: the swap is CSS, so there is
  // no React state left to read, and opacity is what the person actually gets.
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

// A dock tab could be closed with the mouse, the middle button and the context menu — and,
// until this was measured, by no key at all. The × rests at `visibility: hidden`, which does
// not merely hide it: the browser skips it in sequential focus navigation. Seventy Tab
// presses walked the whole dock without landing on one. It has to stay out of the tab order
// — a focusable sibling inside a `tablist` is an unallowed child, which axe rates critical —
// so the key belongs on the tab.
// The middle button is the browser-tab gesture a reader brings with them, and it is the one
// closing affordance that leaves no mark on the strip to notice it is gone.
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

  // The × itself must stay off the tab order, or the violation comes back with it.
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
      // Put the transcript at its resting position before reading the clock —
      // same reason as the agent goldens: stick-to-bottom eases toward a target
      // that `content-visibility` keeps re-measuring, so the same fixture lands
      // a pixel apart between runs and every row in the frame moves with it.
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

// Two panes beyond the one the settings state used to hard-code: the densest list and the
// most form-heavy, which between them carry the row, field and empty-state vocabulary every
// other pane is assembled from.
for (const pane of ["plugins", "providers"] as const) {
  test(`workspace golden settings pane ${pane}`, async ({ page }) => {
    await openWorkspace(page, { state: "settings", pane });
    await waitForWorkspaceState(page, "settings");
    await expect(page.locator('main section [aria-busy="true"]')).toHaveCount(0);
    await expect(page).toHaveScreenshot(`workspace-light-settings-${pane}.png`);
  });
}

// The cron presets are a one-of group, and both halves of "which one" were missing: no
// `aria-pressed` at all, and a selected fill of 4% black against a hover fill of 3%, so
// moving the pointer across the group erased the answer for anyone who could see it.
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

  // The EDGE, because hover cannot forge one: it only ever deepens a fill, and the fills it
  // deepens between are a percent apart. Whatever the pointer is over, exactly one option is
  // outlined and it is the one that answered.
  for (const paints of [resting, hovered]) {
    const chosen = paints.filter((option) => option.chosen);
    expect(chosen).toHaveLength(1);
    for (const other of paints.filter((option) => !option.chosen)) {
      expect(other.edge).not.toBe(chosen[0]!.edge);
    }
  }
});

// A saved schedule is instructions somebody wrote, and its delete was one click on a quiet
// icon wedged between Run and Edit — no menu in front, no undo behind. What this holds is
// that the click ASKS: the row is still there until the dialog is answered.
test("deleting a schedule asks first, and a declined ask changes nothing", async ({ page }) => {
  await openWorkspace(page, { state: "settings", pane: "schedules" });
  await waitForWorkspaceState(page, "settings");

  const rows = page.getByRole("button", { name: "Delete schedule" });
  await expect(rows).toHaveCount(2);
  await rows.first().click();

  const dialog = page.getByRole("alertdialog");
  await expect(dialog).toContainText("Nightly dependency audit");
  await expect(dialog).toContainText("cannot be undone");
  // Named, so the dialog cannot be answered by whichever button happens to be first.
  await dialog.getByRole("button", { name: "Cancel" }).click();

  await expect(rows).toHaveCount(2);
});

// The HITL loop's own trace, which no fixture could hold: an approval-result entry exists
// only once somebody answers, so neither `approved` nor `declined` had ever been drawn.
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

    // The verdict is the MARK; the label states only that the request was answered — as a
    // bare noun it read "granted" in four languages, which a denial is not.
    await expect(timeline.getByRole("img", { name: answer.mark })).toBeVisible();
    const settled = timeline.getByText("Approval settled").locator("xpath=ancestor::*[2]");
    // And WHICH approval, taken from the request rather than restated: two answered
    // approvals in one run are otherwise two identical rows.
    await expect(settled).toContainText("go list -deps ./...");
  });
}
