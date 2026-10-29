import { expect, test } from "./test";
import { VISUAL_SETTINGS_PANES } from "./workspaceFixtureStates";
for (const theme of ["light", "dark"]) {
  test(`provider roles pair their labels and controls and separate removal from saving ${theme}`, async ({
    page,
  }) => {
    for (const width of [1120, 320]) {
      await page.setViewportSize({ width, height: 1000 });
      await page.goto(
        `/visual/?fixture=workspace&state=settings&pane=providers&theme=${theme}&font-size=18`,
      );
      await page.locator("html[data-visual-ready]").waitFor();
      const panel = page.getByRole("tabpanel");
      for (const name of ["Utility model", "Embedding model"]) {
        const label = (await panel.getByText(name, { exact: true }).boundingBox())!;
        const control = (await panel.getByRole("button", { name, exact: true }).boundingBox())!;
        if (width === 1120) expect(control.x).toBeGreaterThanOrEqual(label.x + label.width);
        else expect(control.y).toBeGreaterThanOrEqual(label.y + label.height);
      }
      const remove = panel.getByRole("button", { name: "Remove stored key", exact: true });
      const actions = panel.getByRole("group", { name: "Save", exact: true }).first();
      const test = actions.getByRole("button", { name: "Test", exact: true });
      if (width === 1120) {
        const removal = (await remove.boundingBox())!;
        expect((await test.boundingBox())!.x).toBeGreaterThanOrEqual(removal.x + removal.width);
      }
      expect(await actions.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
    }
  });
}

test("font size selectors reach the full supported scale and restore inherited sizes", async ({
  page,
}) => {
  await page.goto(
    "/visual/?fixture=workspace&state=settings&pane=appearance&theme=light&font-size=14",
  );
  await page.locator("html[data-visual-ready]").waitFor();
  const root = page.locator("html");
  const codeSize = () =>
    root.evaluate((node) => getComputedStyle(node).getPropertyValue("--fs-code").trim());
  const inherited = await codeSize();
  const ui = page.getByRole("combobox", { name: "Interface size", exact: true });
  await ui.focus();
  await ui.press("Enter");
  await expect(page.getByRole("option", { name: "14", exact: true })).toBeFocused();
  await page.keyboard.press("End");
  await expect(page.getByRole("option", { name: "18", exact: true })).toBeFocused();
  await page.keyboard.press("ArrowUp");
  await expect(page.getByRole("option", { name: "17", exact: true })).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(ui).toHaveText("17");
  await expect(page.locator("body")).toHaveCSS("font-size", "17px");
  const scaled = await codeSize();
  const code = page.getByRole("combobox", { name: "Code size", exact: true });
  await code.click();
  await page.getByRole("option", { name: "16", exact: true }).click();
  await expect(root).toHaveCSS("--fs-code", "16px");
  await code.click();
  await page.getByRole("option", { name: "Follow UI", exact: true }).click();
  await expect(code).toBeFocused();
  await expect(root).toHaveCSS("--fs-code", scaled);
  await ui.click();
  await page.getByRole("option", { name: "Default", exact: true }).click();
  await expect(ui).toBeFocused();
  await expect(page.locator("body")).toHaveCSS("font-size", "14px");
  await expect(root).toHaveCSS("--fs-code", inherited);
});

test("MCP adding keeps its opener and returns focus after cancellation", async ({ page }) => {
  await page.goto("/visual/?fixture=workspace&state=settings&pane=mcp-servers&theme=light");
  await page.locator("html[data-visual-ready]").waitFor();
  const panel = page.getByRole("tabpanel");
  const trigger = panel.getByRole("button", { name: "Add server", exact: true }).first();
  await trigger.click();
  await expect(panel.getByRole("textbox", { name: "Server name", exact: true })).toBeFocused();
  await expect(trigger).toBeVisible();
  await expect(trigger).toBeDisabled();
  await panel.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(trigger).toBeFocused();
  await expect(trigger).toBeEnabled();
});

for (const transport of ["stdio", "streamableHttp"] as const) {
  test(`MCP editing focuses its first editable field and returns to its opener ${transport}`, async ({
    page,
  }) => {
    await page.goto("/visual/?fixture=workspace&state=settings&pane=mcp-servers&theme=dark");
    await page.locator("html[data-visual-ready]").waitFor();
    const panel = page.getByRole("tabpanel");
    await expect(panel.locator("[aria-busy=true]")).toHaveCount(0);
    await page.evaluate(async (type) => {
      const moduleUrl = "/src/lib/queryClient.ts";
      const { queryClient }: typeof import("@/lib/queryClient") = await import(moduleUrl);
      queryClient.setQueryData(
        ["mcp-servers"],
        [
          {
            id: { origin: { type: "user" }, name: "workspace-tools" },
            status: "connected",
            type,
            enabled: true,
            command: "npx",
            args: [],
            url: "https://example.invalid/mcp",
            handshakeTimeout: { type: "bounded", seconds: 30 },
            toolCount: 0,
          },
        ],
      );
    }, transport);
    const trigger = panel.getByRole("button", { name: "Edit workspace-tools", exact: true });
    await trigger.click();
    const field = panel.getByRole("textbox", {
      name: transport === "stdio" ? "Command" : "Server URL",
      exact: true,
    });
    await expect(field).toBeFocused();
    await expect(panel.getByRole("textbox", { name: "Server name", exact: true })).toBeDisabled();
    const mode = panel.getByRole("radio", {
      name: transport === "stdio" ? "Streamable HTTP" : "stdio",
      exact: true,
    });
    await mode.click();
    await expect(mode).toBeFocused();
    await panel.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(trigger).toBeFocused();
  });
}

for (const width of [1120, 320]) {
  test(`remembered approval rules show their full target and matching scope ${width}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 1000 });
    await page.goto(
      "/visual/?fixture=workspace&state=settings&pane=approvals&theme=light&font-size=18",
    );
    await page.locator("html[data-visual-ready]").waitFor();
    const panel = page.getByRole("tabpanel");
    await expect(panel.locator("[aria-busy=true]")).toHaveCount(0);
    const server = "project-" + "workspace-".repeat(12) + "tools";
    const source = `mcp/${server}/read_workspace_files`;
    const condition =
      "./scripts/review-permissions --workspace /Users/visual/" +
      "project-".repeat(20) +
      "\n--mode exact";
    const dir = "/Users/visual/" + "workspace-".repeat(24);
    await page.evaluate(
      async ({ server, condition, dir }) => {
        const moduleUrl = "/src/lib/queryClient.ts";
        const { queryClient }: typeof import("@/lib/queryClient") = await import(moduleUrl);
        queryClient.setQueriesData({ queryKey: ["approval-rules"] }, [
          {
            id: "visual-rule-long",
            scope: "project",
            modelName: "read_workspace_files",
            tool: {
              type: "mcp",
              server: { origin: { type: "user" }, name: server },
              name: "read_workspace_files",
            },
            subject: { type: "exact", value: condition },
            dir,
            decision: "deny",
            stale: false,
          },
        ]);
      },
      { server, condition, dir },
    );
    for (const text of [source, `Exact: ${condition}`, dir]) {
      const value = panel.getByText(text, { exact: true });
      await expect(value).toBeVisible();
      expect(await value.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
      await expect(value).toHaveCSS("white-space", "pre-wrap");
      await expect(value).not.toHaveCSS("text-overflow", "ellipsis");
    }
    const remove = panel.getByRole("button", { name: "Forget read_workspace_files", exact: true });
    await expect(remove).toBeVisible();
    expect(
      await remove.evaluate((node) => {
        const box = node.getBoundingClientRect();
        return box.left >= 0 && box.right <= document.documentElement.clientWidth;
      }),
    ).toBe(true);
    expect(await page.locator("html").evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(
      0,
    );
  });
}

for (const theme of ["light", "dark"]) {
  for (const width of [1120, 320]) {
    test(`settings keep controls inside their content measure ${theme} ${width}`, async ({
      page,
    }) => {
      for (const pane of VISUAL_SETTINGS_PANES) {
        await page.setViewportSize({ width, height: 1000 });
        await page.goto(
          `/visual/?fixture=workspace&state=settings&pane=${pane}&theme=${theme}&font-size=18`,
        );
        await page.locator("html[data-visual-ready]").waitFor();
        const panel = page.getByRole("tabpanel");
        await expect(panel.getByRole("heading", { level: 1 })).toBeVisible();
        await expect(panel.locator("[aria-busy=true]")).toHaveCount(0);
        await page.evaluate(() => document.fonts.ready);
        if (pane === "providers") {
          const identity = panel.getByText("openai", { exact: true });
          await expect(identity).toBeVisible();
          expect(
            await identity.evaluate((element) => element.scrollWidth <= element.clientWidth),
            "provider identity remains readable beside its status",
          ).toBe(true);
        }
        const escaped = await panel.evaluate((element) => {
          const bounds = element.getBoundingClientRect();
          return [
            ...element.querySelectorAll<HTMLElement>(
              "button, input:not([type=hidden]), textarea, select",
            ),
          ]
            .filter((control) => {
              const box = control.getBoundingClientRect();
              return (
                box.width > 1 &&
                box.height > 1 &&
                (box.left < bounds.left || box.right > bounds.right)
              );
            })
            .map((control) => control.getAttribute("aria-label") ?? control.textContent);
        });
        expect(escaped, `${pane}: controls escaped the settings content`).toEqual([]);
        expect(
          await page
            .locator("html")
            .evaluate((element) => element.scrollWidth - element.clientWidth),
          `${pane}: page overflow`,
        ).toBe(0);
      }
    });
  }
}

test("plugin management aligns its sections and exposes complete release details", async ({
  page,
}) => {
  await page.goto("/visual/?fixture=workspace&state=settings&pane=plugins&theme=light");
  await page.locator("html[data-visual-ready]").waitFor();
  const panel = page.getByRole("tabpanel");
  await expect(panel.getByRole("heading", { name: "flame.memory", exact: true })).toBeVisible();
  const install = panel.getByRole("button", { name: "Install", exact: true });
  expect((await install.boundingBox())!.width).toBeLessThan((await panel.boundingBox())!.width / 2);
  for (const selector of await panel.getByRole("combobox", { name: "Choose provider" }).all()) {
    await expect(selector).toHaveText("Choose provider");
  }
  const sectionEdges = await panel
    .getByRole("heading", { level: 2 })
    .evaluateAll((headings) => headings.map((heading) => heading.getBoundingClientRect().left));
  expect(new Set(sectionEdges).size).toBe(1);
  const memory = panel
    .getByRole("article")
    .filter({ has: page.getByRole("heading", { name: "flame.memory", exact: true }) });
  await memory.getByRole("button", { name: "Release details" }).click();
  await expect(memory.getByText("/visual/packages/flame.memory", { exact: true })).toBeVisible();
  await memory.getByRole("button", { name: "Package actions", exact: true }).click();
  await expect(page.getByRole("menuitem", { name: "Uninstall", exact: true })).toBeVisible();
  await page.keyboard.press("Escape");
});

for (const [locale, title] of [
  ["zh", "插件"],
  ["de", "Plugins"],
]) {
  test(`localized plugin management mounts each dictionary once ${locale}`, async ({ page }) => {
    await page.goto(
      `/visual/?fixture=workspace&state=settings&pane=plugins&theme=light&locale=${locale}`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    const panel = page.getByRole("tabpanel");
    await expect(panel.getByRole("heading", { name: title, level: 1, exact: true })).toBeVisible();
    await expect(panel.getByRole("heading", { name: "flame.memory", exact: true })).toBeVisible();
    await expect(panel.getByText("kernel", { exact: true })).toHaveCount(0);
  });
}

test("MCP fields retain visible labels after entering values", async ({ page }) => {
  await page.goto("/visual/?fixture=workspace&state=settings&pane=mcp-servers&theme=light");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.getByRole("button", { name: "Add server", exact: true }).click();
  const panel = page.getByRole("tabpanel");
  const transport = panel.getByRole("group", { name: "Transport", exact: true });
  const stdio = transport.getByRole("radio", { name: "stdio", exact: true });
  await expect(stdio).toBeVisible();
  expect(await stdio.evaluate((node) => node.closest("label") === null)).toBe(true);
  await panel.getByRole("textbox", { name: "Server name", exact: true }).fill("Desktop tools");
  await panel.getByRole("textbox", { name: "Command", exact: true }).fill("npx");
  await panel
    .getByRole("textbox", { name: "Arguments (JSON array)", exact: true })
    .fill('["-y", "server"]');
  await panel
    .getByRole("textbox", { name: "Environment (JSON object)", exact: true })
    .fill('{"MODE":"desktop"}');
  await expect(panel.getByText("Server name", { exact: true })).toBeVisible();
  await expect(panel.getByText("Command", { exact: true })).toBeVisible();
  await expect(
    panel
      .getByRole("group", { name: "Add server", exact: true })
      .getByRole("button", { name: "Add server", exact: true }),
  ).toBeEnabled();
  await panel.getByRole("radio", { name: "Streamable HTTP", exact: true }).click();
  await expect(panel.getByText("Server URL", { exact: true })).toBeVisible();
  await expect(panel.getByText("Authorization token", { exact: true })).toBeVisible();
});

for (const width of [1120, 320]) {
  test(`populated MCP permissions keep full labels beside their controls ${width}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      "/visual/?fixture=workspace&state=settings&pane=mcp-servers&theme=light&font-size=18",
    );
    await page.locator("html[data-visual-ready]").waitFor();
    await page.evaluate(async () => {
      const moduleUrl = "/src/lib/queryClient.ts";
      const { queryClient }: typeof import("@/lib/queryClient") = await import(moduleUrl);
      queryClient.setQueryData(
        ["mcp-servers"],
        [
          {
            id: { origin: { type: "user" }, name: "workspace-tools" },
            status: "connected",
            type: "stdio",
            enabled: true,
            command: "npx",
            args: [],
            handshakeTimeout: { type: "bounded", seconds: 30 },
            toolCount: 1,
          },
        ],
      );
    });
    await page.getByRole("button", { name: "Edit workspace-tools", exact: true }).click();
    await page.getByText("Manage tools", { exact: true }).waitFor();
    await page.evaluate(async () => {
      const moduleUrl = "/src/lib/queryClient.ts";
      const { queryClient }: typeof import("@/lib/queryClient") = await import(moduleUrl);
      await queryClient.cancelQueries({ queryKey: ["mcp-tools"] });
      await queryClient.cancelQueries({ queryKey: ["mcp-tool-exposure"] });
      queryClient.setQueriesData({ queryKey: ["mcp-tools"] }, [
        {
          name: "read_workspace_files",
          description: "Read files from the current workspace without changing them.",
          modelName: "read_workspace_files",
          nameConflicts: [],
        },
      ]);
      queryClient.setQueriesData({ queryKey: ["mcp-tool-exposure"] }, []);
    });
    for (const name of ["Enable read_workspace_files", "Auto-approve read_workspace_files"]) {
      const control = page.getByRole("switch", { name, exact: true });
      await expect(control).toBeVisible();
      const label = control.locator("..");
      expect(await label.evaluate((node) => node.tagName)).toBe("LABEL");
      expect(await label.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
    }
    expect(await page.locator("html").evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(
      0,
    );
  });
}

for (const width of [1120, 320]) {
  test(`hook copy wraps without displacing or hiding its controls ${width}`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      "/visual/?fixture=workspace&state=settings&pane=hooks&theme=light&font-size=18",
    );
    await page.locator("html[data-visual-ready]").waitFor();
    const command = "./scripts/review-permissions --workspace /Users/visual/a-long-workspace-name";
    await page.evaluate(async (command) => {
      const moduleUrl = "/src/lib/queryClient.ts";
      const { queryClient }: typeof import("@/lib/queryClient") = await import(moduleUrl);
      queryClient.setQueriesData(
        { queryKey: ["hooks"] },
        {
          projectRoot: "/Users/visual/scope",
          projectTrusted: false,
          hooks: [
            {
              event: "PreToolUse",
              matcher: "shell",
              command,
              scope: "project",
              source: "/Users/visual/scope/.flame/hooks.json",
              active: false,
            },
          ],
        },
      );
    }, command);
    const toggle = page.getByRole("switch", { name: "Trust this project's hooks", exact: true });
    await expect(toggle).toBeVisible();
    const identity = page.getByText("Trust this project's hooks", { exact: true });
    if (width === 1120) {
      expect((await toggle.boundingBox())!.x).toBeGreaterThan(
        (await identity.boundingBox())!.x + (await identity.boundingBox())!.width,
      );
      expect((await toggle.boundingBox())!.y - (await identity.boundingBox())!.y).toBeLessThan(60);
    }
    const fullCommand = page.getByText(command, { exact: true });
    await expect(fullCommand).toBeVisible();
    expect(await fullCommand.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
    await expect(fullCommand).toHaveCSS("white-space", "pre-wrap");
    expect(await page.locator("html").evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(
      0,
    );
  });
}
