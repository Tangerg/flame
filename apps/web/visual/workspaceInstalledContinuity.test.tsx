import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { type Host } from "dougong";
import { StrictMode, useEffect, useState, type ComponentProps, type ComponentType } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  type CapturedToolResult,
  type ComponentContext,
  type PluginComponent,
} from "@flame/component-sdk";
import { type FlameClient } from "@flame/runtime-client";
import { definePlugin } from "@/extensions/definePlugin";
import { startKernel, stopKernel } from "@/extensions/bootstrap";
import { type WorkspaceViewSpec } from "@/features/workspace/public/views";
import { type ContributionLifetime } from "@/extensions/definePlugin";
import { RuntimeConnectionGeneration } from "@/features/connection/public/services";
import {
  InstalledToolResultComponent,
  RenderingComponent,
} from "@/features/plugins/ui/InstalledComponent";
import { ChatPanel } from "@/features/shell/workbench/panel/ChatPanel";
type Sources = Omit<ComponentProps<typeof RenderingComponent>, "entry">;

const model = vi.hoisted(() => ({
  sessionId: "ses_memory",
  activeMainView: null as string | null,
  dock: { open: true, viewIds: [] as string[], activeViewId: null as string | null },
  views: [] as WorkspaceViewSpec[],
  stream: null as ComponentType | null,
}));

vi.mock("@/features/agent/public/session", () => ({
  useActiveSession: () => null,
  useActiveSessionId: () => model.sessionId,
  useAgentSessions: () => ({ isLoading: false, data: [] }),
}));

vi.mock("@/features/agent/public/run", () => ({
  useIsCurrentRootRunning: () => false,
  useCurrentRootMaterial: () => ({ status: "idle", outcome: null }),
}));

vi.mock("@/features/workspace/public/navigation", () => ({
  WORKSPACE_DOCK_CATALOG: "catalog",
  closeAllWorkspaceDockViews: vi.fn(),
  closeOtherWorkspaceDockViews: vi.fn(),
  closeWorkspaceDockView: vi.fn(),
  closeWorkspaceView: vi.fn(),
  collapseWorkspaceDock: vi.fn(),
  openWorkspaceView: vi.fn(),
  openWorkspaceViewInDock: vi.fn(),
  reorderWorkspaceDockView: vi.fn(),
  selectWorkspaceDockView: vi.fn(),
  showWorkspaceDock: vi.fn(),
  useActiveWorkspaceViewId: () => model.activeMainView,
  useWorkspaceDock: () => model.dock,
  useWorkspaceInspection: () => null,
  useWorkspaceSubagentRunId: () => null,
  returnFromWorkspaceInspection: vi.fn(),
}));

vi.mock("@/features/workspace/public/contextDockCatalog", () => ({
  useContextDockCatalog: () => [],
}));

vi.mock("@/features/workspace/public/sidebarDrawer", () => ({
  useDockWidth: () => ({ width: null, setWidth: vi.fn() }),
}));

vi.mock("@/features/workspace/public/views", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/features/workspace/public/views")>();
  return { ...actual, useWorkspaceViews: () => model.views };
});

vi.mock("@/extensions/host/Slot", () => ({ Slot: () => null }));

vi.mock("@/features/shell/workbench/panel/ChatStream", () => ({
  ChatStream: () => (model.stream ? <model.stream /> : null),
}));

vi.mock("@/features/shell/workbench/panel/HeaderDiffStat", () => ({
  HeaderDiffStat: () => null,
}));

let host: Host;

let lifetime: ContributionLifetime;

let resources: AbortController;

let sources: Sources;

let contexts: ComponentContext[];

let mountedEntries: number;

let save: ReturnType<typeof vi.fn<(key: string) => Promise<void>>>;

let pendingSave: PromiseWithResolvers<void>;

function Memory({ context }: ComponentProps<PluginComponent>) {
  if (contexts.at(-1) !== context) contexts.push(context);
  const [query, setQuery] = useState("");
  const [saved, setSaved] = useState("idle");
  useEffect(() => {
    mountedEntries += 1;
    return () => {
      mountedEntries -= 1;
    };
  }, []);
  const submit = () => {
    const key = `memory-save-${save.mock.calls.length + 1}`;
    setSaved(`pending ${key}`);
    void context.track(() => save(key)).then(() => setSaved(`saved ${key}`));
  };
  return (
    <section aria-label="Memory entry">
      <input aria-label="Search memory" value={query} onChange={(e) => setQuery(e.target.value)} />
      <textarea aria-label="Draft memory" />
      <button type="button" onClick={submit}>
        Save memory
      </button>
      <output aria-label="Memory save">{saved}</output>
    </section>
  );
}

function MemoryView() {
  return <RenderingComponent {...sources} entry={Memory} />;
}

