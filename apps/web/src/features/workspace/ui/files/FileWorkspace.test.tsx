import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { definePlugin } from "@/extensions/definePlugin";
import { WORKSPACE_FILE_RENDERER } from "@/features/workspace/public/views";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";
import {
  configureLocalWorkspace,
  localWorkspace,
} from "@/features/workspace/application/ports/localWorkspace";
import * as notifications from "@/features/notifications/public/notifications";
import { navigator } from "@/lib/navigation";
import {
  closeWorkspaceFile,
  openWorkspaceFile,
  rememberWorkspaceView,
} from "@/features/workspace/application/navigation";
import { useWorkspaceFileViewer } from "@/features/workspace/public/navigation";
import { useContextDockStore } from "@/features/workspace/adapters/contextDockStore";
import { installWorkspaceErrorClassifier } from "@/features/workspace/adapters/runtimeWorkspaceErrorClassifier";
import {
  WORKSPACE_LIST_FILES_KEY,
  WORKSPACE_READ_FILE_KEY,
  type WorkspaceFileContent,
  type WorkspaceFileEntry,
  type WorkspaceListFilesQuery,
  type WorkspaceReadFileQuery,
  type WorkspaceFileChange,
} from "@/features/workspace/application/workspaceQueries";
import { FileWorkspace } from "@/features/workspace/ui/files/FileWorkspace";
import { FileTree } from "@/features/workspace/ui/files/FileTree";
import { RpcError } from "@flame/runtime-client";
import { ContentVisibility } from "@/ui/index";
const selection = vi.hoisted(() => ({
  current: { status: "ready" } as
    { status: "ready"; cwd?: string } | { status: "resolving"; sessionId: string },
}));

const changes = vi.hoisted(() => new Map<string, WorkspaceFileChange>());

vi.mock("@/features/workspace/application/workingTreeChanges", () => ({
  useWorkingTreeFiles: () => changes,
}));

vi.mock("@/features/agent/public/session", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agent/public/session")>()),
  useActiveSessionWorkspace: () => selection.current,
}));

let client: QueryClient;

const listFiles = vi.fn<(query: WorkspaceListFilesQuery) => Promise<WorkspaceFileEntry[]>>();

const readFile = vi.fn<(query: WorkspaceReadFileQuery) => Promise<WorkspaceFileContent>>();

const directory: WorkspaceFileEntry = { name: "src", path: "src", type: "dir" };

const file: WorkspaceFileEntry = { name: "main.go", path: "src/main.go", type: "file" };

let uninstallClassifier: () => void;

let uninstallLocalWorkspace: () => void;

let isLocal = false;

beforeEach(async () => {
  changes.clear();
  isLocal = false;
  uninstallLocalWorkspace = configureLocalWorkspace({
    available: () => isLocal,
    open: vi.fn(),
    reveal: vi.fn(),
  });
  uninstallClassifier = installWorkspaceErrorClassifier();
  selection.current = { status: "ready" };
  useContextDockStore.setState({
    fileViewer: null,
    memory: {
      expandedDirs: [],
      lastFilePath: null,
    },
  });
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  listFiles.mockReset().mockImplementation(async (query) => (query.path ? [file] : [directory]));
  readFile.mockReset().mockResolvedValue({ content: "package main", startLine: 1, totalLines: 1 });
  await loadPluginsForTest(
    definePlugin({
      name: "test.workspace-files",
      setup(ctx) {
        ctx.contribute(DATA_PROVIDER, {
          key: WORKSPACE_LIST_FILES_KEY,
          fetcher: (params) => listFiles(params as WorkspaceListFilesQuery),
        });
        ctx.contribute(DATA_PROVIDER, {
          key: WORKSPACE_READ_FILE_KEY,
          fetcher: (params) => readFile(params as WorkspaceReadFileQuery),
        });
        ctx.contribute(
          WORKSPACE_FILE_RENDERER,
          {
            id: "markdown",
            title: "Markdown",
            extension: "md",
            component: ({ content }) => (
              <article data-testid="rendered-markdown">{content}</article>
            ),
          },
          { key: "markdown" },
        );
      },
    }),
  );
});

afterEach(() => {
  vi.restoreAllMocks();
  uninstallClassifier();
  uninstallLocalWorkspace();
  cleanup();
  client.clear();
});

function lineWindow(start: number, end: number, totalLines: number): WorkspaceFileContent {
  const lines = Array.from({ length: end - start + 1 }, (_, i) => `line ${start + i}`);
  return { content: lines.join("\n"), startLine: start, totalLines };
}

function row(line: number) {
  return document.querySelector(`[data-quote-line="${line}"]`);
}

// A branch exposes its children by containing or owning their group; every owned ID must resolve.
function ownedGroup(item: HTMLElement): HTMLElement | null {
  const owned = item.getAttribute("aria-owns")?.split(/\s+/).filter(Boolean) ?? [];
  const targets = owned.map((id) => document.getElementById(id));
  expect(targets).not.toContain(null);
  const groups = [
    ...new Set([item.querySelector<HTMLElement>('[role="group"]'), ...targets]),
  ].filter((group): group is HTMLElement => group?.getAttribute("role") === "group");
  expect(groups.length).toBeLessThanOrEqual(1);
  return groups[0] ?? null;
}

function mount(children: React.ReactNode) {
  return render(<QueryClientProvider client={client}>{children}</QueryClientProvider>);
}

const ROW = 20;

// Happy DOM has no layout: the host's height is derived from rendered rows and
// scrollTop is a plain stored value, so these tests prove which assignments the
// preview owns, not how WebKit lays out or clamps them.
function controlGeometry(host: HTMLElement) {
  let top = 0;
  Object.defineProperty(host, "scrollHeight", {
    configurable: true,
    get: () => host.querySelectorAll("[data-quote-line]").length * ROW,
  });
  Object.defineProperty(host, "scrollTop", {
    configurable: true,
    get: () => top,
    set: (value: number) => {
      top = value;
    },
  });
  return host;
}

function mountRetained() {
  const workspace = (visible: boolean) => (
    <QueryClientProvider client={client}>
      <div hidden={!visible}>
        <ContentVisibility visible={visible}>
          <FileWorkspace />
        </ContentVisibility>
      </div>
    </QueryClientProvider>
  );
  const view = render(workspace(true));
  return {
    hide: () => view.rerender(workspace(false)),
    reveal: () => view.rerender(workspace(true)),
  };
}

