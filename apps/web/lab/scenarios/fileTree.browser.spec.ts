import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "./test";
import { expectLabReady } from "../readiness";
import { type AgentSessionSummary } from "@/features/agent/public/session";
import { en } from "@/lib/i18n/locales/en";

async function directoryIsFetching(page: Page) {
  return page.evaluate(async () => {
    const queryPath = "/src/lib/queryClient.ts";
    const { queryClient }: typeof import("@/lib/queryClient") = await import(queryPath);
    return queryClient
      .getQueryCache()
      .getAll()
      .some((query) => {
        const request = query.queryKey[1] as { path?: string; recursive?: boolean } | undefined;
        return (
          query.queryKey[0] === "list-files" &&
          request?.path === "apps" &&
          request.recursive !== true &&
          query.state.fetchStatus === "fetching"
        );
      });
  });
}

async function relocateWorkspace(page: Page, cwd: string) {
  return page.evaluate(async (cwd) => {
    const runtimePath = "/lab/scenarios/fixtureRuntime.tsx";
    const queryPath = "/src/lib/queryClient.ts";
    const sessionPath = "/src/features/agent/public/session.ts";
    const [{ visualClient }, { queryClient }, { AGENT_SESSIONS_KEY }] = await Promise.all([
      import(runtimePath) as Promise<typeof import("./fixtureRuntime")>,
      import(queryPath) as Promise<typeof import("@/lib/queryClient")>,
      import(sessionPath) as Promise<typeof import("@/features/agent/public/session")>,
    ]);
    const id = visualClient.agent.access.getActiveSessionId();
    const key = [AGENT_SESSIONS_KEY];
    const sessions = queryClient.getQueryData<AgentSessionSummary[]>(key);
    const current = sessions?.find((session) => session.id === id);
    if (!sessions || !current) throw new Error("expected the fixture's active Session");
    queryClient.setQueryData(
      key,
      sessions.map((session) =>
        session.id === id
          ? { ...session, workspace: { ...session.workspace, path: cwd } }
          : session,
      ),
    );
    return current.workspace.path;
  }, cwd);
}

async function expectAccessibleTree(page: Page) {
  const audit = await new AxeBuilder({ page })
    .include('[role="treegrid"]')
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
    .analyze();
  expect(audit.violations).toEqual([]);
}

