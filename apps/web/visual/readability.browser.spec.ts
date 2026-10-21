import { expect, test, type Locator, type Page } from "./test";
async function open(page: Page, query: string, width = 320, height = 720) {
  await page.setViewportSize({ width, height });
  await page.goto(`/visual/?${query}&font-size=18`);
  await page.locator("html[data-visual-ready]").waitFor();
  await page.evaluate(() => document.fonts.ready);
}

async function textFitsRow(text: Locator) {
  const escaped = await text.evaluate((node) => {
    const bounds = node.closest("article")!.getBoundingClientRect();
    const range = document.createRange();
    range.selectNodeContents(node);
    return [...range.getClientRects()]
      .filter((rect) => rect.width > 0 && (rect.left < bounds.left || rect.right > bounds.right))
      .map((rect) => ({ left: rect.left, right: rect.right }));
  });
  expect(escaped).toEqual([]);
}

for (const theme of ["light", "dark"]) {
  test(`a narrow structured notice keeps its copy and action readable in ${theme}`, async ({
    page,
  }) => {
    await open(page, `fixture=atoms&theme=${theme}`, 1120, 900);
    const specimen = page.locator('[data-atom="system-message"]');
    await specimen.evaluate((node) => {
      node.style.width = "280px";
    });
    const message = specimen.getByRole("alert").last();
    const copy = message.locator('[data-slot="system-message-copy"]');
    const action = message.getByRole("button");
    await message.scrollIntoViewIfNeeded();
    expect(
      await message.evaluate((node) => node.scrollWidth - node.clientWidth),
    ).toBeLessThanOrEqual(1);
    await expect(copy).toHaveText(
      `The original command remains unresolved.\n${"installation_digest_".repeat(12)}`,
    );
    const text = (await copy.boundingBox())!;
    const button = (await action.boundingBox())!;
    const bounds = (await message.boundingBox())!;
    expect(text.width).toBeGreaterThan(180);
    expect(button.y).toBeGreaterThanOrEqual(text.y + text.height);
    expect(button.x + button.width).toBeLessThanOrEqual(bounds.x + bounds.width);
    expect(button.y + button.height).toBeLessThanOrEqual(bounds.y + bounds.height);
  });

  test(`a narrow goal keeps the objective ahead of its supporting status in ${theme}`, async ({
    page,
  }) => {
    await open(page, `fixture=agent&state=running&locale=de&theme=${theme}`);
    const row = page.locator('[data-slot="goal-status-row"]');
    const summary = row.locator('[data-goal="summary"]');
    const objective = summary.locator(":scope > span").last();
    await expect(objective).toBeVisible();
    expect((await objective.boundingBox())!.width).toBeGreaterThanOrEqual(100);
    const escaped = await row.getByRole("button").evaluateAll((nodes) =>
      nodes
        .filter((node) => {
          const box = node.getBoundingClientRect();
          const row = node.closest('[data-slot="goal-status-row"]')!.getBoundingClientRect();
          return box.left < row.left || box.right > row.right || box.bottom > row.bottom;
        })
        .map((node) => node.getAttribute("aria-label") ?? node.textContent),
    );
    expect(escaped).toEqual([]);
    const full = await summary.getAttribute("title");
    await summary.click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByRole("textbox")).toHaveValue(full!);
    await page.keyboard.press("Escape");
    await expect(summary).toBeFocused();
  });

  test(`scaled plan labels and submitted multiline answers keep separate line boxes in ${theme}`, async ({
    page,
  }) => {
    await open(page, `fixture=agent&state=running&theme=${theme}`);
    await page.locator('[data-slot="active-plan-pill"]').click();
    const steps = page.getByRole("dialog", { name: "Plan", exact: true }).locator("li");
    await expect(steps).toHaveCount(3);
    const labels = await steps.evaluateAll((nodes) =>
      nodes.map((node) => {
        const range = document.createRange();
        range.selectNodeContents(node.lastElementChild!);
        return {
          line: Number.parseFloat(getComputedStyle(node).lineHeight),
          glyph: Math.max(...[...range.getClientRects()].map((rect) => rect.height)),
        };
      }),
    );
    for (const label of labels) expect(label.line).toBeGreaterThanOrEqual(label.glyph);
    await open(page, `fixture=agent&state=question&theme=${theme}`);
    await page.getByRole("radio", { name: "Race detector", exact: true }).click();
    const answer =
      "Keep each accepted application fact with its owner.\nPreserve the original command and verify the captured result.";
    await page
      .getByRole("textbox", { name: "What should this gate protect?", exact: true })
      .fill(answer);
    await page.getByRole("button", { name: "Next", exact: true }).click();
    await page.getByRole("button", { name: /Asked.*2 questions/ }).click();
    const settled = page.locator("[data-settled-answer]").filter({ hasText: answer });
    await expect(settled).toHaveText(answer);
    const lines = await settled.evaluate((node) => {
      const range = document.createRange();
      range.selectNodeContents(node);
      const rows = new Map<number, number>();
      for (const rect of range.getClientRects())
        if (rect.width > 0)
          rows.set(rect.y, Math.max(rows.get(rect.y) ?? rect.bottom, rect.bottom));
      return [...rows].map(([y, bottom]) => ({ y, bottom })).sort((a, b) => a.y - b.y);
    });
    expect(lines.length).toBeGreaterThan(1);
    for (let index = 1; index < lines.length; index++)
      expect(lines[index]!.y).toBeGreaterThanOrEqual(lines[index - 1]!.bottom);
  });

  test(`a short recorded file stays compact and filtering preserves captured line numbers in ${theme}`, async ({
    page,
  }) => {
    await open(page, `fixture=agent&state=tool-shells&theme=${theme}`, 900, 720);
    await page.getByRole("button", { name: /steps/ }).first().click();
    await page.locator('[data-tool="read"]').first().click();
    const result = page.locator('[data-slot="recorded-result"]').first();
    const viewport = result.getByRole("region", { name: "Result", exact: true });
    const filter = result.getByRole("searchbox", { name: "Filter result", exact: true });
    await expect(viewport.locator("pre")).toHaveCount(6);
    expect(await viewport.evaluate((node) => node.scrollHeight)).toBeLessThan(240);
    await expect(filter).toHaveCount(0);
    await result.getByRole("button", { name: "Find", exact: true }).click();
    await expect(filter).toBeFocused();
    await filter.fill("return");
    await expect(viewport.locator("pre")).toHaveCount(1);
    await expect(viewport).toContainText("Line 5");
    await viewport.focus();
    await viewport.press("ControlOrMeta+f");
    await expect(filter).toBeFocused();
    await expect(filter).toHaveValue("return");
    await result.getByRole("button", { name: "Close find", exact: true }).click();
    await expect(filter).toHaveCount(0);
    await expect(viewport).toBeFocused();
    await expect(viewport.locator("pre")).toHaveCount(6);
    await viewport.press("ControlOrMeta+f");
    await expect(filter).toBeFocused();
    await filter.fill("");
    await page.setViewportSize({ width: 900, height: 360 });
    await filter.scrollIntoViewIfNeeded();
    await expect(filter).toBeInViewport({ ratio: 1 });
    const controls = result.locator('[data-slot="recorded-result-controls"]');
    const y = (await controls.boundingBox())!.y;
    await viewport.evaluate((node) => {
      node.scrollTop = node.scrollHeight;
    });
    expect(await viewport.evaluate((node) => node.scrollTop)).toBeGreaterThan(0);
    expect((await controls.boundingBox())!.y).toBe(y);
    await expect(filter).toBeInViewport({ ratio: 1 });
    await filter.press("Escape");
    await expect(filter).toHaveCount(0);
    await expect(viewport).toBeFocused();
  });

  test(`long MCP identity, diagnostics and hook conditions remain visible inside their rows in ${theme}`, async ({
    page,
  }) => {
    const server = `workspace-${"permissions-".repeat(10)}tools`;
    const detail = `The workspace tool server could not start because the requested configuration path is unavailable: /Users/visual/${"workspace-".repeat(24)}/mcp.json`;
    const matcher = `/Users/visual/${"workspace-".repeat(24)}/**`;
    const command = `./scripts/check-workspace-permissions --root /Users/visual/${"workspace-".repeat(24)}\n--mode exact`;
    await open(page, `fixture=workspace&state=settings&pane=mcp-servers&theme=${theme}`, 320, 900);
    await page.getByRole("heading", { name: "MCP servers", exact: true }).waitFor();
    await page.evaluate(
      async ({ server, detail }) => {
        const moduleUrl = "/src/lib/queryClient.ts";
        const { queryClient }: typeof import("@/lib/queryClient") = await import(moduleUrl);
        await queryClient.cancelQueries({ queryKey: ["mcp-servers"] });
        queryClient.setQueryData(
          ["mcp-servers"],
          [
            {
              id: { origin: { type: "user" }, name: server },
              status: "failed",
              type: "stdio",
              enabled: true,
              command: "npx",
              args: [],
              handshakeTimeout: { type: "bounded", seconds: 30 },
              errorDetail: detail,
            },
          ],
        );
      },
      { server, detail },
    );
    await textFitsRow(page.getByText(server, { exact: true }));
    await textFitsRow(page.getByText(detail, { exact: true }));
    await open(page, `fixture=workspace&state=settings&pane=hooks&theme=${theme}`, 320, 900);
    await page.getByRole("heading", { name: "Hooks", exact: true }).waitFor();
    await page.evaluate(
      async ({ matcher, command }) => {
        const moduleUrl = "/src/lib/queryClient.ts";
        const { queryClient }: typeof import("@/lib/queryClient") = await import(moduleUrl);
        await queryClient.cancelQueries({ queryKey: ["hooks"] });
        queryClient.setQueriesData(
          { queryKey: ["hooks"] },
          {
            projectTrusted: true,
            hooks: [
              {
                event: "PreToolUse",
                matcher,
                command,
                scope: "global",
                source: "user-hooks.json",
                active: false,
              },
            ],
          },
        );
      },
      { matcher, command },
    );
    await textFitsRow(page.getByText(matcher, { exact: true }));
    await textFitsRow(page.getByText(command, { exact: true }));
  });
}