describe("workspace files panel", () => {
  it("shows the rename relationship while preserving unknown binary statistics", () => {
    changes.set(file.path, {
      path: file.path,
      previousPath: "old/main.go",
      change: "renamed",
      binary: true,
    });
    mount(<FileTree onReveal={() => {}} entries={[file]} onSelectFile={() => {}} />);
    const row = screen.getByRole("treeitem", { name: "main.go" });
    expect(row.textContent).toContain("← old/main.go");
    expect(row.textContent).toContain("R");
    expect(row.getAttribute("title")).toBe("old/main.go → src/main.go");
    expect(screen.getByText("Renamed from old/main.go", { selector: "span[hidden]" })).toBeTruthy();
  });
  it("reads a file in the Runtime default workspace", async () => {
    openWorkspaceFile("src/main.go");
    mount(<FileWorkspace />);

    await screen.findByText("package main");
    expect(readFile).toHaveBeenCalledWith({ cwd: undefined, path: "src/main.go" });
  });

  it("keeps the file shown while a wider line window loads", async () => {
    readFile.mockResolvedValue({ content: "middle chunk", startLine: 1300, totalLines: 3000 });
    openWorkspaceFile("src/big.go");
    mount(<FileWorkspace />);
    await screen.findByText("middle chunk");

    const pending = Promise.withResolvers<WorkspaceFileContent>();
    readFile.mockReturnValueOnce(pending.promise);
    fireEvent.click(screen.getByRole("button", { name: /earlier/i }));

    // placeholderData holds the content instead of flashing a skeleton.
    expect(screen.getByText("middle chunk")).toBeTruthy();

    pending.resolve({ content: "earlier\nmiddle chunk", startLine: 1100, totalLines: 3000 });
    await screen.findByText("earlier");
  });

  it("keeps old content shown without letting it settle a pending line navigation", async () => {
    const placed: (string | null)[] = [];
    vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(function (
      this: HTMLElement,
    ) {
      placed.push(this.getAttribute("data-quote-line"));
    });
    readFile.mockResolvedValueOnce(lineWindow(250, 650, 1000));
    openWorkspaceFile("src/big.go", 450);
    mount(<FileWorkspace />);
    await vi.waitFor(() => expect(row(450)).toBeTruthy());
    expect(placed).toEqual(["450"]);

    const pending = Promise.withResolvers<WorkspaceFileContent>();
    readFile.mockReturnValueOnce(pending.promise);
    act(() => openWorkspaceFile("src/big.go", 300));
    expect(row(450)).toBeTruthy();
    expect(placed).toEqual(["450"]);

    await act(async () => pending.resolve(lineWindow(100, 500, 1000)));
    await vi.waitFor(() => expect(row(100)).toBeTruthy());
    expect(placed).toEqual(["450", "300"]);
  });

  describe("range-control focus continuity", () => {
    it.each([
      { edge: "first", control: /earlier/i, start: 1, end: 500, requestedEnd: 500 },
      { edge: "last", control: /more/i, start: 100, end: 600, requestedEnd: 700 },
    ])(
      "keeps the reader focused when the $edge line retires its range control",
      async ({ control, start, end, requestedEnd }) => {
        const placeTarget = vi
          .spyOn(HTMLElement.prototype, "scrollIntoView")
          .mockImplementation(() => {});
        readFile.mockResolvedValueOnce(lineWindow(100, 500, 600));
        openWorkspaceFile("src/big.go", 300);
        mount(<FileWorkspace />);
        await waitFor(() => expect(row(500)).toBeTruthy());
        const host = controlGeometry(screen.getByRole("region", { name: "src/big.go" }));
        host.scrollTop = 10;
        const focusReader = vi.spyOn(host, "focus");
        const button = screen.getByRole("button", { name: control });
        act(() => button.focus());
        const pending = Promise.withResolvers<WorkspaceFileContent>();
        readFile.mockReturnValueOnce(pending.promise);

        fireEvent.click(button);
        const focusWhilePending = document.activeElement;
        expect(host.scrollTop).toBe(10);
        expect(row(100)).toBeTruthy();
        await waitFor(() =>
          expect(readFile).toHaveBeenLastCalledWith({
            cwd: undefined,
            path: "src/big.go",
            startLine: start,
            endLine: requestedEnd,
          }),
        );

        await act(async () => pending.resolve(lineWindow(start, end, 600)));
        await waitFor(() => expect(screen.queryByRole("button", { name: control })).toBeNull());
        expect(document.activeElement).toBe(host);
        expect(focusWhilePending).toBe(host);
        expect(focusReader).toHaveBeenCalledExactlyOnceWith({ preventScroll: true });
        expect(host.scrollTop).toBe(10 + (100 - start) * ROW);
        expect(placeTarget).toHaveBeenCalledTimes(1);
      },
    );

    it("hands focus to the reader even when more lines remain beyond either range control", async () => {
      readFile.mockResolvedValueOnce(lineWindow(400, 402, 1000));
      openWorkspaceFile("src/big.go");
      mount(<FileWorkspace />);
      await waitFor(() => expect(row(402)).toBeTruthy());
      const host = screen.getByRole("region", { name: "src/big.go" });

      for (const { control, content, addedLine } of [
        { control: /earlier/i, content: lineWindow(200, 402, 1000), addedLine: 200 },
        { control: /more/i, content: lineWindow(200, 602, 1000), addedLine: 602 },
      ]) {
        const button = screen.getByRole("button", { name: control });
        act(() => button.focus());
        const pending = Promise.withResolvers<WorkspaceFileContent>();
        readFile.mockReturnValueOnce(pending.promise);

        fireEvent.click(button);
        expect(document.activeElement).toBe(host);
        await act(async () => pending.resolve(content));
        await waitFor(() => expect(row(addedLine)).toBeTruthy());
        expect(screen.getByRole("button", { name: control })).toBe(button);
        expect(document.activeElement).toBe(host);
      }
    });

    it("leaves focus alone when the activated range control does not own it", async () => {
      readFile.mockResolvedValueOnce(lineWindow(400, 402, 1000));
      openWorkspaceFile("src/big.go");
      mount(
        <>
          <textarea aria-label="Message" defaultValue="Retained message draft" />
          <FileWorkspace />
        </>,
      );
      await waitFor(() => expect(row(402)).toBeTruthy());
      const host = screen.getByRole("region", { name: "src/big.go" });
      const focusReader = vi.spyOn(host, "focus");
      const earlier = screen.getByRole("button", { name: /earlier/i });
      const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message" });

      for (const { owner, control, content, addedLine } of [
        { owner: earlier, control: /more/i, content: lineWindow(400, 602, 1000), addedLine: 602 },
        { owner: draft, control: /earlier/i, content: lineWindow(200, 602, 1000), addedLine: 200 },
      ]) {
        act(() => owner.focus());
        const pending = Promise.withResolvers<WorkspaceFileContent>();
        readFile.mockReturnValueOnce(pending.promise);

        fireEvent.click(screen.getByRole("button", { name: control }));
        expect(document.activeElement).toBe(owner);
        await act(async () => pending.resolve(content));
        await waitFor(() => expect(row(addedLine)).toBeTruthy());
        expect(document.activeElement).toBe(owner);
      }
      expect(focusReader).not.toHaveBeenCalled();
      expect(draft.value).toBe("Retained message draft");
    });

    it("preserves a message draft and its focus when typing resumes while a range loads", async () => {
      readFile.mockResolvedValueOnce(lineWindow(100, 500, 600));
      openWorkspaceFile("src/big.go");
      mount(
        <>
          <textarea aria-label="Message" defaultValue="Retained message draft" />
          <FileWorkspace />
        </>,
      );
      await waitFor(() => expect(row(500)).toBeTruthy());
      const host = screen.getByRole("region", { name: "src/big.go" });
      const button = screen.getByRole("button", { name: /more/i });
      act(() => button.focus());
      const pending = Promise.withResolvers<WorkspaceFileContent>();
      readFile.mockReturnValueOnce(pending.promise);
      fireEvent.click(button);
      expect(document.activeElement).toBe(host);
      const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message" });
      act(() => draft.focus());
      fireEvent.change(draft, { target: { value: "Retained message draft, continued" } });

      await act(async () => pending.resolve(lineWindow(100, 600, 600)));
      await waitFor(() => expect(row(600)).toBeTruthy());
      expect(document.activeElement).toBe(draft);
      expect(draft.value).toBe("Retained message draft, continued");
    });

    it("preserves navigation focus when an old range finishes loading", async () => {
      readFile.mockResolvedValueOnce(lineWindow(100, 500, 600));
      openWorkspaceFile("src/big.go");
      mount(<FileWorkspace />);
      await waitFor(() => expect(row(500)).toBeTruthy());
      const host = screen.getByRole("region", { name: "src/big.go" });
      const button = screen.getByRole("button", { name: /more/i });
      act(() => button.focus());
      const pending = Promise.withResolvers<WorkspaceFileContent>();
      readFile.mockReturnValueOnce(pending.promise);
      fireEvent.click(button);
      expect(document.activeElement).toBe(host);

      readFile.mockResolvedValueOnce({ content: "package main", startLine: 1, totalLines: 1 });
      act(() => openWorkspaceFile("src/main.go"));
      await screen.findByText("package main");
      const back = screen.getByRole("button", { name: "Back to files" });
      expect(document.activeElement).toBe(back);

      await act(async () => pending.resolve(lineWindow(100, 600, 600)));
      await waitFor(() => expect(client.isFetching()).toBe(0));
      expect(document.activeElement).toBe(back);
      expect(screen.getByRole("region", { name: "src/main.go" }).textContent).toContain(
        "package main",
      );
      expect(row(600)).toBeNull();
    });
  });

  describe("prepend scroll continuity", () => {
    async function openMiddle() {
      vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(() => {});
      readFile.mockResolvedValueOnce(lineWindow(1300, 1302, 3000));
      openWorkspaceFile("src/big.go");
      mount(<FileWorkspace />);
      await vi.waitFor(() => expect(row(1300)).toBeTruthy());
      const host = controlGeometry(screen.getByRole("region", { name: "src/big.go" }));
      host.scrollTop = 10;
      const earlier = Promise.withResolvers<WorkspaceFileContent>();
      readFile.mockReturnValueOnce(earlier.promise);
      fireEvent.click(screen.getByRole("button", { name: /earlier/i }));
      return { host, earlier };
    }

    it("keeps the reader's rows in place when earlier lines arrive", async () => {
      const { host, earlier } = await openMiddle();
      await act(async () => earlier.resolve(lineWindow(1100, 1302, 3000)));
      await vi.waitFor(() => expect(row(1100)).toBeTruthy());
      expect(host.scrollTop).toBe(10 + 200 * ROW);
    });

    it("drops a superseded prepend when a line navigation replaces the window", async () => {
      const { host, earlier } = await openMiddle();
      const navigation = Promise.withResolvers<WorkspaceFileContent>();
      readFile.mockReturnValueOnce(navigation.promise);
      act(() => openWorkspaceFile("src/big.go", 2000));
      await act(async () => earlier.resolve(lineWindow(1100, 1302, 3000)));
      await act(async () => navigation.resolve(lineWindow(1800, 2200, 3000)));
      await vi.waitFor(() => expect(row(2200)).toBeTruthy());
      expect(host.scrollTop).toBe(10);
    });

    it("drops a superseded prepend when later lines replace the window", async () => {
      const { host, earlier } = await openMiddle();
      const later = Promise.withResolvers<WorkspaceFileContent>();
      readFile.mockReturnValueOnce(later.promise);
      fireEvent.click(screen.getByRole("button", { name: /more/i }));
      await act(async () => earlier.resolve(lineWindow(1100, 1302, 3000)));
      await act(async () => later.resolve(lineWindow(1300, 1502, 3000)));
      await vi.waitFor(() => expect(row(1502)).toBeTruthy());
      expect(host.scrollTop).toBe(10);
    });
  });

  describe("default file arrival", () => {
    it("opens a cached file at its top instead of the previous file's position", async () => {
      readFile.mockImplementation(async ({ path }) =>
        path === "src/main.ts" ? lineWindow(1, 39, 39) : lineWindow(1, 155, 155),
      );
      openWorkspaceFile("src/main.ts");
      const retained = mountRetained();
      await vi.waitFor(() => expect(row(39)).toBeTruthy());
      const host = controlGeometry(screen.getByRole("region", { name: "src/main.ts" }));

      act(() => openWorkspaceFile("package.json"));
      await vi.waitFor(() => expect(row(155)).toBeTruthy());
      expect(screen.getByRole("region", { name: "package.json" })).toBe(host);
      host.scrollTop = 2214;
      await act(() => client.refetchQueries({ type: "active" }));
      retained.hide();
      retained.reveal();
      expect(host.scrollTop).toBe(2214);

      const reads = readFile.mock.calls.length;
      act(() => openWorkspaceFile("src/main.ts"));
      expect(row(39)).toBeTruthy();
      expect(readFile).toHaveBeenCalledTimes(reads);
      expect(host.scrollTop).toBe(0);
    });

    it("reopens a file at its top after a targeted range", async () => {
      vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(() => {});
      readFile.mockImplementation(async ({ startLine, endLine }) =>
        lineWindow(startLine ?? 1, endLine ?? 400, 1000),
      );
      openWorkspaceFile("src/main.ts", 450);
      mount(<FileWorkspace />);
      await vi.waitFor(() => expect(row(450)).toBeTruthy());
      const host = controlGeometry(screen.getByRole("region", { name: "src/main.ts" }));
      host.scrollTop = 3482;

      fireEvent.click(screen.getByRole("button", { name: /more/i }));
      await vi.waitFor(() => expect(row(850)).toBeTruthy());
      expect(host.scrollTop).toBe(3482);

      act(() => openWorkspaceFile("src/main.ts"));
      await vi.waitFor(() => expect(row(450)).toBeNull());
      expect(row(1)).toBeTruthy();
      expect(host.scrollTop).toBe(0);
    });

    it("reopens a rendered file at its top in its preferred mode", async () => {
      readFile.mockResolvedValue({ content: "# Guide", startLine: 1, totalLines: 1 });
      openWorkspaceFile("docs/reading-guide.md");
      mount(<FileWorkspace />);
      await screen.findByTestId("rendered-markdown");
      const host = controlGeometry(screen.getByRole("region", { name: "docs/reading-guide.md" }));
      host.scrollTop = 753;

      act(() => openWorkspaceFile("docs/reading-guide.md"));
      await screen.findByTestId("rendered-markdown");
      expect(host.scrollTop).toBe(0);

      host.scrollTop = 753;
      fireEvent.click(screen.getByRole("tab", { name: "Source" }));
      expect(screen.queryByTestId("rendered-markdown")).toBeNull();
      expect(host.scrollTop).toBe(753);

      act(() => openWorkspaceFile("docs/reading-guide.md"));
      await screen.findByTestId("rendered-markdown");
      expect(host.scrollTop).toBe(0);
    });
  });

  it("renders a whole file through the renderer registered for its extension", async () => {
    readFile.mockResolvedValue({ content: "# Title", startLine: 1, totalLines: 1 });
    openWorkspaceFile("docs/README.md");
    mount(
      <>
        <button>Open file</button>
        <FileWorkspace />
      </>,
    );
    const caller = screen.getByRole("button", { name: "Open file" });
    act(() => caller.focus());

    expect((await screen.findByTestId("rendered-markdown")).textContent).toBe("# Title");
    expect(document.activeElement).toBe(caller);
    const source = screen.getByRole("tab", { name: "Source" });
    act(() => source.focus());
    fireEvent.click(source);
    expect(await screen.findByText("# Title")).toBeTruthy();
    expect(document.activeElement).toBe(source);
    const preview = screen.getByRole("tab", { name: "Preview" });
    act(() => preview.focus());
    fireEvent.click(preview);
    await screen.findByTestId("rendered-markdown");
    expect(document.activeElement).toBe(preview);
  });

  it("keeps truncated files in their source presentation", async () => {
    readFile.mockResolvedValue({
      content: "# Truncated",
      startLine: 1,
      totalLines: 1,
      truncated: true,
    });
    openWorkspaceFile("docs/README.md");
    mount(<FileWorkspace />);
    await screen.findByText("# Truncated");
    expect(screen.queryByTestId("rendered-markdown")).toBeNull();
  });

  it("follows a line reference in source even when the whole file is available", async () => {
    readFile.mockResolvedValue({ content: "# First\nSecond", startLine: 1, totalLines: 2 });
    openWorkspaceFile("docs/README.md", 2);
    mount(<FileWorkspace />);
    await screen.findByText("Second");
    expect(screen.queryByTestId("rendered-markdown")).toBeNull();
  });

  it("retires the file renderer without losing the captured source", async () => {
    readFile.mockResolvedValue({ content: "# Captured", startLine: 1, totalLines: 1 });
    openWorkspaceFile("docs/README.md");
    mount(<FileWorkspace />);
    await screen.findByTestId("rendered-markdown");
    await resetKernelForTest();
    expect(screen.queryByTestId("rendered-markdown")).toBeNull();
    expect(screen.getByText("# Captured")).toBeTruthy();
  });

  it("waits for an unresolved Session workspace before following a line reference", async () => {
    selection.current = { status: "resolving", sessionId: "session-loading" };
    openWorkspaceFile("src/main.go", 450);
    const view = mount(<FileWorkspace />);
    selection.current = { status: "ready", cwd: "/work/project" };
    view.rerender(
      <QueryClientProvider client={client}>
        <FileWorkspace />
      </QueryClientProvider>,
    );
    await screen.findByText("package main");
    expect(readFile.mock.calls).toEqual([
      [
        {
          cwd: "/work/project",
          path: "src/main.go",
          startLine: 250,
          endLine: 650,
        },
      ],
    ]);
  });

  it("browses and reads files in one panel, restoring its tree cursor and focus on return", async () => {
    mount(<FileWorkspace />);
    fireEvent.click(await screen.findByRole("treeitem", { name: "src" }));
    const fileRow = await screen.findByRole("treeitem", { name: "main.go" });
    act(() => fileRow.focus());
    fireEvent.keyDown(fileRow, { key: "Enter" });
    await screen.findByText("package main");
    expect(navigator().get().dock).toBe("file");
    expect(useContextDockStore.getState().dockViewIds).toEqual(["file"]);

    const back = screen.getByRole("button", { name: "Back to files" });
    expect(document.activeElement).toBe(back);
    fireEvent.click(back);
    const returned = await screen.findByRole("treeitem", { name: "main.go" });
    expect(returned.tabIndex).toBe(0);
    expect(document.activeElement).toBe(returned);
    expect(screen.queryByText("package main")).toBeNull();
    expect(useContextDockStore.getState().dockViewIds).toEqual(["file"]);
    fireEvent.keyDown(returned, { key: "ArrowUp" });
    expect(document.activeElement).toBe(screen.getByRole("treeitem", { name: "src" }));
  });

  it("preserves another control's focus when file selection changes", async () => {
    mount(
      <>
        <button>Another control</button>
        <FileWorkspace />
      </>,
    );
    fireEvent.click(await screen.findByRole("treeitem", { name: "src" }));
    const row = await screen.findByRole("treeitem", { name: "main.go" });
    act(() => row.focus());
    fireEvent.keyDown(row, { key: "Enter" });
    await screen.findByText("package main");
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Back to files" }));
    const other = screen.getByRole("button", { name: "Another control" });
    act(() => other.focus());
    act(() => closeWorkspaceFile());
    await screen.findByRole("treeitem", { name: "main.go" });
    expect(document.activeElement).toBe(other);
    act(() => openWorkspaceFile("src/main.go"));
    await screen.findByText("package main");
    expect(document.activeElement).toBe(other);
  });

  it("shows a failed directory read and retries it without closing the directory", async () => {
    listFiles.mockRejectedValueOnce(new Error("directory unavailable"));
    mount(<FileTree onReveal={() => {}} entries={[directory]} onSelectFile={() => {}} />);
    fireEvent.click(screen.getByRole("treeitem", { name: "src" }));

    fireEvent.click(await screen.findByRole("button", { name: "Retry" }));
    expect(await screen.findByRole("treeitem", { name: "main.go" })).toBeTruthy();
    expect(listFiles).toHaveBeenCalledTimes(2);
  });

  it.each([false, true])(
    "keeps a retried directory's folder as the tree cursor through failure and recovery (typing while listing: %s)",
    async (typeWhileListing) => {
      mount(
        <>
          <textarea aria-label="Message" defaultValue="Retained message draft" />
          <FileWorkspace />
        </>,
      );
      const folder = await screen.findByRole("treeitem", { name: "src" });
      const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message" });
      act(() => folder.focus());
      listFiles.mockRejectedValueOnce(new Error("directory unavailable"));
      fireEvent.keyDown(folder, { key: "ArrowRight" });

      for (const { typing, settle } of [
        {
          typing: false,
          settle: (listing: PromiseWithResolvers<WorkspaceFileEntry[]>) =>
            listing.reject(new Error("directory unavailable")),
        },
        {
          typing: typeWhileListing,
          settle: (listing: PromiseWithResolvers<WorkspaceFileEntry[]>) => listing.resolve([file]),
        },
      ]) {
        const retry = await screen.findByRole("button", { name: "Retry" });
        act(() => retry.focus());
        const listing = Promise.withResolvers<WorkspaceFileEntry[]>();
        listFiles.mockReturnValueOnce(listing.promise);

        fireEvent.click(retry);
        expect(document.activeElement).toBe(folder);
        await waitFor(() => expect(screen.queryByRole("button", { name: "Retry" })).toBeNull());
        expect(document.activeElement).toBe(folder);
        expect(folder.getAttribute("aria-expanded")).toBe("true");
        if (typing) {
          act(() => draft.focus());
          fireEvent.change(draft, { target: { value: "Retained message draft, continued" } });
        }

        await act(async () => settle(listing));
        await waitFor(() => expect(client.isFetching()).toBe(0));
        expect(document.activeElement).toBe(typing ? draft : folder);
        expect(folder.tabIndex).toBe(0);
        expect(folder.getAttribute("aria-expanded")).toBe("true");
      }
      expect(listFiles.mock.calls).toEqual([
        [{ cwd: undefined }],
        ...Array.from({ length: 3 }, () => [{ cwd: undefined, path: "src" }]),
      ]);
      expect(draft.value).toBe(
        typeWhileListing ? "Retained message draft, continued" : "Retained message draft",
      );
      act(() => screen.getByRole("tree", { name: "Files" }).focus());
      expect(document.activeElement).toBe(folder);

      fireEvent.keyDown(folder, { key: "ArrowRight" });
      const child = screen.getByRole("treeitem", { name: "main.go" });
      expect(document.activeElement).toBe(child);
      fireEvent.keyDown(child, { key: "Enter" });
      await screen.findByText("package main");
      expect(readFile).toHaveBeenCalledWith({ cwd: undefined, path: "src/main.go" });

      const back = screen.getByRole("button", { name: "Back to files" });
      expect(document.activeElement).toBe(back);
      fireEvent.click(back);
      const returned = await screen.findByRole("treeitem", { name: "main.go" });
      expect(returned.getAttribute("aria-current")).toBe("true");
      expect(document.activeElement).toBe(returned);
    },
  );

  it("keeps a retried file read's focus in the reader through failure and recovery", async () => {
    readFile.mockRejectedValueOnce(new Error("file unavailable"));
    openWorkspaceFile("src/main.go");
    mount(
      <>
        <textarea aria-label="Message" defaultValue="Retained message draft" />
        <FileWorkspace />
      </>,
    );
    const host = screen.getByRole("region", { name: "src/main.go" });
    const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message" });
    const fail = (read: PromiseWithResolvers<WorkspaceFileContent>) =>
      read.reject(new Error("file unavailable"));

    for (const { typeWhileReading, settle } of [
      { typeWhileReading: false, settle: fail },
      { typeWhileReading: true, settle: fail },
      {
        typeWhileReading: false,
        settle: (read: PromiseWithResolvers<WorkspaceFileContent>) =>
          read.resolve({ content: "package main", startLine: 1, totalLines: 1 }),
      },
    ]) {
      const retry = await screen.findByRole("button", { name: "Retry" });
      act(() => retry.focus());
      const read = Promise.withResolvers<WorkspaceFileContent>();
      readFile.mockReturnValueOnce(read.promise);

      fireEvent.click(retry);
      expect(document.activeElement).toBe(host);
      await waitFor(() => expect(screen.queryByRole("button", { name: "Retry" })).toBeNull());
      expect(document.activeElement).toBe(host);
      if (typeWhileReading) {
        act(() => draft.focus());
        fireEvent.change(draft, { target: { value: "Retained message draft, continued" } });
      }

      await act(async () => settle(read));
      await waitFor(() => expect(client.isFetching()).toBe(0));
      expect(screen.getByRole("region", { name: "src/main.go" })).toBe(host);
      expect(document.activeElement).toBe(typeWhileReading ? draft : host);
    }
    expect(host.textContent).toContain("package main");
    expect(draft.value).toBe("Retained message draft, continued");
    expect(readFile.mock.calls).toEqual(
      Array.from({ length: 4 }, () => [{ cwd: undefined, path: "src/main.go" }]),
    );
  });

  it("finds the file it last opened in the tree, with its folders open", async () => {
    const listing = Promise.withResolvers<WorkspaceFileEntry[]>();
    listFiles.mockImplementation(async (query) => (query.path ? [file] : listing.promise));
    openWorkspaceFile("src/main.go");
    mount(<FileWorkspace />);
    await screen.findByText("package main");

    const back = screen.getByRole("button", { name: "Back to files" });
    act(() => back.focus());
    fireEvent.click(back);
    expect(screen.queryByRole("treeitem", { name: "src" })).toBeNull();
    expect(document.activeElement).toBe(document.body);
    await act(async () => listing.resolve([directory]));
    const row = await screen.findByRole("treeitem", { name: "main.go" });
    expect(row.getAttribute("aria-current")).toBe("true");
    expect(row.getAttribute("title")).toBe("src/main.go");
    expect(document.activeElement).toBe(screen.getByRole("treeitem", { name: "src" }));
  });

  it("keeps focus where the user moved it while the returning file tree loads", async () => {
    const listing = Promise.withResolvers<WorkspaceFileEntry[]>();
    listFiles.mockImplementation(async (query) => (query.path ? [file] : listing.promise));
    openWorkspaceFile("src/main.go");
    mount(
      <>
        <button>Another control</button>
        <FileWorkspace />
      </>,
    );
    await screen.findByText("package main");
    const back = screen.getByRole("button", { name: "Back to files" });
    act(() => back.focus());
    fireEvent.click(back);
    expect(screen.queryByRole("treeitem", { name: "src" })).toBeNull();
    const control = screen.getByRole("button", { name: "Another control" });
    act(() => control.focus());

    await act(async () => listing.resolve([directory]));
    await screen.findByRole("treeitem", { name: "main.go" });
    expect(document.activeElement).toBe(control);
  });

  it.each([true, false])(
    "offers native file actions only for a local Runtime (%s)",
    async (local) => {
      isLocal = local;
      selection.current = { status: "ready", cwd: "/work/project" };
      readFile.mockRejectedValue(
        new RpcError({ code: -32602, message: "binary", data: { type: "unsupported_mime" } }),
      );
      openWorkspaceFile("assets/logo.png");
      mount(<FileWorkspace />);

      expect(await screen.findByText("No image preview here yet")).toBeTruthy();
      expect(Boolean(screen.queryByRole("button", { name: "Open" }))).toBe(local);
      expect(Boolean(screen.queryByRole("button", { name: "Show in Finder" }))).toBe(local);
      expect(screen.queryByRole("button", { name: "Go to line" })).toBeNull();
    },
  );
});

