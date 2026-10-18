import { expect, test } from "./test";

test("a failed process keeps its exit code when secondary tool metadata folds away", async ({
  page,
}) => {
  await page.setViewportSize({ width: 360, height: 900 });
  await page.goto("/visual/?fixture=agent&state=tool-shells&theme=dark&font-size=18");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.evaluate(async () => {
    if (!location.pathname.startsWith("/visual/"))
      throw new Error("An isolated fixture is required");
    const fixturePath = "/visual/agentSessionSnapshots.ts";
    const storePath = "/src/plugins/builtin/agent/adapters/agentStore.ts";
    const projectionPath = "/src/plugins/builtin/agent/application/session/sessionSnapshot.ts";
    const fixture = (await import(fixturePath)) as typeof import("./agentSessionSnapshots");
    const { useAgentStore } = await import(storePath);
    const { projectAgentSessionSnapshot } = await import(projectionPath);
    const snapshot = structuredClone(fixture.RUNTIME_AGENT_SESSION_SNAPSHOTS["tool-shells"]);
    snapshot.items = snapshot.items.filter(
      (item) =>
        item.type === "userMessage" ||
        item.type === "agentMessage" ||
        item.id === "item_shells_command",
    );
    const store = useAgentStore.getState();
    const refresh = store.beginViewRefresh("ses_visual", true);
    if (
      !refresh ||
      !store.commitViewRefresh(
        "ses_visual",
        refresh,
        projectAgentSessionSnapshot(fixture.projectRuntimeAgentSnapshot(snapshot)),
      )
    )
      throw new Error("The fixture snapshot could not be installed");
  });

  const tool = page.locator('[data-tool="shell"]');
  await expect(tool.getByText("exit 1", { exact: true })).toBeVisible();
  await expect(tool.getByText("8.4s", { exact: true })).toBeHidden();
  await expect(tool.getByText("failed", { exact: true })).toHaveCount(0);
  expect(await tool.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);

  await page.setViewportSize({ width: 1120, height: 900 });
  await expect(tool.getByText("exit 1", { exact: true })).toBeVisible();
  await expect(tool.getByText("8.4s", { exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
});

test("search results omit absent snippets while preserving recorded empty lines", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=agent&state=tool-search&theme=dark&font-size=18");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.evaluate(async () => {
    if (!location.pathname.startsWith("/visual/"))
      throw new Error("An isolated fixture is required");
    const fixturePath = "/visual/agentSessionSnapshots.ts";
    const storePath = "/src/plugins/builtin/agent/adapters/agentStore.ts";
    const projectionPath = "/src/plugins/builtin/agent/application/session/sessionSnapshot.ts";
    const fixture = (await import(fixturePath)) as typeof import("./agentSessionSnapshots");
    const { useAgentStore } = await import(storePath);
    const { projectAgentSessionSnapshot } = await import(projectionPath);
    const snapshot = structuredClone(fixture.RUNTIME_AGENT_SESSION_SNAPSHOTS["tool-search"]);
    const glob = snapshot.items.find(
      (item) => item.type === "toolCall" && item.tool.name === "glob",
    );
    const read = structuredClone(
      fixture.RUNTIME_AGENT_SESSION_SNAPSHOTS["tool-shells"].items.find(
        (item) => item.type === "toolCall" && item.tool.name === "read",
      ),
    );
    const web = structuredClone(
      fixture.RUNTIME_AGENT_SESSION_SNAPSHOTS["tool-remote"].items.find(
        (item) => item.type === "toolCall" && item.tool.name === "web_search",
      ),
    );
    if (glob?.type !== "toolCall" || read?.type !== "toolCall" || web?.type !== "toolCall")
      throw new Error("The canonical search and read calls are required");
    glob.tool.result = JSON.stringify({
      hits: [
        { path: "runtime/internal/session/store_test.go" },
        {
          path: "runtime/internal/session/atomicity_test.go",
          snippet: "func TestCommitAtomicity(t *testing.T) {",
        },
        { path: "runtime/internal/run/segment_test.go", snippet: "" },
      ],
    });
    web.tool.result = JSON.stringify({
      results: [
        { title: "Write-Ahead Logging", url: "https://www.sqlite.org/wal.html" },
        {
          title: "WAL grows without bound under a long reader",
          url: "https://sqlite.org/forum/forumpost/2c7b9f",
          snippet: "A reader open across the checkpoint keeps frames alive.",
        },
        { title: "Recorded blank", url: "https://www.sqlite.org/", snippet: "" },
      ],
    });
    snapshot.items = [
      ...snapshot.items.filter((item) => item.type === "userMessage"),
      glob,
      read,
      web,
      ...snapshot.items.filter((item) => item.type === "agentMessage"),
    ];
    const store = useAgentStore.getState();
    const refresh = store.beginViewRefresh("ses_visual", true);
    if (
      !refresh ||
      !store.commitViewRefresh(
        "ses_visual",
        refresh,
        projectAgentSessionSnapshot(fixture.projectRuntimeAgentSnapshot(snapshot)),
      )
    )
      throw new Error("The fixture snapshot could not be installed");
  });

  await page
    .getByRole("button", { name: /steps|calls/ })
    .first()
    .click();
  const finding = page
    .locator('[data-slot="recorded-result"]')
    .filter({ has: page.getByRole("search", { name: "Result search" }) });
  const filteredRows = finding.locator('[data-slot="recorded-result-viewport"] > div');
  await page.getByRole("button", { name: /^Found files/ }).click();
  const files = page
    .locator('[data-slot="recorded-result"]')
    .filter({ hasText: "runtime/internal/session/store_test.go" });
  const fileRows = files.locator('[data-slot="recorded-result-viewport"] > div');
  await expect(fileRows).toHaveCount(3);
  await expect(fileRows.nth(0).locator("pre")).toHaveCount(0);
  await expect(fileRows.nth(1).locator("pre")).toHaveText(
    "func TestCommitAtomicity(t *testing.T) {",
  );
  await expect(fileRows.nth(2).locator("pre")).toHaveText(" ");
  await expect(fileRows.nth(2).locator("pre")).toBeVisible();
  await files.getByRole("button", { name: "Find", exact: true }).click();
  await finding.getByRole("searchbox", { name: "Filter result" }).fill("TestCommitAtomicity");
  await expect(filteredRows).toHaveCount(1);
  await expect(filteredRows.first()).toContainText("atomicity_test.go");
  await finding.getByRole("searchbox", { name: "Filter result" }).fill("store_test.go");
  await expect(filteredRows).toHaveCount(1);
  await expect(filteredRows.first().locator("pre")).toHaveCount(0);
  await finding.getByRole("button", { name: "Close find" }).click();
  await page.getByRole("button", { name: /^Found files/ }).click();

  await page.getByRole("button", { name: /^Searched the web/ }).click();
  const web = page.locator('[data-slot="recorded-result"]').filter({ hasText: "Recorded blank" });
  const webRows = web.locator('[data-slot="recorded-result-viewport"] > div');
  await expect(webRows).toHaveCount(3);
  await expect(webRows.nth(0).locator("pre")).toHaveCount(0);
  await expect(webRows.nth(1).locator("pre")).toHaveText(
    "A reader open across the checkpoint keeps frames alive.",
  );
  await expect(webRows.nth(2).locator("pre")).toHaveText(" ");
  await expect(webRows.nth(2).locator("pre")).toBeVisible();
  await web.getByRole("button", { name: "Find", exact: true }).click();
  await finding.getByRole("searchbox", { name: "Filter result" }).fill("keeps frames alive");
  await expect(filteredRows).toHaveCount(1);
  await expect(filteredRows.first()).toContainText("long reader");
  await finding.getByRole("button", { name: "Close find" }).click();
  await page.getByRole("button", { name: /^Searched the web/ }).click();

  await page.getByRole("button", { name: /^Read / }).click();
  const source = page
    .locator('[data-slot="recorded-result"]')
    .filter({ hasText: "package session" });
  const sourceRows = source.locator('[data-slot="recorded-result-viewport"] > div');
  await expect(sourceRows).toHaveCount(6);
  await expect(sourceRows.nth(1).locator("pre")).toHaveText(" ");
  await expect(sourceRows.nth(1).locator("pre")).toBeVisible();
  await source.getByRole("button", { name: "Find", exact: true }).click();
  await finding.getByRole("searchbox", { name: "Filter result" }).fill("flushLocked");
  await expect(filteredRows).toHaveCount(1);
  await expect(filteredRows.first()).toContainText("Line 4");
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
});