const toolResult: CapturedToolResult = Object.freeze({
  id: "call_grep",
  runId: "run_root",
  name: "grep",
  fn: "grep",
  args: "{}",
  status: "ok",
});

let resultContexts: ComponentContext[];

function ResultEntry({ context }: ComponentProps<PluginComponent>) {
  if (resultContexts.at(-1) !== context) resultContexts.push(context);
  const [filter, setFilter] = useState("");
  return (
    <input aria-label="Filter result" value={filter} onChange={(e) => setFilter(e.target.value)} />
  );
}

function ConversationStream() {
  return <InstalledToolResultComponent {...sources} toolResult={toolResult} entry={ResultEntry} />;
}

function NotesView() {
  return <p>Notes body</p>;
}

function OtherMainView() {
  return <p>Other main body</p>;
}

const views: WorkspaceViewSpec[] = [
  { id: "memory", title: "Memory", icon: "tool", dock: "session", component: MemoryView },
  { id: "notes", title: "Notes", icon: "tool", dock: "session", component: NotesView },
  { id: "other", title: "Other", icon: "tool", component: OtherMainView },
];

const send = () => true;

function mount(strict: boolean) {
  const view = testClient.render(<ChatPanel onSend={send} />, {
    wrapper: strict ? StrictMode : undefined,
  });
  return (change: () => void) =>
    act(() => {
      change();
      view.rerender(<ChatPanel onSend={send} />);
    });
}

function dock(open: boolean, viewIds: string[], activeViewId: string | null) {
  model.dock = { open, viewIds, activeViewId };
}

function editMemory() {
  const search = screen.getByRole<HTMLInputElement>("textbox", { name: "Search memory" });
  const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Draft memory" });
  fireEvent.change(search, { target: { value: "release notes" } });
  fireEvent.change(draft, { target: { value: "unsent draft" } });
  return { search, draft, context: contexts.at(-1)! };
}

let clientWidth: { mockRestore: () => void };

beforeEach(async () => {
  clientWidth = vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(1440);
  contexts = [];
  resultContexts = [];
  mountedEntries = 0;
  pendingSave = Promise.withResolvers<void>();
  save = vi.fn((_key: string) => pendingSave.promise);
  resources = new AbortController();
  host = await startKernel([
    definePlugin({
      name: "test.installed-continuity",
      setup(ctx) {
        lifetime = ctx.lifetime("installed-continuity");
      },
    }),
  ]);
  sources = {
    component: { installationId: "memory", digest: "a".repeat(64), componentId: "memory" },
    generation: RuntimeConnectionGeneration.forProcess("continuity-test"),
    compose: () => null,
    saveFile: async () => "requested",
    runtime: {} as FlameClient,
    binding: undefined,
    lifetime,
    resources: resources.signal,
    workspace: { ref: { path: "/captured" } },
  } as Sources;
  model.sessionId = "ses_memory";
  model.activeMainView = null;
  model.views = views;
  model.stream = null;
  dock(true, ["memory", "notes"], "memory");
});

afterEach(async () => {
  cleanup();
  pendingSave.resolve();
  resources.abort();
  await stopKernel(host);
  clientWidth.mockRestore();
});

it.each([false, true])(
  "keeps one installed view instance across dock collapse, tab switching and main promotion (strict: %s)",
  async (strict) => {
    const navigate = mount(strict);
    const { search, draft, context } = editMemory();
    fireEvent.click(screen.getByRole("button", { name: "Save memory" }));
    expect(save).toHaveBeenCalledExactlyOnceWith("memory-save-1");

    const expectRetained = (presented: boolean) => {
      expect(context.signal.aborted).toBe(false);
      expect(contexts.at(-1)).toBe(context);
      expect(mountedEntries).toBe(1);
      expect(screen.queryByRole("alert")).toBeNull();
      expect(screen.getByLabelText("Search memory")).toBe(search);
      expect(screen.getByLabelText("Draft memory")).toBe(draft);
      expect(search.value).toBe("release notes");
      expect(draft.value).toBe("unsent draft");
      expect(screen.queryByRole("textbox", { name: "Search memory" })).toBe(
        presented ? search : null,
      );
    };

    await navigate(() => dock(false, ["memory", "notes"], null));
    expectRetained(false);
    await act(async () => pendingSave.resolve());
    await navigate(() => dock(true, ["memory", "notes"], "memory"));
    expectRetained(true);
    expect(screen.getByLabelText("Memory save").textContent).toBe("saved memory-save-1");

    await navigate(() => dock(true, ["memory", "notes"], "notes"));
    expectRetained(false);
    expect(screen.getByText("Notes body")).toBeTruthy();
    await navigate(() => dock(true, ["memory", "notes"], "memory"));
    expectRetained(true);

    await navigate(() => (model.activeMainView = "memory"));
    expectRetained(true);
    expect(screen.getAllByRole("region", { name: "Memory", hidden: true })).toHaveLength(1);
    await navigate(() => (model.activeMainView = null));
    expectRetained(true);

    await navigate(() => (model.activeMainView = "other"));
    expectRetained(false);
    expect(screen.getByText("Other main body")).toBeTruthy();
    await navigate(() => (model.activeMainView = "memory"));
    expectRetained(true);
    await navigate(() => {
      model.activeMainView = null;
      dock(false, ["memory", "notes"], null);
    });
    expectRetained(false);
    await navigate(() => (model.activeMainView = "memory"));
    expectRetained(true);

    expect(save).toHaveBeenCalledOnce();

    await navigate(() => {
      model.activeMainView = null;
      dock(true, ["notes"], "notes");
    });
    expect(screen.queryByLabelText("Search memory")).toBeNull();
    expect(mountedEntries).toBe(0);
    expect(context.signal.aborted).toBe(true);
  },
);