describe("file preview path identity", () => {
  const directoryPath = "packages/runtime/internal/adapters/storage/sqlite/migrations/versioned";
  const longName = "0042_rename_session_segment_items_to_run_ledger_entries_with_backfill.go";
  const longPath = `${directoryPath}/${longName}`;
  const header = () => document.querySelector<HTMLElement>('[data-slot="workspace-view-header"]')!;

  // The reader shows the path itself; its region name and the header title are not visible text.
  function shownPath(path: string) {
    const region = screen.getByRole("region", { name: path });
    const shown = within(region).getByText(path);
    expect(shown.closest(".sr-only, caption, [hidden], [aria-hidden='true']")).toBeNull();
    return { region, shown };
  }

  function precedes(first: Node, second: Node) {
    return Boolean(first.compareDocumentPosition(second) & Node.DOCUMENT_POSITION_FOLLOWING);
  }

  it("keeps the header to the filename and shows the complete path ahead of the source", async () => {
    openWorkspaceFile(longPath);
    mount(<FileWorkspace />);
    await screen.findByText("package main");

    const { shown } = shownPath(longPath);
    expect(precedes(shown, document.querySelector('[data-quote-source="file"]')!)).toBe(true);
    const bar = header();
    expect(bar.contains(shown)).toBe(false);
    expect(bar.textContent).not.toContain(directoryPath);
    expect(within(bar).getByText(longName).closest("[title]")?.getAttribute("title")).toBe(
      longPath,
    );
    expect(within(bar).getByText("1 line")).toBeTruthy();
    expect(within(bar).getByRole("button", { name: "Back to files" })).toBeTruthy();
    expect(within(bar).getByRole("button", { name: "Go to line" })).toBeTruthy();
    expect(within(bar).getByRole("button", { name: "Copy path" })).toBeTruthy();
  });

  it("follows the opened file's path in the same reader", async () => {
    readFile.mockResolvedValueOnce({ content: "package first", startLine: 1, totalLines: 1 });
    openWorkspaceFile(longPath);
    mount(<FileWorkspace />);
    await screen.findByText("package first");
    const { region } = shownPath(longPath);

    readFile.mockResolvedValueOnce({ content: "package next", startLine: 1, totalLines: 1 });
    act(() => openWorkspaceFile("cmd/flame/main.go"));
    await screen.findByText("package next");
    expect(shownPath("cmd/flame/main.go").region).toBe(region);
    expect(within(region).queryByText(longPath)).toBeNull();
    expect(header().textContent).not.toContain("cmd/flame");
  });

  it("shows the complete path ahead of both rendered and source presentations", async () => {
    const path = "docs/guides/runtime/reading-guide.md";
    readFile.mockResolvedValue({ content: "# Guide", startLine: 1, totalLines: 1 });
    openWorkspaceFile(path);
    mount(<FileWorkspace />);

    const rendered = await screen.findByTestId("rendered-markdown");
    expect(precedes(shownPath(path).shown, rendered)).toBe(true);
    fireEvent.click(screen.getByRole("tab", { name: "Source" }));
    expect(
      precedes(shownPath(path).shown, document.querySelector('[data-quote-source="file"]')!),
    ).toBe(true);
    expect(header().textContent).not.toContain("docs/guides/runtime");
  });

  it("shows the complete path while a read is pending and after it fails", async () => {
    const read = Promise.withResolvers<WorkspaceFileContent>();
    readFile.mockReturnValueOnce(read.promise);
    openWorkspaceFile(longPath);
    mount(<FileWorkspace />);
    shownPath(longPath);

    await act(async () => read.reject(new Error("file unavailable")));
    expect(await screen.findByText("Couldn't open file")).toBeTruthy();
    shownPath(longPath);
    expect(header().textContent).not.toContain(directoryPath);
    expect(within(header()).getByText(longName)).toBeTruthy();
  });

  it("shows the complete path beside an unsupported file", async () => {
    const path = "assets/brand/marketing/launch/logo.png";
    readFile.mockRejectedValue(
      new RpcError({ code: -32602, message: "binary", data: { type: "unsupported_mime" } }),
    );
    openWorkspaceFile(path);
    mount(<FileWorkspace />);

    expect(await screen.findByText("No image preview here yet")).toBeTruthy();
    shownPath(path);
    expect(header().textContent).not.toContain("assets/brand");
    expect(within(header()).getByText("logo.png")).toBeTruthy();
    expect(within(header()).getByRole("button", { name: "Copy path" })).toBeTruthy();
  });
});