for (const browserName of ["chromium", "webkit"] as const) {
  test.extend({ browserName })(
    `${browserName} retries a failed directory read through its real Files action`,
    async ({ page }) => {
      await page.goto(
        "/lab/fixture.html?fixture=workspace&state=dock-files&agent-state=idle&directory-state=error&locale=en&motion=off",
      );
      await expectLabReady(page);
      const tree = page.getByRole("treegrid", { name: "Files", exact: true });
      const folder = tree.getByRole("row", { name: /^apps(?:\s|$)/ });
      await folder.focus();
      await folder.press("ArrowRight");
      const failure = page.getByText(en["dataView.error.title"]!, { exact: true });
      await expect(failure).toBeVisible();
      await expect(folder).toHaveAccessibleName("apps");
      await expect(tree.getByRole("row", { name: "README.md", exact: true })).toBeVisible();
      await expect(tree.locator('[data-file-path][aria-level="2"]')).toHaveCount(0);
      await expect(
        page.getByRole("button", { name: "Notify apps change", exact: true }),
      ).toHaveCount(0);

      const bounds = await failure.boundingBox();
      if (!bounds) throw new Error("Expected the visible directory failure title");
      const y = bounds.y + bounds.height / 2;
      await page.mouse.move(bounds.x - 1, y);
      await page.mouse.down();
      await page.mouse.move(bounds.x + bounds.width + 1, y, { steps: 8 });
      await page.mouse.up();
      await expect
        .poll(() => page.evaluate(() => window.getSelection()?.toString() ?? ""))
        .toContain(en["dataView.error.title"]!);
      await expect(folder).toHaveAttribute("aria-expanded", "true");
      await failure.click();
      await expect(folder).toHaveAttribute("aria-expanded", "true");
      await expectAccessibleTree(page);

      const retry = page.getByRole("button", { name: en["common.retry"]!, exact: true });
      await folder.focus();
      await folder.press("ArrowDown");
      await expect(retry).toBeFocused();
      await retry.press("Enter");
      const child = tree.locator('[data-file-path][aria-level="2"]').first();
      await expect(child).toBeVisible();
      await expect(folder).toHaveAccessibleName("apps");
      await expect(retry).toBeHidden();
      await folder.focus();
      await folder.press("ArrowDown");
      await expect(child).toBeFocused();
      await child.press("Enter");
      const back = page.getByRole("button", { name: "Back to files", exact: true });
      await expect(back).toBeFocused();
      await back.press("Enter");
      await expect(child).toBeFocused();
      await child.press("ArrowLeft");
      await expect(folder).toBeFocused();
    },
  );

  test.extend({ browserName })(
    `${browserName} retains directory children through a notified refresh failure and Retry`,
    async ({ page }) => {
      await page.goto(
        "/lab/fixture.html?fixture=workspace&state=dock-files&agent-state=idle&directory-state=refresh-error&locale=en&motion=off",
      );
      await expectLabReady(page);
      const tree = page.getByRole("treegrid", { name: "Files", exact: true });
      const folder = tree.getByRole("row", { name: /^apps(?:\s|$)/ });
      await folder.focus();
      await folder.press("ArrowRight");
      const child = tree.locator('[data-file-path][aria-level="2"]').first();
      await expect(child).toBeVisible();
      await expect(folder).toHaveAccessibleName("apps");
      await folder.press("ArrowDown");
      await expect(child).toBeFocused();
      const original = await child.elementHandle();
      const notify = page.getByRole("button", { name: "Notify apps change", exact: true });
      await notify.focus();
      await notify.press("Enter");
      const failure = page.getByText(en["file.refreshError.title"]!, { exact: true });
      await expect(failure).toBeVisible();
      await expect(folder).toHaveAccessibleName("apps");
      await expect(folder).not.toHaveAccessibleName(/DockResizer\.tsx/);
      await expect(
        page.getByText("Showing the last loaded files in apps.", { exact: true }),
      ).toBeVisible();
      await expect(notify).toBeFocused();
      await expect(folder).toHaveAttribute("aria-expanded", "true");
      expect(await child.evaluate((node, before) => node === before, original)).toBe(true);
      await expect(child).toHaveAttribute("tabindex", "0");
      await expectAccessibleTree(page);

      const retry = page.getByRole("button", { name: en["common.retry"]!, exact: true });
      await child.focus();
      await child.press("ArrowDown");
      await expect(retry).toBeFocused();
      await expect(child).toHaveAttribute("tabindex", "0");
      expect(await folder.evaluate((node) => getComputedStyle(node).outlineStyle)).toBe("none");
      await retry.press("Enter");
      await expect(failure).toBeHidden();
      await expect(retry).toBeHidden();
      await expect(child).toBeVisible();
      expect(await child.evaluate((node, before) => node === before, original)).toBe(true);
      await expect(folder).toBeFocused();
      await expect(folder).toHaveAccessibleName("apps");
      await folder.focus();
      await folder.press("ArrowDown");
      await expect(child).toBeFocused();
      await child.press("ArrowLeft");
      await expect(folder).toBeFocused();
    },
  );

  for (const directoryState of ["loading", "empty"] as const) {
    test.extend({ browserName })(
      `${browserName} keeps a ${directoryState} directory scoped to its expanded branch`,
      async ({ page }) => {
        await page.goto(
          `/lab/fixture.html?fixture=workspace&state=dock-files&agent-state=idle&directory-state=${directoryState}&locale=en&motion=off`,
        );
        await expectLabReady(page);
        const tree = page.getByRole("treegrid", { name: "Files", exact: true });
        const folder = tree.getByRole("row", { name: /^apps(?:\s|$)/ });
        await folder.focus();
        await folder.press("ArrowRight");
        const feedback = page.getByText(
          en[directoryState === "loading" ? "common.loading" : "file.empty.title"]!,
          { exact: true },
        );
        await expect(feedback).toBeVisible();
        await expect(tree.getByRole("row", { name: "README.md", exact: true })).toBeVisible();
        await expect(tree.locator('[data-file-path][aria-level="2"]')).toHaveCount(0);
        if (directoryState === "loading") {
          await expect(folder.locator("[title]")).toHaveAttribute("aria-busy", "true");
          await expect.poll(() => directoryIsFetching(page)).toBe(true);
          await folder.press("ArrowLeft");
          await expect(feedback).toBeHidden();
          await expect.poll(() => directoryIsFetching(page)).toBe(false);
          await folder.press("ArrowRight");
          await expect(feedback).toBeVisible();
          await expect.poll(() => directoryIsFetching(page)).toBe(true);
        } else {
          await expect(folder.locator("[title]")).not.toHaveAttribute("aria-busy", "true");
        }
        await folder.press("ArrowDown");
        await expect(tree.getByRole("row", { name: "go.mod", exact: true })).toBeFocused();
      },
    );
  }

  test.extend({ browserName })(
    `${browserName} file tree keeps navigation separate from activation`,
    async ({ page }) => {
      await page.goto(
        "/lab/fixture.html?fixture=workspace&state=dock-files&agent-state=idle&locale=en&motion=off",
      );
      await expectLabReady(page);
      const tree = page.getByRole("treegrid", { name: "Files", exact: true });
      const folder = tree.getByRole("row", { name: /^apps(?:\s|$)/ });
      await folder.focus();
      await folder.press("ArrowRight");
      await expect(folder).toHaveAttribute("aria-expanded", "true");
      await expect(tree.locator('[data-file-path][aria-level="2"]').first()).toBeVisible();
      await folder.press("ArrowDown");
      const child = tree.locator('[role="row"]:focus');
      await expect(child).toHaveAttribute("aria-level", "2");
      const name = await child.getAttribute("aria-label");
      await expect(page.getByRole("button", { name: "Back to files", exact: true })).toBeHidden();
      await child.press("Enter");
      const back = page.getByRole("button", { name: "Back to files", exact: true });
      await expect(back).toBeFocused();
      await back.press("Enter");
      const returned = tree.getByRole("row", { name: name!, exact: true });
      await expect(returned).toBeFocused();
      await expect(returned.locator("[title]")).toHaveAttribute("aria-current", "true");
      await returned.press("ArrowLeft");
      await expect(folder).toBeFocused();
      await folder.press(" ");
      await expect(folder).toHaveAttribute("aria-expanded", "false");
      await folder.press(" ");
      await expect(folder).toHaveAttribute("aria-expanded", "true");
      await folder.click({ button: "right" });
      const copy = page.getByRole("menuitem", { name: "Copy path", exact: true });
      await expect(copy).toBeVisible();
      await page.keyboard.press("Escape");
      await expect(folder).toBeFocused();
      await expectAccessibleTree(page);

      await page.keyboard.press("Meta+l");
      const composer = page.getByRole("textbox", {
        name: en["composer.input.label"]!,
        exact: true,
      });
      await expect(composer).toBeFocused();
      const prefix = "Keep this draft. ";
      const suffix = "Continue here.";
      await composer.fill(prefix + suffix);
      for (let index = 0; index < suffix.length; index++) await composer.press("ArrowLeft");
      const caret = () =>
        composer.evaluate((node) => {
          const selection = node.ownerDocument.getSelection();
          if (!selection?.anchorNode || !selection.focusNode)
            throw new Error("missing native editor selection");
          const offset = (end: Node, position: number) => {
            const range = node.ownerDocument.createRange();
            range.selectNodeContents(node);
            range.setEnd(end, position);
            return range.toString().length;
          };
          return [
            offset(selection.anchorNode, selection.anchorOffset),
            offset(selection.focusNode, selection.focusOffset),
          ];
        });
      expect(await caret()).toEqual([prefix.length, prefix.length]);
      await folder.click();
      await expect(folder).toHaveAttribute("aria-expanded", "false");
      await folder.press("ArrowDown");
      const next = tree.getByRole("row", { name: "go.mod", exact: true });
      await expect(next).toBeFocused();
      await next.press("Home");
      await expect(folder).toBeFocused();
      await folder.press("ArrowRight");
      await expect(returned.locator("[title]")).toHaveAttribute("aria-current", "true");
      await folder.press("End");
      await expect(tree.getByRole("row", { name: "README.md", exact: true })).toBeFocused();
      await page.keyboard.press("Meta+l");
      await expect(composer).toBeFocused();
      expect(await caret()).toEqual([prefix.length, prefix.length]);
      await page.keyboard.type("Returned. ");
      await expect(composer).toHaveText(prefix + "Returned. " + suffix);
    },
  );

  test.extend({ browserName })(
    `${browserName} workspace round trips retire preview mode and popup drafts`,
    async ({ page }) => {
      await page.goto(
        "/lab/fixture.html?fixture=workspace&state=dock-files&agent-state=idle&locale=en&motion=off",
      );
      await expectLabReady(page);
      await page.getByRole("row", { name: "README.md", exact: true }).click();
      const source = page.getByRole("radio", { name: "Source", exact: true });
      const preview = page.getByRole("radio", { name: "Preview", exact: true });
      const goToLine = page.getByRole("button", { name: "Go to line", exact: true });
      const field = page.getByRole("textbox", { name: "Go to line", exact: true });
      await source.click();
      await goToLine.click();
      await field.fill("37");

      const expectFreshPreview = async () => {
        await expect(preview).toHaveAttribute("aria-checked", "true");
        await expect(page.getByRole("dialog")).toHaveCount(0);
        await goToLine.click();
        await expect(field).toHaveValue("");
        await page.keyboard.press("Escape");
        await expect(goToLine).toBeFocused();
      };
      const original = await relocateWorkspace(page, "/visual/relocated-workspace");
      await expectFreshPreview();
      await source.click();
      await goToLine.click();
      await field.fill("44");
      await relocateWorkspace(page, original);
      await expectFreshPreview();
    },
  );

  test.extend({ browserName })(
    `${browserName} changing a cached file retires its line-navigation popup`,
    async ({ page }) => {
      await page.goto(
        "/lab/fixture.html?fixture=workspace&state=dock-files&agent-state=idle&locale=en&motion=off",
      );
      await expectLabReady(page);
      const tree = page.getByRole("treegrid", { name: "Files", exact: true });
      await tree.getByRole("row", { name: "go.mod", exact: true }).click();
      const goToLine = page.getByRole("button", { name: "Go to line", exact: true });
      await expect(goToLine).toBeVisible();
      await page.getByRole("button", { name: "Back to files", exact: true }).click();
      await tree.getByRole("row", { name: "README.md", exact: true }).click();
      await goToLine.click();
      const field = page.getByRole("textbox", { name: "Go to line", exact: true });
      await field.fill("37");

      await page.evaluate(async () => {
        const runtimePath = "/lab/scenarios/fixtureRuntime.tsx";
        const navigationPath = "/src/features/workspace/application/navigation.ts";
        const [{ visualClient }, { openWorkspaceFile }] = await Promise.all([
          import(runtimePath),
          import(navigationPath),
        ]);
        openWorkspaceFile(visualClient.workspace.access, "go.mod");
      });

      await expect(page.getByRole("dialog")).toHaveCount(0);
      await expect(page.getByRole("region", { name: "go.mod", exact: true })).toBeVisible();
      await goToLine.click();
      await expect(field).toHaveValue("");
      await page.keyboard.press("Escape");
      await expect(goToLine).toBeFocused();
    },
  );

  test.extend({ browserName })(
    `${browserName} a file preview retires the retained tree menu`,
    async ({ page }) => {
      await page.goto(
        "/lab/fixture.html?fixture=workspace&state=dock-files&agent-state=idle&locale=en&motion=off",
      );
      await expectLabReady(page);
      const tree = page.getByRole("treegrid", { name: "Files", exact: true });
      const folder = tree.getByRole("row", { name: /^apps(?:\s|$)/ });
      await folder.focus();
      await folder.press("ArrowRight");
      const file = tree.locator('[data-file-path][aria-level="2"]').first();
      await expect(file).toBeVisible();
      const path = await file.getAttribute("data-file-path");
      expect(path).toBeTruthy();
      await file.focus();
      await file.click({ button: "right" });
      await expect(page.getByRole("menuitem", { name: "Copy path", exact: true })).toBeVisible();

      await page.evaluate(async (path) => {
        const runtimePath = "/lab/scenarios/fixtureRuntime.tsx";
        const navigationPath = "/src/features/workspace/application/navigation.ts";
        const [{ visualClient }, { openWorkspaceFile }] = await Promise.all([
          import(runtimePath),
          import(navigationPath),
        ]);
        openWorkspaceFile(visualClient.workspace.access, path);
      }, path!);
      const back = page.getByRole("button", { name: "Back to files", exact: true });
      await expect(back).toBeFocused();
      await expect(page.getByRole("menu")).toHaveCount(0);
      await back.press("Enter");
      await expect(file).toBeFocused();
      await expect(page.getByRole("menu")).toHaveCount(0);
      await expect(folder).toHaveAttribute("aria-expanded", "true");
    },
  );
}