test("recorded results scroll internally when bounded and continue through the transcript", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1120, height: 720 });
  await page.goto("/visual/?fixture=agent&state=tool-agentic&theme=light&font-size=18");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.getByRole("button", { name: /calls/ }).first().click();
  await page.locator('[data-tool="list_skills"]').click();
  const result = page.locator('[data-slot="recorded-result-viewport"]');
  const transcript = page.locator(".msg-scroll-viewport");
  await expect(result).toContainText("trace-flaky-test");
  const bounds = (await transcript.boundingBox())!;
  await page.mouse.move(bounds.x + bounds.width - 8, bounds.y + 150);
  await page.mouse.wheel(0, -2000);
  await expect.poll(() => transcript.evaluate((node) => node.scrollTop)).toBe(0);
  expect(
    await transcript.evaluate((node) => node.scrollHeight - node.clientHeight),
  ).toBeGreaterThan(0);
  expect(await result.evaluate((node) => node.scrollHeight - node.clientHeight)).toBe(0);
  await expect(result).toBeInViewport({ ratio: 1 });
  const shortBounds = (await result.boundingBox())!;
  await page.mouse.move(
    shortBounds.x + shortBounds.width / 2,
    shortBounds.y + shortBounds.height / 2,
  );
  await page.mouse.wheel(0, 240);
  await expect.poll(() => transcript.evaluate((node) => node.scrollTop)).toBeGreaterThan(0);
  expect(await result.evaluate((node) => node.scrollTop)).toBe(0);

  await page.setViewportSize({ width: 1120, height: 360 });
  await result.scrollIntoViewIfNeeded();
  const boundedRange = await result.evaluate((node) => node.scrollHeight - node.clientHeight);
  expect(boundedRange).toBeGreaterThan(5);
  await result.evaluate((node) => {
    node.scrollTop = 0;
  });
  const boundedBounds = (await result.boundingBox())!;
  await page.mouse.move(
    boundedBounds.x + boundedBounds.width / 2,
    boundedBounds.y + boundedBounds.height / 2,
  );
  const beforeInnerScroll = await transcript.evaluate((node) => node.scrollTop);
  await page.mouse.wheel(0, 5);
  await expect.poll(() => result.evaluate((node) => node.scrollTop)).toBeGreaterThan(0);
  expect(await transcript.evaluate((node) => node.scrollTop)).toBe(beforeInnerScroll);
  await result.evaluate((node) => {
    node.scrollTop = node.scrollHeight;
  });
  await expect.poll(() => result.evaluate((node) => node.scrollTop)).toBe(boundedRange);
  const beforeBoundary = await transcript.evaluate((node) => ({
    top: node.scrollTop,
    room: node.scrollHeight - node.clientHeight - node.scrollTop,
  }));
  expect(beforeBoundary.room).toBeGreaterThan(0);
  await page.mouse.wheel(0, 120);
  await expect
    .poll(() => transcript.evaluate((node) => node.scrollTop))
    .toBeGreaterThan(beforeBoundary.top);
  expect(await result.evaluate((node) => node.scrollTop)).toBe(boundedRange);
});