describe("root file listing refresh continuity", () => {
  const readme: WorkspaceFileEntry = { name: "README.md", path: "README.md", type: "file" };
  const guide: WorkspaceFileEntry = { name: "guide.txt", path: "guide.txt", type: "file" };
  const notes: WorkspaceFileEntry = { name: "NOTES.md", path: "NOTES.md", type: "file" };
  const rootQuery = (cwd?: string) => ({
    queryKey: [WORKSPACE_LIST_FILES_KEY, { cwd }],
    exact: true,
  });
  const unavailable = () => new Error("listing unavailable");

  it("keeps the accepted tree, its cursor and focus through failed refreshes and retries", async () => {
    listFiles.mockImplementation(async (query) =>
      query.path ? [file] : [directory, readme, guide],
    );
    mount(
      <>
        <textarea aria-label="Message" defaultValue="Retained message draft" />
        <FileWorkspace />
      </>,
    );
    const opened = await screen.findByRole("treeitem", { name: "guide.txt" });
    act(() => opened.focus());
    fireEvent.keyDown(opened, { key: "Enter" });
    await screen.findByText("package main");
    fireEvent.click(screen.getByRole("button", { name: "Back to files" }));
    const selected = await screen.findByRole("treeitem", { name: "guide.txt" });
    expect(document.activeElement).toBe(selected);
    fireEvent.keyDown(selected, { key: "ArrowUp" });
    const cursor = screen.getByRole("treeitem", { name: "README.md" });
    expect(document.activeElement).toBe(cursor);
    const tree = screen.getByRole("tree", { name: "Files" });
    const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message" });

    const refresh = Promise.withResolvers<WorkspaceFileEntry[]>();
    listFiles.mockReturnValueOnce(refresh.promise);
    act(() => void client.refetchQueries(rootQuery()));
    await waitFor(() => expect(listFiles).toHaveBeenCalledTimes(2));
    expect(screen.queryByRole("status")).toBeNull();

    await act(async () => refresh.reject(unavailable()));
    const feedback = await screen.findByRole("status");
    expect(feedback.textContent).toContain("Couldn't refresh files");
    expect(feedback.textContent).toContain("Showing the last loaded files.");
    expect(screen.getByRole("tree", { name: "Files" })).toBe(tree);
    expect(screen.getByRole("treeitem", { name: "README.md" })).toBe(cursor);
    expect(cursor.tabIndex).toBe(0);
    expect(document.activeElement).toBe(cursor);

    for (const { typing, settle } of [
      {
        typing: false,
        settle: (listing: PromiseWithResolvers<WorkspaceFileEntry[]>) =>
          listing.reject(unavailable()),
      },
      {
        typing: true,
        settle: (listing: PromiseWithResolvers<WorkspaceFileEntry[]>) =>
          listing.resolve([notes, directory, readme, guide]),
      },
    ]) {
      const retry = await screen.findByRole("button", { name: "Retry" });
      act(() => retry.focus());
      const listing = Promise.withResolvers<WorkspaceFileEntry[]>();
      listFiles.mockReturnValueOnce(listing.promise);

      fireEvent.click(retry);
      expect(document.activeElement).toBe(cursor);
      await waitFor(() => expect(screen.queryByRole("button", { name: "Retry" })).toBeNull());
      expect(screen.getByRole("status").textContent).toContain("Loading");
      expect(screen.getByRole("treeitem", { name: "README.md" })).toBe(cursor);
      expect(document.activeElement).toBe(cursor);
      if (typing) {
        act(() => draft.focus());
        fireEvent.change(draft, { target: { value: "Retained message draft, continued" } });
      }

      await act(async () => settle(listing));
      await waitFor(() => expect(client.isFetching()).toBe(0));
      expect(screen.getByRole("tree", { name: "Files" })).toBe(tree);
      expect(screen.getByRole("treeitem", { name: "README.md" })).toBe(cursor);
      expect(cursor.tabIndex).toBe(0);
      expect(document.activeElement).toBe(typing ? draft : cursor);
    }
    expect(screen.queryByRole("status")).toBeNull();
    expect(screen.getByRole("treeitem", { name: "NOTES.md" })).toBeTruthy();
    expect(draft.value).toBe("Retained message draft, continued");
    expect(cursor.getAttribute("aria-current")).toBeNull();
    expect(selected.getAttribute("aria-current")).toBe("true");
    expect(listFiles.mock.calls).toEqual(Array.from({ length: 4 }, () => [{ cwd: undefined }]));

    act(() => tree.focus());
    expect(document.activeElement).toBe(cursor);
    fireEvent.keyDown(cursor, { key: "ArrowUp" });
    fireEvent.keyDown(document.activeElement!, { key: "ArrowUp" });
    const added = screen.getByRole("treeitem", { name: "NOTES.md" });
    expect(document.activeElement).toBe(added);
    fireEvent.keyDown(added, { key: "Enter" });
    const back = await screen.findByRole("button", { name: "Back to files" });
    expect(readFile).toHaveBeenLastCalledWith({ cwd: undefined, path: "NOTES.md" });
    fireEvent.click(back);
    const returned = await screen.findByRole("treeitem", { name: "NOTES.md" });
    expect(returned.getAttribute("aria-current")).toBe("true");
    expect(document.activeElement).toBe(returned);
  });

  it.each([
    { label: "a displaced visible row", before: 20, after: 120, drafting: false, scrolls: true },
    { label: "a row that stays visible", before: 20, after: 40, drafting: false, scrolls: false },
    {
      label: "a row already scrolled away",
      before: 120,
      after: 220,
      drafting: false,
      scrolls: false,
    },
    { label: "later draft focus", before: 20, after: 120, drafting: true, scrolls: false },
  ])("preserves viewport intent for $label when refresh feedback changes layout", async (scene) => {
    mount(
      <>
        <textarea aria-label="Message" defaultValue="Keep drafting" />
        <FileWorkspace />
      </>,
    );
    const row = await screen.findByRole("treeitem", { name: "src" });
    const tree = screen.getByRole("tree", { name: "Files" });
    const viewport = tree.parentElement!;
    const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message" });
    vi.spyOn(viewport, "getBoundingClientRect").mockReturnValue(new DOMRect(0, 0, 100, 100));
    vi.spyOn(row, "getBoundingClientRect").mockImplementation(
      () => new DOMRect(0, screen.queryByRole("status") ? scene.after : scene.before, 100, 20),
    );
    const reveal = vi.spyOn(row, "scrollIntoView").mockImplementation(() => {});
    act(() => row.focus());
    const refresh = Promise.withResolvers<WorkspaceFileEntry[]>();
    listFiles.mockReturnValueOnce(refresh.promise);
    act(() => void client.refetchQueries(rootQuery()));
    await waitFor(() => expect(listFiles).toHaveBeenCalledTimes(2));
    if (scene.drafting) {
      act(() => draft.focus());
      fireEvent.change(draft, { target: { value: "Keep drafting, continued" } });
    }

    await act(async () => refresh.reject(unavailable()));
    await screen.findByRole("status");
    expect(screen.getByRole("treeitem", { name: "src" })).toBe(row);
    expect(reveal.mock.calls).toEqual(
      scene.scrolls ? [[{ block: "nearest", inline: "nearest" }]] : [],
    );
    expect(document.activeElement).toBe(scene.drafting ? draft : row);
    expect(draft.value).toBe(scene.drafting ? "Keep drafting, continued" : "Keep drafting");
  });

  it.each([false, true])(
    "keeps ordinary failure recovery when no files were accepted (empty listing accepted: %s)",
    async (acceptEmpty) => {
      if (acceptEmpty) listFiles.mockResolvedValueOnce([]);
      listFiles.mockRejectedValueOnce(unavailable());
      mount(<FileWorkspace />);
      if (acceptEmpty) {
        await screen.findByText("No files");
        await act(() => client.refetchQueries(rootQuery()));
      }

      const retry = await screen.findByRole("button", { name: "Retry" });
      expect(screen.getByText("Couldn't load")).toBeTruthy();
      expect(screen.queryByText("Couldn't refresh files")).toBeNull();
      expect(screen.queryByRole("tree")).toBeNull();
      fireEvent.click(retry);
      expect(await screen.findByRole("treeitem", { name: "src" })).toBeTruthy();
    },
  );

  it("explains an unsupported refresh beside the accepted files without a retry", async () => {
    mount(<FileWorkspace />);
    const row = await screen.findByRole("treeitem", { name: "src" });
    listFiles.mockRejectedValueOnce(
      new RpcError({ message: "method not found", data: { type: "method_not_found" } }),
    );
    await act(() => client.refetchQueries(rootQuery()));

    const tree = screen.getByRole("tree", { name: "Files" });
    const feedback = await screen.findByRole("status");
    expect(feedback.textContent).toContain("Not supported by this runtime");
    expect(feedback.textContent).toContain("Showing the last loaded files.");
    expect(feedback.querySelector('[data-icon-name="folder"]')).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
    expect(screen.getByRole("treeitem", { name: "src" })).toBe(row);
    expect(tree.contains(row)).toBe(true);
  });

  it("drops the previous workspace's files when the workspace changes under refresh feedback", async () => {
    selection.current = { status: "ready", cwd: "/work/one" };
    const view = mount(<FileWorkspace />);
    await screen.findByRole("treeitem", { name: "src" });
    listFiles.mockRejectedValueOnce(unavailable());
    await act(() => client.refetchQueries(rootQuery("/work/one")));
    await screen.findByText("Couldn't refresh files");

    const next = Promise.withResolvers<WorkspaceFileEntry[]>();
    listFiles.mockReturnValueOnce(next.promise);
    selection.current = { status: "ready", cwd: "/work/two" };
    view.rerender(
      <QueryClientProvider client={client}>
        <FileWorkspace />
      </QueryClientProvider>,
    );
    expect(screen.queryByRole("tree")).toBeNull();
    expect(screen.queryByText("Couldn't refresh files")).toBeNull();
    expect(screen.getByText("Loading")).toBeTruthy();

    await act(async () => next.resolve([readme]));
    expect(await screen.findByRole("treeitem", { name: "README.md" })).toBeTruthy();
    expect(screen.queryByRole("treeitem", { name: "src" })).toBeNull();
    expect(listFiles).toHaveBeenLastCalledWith({ cwd: "/work/two" });
  });
});

