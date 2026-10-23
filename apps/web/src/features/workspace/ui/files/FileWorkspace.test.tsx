import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { definePlugin } from "@/extensions/definePlugin";
import { WORKSPACE_FILE_RENDERER } from "@/features/workspace/public/views";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";
import { configureLocalWorkspace } from "@/features/workspace/application/ports/localWorkspace";
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

function mount(children: React.ReactNode) {
  return render(<QueryClientProvider client={client}>{children}</QueryClientProvider>);
}

describe("workspace files panel", () => {
  it("shows the rename relationship while preserving unknown binary statistics", () => {
    changes.set(file.path, {
      path: file.path,
      previousPath: "old/main.go",
      change: "renamed",
      binary: true,
    });
    mount(<FileTree entries={[file]} onSelectFile={() => {}} />);
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

  describe("prepend scroll continuity", () => {
    const ROW = 20;

    // Happy DOM has no layout: the host's height is derived from rendered rows and
    // scrollTop is a plain stored value, so these tests prove which height delta is
    // applied, not how WebKit paints it.
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
    mount(<FileTree entries={[directory]} onSelectFile={() => {}} />);
    fireEvent.click(screen.getByRole("treeitem", { name: "src" }));

    fireEvent.click(await screen.findByRole("button", { name: "Retry" }));
    expect(await screen.findByRole("treeitem", { name: "main.go" })).toBeTruthy();
    expect(listFiles).toHaveBeenCalledTimes(2);
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

describe("workspace file tree keyboard model", () => {
  const readme: WorkspaceFileEntry = { name: "README.md", path: "README.md", type: "file" };

  it("moves, opens and closes folders the way a tree does", async () => {
    const onSelectFile = vi.fn();
    mount(<FileTree entries={[directory, readme]} onSelectFile={onSelectFile} />);
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
    const view = mount(<FileTree entries={[directory, readme]} onSelectFile={() => {}} />);
    const row = screen.getByRole("treeitem", { name: "README.md" });
    act(() => row.focus());

    view.rerender(
      <QueryClientProvider client={client}>
        <FileTree entries={[notes, directory, readme]} onSelectFile={() => {}} />
      </QueryClientProvider>,
    );
    expect(screen.getByRole("treeitem", { name: "README.md" }).tabIndex).toBe(0);
    expect(document.activeElement).toBe(row);

    view.rerender(
      <QueryClientProvider client={client}>
        <FileTree entries={[notes, directory]} onSelectFile={() => {}} />
      </QueryClientProvider>,
    );
    const entry = screen.getByRole("treeitem", { name: "NOTES.md" });
    expect(entry.tabIndex).toBe(0);
    expect(document.activeElement).toBe(entry);
  });

  it("moves a removed nested cursor and its focus to the surviving folder", async () => {
    const other: WorkspaceFileEntry = { name: "other.go", path: "src/other.go", type: "file" };
    mount(<FileTree entries={[directory, readme]} onSelectFile={() => {}} />);
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
        <FileTree entries={[readme, directory]} onSelectFile={() => {}} />
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
});

describe("retained file workspace popups", () => {
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
