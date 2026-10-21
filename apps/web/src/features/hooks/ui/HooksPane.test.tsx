import { QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import { type Host } from "dougong";
import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { startKernel, stopKernel } from "@/extensions/bootstrap";
import { definePlugin } from "@/extensions/definePlugin";
import { type HookCatalog, HOOKS_KEY } from "@/features/hooks/application/hookQueries";
import { HooksPane } from "@/features/hooks/ui/HooksPane";
let host: Host | undefined;

vi.mock("@/features/agent/public/session", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agent/public/session")>()),
  useActiveSessionWorkspace: () => ({ status: "ready", cwd: "/repo" }),
}));

afterEach(async () => {
  cleanup();
  queryClient.clear();
  if (!host) return;
  const owned = host;
  host = undefined;
  await stopKernel(owned);
});

const literal = { normalizer: (text: string) => text };

async function renderCatalog(catalog: HookCatalog) {
  host = await startKernel([
    definePlugin({
      name: "test.hooks",
      setup(ctx) {
        ctx.contribute(DATA_PROVIDER, { key: HOOKS_KEY, fetcher: async () => catalog });
      },
    }),
  ]);
  render(
    <QueryClientProvider client={queryClient}>
      <HooksPane />
    </QueryClientProvider>,
  );
}

async function articleWithSource(source: string) {
  const article = (await screen.findByText(source, literal)).closest("article");
  expect(article).not.toBeNull();
  return article as HTMLElement;
}

describe("HooksPane records", () => {
  it("identifies the scope and exact source of otherwise identical hooks", async () => {
    const globalSource = "/Users/dev/.flame/hooks.json";
    const projectSource =
      "/Users/dev/我的 项目/a-very-long-segment-without-any-break-opportunities-at-all-0123456789/.flame/hooks.json";
    const shared = { event: "PreToolUse", matcher: "shell", command: "./.flame/hooks/guard.sh" };
    await renderCatalog({
      hooks: [
        { ...shared, scope: "global", source: globalSource, active: true },
        { ...shared, scope: "project", source: projectSource, active: false },
      ],
      projectRoot: "/Users/dev/我的 项目",
      projectTrusted: false,
    });

    const global = await articleWithSource(globalSource);
    const project = await articleWithSource(projectSource);
    expect(global).not.toBe(project);

    for (const article of [global, project]) {
      expect(within(article).getByText("PreToolUse")).toBeTruthy();
      expect(within(article).getByText("shell")).toBeTruthy();
      expect(within(article).getByText("./.flame/hooks/guard.sh")).toBeTruthy();
    }

    expect(within(global).getByText("Global")).toBeTruthy();
    expect(within(global).queryByText("Project")).toBeNull();
    expect(within(global).queryByText("inactive")).toBeNull();
    expect(within(global).queryByText(projectSource, literal)).toBeNull();

    expect(within(project).getByText("Project")).toBeTruthy();
    expect(within(project).queryByText("Global")).toBeNull();
    expect(within(project).getByText("inactive")).toBeTruthy();
    expect(within(project).queryByText(globalSource, literal)).toBeNull();
  });

  it("keeps an inject-only payload and its source literal", async () => {
    const source = "/Users/dev/.flame/hooks.json";
    const payload = "Reminder:  keep   the\nrelease notes current.";
    await renderCatalog({
      hooks: [
        { event: "UserPromptSubmit", inject: payload, scope: "global", source, active: true },
      ],
      projectTrusted: false,
    });

    const article = await articleWithSource(source);
    expect(within(article).getByText(payload, literal)).toBeTruthy();
    expect(within(article).getByText("UserPromptSubmit")).toBeTruthy();
    expect(within(article).getByText("Global")).toBeTruthy();
    expect(within(article).getByText("inject")).toBeTruthy();
  });
});