describe("expanded directory refresh continuity", () => {
  const readme: WorkspaceFileEntry = { name: "README.md", path: "README.md", type: "file" };
  const util: WorkspaceFileEntry = { name: "util.go", path: "src/util.go", type: "file" };
  const directoryQuery = (cwd?: string) => ({
    queryKey: [WORKSPACE_LIST_FILES_KEY, { cwd, path: "src" }],
    exact: true,
  });
  const unavailable = () => new Error("directory unavailable");
  const labels = () => screen.getAllByRole("treeitem").map((row) => row.getAttribute("aria-label"));

  async function expandWithCursorInChild() {
    const folder = await screen.findByRole("treeitem", { name: "src" });
    fireEvent.click(folder);
    const child = await screen.findByRole("treeitem", { name: "main.go" });
    act(() => child.focus());
    return { folder, child };
  }

  it("keeps the directory's accepted children, cursor and focus through failed refreshes and retries", async () => {
    listFiles.mockImplementation(async (query) => (query.path ? [file] : [directory, readme]));
    useContextDockStore.setState({ memory: { expandedDirs: [], lastFilePath: file.path } });
    mount(
      <>
        <textarea aria-label="Message" defaultValue="Retained message draft" />
        <FileWorkspace />
      </>,
    );
    const { folder, child } = await expandWithCursorInChild();
    const tree = screen.getByRole("tree", { name: "Files" });
    const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message" });
    expect(child.getAttribute("aria-current")).toBe("true");
    const group = ownedGroup(folder)!;
    expect(group.contains(child)).toBe(true);

    const refresh = Promise.withResolvers<WorkspaceFileEntry[]>();
    listFiles.mockReturnValueOnce(refresh.promise);
    act(() => void client.refetchQueries(directoryQuery()));
    await waitFor(() => expect(listFiles).toHaveBeenCalledTimes(3));
    expect(screen.queryByRole("status")).toBeNull();

    await act(async () => refresh.reject(unavailable()));
    const feedback = await screen.findByRole("status");
    expect(feedback.textContent).toContain("Couldn't refresh files");
    expect(feedback.textContent).toContain("Showing the last loaded files in src.");
    expect(child.closest('[role="group"]')!.lastElementChild).toBe(feedback);
    expect(ownedGroup(folder)).toBe(group);
    expect(labels()).toEqual(["src", "main.go", "README.md"]);
    expect(screen.getByRole("tree", { name: "Files" })).toBe(tree);
    expect(screen.getByRole("treeitem", { name: "main.go" })).toBe(child);
    expect(child.tabIndex).toBe(0);
    expect(folder.tabIndex).toBe(-1);
    expect(folder.getAttribute("aria-expanded")).toBe("true");
    expect(document.activeElement).toBe(child);

    for (const { typing, settle } of [
      {
        typing: false,
        settle: (listing: PromiseWithResolvers<WorkspaceFileEntry[]>) =>
          listing.reject(unavailable()),
      },
      {
        typing: true,
        settle: (listing: PromiseWithResolvers<WorkspaceFileEntry[]>) =>
          listing.reject(unavailable()),
      },
      {
        typing: true,
        settle: (listing: PromiseWithResolvers<WorkspaceFileEntry[]>) =>
          listing.resolve([file, util]),
      },
    ]) {
      const retry = await screen.findByRole("button", { name: "Retry" });
      act(() => retry.focus());
      const listing = Promise.withResolvers<WorkspaceFileEntry[]>();
      listFiles.mockReturnValueOnce(listing.promise);

      fireEvent.click(retry);
      expect(document.activeElement).toBe(child);
      await waitFor(() => expect(screen.queryByRole("button", { name: "Retry" })).toBeNull());
      expect(screen.getByRole("status").textContent).toContain("Loading");
      expect(screen.getByRole("treeitem", { name: "main.go" })).toBe(child);
      expect(document.activeElement).toBe(child);
      if (typing) {
        act(() => draft.focus());
        fireEvent.change(draft, { target: { value: "Retained message draft, continued" } });
      }

      await act(async () => settle(listing));
      await waitFor(() => expect(client.isFetching()).toBe(0));
      expect(screen.getByRole("tree", { name: "Files" })).toBe(tree);
      expect(screen.getByRole("treeitem", { name: "main.go" })).toBe(child);
      expect(child.tabIndex).toBe(0);
      expect(child.getAttribute("aria-current")).toBe("true");
      expect(folder.getAttribute("aria-expanded")).toBe("true");
      expect(ownedGroup(folder)).toBe(group);
      expect(group.contains(child)).toBe(true);
      expect(document.activeElement).toBe(typing ? draft : child);
    }
    expect(screen.queryByRole("status")).toBeNull();
    expect(labels()).toEqual(["src", "main.go", "util.go", "README.md"]);
    expect(draft.value).toBe("Retained message draft, continued");
    expect(listFiles.mock.calls).toEqual([
      [{ cwd: undefined }],
      ...Array.from({ length: 5 }, () => [{ cwd: undefined, path: "src" }]),
    ]);

    act(() => tree.focus());
    expect(document.activeElement).toBe(child);
    fireEvent.keyDown(child, { key: "ArrowDown" });
    const added = screen.getByRole("treeitem", { name: "util.go" });
    expect(document.activeElement).toBe(added);
    fireEvent.keyDown(added, { key: "ArrowLeft" });
    expect(document.activeElement).toBe(folder);
  });

  it("keeps a loaded nested branch usable when its parent refresh fails", async () => {
    const nested: WorkspaceFileEntry = { name: "nested", path: "src/nested", type: "dir" };
    const deep: WorkspaceFileEntry = { name: "main.go", path: "src/nested/main.go", type: "file" };
    listFiles.mockImplementation(async (query) =>
      query.path === "src" ? [nested] : query.path === nested.path ? [deep] : [directory],
    );
    mount(<FileWorkspace />);
    const folder = await screen.findByRole("treeitem", { name: "src" });
    fireEvent.click(folder);
    const branch = await screen.findByRole("treeitem", { name: "nested" });
    fireEvent.click(branch);
    const child = await screen.findByRole("treeitem", { name: "main.go" });
    act(() => child.focus());

    listFiles.mockRejectedValueOnce(unavailable());
    await act(() => client.refetchQueries(directoryQuery()));
    await screen.findByText("Showing the last loaded files in src.");
    expect(screen.getByRole("treeitem", { name: "nested" })).toBe(branch);
    expect(screen.getByRole("treeitem", { name: "main.go" })).toBe(child);
    expect(child.tabIndex).toBe(0);
    expect(branch.getAttribute("aria-expanded")).toBe("true");
    expect(document.activeElement).toBe(child);

    fireEvent.keyDown(child, { key: "ArrowLeft" });
    fireEvent.keyDown(branch, { key: "ArrowLeft" });
    expect(document.activeElement).toBe(branch);
    expect(branch.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByRole("treeitem", { name: "main.go" })).toBeNull();
    fireEvent.keyDown(branch, { key: "ArrowRight" });
    const reopened = await screen.findByRole("treeitem", { name: "main.go" });
    fireEvent.keyDown(branch, { key: "ArrowRight" });
    expect(document.activeElement).toBe(reopened);
    expect(screen.getByText("Showing the last loaded files in src.")).toBeTruthy();
    fireEvent.keyDown(reopened, { key: "Enter" });
    await screen.findByText("package main");
    expect(readFile).toHaveBeenCalledWith({ cwd: undefined, path: deep.path });
    const returning = Promise.withResolvers<WorkspaceFileEntry[]>();
    listFiles.mockReturnValueOnce(returning.promise);
    fireEvent.click(screen.getByRole("button", { name: "Back to files" }));
    const returned = await screen.findByRole("treeitem", { name: "main.go" });
    expect(returned).toBe(reopened);
    expect(returned.getAttribute("aria-current")).toBe("true");
    expect(screen.getByRole("status").textContent).toContain("Loading");
    await act(async () => returning.resolve([nested]));
    await waitFor(() => expect(client.isFetching()).toBe(0));
    expect(screen.queryByRole("status")).toBeNull();
    expect(screen.getByRole("treeitem", { name: "main.go" })).toBe(returned);
    expect(document.activeElement).toBe(returned);
  });

  it("explains an unsupported directory refresh beside its accepted children without a retry", async () => {
    mount(<FileTree onReveal={() => {}} entries={[directory, readme]} onSelectFile={() => {}} />);
    const { child } = await expandWithCursorInChild();
    listFiles.mockRejectedValueOnce(
      new RpcError({ message: "method not found", data: { type: "method_not_found" } }),
    );
    await act(() => client.refetchQueries(directoryQuery()));

    const feedback = await screen.findByRole("status");
    expect(feedback.textContent).toContain("Not supported by this runtime");
    expect(feedback.textContent).toContain("Showing the last loaded files in src.");
    expect(feedback.querySelector('[data-icon-name="folder"]')).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
    expect(screen.getByRole("treeitem", { name: "main.go" })).toBe(child);
    expect(document.activeElement).toBe(child);
  });

  it("moves focus to the folder when a retried refresh actually removes the cursor's child", async () => {
    mount(<FileTree onReveal={() => {}} entries={[directory, readme]} onSelectFile={() => {}} />);
    const { folder } = await expandWithCursorInChild();
    listFiles.mockRejectedValueOnce(unavailable());
    await act(() => client.refetchQueries(directoryQuery()));
    const retry = await screen.findByRole("button", { name: "Retry" });
    act(() => retry.focus());
    listFiles.mockResolvedValueOnce([util]);

    fireEvent.click(retry);
    await screen.findByRole("treeitem", { name: "util.go" });
    expect(screen.queryByRole("treeitem", { name: "main.go" })).toBeNull();
    expect(screen.queryByRole("status")).toBeNull();
    expect(folder.tabIndex).toBe(0);
    expect(document.activeElement).toBe(folder);
  });

  it("keeps ordinary failure recovery after an accepted empty directory", async () => {
    listFiles.mockResolvedValueOnce([]);
    mount(<FileTree onReveal={() => {}} entries={[directory]} onSelectFile={() => {}} />);
    fireEvent.click(screen.getByRole("treeitem", { name: "src" }));
    await screen.findByText("No files");
    listFiles.mockRejectedValueOnce(unavailable());
    await act(() => client.refetchQueries(directoryQuery()));

    const retry = await screen.findByRole("button", { name: "Retry" });
    expect(screen.getByText("Couldn't load")).toBeTruthy();
    expect(screen.queryByText("Couldn't refresh files")).toBeNull();
    fireEvent.click(retry);
    expect(await screen.findByRole("treeitem", { name: "main.go" })).toBeTruthy();
  });

  it("drops the previous workspace's children when the workspace changes under refresh feedback", async () => {
    const tree = (cwd: string) => (
      <QueryClientProvider client={client}>
        <FileTree onReveal={() => {}} entries={[directory]} cwd={cwd} onSelectFile={() => {}} />
      </QueryClientProvider>
    );
    const view = render(tree("/work/one"));
    const { folder } = await expandWithCursorInChild();
    listFiles.mockRejectedValueOnce(unavailable());
    await act(() => client.refetchQueries(directoryQuery("/work/one")));
    await screen.findByText("Couldn't refresh files");

    const next = Promise.withResolvers<WorkspaceFileEntry[]>();
    listFiles.mockReturnValueOnce(next.promise);
    view.rerender(tree("/work/two"));
    expect(screen.queryByRole("treeitem", { name: "main.go" })).toBeNull();
    expect(screen.queryByText("Couldn't refresh files")).toBeNull();
    expect(screen.getByRole("status").textContent).toContain("Loading");
    expect(ownedGroup(folder)!.contains(screen.getByRole("status"))).toBe(true);

    await act(async () => next.resolve([util]));
    const added = await screen.findByRole("treeitem", { name: "util.go" });
    expect(ownedGroup(folder)!.contains(added)).toBe(true);
    expect(listFiles).toHaveBeenLastCalledWith({ cwd: "/work/two", path: "src" });
  });
});