it.each([false, true])(
  "keeps installed conversation results alive behind a main view until the conversation unmounts (strict: %s)",
  async (strict) => {
    model.stream = ConversationStream;
    const navigate = mount(strict);
    const filter = screen.getByRole<HTMLInputElement>("textbox", { name: "Filter result" });
    fireEvent.change(filter, { target: { value: "needle" } });
    const context = resultContexts.at(-1)!;
    const body = screen.getByRole("tabpanel", { name: "Memory" });
    expect(screen.getByRole("complementary", { name: "Memory" })).toBeTruthy();

    const expectRetained = (presented: boolean) => {
      expect(context.signal.aborted).toBe(false);
      expect(resultContexts.at(-1)).toBe(context);
      expect(screen.getByLabelText("Filter result")).toBe(filter);
      expect(filter.value).toBe("needle");
      expect(screen.queryByRole("textbox", { name: "Filter result" })).toBe(
        presented ? filter : null,
      );
    };

    await navigate(() => (model.activeMainView = "memory"));
    expectRetained(false);
    expect(screen.queryByRole("region", { name: "Conversation" })).toBeNull();
    expect(screen.queryByRole("complementary")).toBeNull();
    expect(screen.getByRole("region", { name: "Memory" })).toBe(body);

    await navigate(() => (model.activeMainView = "other"));
    expectRetained(false);
    expect(screen.queryByRole("complementary")).toBeNull();
    expect(screen.getByRole("region", { name: "Other" })).toBeTruthy();

    await navigate(() => (model.activeMainView = null));
    expectRetained(true);
    expect(screen.getByRole("complementary", { name: "Memory" })).toBeTruthy();
    expect(screen.getByRole("tabpanel", { name: "Memory" })).toBe(body);

    cleanup();
    expect(context.signal.aborted).toBe(true);
  },
);

it("retires the installed view instance when the exact Session changes", async () => {
  const navigate = mount(false);
  const { search, context } = editMemory();

  await navigate(() => (model.sessionId = "ses_replacement"));

  expect(context.signal.aborted).toBe(true);
  const replacement = screen.getByRole<HTMLInputElement>("textbox", { name: "Search memory" });
  expect(replacement).not.toBe(search);
  expect(replacement.value).toBe("");
  expect(mountedEntries).toBe(1);
  expect(contexts.at(-1)!.signal.aborted).toBe(false);
});

it("retires a main-only view when another main view replaces it", async () => {
  model.activeMainView = "memory";
  dock(false, [], null);
  const navigate = mount(false);
  const { context } = editMemory();

  await navigate(() => (model.activeMainView = "other"));

  expect(context.signal.aborted).toBe(true);
  expect(screen.queryByLabelText("Search memory")).toBeNull();
  expect(mountedEntries).toBe(0);
});

it.each([
  ["resources", () => resources.abort()],
  ["lifetime", () => lifetime.dispose()],
] as const)("does not revive the entry when its %s retire while hidden", async (_, retire) => {
  const navigate = mount(false);
  editMemory();
  await navigate(() => dock(false, ["memory", "notes"], null));
  await act(async () => {
    await retire();
  });

  await navigate(() => dock(true, ["memory", "notes"], "memory"));
  expect(screen.queryByLabelText("Search memory")).toBeNull();
  expect(screen.getByRole("alert")).toBeTruthy();
  expect(mountedEntries).toBe(0);
  expect(contexts.every((context) => context.signal.aborted)).toBe(true);

  await navigate(() => (model.activeMainView = "memory"));
  expect(screen.queryByLabelText("Search memory")).toBeNull();
  expect(contexts.every((context) => context.signal.aborted)).toBe(true);
});
