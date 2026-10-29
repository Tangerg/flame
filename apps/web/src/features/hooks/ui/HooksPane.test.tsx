import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { onlineManager, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
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
  const list = vi.fn(async () => catalog);
  host = await startKernel([
    definePlugin({
      name: "test.hooks",
      setup(ctx) {
        ctx.contribute(DATA_PROVIDER, { key: HOOKS_KEY, fetcher: list });
      },
    }),
  ]);
  testClient.render(
    <QueryClientProvider client={queryClient}>
      <HooksPane />
      <textarea aria-label="Later draft" defaultValue="Keep this hooks draft." />
    </QueryClientProvider>,
  );
  return list;
}

function pauseQueries() {
  const wasOnline = onlineManager.isOnline();
  onlineManager.setOnline(false);
  finishTestClient(() => onlineManager.setOnline(wasOnline));
}

const catalogCases = [
  { name: "empty", hooks: [] as HookCatalog["hooks"], visibleText: "No hooks configured" },
  {
    name: "nonempty",
    hooks: [
      {
        event: "PreToolUse",
        command: "guard",
        scope: "global",
        source: "/captured/hooks.json",
        active: true,
      },
    ] as HookCatalog["hooks"],
    visibleText: "PreToolUse",
  },
];

it.each(catalogCases)(
  "waits for the first paused $name hooks result",
  async ({ hooks, visibleText }) => {
    pauseQueries();
    const list = await renderCatalog({ hooks, projectTrusted: false });
    await waitFor(() =>
      expect(queryClient.getQueryState([HOOKS_KEY, { cwd: "/repo" }])).toMatchObject({
        status: "pending",
        fetchStatus: "paused",
        data: undefined,
      }),
    );
    expect(list).not.toHaveBeenCalled();
    expect(screen.getByRole("status").textContent).toBe("Loading");
    expect(screen.queryByText("No hooks configured")).toBeNull();
    const draft = screen.getByRole("textbox", { name: "Later draft" });
    act(() => draft.focus());

    await act(async () => onlineManager.setOnline(true));
    expect(await screen.findByText(visibleText)).toBeTruthy();
    expect(document.activeElement).toBe(draft);
    expect(draft).toHaveProperty("value", "Keep this hooks draft.");
  },
);

it.each(catalogCases)(
  "keeps accepted $name hooks during a paused reread",
  async ({ hooks, visibleText }) => {
    const catalog = { hooks, projectTrusted: false };
    const list = await renderCatalog(catalog);
    await screen.findByText(visibleText);
    list.mockClear();
    pauseQueries();
    act(() => void queryClient.refetchQueries({ queryKey: [HOOKS_KEY] }));
    await waitFor(() =>
      expect(queryClient.getQueryState([HOOKS_KEY, { cwd: "/repo" }])).toMatchObject({
        status: "success",
        fetchStatus: "paused",
        data: catalog,
      }),
    );
    expect(list).not.toHaveBeenCalled();
    expect(screen.getByText(visibleText)).toBeTruthy();
  },
);

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