describe("file read refresh continuity", () => {
  const guide = "docs/reading-guide.md";
  const readQuery = (path: string, cwd?: string) => ({
    queryKey: [WORKSPACE_READ_FILE_KEY, { cwd, path }],
    exact: true,
  });
  const unavailable = () => new Error("file unavailable");
  const sourceView = () => {
    const view = document.querySelector<HTMLElement>('[data-quote-source="file"]');
    if (!view) throw new Error("source view is not shown");
    return view;
  };

  it("keeps the accepted source, its reader and focus through failed refreshes and retries", async () => {
    readFile.mockResolvedValueOnce({
      content: "# Guide\nAccepted line",
      startLine: 1,
      totalLines: 2,
    });
    openWorkspaceFile(guide);
    mount(
      <>
        <textarea aria-label="Message" defaultValue="Retained message draft" />
        <FileWorkspace />
      </>,
    );
    await screen.findByTestId("rendered-markdown");
    fireEvent.click(screen.getByRole("tab", { name: "Source" }));
    const host = controlGeometry(screen.getByRole("region", { name: guide }));
    const view = sourceView();
    expect(view.textContent).toContain("Accepted line");
    act(() => host.focus());
    host.scrollTop = 713;
    const back = screen.getByRole("button", { name: "Back to files" });
    const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message" });

    const refresh = Promise.withResolvers<WorkspaceFileContent>();
    readFile.mockReturnValueOnce(refresh.promise);
    act(() => void client.refetchQueries(readQuery(guide)));
    await waitFor(() => expect(readFile).toHaveBeenCalledTimes(2));
    expect(screen.queryByText("Couldn't refresh file")).toBeNull();

    await act(async () => refresh.reject(unavailable()));
    const feedback = (await screen.findByText("Couldn't refresh file")).closest('[role="status"]')!;
    expect(feedback.textContent).toContain("Showing the last loaded content.");
    expect(host.contains(feedback)).toBe(false);
    expect(screen.getByRole("region", { name: guide })).toBe(host);
    expect(sourceView()).toBe(view);
    expect(view.textContent).toContain("Accepted line");
    expect(screen.getByRole("tab", { name: "Source", selected: true })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Back to files" })).toBe(back);
    expect(screen.getByRole("button", { name: "Go to line" })).toBeTruthy();
    expect(document.activeElement).toBe(host);
    expect(host.scrollTop).toBe(713);

    for (const { typing, settle } of [
      {
        typing: false,
        settle: (read: PromiseWithResolvers<WorkspaceFileContent>) => read.reject(unavailable()),
      },
      {
        typing: true,
        settle: (read: PromiseWithResolvers<WorkspaceFileContent>) =>
          read.resolve({ content: "# Guide\nUpdated line", startLine: 1, totalLines: 2 }),
      },
    ]) {
      const retry = await screen.findByRole("button", { name: "Retry" });
      act(() => retry.focus());
      const read = Promise.withResolvers<WorkspaceFileContent>();
      readFile.mockReturnValueOnce(read.promise);

      fireEvent.click(retry);
      expect(document.activeElement).toBe(host);
      await waitFor(() => expect(screen.queryByRole("button", { name: "Retry" })).toBeNull());
      const pending = screen.getByRole("status");
      expect(pending.textContent).toContain("Loading");
      expect(host.contains(pending)).toBe(false);
      expect(sourceView()).toBe(view);
      expect(document.activeElement).toBe(host);
      if (typing) {
        act(() => draft.focus());
        fireEvent.change(draft, { target: { value: "Retained message draft, continued" } });
      }

      await act(async () => settle(read));
      await waitFor(() => expect(client.isFetching()).toBe(0));
      expect(screen.getByRole("region", { name: guide })).toBe(host);
      expect(sourceView()).toBe(view);
      expect(document.activeElement).toBe(typing ? draft : host);
      expect(host.scrollTop).toBe(713);
    }
    expect(screen.queryByRole("status")).toBeNull();
    expect(view.textContent).toContain("Updated line");
    expect(view.textContent).not.toContain("Accepted line");
    expect(screen.getByRole("tab", { name: "Source", selected: true })).toBeTruthy();
    expect(draft.value).toBe("Retained message draft, continued");
    expect(readFile.mock.calls).toEqual(
      Array.from({ length: 4 }, () => [{ cwd: undefined, path: guide }]),
    );
  });

  it.each([
    {
      label: "a selected Preview",
      path: guide,
      content: "# Guide",
      updated: "# Updated guide",
      mode: "Preview",
      shown: () => screen.getByTestId("rendered-markdown"),
    },
    {
      label: "accepted empty text",
      path: "src/empty.go",
      content: "",
      updated: "package empty",
      mode: undefined,
      shown: sourceView,
    },
  ])(
    "keeps $label mounted through a failed refresh until updated content arrives",
    async ({ path, content, updated, mode, shown }) => {
      readFile.mockResolvedValueOnce({ content, startLine: 1, totalLines: 1 });
      openWorkspaceFile(path);
      mount(<FileWorkspace />);
      const node = await waitFor(shown);
      const host = screen.getByRole("region", { name: path });
      const text = node.textContent;

      readFile.mockRejectedValueOnce(unavailable());
      await act(() => client.refetchQueries(readQuery(path)));
      await screen.findByText("Couldn't refresh file");
      expect(screen.queryByText("Couldn't open file")).toBeNull();
      expect(screen.getByRole("region", { name: path })).toBe(host);
      expect(shown()).toBe(node);
      expect(node.textContent).toBe(text);
      expect(screen.queryByRole("tab", { selected: true })?.textContent).toBe(mode);

      readFile.mockResolvedValueOnce({ content: updated, startLine: 1, totalLines: 1 });
      fireEvent.click(screen.getByRole("button", { name: "Retry" }));
      await waitFor(() => expect(node.textContent).toContain(updated));
      expect(shown()).toBe(node);
      expect(screen.getByRole("region", { name: path })).toBe(host);
      expect(screen.queryByText("Couldn't refresh file")).toBeNull();
      expect(screen.queryByRole("tab", { selected: true })?.textContent).toBe(mode);
    },
  );

  it("explains an unsupported refresh beside the accepted source without a retry", async () => {
    openWorkspaceFile("src/main.go");
    mount(<FileWorkspace />);
    await screen.findByText("package main");
    const host = screen.getByRole("region", { name: "src/main.go" });
    const view = sourceView();
    readFile.mockRejectedValueOnce(
      new RpcError({ message: "method not found", data: { type: "method_not_found" } }),
    );
    await act(() => client.refetchQueries(readQuery("src/main.go")));

    const feedback = (await screen.findByText("Not supported by this runtime")).closest(
      '[role="status"]',
    )!;
    expect(feedback.textContent).toContain("Showing the last loaded content.");
    expect(host.contains(feedback)).toBe(false);
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
    expect(sourceView()).toBe(view);
    expect(view.textContent).toContain("package main");
  });

  it.each([
    { path: "assets/logo.png", title: "No image preview here yet", hasMode: false },
    { path: guide, title: "This file isn't text", hasMode: true },
  ])(
    "keeps the native classification when cached $path becomes unsupported",
    async ({ path, title, hasMode }) => {
      isLocal = true;
      selection.current = { status: "ready", cwd: "/work/project" };
      readFile.mockResolvedValueOnce({ content: "cached pixels", startLine: 1, totalLines: 1 });
      openWorkspaceFile(path);
      mount(<FileWorkspace />);
      await screen.findByText("cached pixels");
      expect(screen.getByText("1 line")).toBeTruthy();
      if (hasMode) fireEvent.click(screen.getByRole("tab", { name: "Source" }));
      readFile.mockRejectedValueOnce(
        new RpcError({ code: -32602, message: "binary", data: { type: "unsupported_mime" } }),
      );
      await act(() => client.refetchQueries(readQuery(path, "/work/project")));

      expect(await screen.findByText(title)).toBeTruthy();
      expect(screen.getByRole("button", { name: "Open" })).toBeTruthy();
      expect(screen.getByRole("button", { name: "Show in Finder" })).toBeTruthy();
      expect(screen.queryByText("cached pixels")).toBeNull();
      expect(screen.queryByText("Couldn't refresh file")).toBeNull();
      expect(screen.queryByRole("button", { name: "Go to line" })).toBeNull();
      expect(screen.queryByRole("tab", { name: "Source" })).toBeNull();
      expect(screen.queryByRole("tab", { name: "Preview" })).toBeNull();
      expect(screen.queryByText(/^\d+ lines?$/)).toBeNull();
    },
  );

  it.each([
    { label: "another file", change: "path", next: { cwd: "/work/one", path: "src/other.go" } },
    { label: "another workspace", change: "cwd", next: { cwd: "/work/two", path: "src/main.go" } },
  ])(
    "drops the previous read's content when $label opens under refresh feedback",
    async ({ change, next }) => {
      selection.current = { status: "ready", cwd: "/work/one" };
      openWorkspaceFile("src/main.go");
      const view = mount(<FileWorkspace />);
      await screen.findByText("package main");
      readFile.mockRejectedValueOnce(unavailable());
      await act(() => client.refetchQueries(readQuery("src/main.go", "/work/one")));
      await screen.findByText("Couldn't refresh file");

      const read = Promise.withResolvers<WorkspaceFileContent>();
      readFile.mockReturnValueOnce(read.promise);
      selection.current = { status: "ready", cwd: next.cwd };
      if (change === "path") {
        act(() => openWorkspaceFile(next.path));
      } else {
        view.rerender(
          <QueryClientProvider client={client}>
            <FileWorkspace />
          </QueryClientProvider>,
        );
      }
      expect(screen.queryByText("Couldn't refresh file")).toBeNull();
      expect(screen.queryByText("package main")).toBeNull();
      expect(screen.getByRole("status").textContent).toContain("Loading");

      await act(async () => read.resolve({ content: "package next", startLine: 1, totalLines: 1 }));
      expect(await screen.findByText("package next")).toBeTruthy();
      expect(readFile).toHaveBeenLastCalledWith(next);
    },
  );

  it("keeps the ordinary read failure for a new line window without accepted data", async () => {
    readFile.mockResolvedValueOnce(lineWindow(100, 500, 600));
    openWorkspaceFile("src/big.go");
    mount(<FileWorkspace />);
    await waitFor(() => expect(row(500)).toBeTruthy());
    readFile.mockRejectedValueOnce(unavailable());
    await act(() => client.refetchQueries(readQuery("src/big.go")));
    await screen.findByText("Couldn't refresh file");
    expect(row(100)).toBeTruthy();

    const later = Promise.withResolvers<WorkspaceFileContent>();
    readFile.mockReturnValueOnce(later.promise);
    fireEvent.click(screen.getByRole("button", { name: /more/i }));
    expect(row(100)).toBeTruthy();
    expect(screen.queryByText("Couldn't refresh file")).toBeNull();

    await act(async () => later.reject(unavailable()));
    expect(await screen.findByText("Couldn't open file")).toBeTruthy();
    expect(row(100)).toBeNull();
    expect(screen.queryByText("Couldn't refresh file")).toBeNull();
    expect(readFile).toHaveBeenLastCalledWith({
      cwd: undefined,
      path: "src/big.go",
      startLine: 100,
      endLine: 700,
    });
  });
});

describe("workspace file tree keyboard model", () => {
  const readme: WorkspaceFileEntry = { name: "README.md", path: "README.md", type: "file" };

  it("moves, opens and closes folders the way a tree does", async () => {
    const onSelectFile = vi.fn();
    mount(
      <FileTree onReveal={() => {}} entries={[directory, readme]} onSelectFile={onSelectFile} />,
    );
    const src = screen.getByRole("treeitem", { name: "src" });
    expect(src.tabIndex).toBe(0);
    expect(screen.getByRole("treeitem", { name: "README.md" }).tabIndex).toBe(-1);
    src.focus();

    fireEvent.keyDown(src, { key: "ArrowRight" });
    expect(src.getAttribute("aria-expanded")).toBe("true");
    const child = await screen.findByRole("treeitem", { name: "main.go" });
    expect(child.getAttribute("aria-level")).toBe("2");

    fireEvent.keyDown(src, { key: "ArrowRight" });
    expect(document.activeElement).toBe(child);
    fireEvent.keyDown(child, { key: "ArrowLeft" });
    expect(document.activeElement).toBe(src);
    fireEvent.keyDown(src, { key: "End" });
    expect(document.activeElement?.textContent).toContain("README.md");
    fireEvent.keyDown(document.activeElement!, { key: "Enter" });
    expect(onSelectFile).toHaveBeenCalledWith("README.md");
    fireEvent.keyDown(document.activeElement!, { key: "Home" });
    expect(document.activeElement).toBe(src);
    fireEvent.keyDown(src, { key: "ArrowLeft" });
    expect(src.getAttribute("aria-expanded")).toBe("false");
  });

  it("keeps one keyboard entry when the root listing changes under its cursor", () => {
    const notes: WorkspaceFileEntry = { name: "NOTES.md", path: "NOTES.md", type: "file" };
    const view = mount(
      <FileTree onReveal={() => {}} entries={[directory, readme]} onSelectFile={() => {}} />,
    );
    const row = screen.getByRole("treeitem", { name: "README.md" });
    act(() => row.focus());

    view.rerender(
      <QueryClientProvider client={client}>
        <FileTree
          onReveal={() => {}}
          entries={[notes, directory, readme]}
          onSelectFile={() => {}}
        />
      </QueryClientProvider>,
    );
    expect(screen.getByRole("treeitem", { name: "README.md" }).tabIndex).toBe(0);
    expect(document.activeElement).toBe(row);

    view.rerender(
      <QueryClientProvider client={client}>
        <FileTree onReveal={() => {}} entries={[notes, directory]} onSelectFile={() => {}} />
      </QueryClientProvider>,
    );
    const entry = screen.getByRole("treeitem", { name: "NOTES.md" });
    expect(entry.tabIndex).toBe(0);
    expect(document.activeElement).toBe(entry);
  });

  it("moves a removed nested cursor and its focus to the surviving folder", async () => {
    const other: WorkspaceFileEntry = { name: "other.go", path: "src/other.go", type: "file" };
    mount(<FileTree onReveal={() => {}} entries={[directory, readme]} onSelectFile={() => {}} />);
    fireEvent.click(screen.getByRole("treeitem", { name: "src" }));
    const row = await screen.findByRole("treeitem", { name: "main.go" });
    act(() => row.focus());

    listFiles.mockImplementation(async (query) => (query.path ? [other] : [directory]));
    await act(() => client.refetchQueries());
    await screen.findByRole("treeitem", { name: "other.go" });
    const src = screen.getByRole("treeitem", { name: "src" });
    expect(src.tabIndex).toBe(0);
    expect(document.activeElement).toBe(src);
  });

  it("stands in for a collapsed cursor without taking another control's focus", async () => {
    mount(
      <>
        <button>Another control</button>
        <FileTree onReveal={() => {}} entries={[readme, directory]} onSelectFile={() => {}} />
      </>,
    );
    fireEvent.click(screen.getByRole("treeitem", { name: "src" }));
    const row = await screen.findByRole("treeitem", { name: "main.go" });
    act(() => row.focus());
    const control = screen.getByRole("button", { name: "Another control" });
    act(() => control.focus());

    act(() => rememberWorkspaceView({ expandedDirs: [] }));
    expect(screen.queryByRole("treeitem", { name: "main.go" })).toBeNull();
    expect(screen.getByRole("treeitem", { name: "src" }).tabIndex).toBe(0);
    expect(screen.getByRole("treeitem", { name: "README.md" }).tabIndex).toBe(-1);
    expect(document.activeElement).toBe(control);

    act(() => rememberWorkspaceView({ expandedDirs: ["src"] }));
    expect((await screen.findByRole("treeitem", { name: "main.go" })).tabIndex).toBe(0);
    expect(screen.getByRole("treeitem", { name: "src" }).tabIndex).toBe(-1);
    expect(document.activeElement).toBe(control);
  });

  it("declares each shown branch's children and keeps folders out of file selection", async () => {
    const nested: WorkspaceFileEntry = { name: "nested", path: "src/nested", type: "dir" };
    const deep: WorkspaceFileEntry = { name: "deep.go", path: "src/nested/deep.go", type: "file" };
    listFiles.mockImplementation(async (query) =>
      query.path === directory.path ? [nested, file] : query.path ? [deep] : [directory],
    );
    useContextDockStore.setState({ memory: { expandedDirs: [], lastFilePath: file.path } });
    const onSelectFile = vi.fn();
    mount(
      <FileTree onReveal={() => {}} entries={[directory, readme]} onSelectFile={onSelectFile} />,
    );
    const src = screen.getByRole("treeitem", { name: "src" });
    const readmeRow = screen.getByRole("treeitem", { name: "README.md" });
    expect(ownedGroup(src)).toBeNull();
    expect(ownedGroup(readmeRow)).toBeNull();
    expect(src.hasAttribute("aria-selected")).toBe(false);
    expect(readmeRow.getAttribute("aria-selected")).toBe("false");

    fireEvent.click(src);
    const branch = await screen.findByRole("treeitem", { name: "nested" });
    const child = screen.getByRole("treeitem", { name: "main.go" });
    expect(ownedGroup(src)!.contains(branch)).toBe(true);
    expect(ownedGroup(src)!.contains(child)).toBe(true);
    expect(ownedGroup(branch)).toBeNull();
    expect(ownedGroup(child)).toBeNull();
    expect(src.hasAttribute("aria-selected")).toBe(false);
    expect(branch.hasAttribute("aria-selected")).toBe(false);
    expect(child.getAttribute("aria-selected")).toBe("true");
    expect(child.getAttribute("aria-current")).toBe("true");

    for (let opening = 1; opening <= 2; opening += 1) {
      fireEvent.click(branch);
      const leaf = await screen.findByRole("treeitem", { name: "deep.go" });
      const group = ownedGroup(branch)!;
      expect(group.contains(leaf)).toBe(true);
      expect(ownedGroup(src)!.contains(group)).toBe(true);
      expect(leaf.getAttribute("aria-level")).toBe("3");
      expect(leaf.getAttribute("aria-selected")).toBe("false");

      fireEvent.click(branch);
      expect(screen.queryByRole("treeitem", { name: "deep.go" })).toBeNull();
      expect(ownedGroup(branch)).toBeNull();
      expect(ownedGroup(src)!.contains(branch)).toBe(true);
    }

    fireEvent.click(src);
    expect(screen.queryByRole("treeitem", { name: "nested" })).toBeNull();
    expect(ownedGroup(src)).toBeNull();
    expect(onSelectFile).not.toHaveBeenCalled();
    expect(useContextDockStore.getState().memory.lastFilePath).toBe(file.path);
    fireEvent.click(readmeRow);
    expect(onSelectFile).toHaveBeenCalledExactlyOnceWith("README.md");
  });

  it("keeps each tree's branch owning only its own group for the same paths", async () => {
    useContextDockStore.setState({ memory: { expandedDirs: ["src"], lastFilePath: null } });
    mount(
      <>
        <FileTree
          onReveal={() => {}}
          entries={[directory]}
          cwd="/work/one"
          onSelectFile={() => {}}
        />
        <FileTree
          onReveal={() => {}}
          entries={[directory]}
          cwd="/work/two"
          onSelectFile={() => {}}
        />
      </>,
    );
    const groups: HTMLElement[] = [];
    for (const tree of screen.getAllByRole("tree")) {
      const child = await within(tree).findByRole("treeitem", { name: "main.go" });
      const group = ownedGroup(within(tree).getByRole("treeitem", { name: "src" }))!;
      expect(tree.contains(group)).toBe(true);
      expect(group.contains(child)).toBe(true);
      groups.push(group);
    }
    expect(groups).toHaveLength(2);
    expect(groups[0]).not.toBe(groups[1]);
    expect(listFiles).toHaveBeenCalledWith({ cwd: "/work/one", path: "src" });
    expect(listFiles).toHaveBeenCalledWith({ cwd: "/work/two", path: "src" });
  });
});

describe("Go to line target validation", () => {
  afterEach(() => vi.restoreAllMocks());

  async function openLineEditor() {
    readFile.mockResolvedValue({
      content: "one\ntwo\nthree\nfour\nfive",
      startLine: 1,
      totalLines: 5,
    });
    const viewer = renderHook(useWorkspaceFileViewer);
    act(() => openWorkspaceFile("src/main.go", 2));
    mount(<FileWorkspace />);
    await screen.findByText("three");
    const trigger = screen.getByRole("button", { name: "Go to line" });
    fireEvent.click(trigger);
    const dialog = await screen.findByRole("dialog");
    const field = within(dialog).getByRole<HTMLInputElement>("textbox", { name: "Go to line" });
    await waitFor(() => expect(document.activeElement).toBe(field));
    const navigate = vi.spyOn(
      await import("@/features/workspace/public/navigation"),
      "openWorkspaceFile",
    );
    return { viewer, trigger, dialog, field, navigate };
  }

  it.each(["-4", "1.5", "1e2", "line 4", "9007199254740993", "0", ""])(
    "keeps invalid target %j editable and refuses both submission paths",
    async (value) => {
      const { viewer, dialog, field, navigate } = await openLineEditor();
      const before = viewer.result.current;
      expect(field.getAttribute("aria-invalid")).toBeNull();
      expect(field.getAttribute("aria-describedby")).toBeNull();
      expect(within(dialog).queryByText("Enter a positive whole line number.")).toBeNull();

      fireEvent.change(field, { target: { value } });
      expect(field.value).toBe(value);
      fireEvent.keyDown(field, { key: "Enter" });

      expect(viewer.result.current).toBe(before);
      expect(navigate).not.toHaveBeenCalled();
      expect(dialog.isConnected).toBe(true);
      expect(field.value).toBe(value);
      expect(field.getAttribute("aria-invalid")).toBe("true");
      const message = document.getElementById(field.getAttribute("aria-describedby")!)!;
      expect(message.textContent).toBe("Enter a positive whole line number.");
      expect(dialog.contains(message)).toBe(true);
      expect(document.activeElement).toBe(field);

      const submit = within(dialog).getByRole<HTMLButtonElement>("button", { name: "Go" });
      expect(submit.disabled).toBe(false);
      act(() => submit.focus());
      fireEvent.click(submit);

      expect(viewer.result.current).toBe(before);
      expect(navigate).not.toHaveBeenCalled();
      expect(dialog.isConnected).toBe(true);
      expect(field.value).toBe(value);
      expect(document.activeElement).toBe(field);
    },
  );

  it.each([
    ["Enter", " 004 "],
    ["Go", "4"],
  ])("corrects the target and navigates once through %s", async (method, value) => {
    const { viewer, trigger, dialog, field, navigate } = await openLineEditor();
    fireEvent.change(field, { target: { value: "1.5" } });
    fireEvent.keyDown(field, { key: "Enter" });
    expect(field.getAttribute("aria-invalid")).toBe("true");

    fireEvent.change(field, { target: { value } });

    expect(field.value).toBe(value);
    expect(field.getAttribute("aria-invalid")).toBeNull();
    expect(field.getAttribute("aria-describedby")).toBeNull();
    expect(within(dialog).queryByText("Enter a positive whole line number.")).toBeNull();
    if (method === "Enter") fireEvent.keyDown(field, { key: "Enter" });
    else fireEvent.click(within(dialog).getByRole("button", { name: "Go" }));

    expect(navigate).toHaveBeenCalledExactlyOnceWith("src/main.go", 4);
    expect(viewer.result.current).toEqual({ path: "src/main.go", line: 4 });
    await waitFor(() => expect(dialog.isConnected).toBe(false));
    await waitFor(() => expect(document.activeElement).toBe(trigger));

    fireEvent.click(trigger);
    const reopened = await screen.findByRole("dialog");
    expect(within(reopened).getByRole<HTMLInputElement>("textbox").value).toBe("");
    expect(within(reopened).queryByText("Enter a positive whole line number.")).toBeNull();
  });

  it.each([{ isComposing: true }, { keyCode: 229 }])(
    "keeps an IME Enter inside the editor (%j)",
    async (composition) => {
      const { viewer, dialog, field, navigate } = await openLineEditor();
      const before = viewer.result.current;
      fireEvent.change(field, { target: { value: "4" } });

      fireEvent.keyDown(field, { key: "Enter", ...composition });

      expect(viewer.result.current).toBe(before);
      expect(navigate).not.toHaveBeenCalled();
      expect(dialog.isConnected).toBe(true);
      expect(field.value).toBe("4");
      expect(field.getAttribute("aria-invalid")).toBeNull();
      expect(document.activeElement).toBe(field);

      fireEvent.keyDown(field, { key: "Enter" });
      expect(navigate).toHaveBeenCalledExactlyOnceWith("src/main.go", 4);
    },
  );

  it("keeps the exact dismissed draft for explicit reopening", async () => {
    const { viewer, trigger, dialog, field, navigate } = await openLineEditor();
    const before = viewer.result.current;
    fireEvent.change(field, { target: { value: "-4" } });
    fireEvent.keyDown(field, { key: "Enter" });
    fireEvent.keyDown(field, { key: "Escape" });
    await waitFor(() => expect(dialog.isConnected).toBe(false));
    await waitFor(() => expect(document.activeElement).toBe(trigger));
    expect(viewer.result.current).toBe(before);
    expect(navigate).not.toHaveBeenCalled();

    fireEvent.click(trigger);
    const reopened = await screen.findByRole("dialog");
    const draft = within(reopened).getByRole<HTMLInputElement>("textbox", { name: "Go to line" });
    expect(draft.value).toBe("-4");
    await waitFor(() => expect(document.activeElement).toBe(draft));
  });
});

describe("local file action feedback", () => {
  function prepareLocalActions() {
    isLocal = true;
    selection.current = { status: "ready", cwd: "/work/project" };
    vi.mocked(localWorkspace().open).mockResolvedValue(false);
    vi.mocked(localWorkspace().reveal).mockResolvedValue(false);
    return vi.spyOn(notifications.useNotificationStore.getState(), "push");
  }

  it("reports a refused header reveal without replacing the reader or moving focus", async () => {
    const record = prepareLocalActions();
    openWorkspaceFile(file.path);
    mount(<FileWorkspace />);
    const reader = await screen.findByRole("region", { name: file.path });
    await screen.findByText("package main");
    const reveal = screen.getByRole("button", { name: "Show in Finder" });
    act(() => reveal.focus());
    const before = useContextDockStore.getState().fileViewer;

    fireEvent.click(reveal);

    const feedback = await screen.findByRole("alert");
    expect(feedback.textContent).toContain(file.path);
    expect(feedback.textContent).toContain("Finder");
    expect(record).toHaveBeenCalledOnce();
    expect(record.mock.calls[0]![0].message).toContain(file.path);
    expect(localWorkspace().reveal).toHaveBeenCalledExactlyOnceWith("/work/project", file.path);
    expect(screen.getByRole("region", { name: file.path })).toBe(reader);
    expect(useContextDockStore.getState().fileViewer).toBe(before);
    expect(document.activeElement).toBe(reveal);
  });

  it("reports a refused tree reveal after the menu returns to its original row", async () => {
    const record = prepareLocalActions();
    listFiles.mockResolvedValue([file]);
    mount(<FileWorkspace />);
    const item = await screen.findByRole("treeitem", { name: file.name });
    act(() => item.focus());
    fireEvent.contextMenu(item);
    const menu = await screen.findByRole("menu", { name: file.path });

    fireEvent.click(within(menu).getByRole("menuitem", { name: "Show in Finder" }));

    const feedback = await screen.findByRole("alert");
    expect(feedback.textContent).toContain(file.path);
    expect(record).toHaveBeenCalledOnce();
    expect(localWorkspace().reveal).toHaveBeenCalledExactlyOnceWith("/work/project", file.path);
    await waitFor(() => expect(menu.isConnected).toBe(false));
    await waitFor(() => expect(document.activeElement).toBe(item));
    expect(screen.getByRole("treeitem", { name: file.name })).toBe(item);
    expect(readFile).not.toHaveBeenCalled();
  });

  async function unsupportedFile() {
    const path = "assets/logo.png";
    readFile.mockRejectedValue(
      new RpcError({ code: -32602, message: "binary", data: { type: "unsupported_mime" } }),
    );
    openWorkspaceFile(path);
    mount(
      <>
        <FileWorkspace />
        <button type="button">Elsewhere</button>
      </>,
    );
    return {
      path,
      open: await screen.findByRole("button", { name: "Open" }),
      reader: screen.getByRole("region", { name: path }),
    };
  }

  it("reports a refused external open while retaining the unsupported file and its action", async () => {
    const record = prepareLocalActions();
    const { path, open } = await unsupportedFile();
    const before = useContextDockStore.getState().fileViewer;
    act(() => open.focus());

    fireEvent.click(open);

    expect((await screen.findByRole("alert")).textContent).toContain(path);
    expect(record).toHaveBeenCalledOnce();
    expect(localWorkspace().open).toHaveBeenCalledExactlyOnceWith("/work/project", path);
    expect(screen.getByRole("button", { name: "Open" })).toBe(open);
    expect(screen.getByText("No image preview here yet")).toBeTruthy();
    expect(useContextDockStore.getState().fileViewer).toBe(before);
    expect(document.activeElement).toBe(open);
  });

  it.each([
    { label: "current visible action", laterFocus: false, wasVisible: true, reveals: 1 },
    { label: "later outside focus", laterFocus: true, wasVisible: true, reveals: 0 },
    { label: "deliberately out-of-view action", laterFocus: false, wasVisible: false, reveals: 0 },
  ])(
    "preserves layout intent for $label when action feedback appears",
    async ({ laterFocus, wasVisible, reveals }) => {
      prepareLocalActions();
      const pending = Promise.withResolvers<boolean>();
      vi.mocked(localWorkspace().open).mockReturnValue(pending.promise);
      const { open, reader } = await unsupportedFile();
      vi.spyOn(reader, "getBoundingClientRect").mockImplementation(
        () => new DOMRect(0, 0, 200, screen.queryByRole("alert") || !wasVisible ? 100 : 400),
      );
      vi.spyOn(open, "getBoundingClientRect").mockImplementation(
        () => new DOMRect(20, 300, 50, 30),
      );
      const reveal = vi.spyOn(open, "scrollIntoView").mockImplementation(() => {});
      act(() => open.focus());
      fireEvent.click(open);
      const outside = screen.getByRole("button", { name: "Elsewhere" });
      if (laterFocus) act(() => outside.focus());

      await act(async () => pending.resolve(false));

      expect(await screen.findByRole("alert")).toBeTruthy();
      expect(reveal.mock.calls).toEqual(
        Array.from({ length: reveals }, () => [{ block: "nearest", inline: "nearest" }]),
      );
      expect(document.activeElement).toBe(laterFocus ? outside : open);
    },
  );

  it("dismisses local feedback into its surviving reader", async () => {
    prepareLocalActions();
    const { open, reader } = await unsupportedFile();
    fireEvent.click(open);
    const feedback = await screen.findByRole("alert");
    const close = within(feedback).getByRole("button", { name: "Close" });
    act(() => close.focus());

    fireEvent.click(close);

    expect(screen.queryByRole("alert")).toBeNull();
    expect(document.activeElement).toBe(reader);
    expect(screen.getByRole("button", { name: "Open" })).toBe(open);
  });

  it("keeps late feedback tied to its original path without reclaiming later focus", async () => {
    prepareLocalActions();
    const pending = Promise.withResolvers<boolean>();
    vi.mocked(localWorkspace().reveal).mockReturnValue(pending.promise);
    openWorkspaceFile(file.path);
    mount(
      <>
        <FileWorkspace />
        <button type="button">Elsewhere</button>
      </>,
    );
    await screen.findByText("package main");
    fireEvent.click(screen.getByRole("button", { name: "Show in Finder" }));
    act(() => openWorkspaceFile("src/other.go"));
    await screen.findByRole("region", { name: "src/other.go" });
    const outside = screen.getByRole("button", { name: "Elsewhere" });
    act(() => outside.focus());

    await act(async () => pending.reject(new Error("Original file is unavailable")));

    const feedback = await screen.findByRole("alert");
    expect(feedback.textContent).toContain(file.path);
    expect(feedback.textContent).not.toContain("src/other.go");
    expect(useContextDockStore.getState().fileViewer?.path).toBe("src/other.go");
    expect(document.activeElement).toBe(outside);
  });
});

describe("retained file workspace popups", () => {
  it("retires Go to line when hidden and keeps its draft for an explicit reopen", async () => {
    readFile.mockResolvedValue({
      content: "one\ntwo\nthree\nfour\nfive",
      startLine: 1,
      totalLines: 5,
    });
    const viewer = renderHook(useWorkspaceFileViewer);
    act(() => openWorkspaceFile("src/main.go"));
    const host = mountRetained();
    await screen.findByText("four");
    const opened = viewer.result.current;
    const goToLine = screen.getByRole("button", { name: "Go to line" });
    fireEvent.click(goToLine);
    const dialog = await screen.findByRole("dialog");
    await vi.waitFor(() => expect(dialog.contains(document.activeElement)).toBe(true));
    fireEvent.change(within(dialog).getByRole("textbox", { name: "Go to line" }), {
      target: { value: "4" },
    });

    host.hide();
    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
    host.reveal();

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByText("four")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Back to files" })).toBeTruthy();
    expect(viewer.result.current).toBe(opened);

    fireEvent.click(goToLine);
    const reopened = await screen.findByRole("dialog");
    const draft = within(reopened).getByRole<HTMLInputElement>("textbox", { name: "Go to line" });
    expect(draft.value).toBe("4");
    act(() => draft.focus());
    fireEvent.keyDown(draft, { key: "Enter" });

    expect(viewer.result.current).toEqual({ path: "src/main.go", line: 4 });
    await vi.waitFor(() => expect(reopened.isConnected).toBe(false));
  });

  it("retires a file row's context menu when hidden and keeps the tree state", async () => {
    useContextDockStore.setState({
      memory: { expandedDirs: ["src"], lastFilePath: "src/main.go" },
    });
    const host = mountRetained();
    const row = await screen.findByRole("treeitem", { name: "main.go" });
    act(() => row.focus());
    fireEvent.contextMenu(row);
    const menu = await screen.findByRole("menu");
    await vi.waitFor(() => expect(menu.contains(document.activeElement)).toBe(true));

    host.hide();
    await vi.waitFor(() => expect(menu.isConnected).toBe(false));
    host.reveal();

    expect(screen.queryByRole("menu")).toBeNull();
    expect(screen.getByRole("treeitem", { name: "main.go" })).toBe(row);
    expect(row.tabIndex).toBe(0);
    expect(row.getAttribute("aria-current")).toBe("true");
    expect(screen.getByRole("treeitem", { name: "src" }).getAttribute("aria-expanded")).toBe(
      "true",
    );
  });
});
