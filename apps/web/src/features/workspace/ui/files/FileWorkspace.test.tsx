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
    mount(<FileTree entries={[directory]} onSelectFile={() => {}} />);
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
