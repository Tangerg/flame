import { stubInstance } from "@/test/stubInstance";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { onlineManager, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { definePlugin } from "@/extensions/definePlugin";
import { WORKSPACE_FILE_RENDERER } from "@/features/workspace/public/views";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";

import { closeWorkspaceFile, openWorkspaceFile } from "@/features/workspace/application/navigation";
import { useWorkspaceFileViewer } from "@/features/workspace/public/navigation";

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
import { CodeBlock } from "@flame/ui";
import { ContentVisibility } from "@/ui/index";
const selection = vi.hoisted(() => ({
  current: { status: "ready" } as
    { status: "ready"; cwd?: string } | { status: "resolving"; sessionId: string },
}));

const changes = vi.hoisted(() => new Map<string, WorkspaceFileChange>());

vi.mock("@/features/workspace/application/workingTreeChanges", () => ({
  useWorkingTreeFiles: () => changes,
}));

// Browser tests exercise the real virtualized layout; this suite owns file material and actions.
vi.mock("react-aria-components", async (importOriginal) => ({
  ...(await importOriginal<typeof import("react-aria-components")>()),
  Virtualizer: ({ children }: { children: React.ReactNode }) => children,
}));

vi.mock("@/features/agent/public/session", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agent/public/session")>()),
  useActiveSessionWorkspace: () => selection.current,
}));

function pressTreeKey(target: Element, key: string) {
  if (target instanceof HTMLElement) act(() => target.focus());
  const code = key === " " ? "Space" : key;
  fireEvent.keyDown(target, { key, code });
  fireEvent.keyUp(target, { key, code });
}

let client: QueryClient;

const listFiles = vi.fn<(query: WorkspaceListFilesQuery) => Promise<WorkspaceFileEntry[]>>();

const readFile = vi.fn<(query: WorkspaceReadFileQuery) => Promise<WorkspaceFileContent>>();

const directory: WorkspaceFileEntry = { name: "src", path: "src", type: "dir" };

const file: WorkspaceFileEntry = { name: "main.go", path: "src/main.go", type: "file" };

let uninstallLocalWorkspace: () => void;

let isLocal = false;

let renderedExtra: React.ReactNode = null;

beforeEach(async () => {
  changes.clear();
  isLocal = false;
  renderedExtra = null;
  uninstallLocalWorkspace = stubInstance(testClient.workspace.access.local, {
    available: () => isLocal,
    open: vi.fn(),
    reveal: vi.fn(),
  });

  selection.current = { status: "ready" };
  testClient.workspace.dock.store.setState({
    fileViewer: null,
    memory: {
      expandedDirs: [],
      lastFilePath: null,
    },
  });
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  listFiles.mockReset().mockImplementation(async (query) => (query.path ? [file] : [directory]));
  readFile
    .mockReset()
    .mockResolvedValue({ content: "package main", startLine: 1, endLine: 1, totalLines: 1 });
  await loadPluginsForTest(
    definePlugin({
      name: "test.workspace-files",
      setup(ctx) {
        ctx.contributions.add(DATA_PROVIDER, {
          key: WORKSPACE_LIST_FILES_KEY,
          fetcher: async (params) => ({
            data: await listFiles(params as WorkspaceListFilesQuery),
            complete: true,
            coverage: { scannedEntries: 1, excludedEntries: 0, budgetExhausted: false },
          }),
        });
        ctx.contributions.add(DATA_PROVIDER, {
          key: WORKSPACE_READ_FILE_KEY,
          fetcher: (params) => readFile(params as WorkspaceReadFileQuery),
        });
        ctx.contributions.add(
          WORKSPACE_FILE_RENDERER,
          {
            id: "markdown",
            title: "Markdown",
            extension: "md",
            component: ({ content }) => (
              <article data-testid="rendered-markdown">
                {content}
                {renderedExtra}
              </article>
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

  uninstallLocalWorkspace();
  cleanup();
  client.clear();
});

function directoryKey(cwd?: string, path = "") {
  return [WORKSPACE_LIST_FILES_KEY, { cwd, path, limit: 100 }, "pages"];
}

function seedDirectory(entries: WorkspaceFileEntry[], cwd?: string) {
  client.setQueryData(directoryKey(cwd), { pages: [page(entries)], pageParams: [undefined] });
}

function pauseQueries() {
  const wasOnline = onlineManager.isOnline();
  onlineManager.setOnline(false);
  finishTestClient(() => onlineManager.setOnline(wasOnline));
}

function page(data: WorkspaceFileEntry[]) {
  return {
    data,
    complete: true,
    coverage: { scannedEntries: data.length, excludedEntries: 0, budgetExhausted: false },
  };
}

function lineWindow(start: number, end: number, totalLines: number): WorkspaceFileContent {
  const lines = Array.from({ length: end - start + 1 }, (_, i) => `line ${start + i}`);
  return { content: lines.join("\n"), startLine: start, endLine: end, totalLines };
}

function row(line: number) {
  return document.querySelector(`[data-quote-line="${line}"]`);
}

function mount(children: React.ReactNode) {
  return testClient.render(<QueryClientProvider client={client}>{children}</QueryClientProvider>);
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
  const view = testClient.render(workspace(true));
  return {
    hide: () => view.rerender(workspace(false)),
    reveal: () => view.rerender(workspace(true)),
  };
}

describe("first file reads", () => {
  it("waits for the first root listing while its query is paused", async () => {
    pauseQueries();
    mount(<FileWorkspace />);
    await waitFor(() => {
      expect(client.getQueryState(directoryKey())).toMatchObject({
        status: "pending",
        fetchStatus: "paused",
        data: undefined,
      });
    });
    expect(listFiles).not.toHaveBeenCalled();
    expect(screen.getByRole("status").textContent).toBe("Loading");
    expect(screen.queryByText("No files")).toBeNull();

    await act(async () => onlineManager.setOnline(true));
    expect(await screen.findByRole("row", { name: "src" })).toBeTruthy();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("waits for an expanded directory's first paused read", async () => {
    mount(<FileWorkspace />);
    const parent = await screen.findByRole("row", { name: "src" });
    pauseQueries();
    act(() => parent.focus());
    pressTreeKey(parent, "ArrowRight");
    await waitFor(() => {
      expect(client.getQueryState(directoryKey(undefined, "src"))).toMatchObject({
        status: "pending",
        fetchStatus: "paused",
        data: undefined,
      });
    });
    expect(listFiles).toHaveBeenCalledOnce();
    expect(screen.getByRole("status").textContent).toBe("Loading");
    expect(screen.queryByText("No files")).toBeNull();
    expect(document.activeElement).toBe(parent);

    await act(async () => onlineManager.setOnline(true));
    expect(await screen.findByRole("row", { name: "main.go" })).toBeTruthy();
    expect(document.activeElement).toBe(parent);
  });

  it("waits for the selected file's first paused read", async () => {
    pauseQueries();
    openWorkspaceFile(testClient.workspace.access, "src/main.go");
    mount(<FileWorkspace />);
    await waitFor(() => {
      expect(
        client.getQueryState([WORKSPACE_READ_FILE_KEY, { cwd: undefined, path: "src/main.go" }]),
      ).toMatchObject({ status: "pending", fetchStatus: "paused", data: undefined });
    });
    expect(readFile).not.toHaveBeenCalled();
    expect(screen.getByText("src/main.go")).toBeTruthy();
    expect(screen.getByRole("status").textContent).toBe("Loading");
    const back = screen.getByRole("button", { name: "Back to files" });
    expect(document.activeElement).toBe(back);

    await act(async () => onlineManager.setOnline(true));
    expect(await screen.findByText("package main")).toBeTruthy();
    expect(document.activeElement).toBe(back);
  });

  it.each([
    {
      outcome: "readable content",
      path: "src/main.go",
      settle: (read: PromiseWithResolvers<WorkspaceFileContent>) =>
        read.resolve({ content: "package main", startLine: 1, endLine: 1, totalLines: 1 }),
      settled: "package main",
      offersLines: true,
    },
    {
      outcome: "an unsupported file",
      path: "assets/logo.png",
      settle: (read: PromiseWithResolvers<WorkspaceFileContent>) =>
        read.reject(
          new RpcError({ code: -32602, message: "binary", data: { type: "unsupported_mime" } }),
        ),
      settled: "No image preview here yet",
      offersLines: false,
    },
    {
      outcome: "a read failure",
      path: "src/main.go",
      settle: (read: PromiseWithResolvers<WorkspaceFileContent>) =>
        read.reject(new Error("file unavailable")),
      settled: "Couldn't open file",
      offersLines: false,
    },
  ])(
    "offers line navigation only after the first read returns $outcome",
    async ({ path, settle, settled, offersLines }) => {
      const read = Promise.withResolvers<WorkspaceFileContent>();
      readFile.mockReturnValueOnce(read.promise);
      openWorkspaceFile(testClient.workspace.access, path);
      mount(
        <>
          <button>Another control</button>
          <FileWorkspace />
        </>,
      );
      await waitFor(() => expect(readFile).toHaveBeenCalledOnce());
      expect(screen.getByRole("status").textContent).toBe("Loading");
      expect(screen.queryByRole("button", { name: "Go to line" })).toBeNull();
      const back = screen.getByRole("button", { name: "Back to files" });
      const copyPath = screen.getByRole("button", { name: "Copy path" });
      expect(document.activeElement).toBe(back);
      const other = screen.getByRole("button", { name: "Another control" });
      act(() => other.focus());

      await act(async () => settle(read));
      expect(await screen.findByText(settled)).toBeTruthy();
      await waitFor(() => expect(client.isFetching()).toBe(0));
      expect(Boolean(screen.queryByRole("button", { name: "Go to line" }))).toBe(offersLines);
      expect(screen.getByRole("button", { name: "Back to files" })).toBe(back);
      expect(screen.getByRole("button", { name: "Copy path" })).toBe(copyPath);
      expect(document.activeElement).toBe(other);
    },
  );

  it.each([
    { name: "empty", accepted: [] as WorkspaceFileEntry[], visibleText: "No files" },
    { name: "nonempty", accepted: [directory], visibleText: "src" },
  ])(
    "keeps an accepted $name listing during a paused reread",
    async ({ accepted, visibleText }) => {
      client.setQueryData(directoryKey(), { pages: [page(accepted)], pageParams: [undefined] });
      pauseQueries();
      mount(<FileWorkspace />);
      act(() => void client.refetchQueries({ queryKey: [WORKSPACE_LIST_FILES_KEY] }));
      await waitFor(() => {
        expect(client.getQueryState(directoryKey())).toMatchObject({
          status: "success",
          fetchStatus: "paused",
          data: { pages: [page(accepted)], pageParams: [undefined] },
        });
      });
      expect(listFiles).not.toHaveBeenCalled();
      expect(screen.queryByRole("status")).toBeNull();
      expect(screen.getByText(visibleText)).toBeTruthy();
    },
  );
});

describe("workspace files panel", () => {
  it("shows the rename relationship while preserving unknown binary statistics", () => {
    changes.set(file.path, {
      path: file.path,
      previousPath: "old/main.go",
      change: "renamed",
      binary: true,
    });
    seedDirectory([file], undefined);
    mount(<FileTree onReveal={() => {}} onSelectFile={() => {}} />);
    const row = screen.getByRole("row", { name: "main.go" });
    expect(row.textContent).toContain("← old/main.go");
    expect(row.textContent).toContain("R");
    expect(row.querySelector("[title]")?.getAttribute("title")).toBe("old/main.go → src/main.go");
    expect(row.getAttribute("aria-label")).toBe("main.go");
  });
  it("reads a file in the Runtime default workspace", async () => {
    openWorkspaceFile(testClient.workspace.access, "src/main.go");
    mount(<FileWorkspace />);

    await screen.findByText("package main");
    expect(readFile).toHaveBeenCalledWith({ cwd: undefined, path: "src/main.go" });
  });

  it("keeps the file shown while a preceding page loads", async () => {
    readFile.mockResolvedValue({
      content: "middle chunk",
      startLine: 1300,
      endLine: 1300,
      totalLines: 3000,
    });
    openWorkspaceFile(testClient.workspace.access, "src/big.go");
    mount(<FileWorkspace />);
    await screen.findByText("middle chunk");

    const pending = Promise.withResolvers<WorkspaceFileContent>();
    readFile.mockReturnValueOnce(pending.promise);
    fireEvent.click(screen.getByRole("button", { name: /earlier/i }));

    // placeholderData holds the content instead of flashing a skeleton.
    expect(screen.getByText("middle chunk")).toBeTruthy();

    pending.resolve({
      content: "earlier\nprevious page",
      startLine: 1298,
      endLine: 1299,
      totalLines: 3000,
    });
    await screen.findByText("earlier");
  });

  it("keeps old content shown without letting it settle a pending line navigation", async () => {
    readFile.mockResolvedValueOnce(lineWindow(250, 650, 1000));
    openWorkspaceFile(testClient.workspace.access, "src/big.go", 450);
    mount(<FileWorkspace />);
    await vi.waitFor(() => expect(row(450)).toBeTruthy());
    expect(
      document.querySelector("[data-presented-target]")?.getAttribute("data-presented-target"),
    ).toBe("450");

    const pending = Promise.withResolvers<WorkspaceFileContent>();
    readFile.mockReturnValueOnce(pending.promise);
    act(() => openWorkspaceFile(testClient.workspace.access, "src/big.go", 300));
    expect(row(450)).toBeTruthy();
    expect(document.querySelector("[data-presented-target]")).toBeNull();

    await act(async () => pending.resolve(lineWindow(100, 500, 1000)));
    await vi.waitFor(() => expect(row(100)).toBeTruthy());
    expect(
      document.querySelector("[data-presented-target]")?.getAttribute("data-presented-target"),
    ).toBe("300");
  });

  describe("range-control focus continuity", () => {
    it.each([
      { edge: "first", control: /earlier/i, start: 1, end: 99, requestedEnd: 99, fromEnd: true },
      { edge: "last", control: /more/i, start: 501, end: 600, requestedEnd: 700, fromEnd: false },
    ])(
      "keeps the reader focused when the $edge line retires its range control",
      async ({ control, start, end, requestedEnd, fromEnd }) => {
        readFile.mockResolvedValueOnce(lineWindow(100, 500, 600));
        openWorkspaceFile(testClient.workspace.access, "src/big.go", 300);
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
            ...(fromEnd ? { fromEnd: true } : {}),
          }),
        );

        await act(async () => pending.resolve(lineWindow(start, end, 600)));
        await waitFor(() => expect(screen.queryByRole("button", { name: control })).toBeNull());
        expect(document.activeElement).toBe(host);
        expect(focusWhilePending).toBe(host);
        expect(focusReader).toHaveBeenCalledExactlyOnceWith({ preventScroll: true });
        expect(host.scrollTop).toBe(0);
      },
    );

    it("hands focus to the reader even when more lines remain beyond either range control", async () => {
      readFile.mockResolvedValueOnce(lineWindow(400, 402, 1000));
      openWorkspaceFile(testClient.workspace.access, "src/big.go");
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
      openWorkspaceFile(testClient.workspace.access, "src/big.go");
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
      openWorkspaceFile(testClient.workspace.access, "src/big.go");
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

      await act(async () => pending.resolve(lineWindow(501, 600, 600)));
      await waitFor(() => expect(row(600)).toBeTruthy());
      expect(document.activeElement).toBe(draft);
      expect(draft.value).toBe("Retained message draft, continued");
    });

    it("preserves navigation focus when an old range finishes loading", async () => {
      readFile.mockResolvedValueOnce(lineWindow(100, 500, 600));
      openWorkspaceFile(testClient.workspace.access, "src/big.go");
      mount(<FileWorkspace />);
      await waitFor(() => expect(row(500)).toBeTruthy());
      const host = screen.getByRole("region", { name: "src/big.go" });
      const button = screen.getByRole("button", { name: /more/i });
      act(() => button.focus());
      const pending = Promise.withResolvers<WorkspaceFileContent>();
      readFile.mockReturnValueOnce(pending.promise);
      fireEvent.click(button);
      expect(document.activeElement).toBe(host);

      readFile.mockResolvedValueOnce({
        content: "package main",
        startLine: 1,
        endLine: 1,
        totalLines: 1,
      });
      act(() => openWorkspaceFile(testClient.workspace.access, "src/main.go"));
      await screen.findByText("package main");
      const back = screen.getByRole("button", { name: "Back to files" });
      expect(document.activeElement).toBe(back);

      await act(async () => pending.resolve(lineWindow(501, 600, 600)));
      await waitFor(() => expect(client.isFetching()).toBe(0));
      expect(document.activeElement).toBe(back);
      expect(screen.getByRole("region", { name: "src/main.go" }).textContent).toContain(
        "package main",
      );
      expect(row(600)).toBeNull();
    });
  });

  describe("page scroll ownership", () => {
    async function openMiddle() {
      vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(() => {});
      readFile.mockResolvedValueOnce(lineWindow(1300, 1302, 3000));
      openWorkspaceFile(testClient.workspace.access, "src/big.go");
      mount(<FileWorkspace />);
      await vi.waitFor(() => expect(row(1300)).toBeTruthy());
      const host = controlGeometry(screen.getByRole("region", { name: "src/big.go" }));
      host.scrollTop = 10;
      const earlier = Promise.withResolvers<WorkspaceFileContent>();
      readFile.mockReturnValueOnce(earlier.promise);
      fireEvent.click(screen.getByRole("button", { name: /earlier/i }));
      return { host, earlier };
    }

    it("places the preceding page at its start when its material arrives", async () => {
      const { host, earlier } = await openMiddle();
      await act(async () => earlier.resolve(lineWindow(1100, 1299, 3000)));
      await vi.waitFor(() => expect(row(1100)).toBeTruthy());
      expect(host.scrollTop).toBe(0);
    });

    it("retires a superseded previous page when a line navigation replaces the window", async () => {
      const { host, earlier } = await openMiddle();
      const navigation = Promise.withResolvers<WorkspaceFileContent>();
      readFile.mockReturnValueOnce(navigation.promise);
      act(() => openWorkspaceFile(testClient.workspace.access, "src/big.go", 2000));
      await act(async () => earlier.resolve(lineWindow(1100, 1299, 3000)));
      await act(async () => navigation.resolve(lineWindow(1800, 2200, 3000)));
      await vi.waitFor(() => expect(row(2200)).toBeTruthy());
      expect(host.scrollTop).toBe(10);
    });

    it("retires a superseded previous page when later lines replace the window", async () => {
      const { host, earlier } = await openMiddle();
      const later = Promise.withResolvers<WorkspaceFileContent>();
      readFile.mockReturnValueOnce(later.promise);
      fireEvent.click(screen.getByRole("button", { name: /more/i }));
      await act(async () => earlier.resolve(lineWindow(1100, 1299, 3000)));
      await act(async () => later.resolve(lineWindow(1303, 1502, 3000)));
      await vi.waitFor(() => expect(row(1502)).toBeTruthy());
      expect(host.scrollTop).toBe(0);
    });
  });

  describe("default file arrival", () => {
    it("opens a cached file at its top instead of the previous file's position", async () => {
      readFile.mockImplementation(async ({ path }) =>
        path === "src/main.ts" ? lineWindow(1, 39, 39) : lineWindow(1, 155, 155),
      );
      openWorkspaceFile(testClient.workspace.access, "src/main.ts");
      const retained = mountRetained();
      await vi.waitFor(() => expect(row(39)).toBeTruthy());
      const host = controlGeometry(screen.getByRole("region", { name: "src/main.ts" }));

      act(() => openWorkspaceFile(testClient.workspace.access, "package.json"));
      await vi.waitFor(() => expect(row(155)).toBeTruthy());
      expect(screen.getByRole("region", { name: "package.json" })).toBe(host);
      host.scrollTop = 2214;
      await act(() => client.refetchQueries({ type: "active" }));
      retained.hide();
      retained.reveal();
      expect(host.scrollTop).toBe(2214);

      const reads = readFile.mock.calls.length;
      act(() => openWorkspaceFile(testClient.workspace.access, "src/main.ts"));
      expect(row(39)).toBeTruthy();
      expect(readFile).toHaveBeenCalledTimes(reads);
      expect(host.scrollTop).toBe(0);
    });

    it("reopens a file at its top after a targeted range", async () => {
      vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(() => {});
      readFile.mockImplementation(async ({ startLine, endLine }) =>
        lineWindow(startLine ?? 1, endLine ?? 400, 1000),
      );
      openWorkspaceFile(testClient.workspace.access, "src/main.ts", 450);
      mount(<FileWorkspace />);
      await vi.waitFor(() => expect(row(450)).toBeTruthy());
      const host = controlGeometry(screen.getByRole("region", { name: "src/main.ts" }));
      host.scrollTop = 3482;

      fireEvent.click(screen.getByRole("button", { name: /more/i }));
      await vi.waitFor(() => expect(row(850)).toBeTruthy());
      expect(host.scrollTop).toBe(0);

      act(() => openWorkspaceFile(testClient.workspace.access, "src/main.ts"));
      await vi.waitFor(() => expect(row(1)).toBeTruthy());
      expect(row(450)).toBeNull();
      expect(host.scrollTop).toBe(0);
    });

    it("reopens a rendered file at its top in its preferred mode", async () => {
      readFile.mockResolvedValue({ content: "# Guide", startLine: 1, endLine: 1, totalLines: 1 });
      openWorkspaceFile(testClient.workspace.access, "docs/reading-guide.md");
      mount(<FileWorkspace />);
      await screen.findByTestId("rendered-markdown");
      const host = controlGeometry(screen.getByRole("region", { name: "docs/reading-guide.md" }));
      host.scrollTop = 753;

      act(() => openWorkspaceFile(testClient.workspace.access, "docs/reading-guide.md"));
      await screen.findByTestId("rendered-markdown");
      expect(host.scrollTop).toBe(0);

      host.scrollTop = 753;
      fireEvent.click(screen.getByRole("radio", { name: "Source" }));
      expect(screen.queryByTestId("rendered-markdown")).toBeNull();
      expect(host.scrollTop).toBe(753);

      act(() => openWorkspaceFile(testClient.workspace.access, "docs/reading-guide.md"));
      await screen.findByTestId("rendered-markdown");
      expect(host.scrollTop).toBe(0);
    });
  });

  it("renders a whole file through the renderer registered for its extension", async () => {
    readFile.mockResolvedValue({ content: "# Title", startLine: 1, endLine: 1, totalLines: 1 });
    openWorkspaceFile(testClient.workspace.access, "docs/README.md");
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
    const source = screen.getByRole("radio", { name: "Source" });
    act(() => source.focus());
    fireEvent.click(source);
    expect(await screen.findByText("# Title")).toBeTruthy();
    expect(document.activeElement).toBe(source);
    const preview = screen.getByRole("radio", { name: "Preview" });
    act(() => preview.focus());
    fireEvent.click(preview);
    await screen.findByTestId("rendered-markdown");
    expect(document.activeElement).toBe(preview);
  });

  it("keeps truncated files in their source presentation", async () => {
    readFile.mockResolvedValue({
      content: "# Truncated",
      startLine: 1,
      endLine: 1,
      totalLines: 1,
      truncated: true,
    });
    openWorkspaceFile(testClient.workspace.access, "docs/README.md");
    mount(<FileWorkspace />);
    await screen.findByText("# Truncated");
    expect(screen.queryByTestId("rendered-markdown")).toBeNull();
  });

  it("follows a line reference in source even when the whole file is available", async () => {
    readFile.mockResolvedValue({
      content: "# First\nSecond",
      startLine: 1,
      endLine: 2,
      totalLines: 2,
    });
    openWorkspaceFile(testClient.workspace.access, "docs/README.md", 2);
    mount(<FileWorkspace />);
    await screen.findByText("Second");
    expect(screen.queryByTestId("rendered-markdown")).toBeNull();
  });

  it("retires the file renderer without losing the captured source", async () => {
    readFile.mockResolvedValue({ content: "# Captured", startLine: 1, endLine: 1, totalLines: 1 });
    openWorkspaceFile(testClient.workspace.access, "docs/README.md");
    mount(<FileWorkspace />);
    await screen.findByTestId("rendered-markdown");
    await resetKernelForTest();
    expect(screen.queryByTestId("rendered-markdown")).toBeNull();
    expect(screen.getByText("# Captured")).toBeTruthy();
  });

  it("waits for an unresolved Session workspace before following a line reference", async () => {
    selection.current = { status: "resolving", sessionId: "session-loading" };
    openWorkspaceFile(testClient.workspace.access, "src/main.go", 450);
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
          startLine: 450,
          endLine: 650,
        },
      ],
    ]);
  });

  it("browses and reads files in one panel, restoring its tree cursor and focus on return", async () => {
    mount(<FileWorkspace />);
    pressTreeKey(await screen.findByRole("row", { name: "src" }), "ArrowRight");
    const fileRow = await screen.findByRole("row", { name: "main.go" });
    act(() => fileRow.focus());
    pressTreeKey(fileRow, "Enter");
    await screen.findByText("package main");
    expect(testClient.navigation.get().dock).toBe("file");
    expect(testClient.workspace.dock.store.getState().dockViewIds).toEqual(["file"]);

    const back = screen.getByRole("button", { name: "Back to files" });
    expect(document.activeElement).toBe(back);
    fireEvent.click(back);
    const returned = await screen.findByRole("row", { name: "main.go" });
    expect(returned.tabIndex).toBe(0);
    expect(document.activeElement).toBe(returned);
    expect(screen.queryByText("package main")).toBeNull();
    expect(testClient.workspace.dock.store.getState().dockViewIds).toEqual(["file"]);
    pressTreeKey(returned, "ArrowUp");
    expect(document.activeElement).toBe(screen.getByRole("row", { name: "src" }));
  });

  it("preserves another control's focus when file selection changes", async () => {
    mount(
      <>
        <button>Another control</button>
        <FileWorkspace />
      </>,
    );
    pressTreeKey(await screen.findByRole("row", { name: "src" }), "ArrowRight");
    const row = await screen.findByRole("row", { name: "main.go" });
    act(() => row.focus());
    pressTreeKey(row, "Enter");
    await screen.findByText("package main");
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Back to files" }));
    const other = screen.getByRole("button", { name: "Another control" });
    act(() => other.focus());
    act(() => closeWorkspaceFile(testClient.workspace.access));
    await screen.findByRole("row", { name: "main.go" });
    expect(document.activeElement).toBe(other);
    act(() => openWorkspaceFile(testClient.workspace.access, "src/main.go"));
    await screen.findByText("package main");
    expect(document.activeElement).toBe(other);
  });

  it("shows a failed directory read and retries it without closing the directory", async () => {
    listFiles.mockRejectedValueOnce(new Error("directory unavailable"));
    seedDirectory([directory], undefined);
    mount(<FileTree onReveal={() => {}} onSelectFile={() => {}} />);
    pressTreeKey(screen.getByRole("row", { name: "src" }), "ArrowRight");

    fireEvent.click(await screen.findByRole("button", { name: "Retry" }));
    expect(await screen.findByRole("row", { name: "main.go" })).toBeTruthy();
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
      const folder = await screen.findByRole("row", { name: "src" });
      const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message" });
      act(() => folder.focus());
      listFiles.mockRejectedValueOnce(new Error("directory unavailable"));
      pressTreeKey(folder, "ArrowRight");

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
        expect(
          screen.getByRole("treegrid", { name: "Files" }).contains(document.activeElement),
        ).toBe(true);
        await waitFor(() => expect(screen.queryByRole("button", { name: "Retry" })).toBeNull());
        expect(
          screen.getByRole("treegrid", { name: "Files" }).contains(document.activeElement),
        ).toBe(true);
        expect(folder.getAttribute("aria-expanded")).toBe("true");
        if (typing) {
          act(() => draft.focus());
          fireEvent.change(draft, { target: { value: "Retained message draft, continued" } });
        }

        await act(async () => settle(listing));
        await waitFor(() => expect(client.isFetching()).toBe(0));
        expect(
          typing
            ? document.activeElement === draft
            : screen.getByRole("treegrid", { name: "Files" }).contains(document.activeElement),
        ).toBe(true);
        expect(folder.tabIndex).toBe(0);
        expect(folder.getAttribute("aria-expanded")).toBe("true");
      }
      expect(listFiles.mock.calls).toEqual([
        [{ cwd: undefined, path: "", limit: 100, cursor: undefined }],
        ...Array.from({ length: 3 }, () => [
          { cwd: undefined, path: "src", limit: 100, cursor: undefined },
        ]),
      ]);
      expect(draft.value).toBe(
        typeWhileListing ? "Retained message draft, continued" : "Retained message draft",
      );
      act(() => screen.getByRole("treegrid", { name: "Files" }).focus());
      expect(screen.getByRole("treegrid", { name: "Files" }).contains(document.activeElement)).toBe(
        true,
      );

      const child = screen.getByRole("row", { name: "main.go" });
      pressTreeKey(child, "Enter");
      await screen.findByText("package main");
      expect(readFile).toHaveBeenCalledWith({ cwd: undefined, path: "src/main.go" });

      const back = screen.getByRole("button", { name: "Back to files" });
      expect(document.activeElement).toBe(back);
      fireEvent.click(back);
      const returned = await screen.findByRole("row", { name: "main.go" });
      expect(returned.querySelector("[aria-current]")?.getAttribute("aria-current")).toBe("true");
      expect(document.activeElement).toBe(returned);
    },
  );

  it("keeps a retried file read's focus in the reader through failure and recovery", async () => {
    readFile.mockRejectedValueOnce(new Error("file unavailable"));
    openWorkspaceFile(testClient.workspace.access, "src/main.go");
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
          read.resolve({ content: "package main", startLine: 1, endLine: 1, totalLines: 1 }),
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
    openWorkspaceFile(testClient.workspace.access, "src/main.go");
    mount(<FileWorkspace />);
    await screen.findByText("package main");

    const back = screen.getByRole("button", { name: "Back to files" });
    act(() => back.focus());
    fireEvent.click(back);
    expect(screen.queryByRole("row", { name: "src" })).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("treegrid", { name: "Files" }));
    await act(async () => listing.resolve([directory]));
    const row = await screen.findByRole("row", { name: "main.go" });
    expect(row.querySelector("[aria-current]")?.getAttribute("aria-current")).toBe("true");
    expect(row.querySelector("[title]")?.getAttribute("title")).toBe("src/main.go");
    expect(screen.getByRole("treegrid", { name: "Files" }).contains(document.activeElement)).toBe(
      true,
    );
  });

  it("keeps focus where the user moved it while the returning file tree loads", async () => {
    const listing = Promise.withResolvers<WorkspaceFileEntry[]>();
    listFiles.mockImplementation(async (query) => (query.path ? [file] : listing.promise));
    openWorkspaceFile(testClient.workspace.access, "src/main.go");
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
    expect(screen.queryByRole("row", { name: "src" })).toBeNull();
    const control = screen.getByRole("button", { name: "Another control" });
    act(() => control.focus());

    await act(async () => listing.resolve([directory]));
    await screen.findByRole("row", { name: "main.go" });
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
      openWorkspaceFile(testClient.workspace.access, "assets/logo.png");
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
    openWorkspaceFile(testClient.workspace.access, longPath);
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
    readFile.mockResolvedValueOnce({
      content: "package first",
      startLine: 1,
      endLine: 1,
      totalLines: 1,
    });
    openWorkspaceFile(testClient.workspace.access, longPath);
    mount(<FileWorkspace />);
    await screen.findByText("package first");
    const { region } = shownPath(longPath);

    readFile.mockResolvedValueOnce({
      content: "package next",
      startLine: 1,
      endLine: 1,
      totalLines: 1,
    });
    act(() => openWorkspaceFile(testClient.workspace.access, "cmd/flame/main.go"));
    await screen.findByText("package next");
    expect(shownPath("cmd/flame/main.go").region).toBe(region);
    expect(within(region).queryByText(longPath)).toBeNull();
    expect(header().textContent).not.toContain("cmd/flame");
  });

  it("shows the complete path ahead of both rendered and source presentations", async () => {
    const path = "docs/guides/runtime/reading-guide.md";
    readFile.mockResolvedValue({ content: "# Guide", startLine: 1, endLine: 1, totalLines: 1 });
    openWorkspaceFile(testClient.workspace.access, path);
    mount(<FileWorkspace />);

    const rendered = await screen.findByTestId("rendered-markdown");
    expect(precedes(shownPath(path).shown, rendered)).toBe(true);
    fireEvent.click(screen.getByRole("radio", { name: "Source" }));
    expect(
      precedes(shownPath(path).shown, document.querySelector('[data-quote-source="file"]')!),
    ).toBe(true);
    expect(header().textContent).not.toContain("docs/guides/runtime");
  });

  it("shows the complete path while a read is pending and after it fails", async () => {
    const read = Promise.withResolvers<WorkspaceFileContent>();
    readFile.mockReturnValueOnce(read.promise);
    openWorkspaceFile(testClient.workspace.access, longPath);
    mount(<FileWorkspace />);
    shownPath(longPath);

    await act(async () => read.reject(new Error("file unavailable")));
    expect(await screen.findByText("Couldn't open file")).toBeTruthy();
    shownPath(longPath);
    expect(header().textContent).not.toContain(directoryPath);
    expect(within(header()).getByText(longName)).toBeTruthy();
  });

  it.each([
    "apps/web/src/features/shell/workbench/panel/DockResizer.tsx",
    "docs/设计稿/第二版界面/阅读器路径说明.md",
    "README.md",
    "notes/drafts\\2026/a\\b.txt",
  ])("shows, selects and copies the exact path %j", async (path) => {
    const writeText = vi.spyOn(navigator.clipboard, "writeText").mockResolvedValue(undefined);
    openWorkspaceFile(testClient.workspace.access, path);
    mount(<FileWorkspace />);
    await screen.findByText("package main");

    const { shown } = shownPath(path);
    expect(shown.textContent).toBe(path);
    const range = document.createRange();
    range.selectNodeContents(shown);
    const selection = document.getSelection()!;
    selection.removeAllRanges();
    selection.addRange(range);
    expect(selection.toString()).toBe(path);
    selection.removeAllRanges();

    fireEvent.click(within(header()).getByRole("button", { name: "Copy path" }));
    await waitFor(() => expect(writeText).toHaveBeenCalledExactlyOnceWith(path));
  });

  it("shows the complete path beside an unsupported file", async () => {
    const path = "assets/brand/marketing/launch/logo.png";
    readFile.mockRejectedValue(
      new RpcError({ code: -32602, message: "binary", data: { type: "unsupported_mime" } }),
    );
    openWorkspaceFile(testClient.workspace.access, path);
    mount(<FileWorkspace />);

    expect(await screen.findByText("No image preview here yet")).toBeTruthy();
    shownPath(path);
    expect(header().textContent).not.toContain("assets/brand");
    expect(within(header()).getByText("logo.png")).toBeTruthy();
    expect(within(header()).getByRole("button", { name: "Copy path" })).toBeTruthy();
  });
});

describe("file source selection", () => {
  const path = "src/review.go";
  const content =
    'package main\n\n// 核对空白与缩进\nfunc main() {\n\tmessage := "保留  double spaces"\n\tprintln(message)\n\n}\n\n';
  const captured = (text = content, totalLines = text.split("\n").length) => ({
    content: text,
    startLine: 1,
    endLine: text.split("\n").length,
    totalLines,
  });
  const sourceView = () => document.querySelector<HTMLElement>('[data-quote-source="file"]')!;
  const clipboard = (host: HTMLElement) => {
    const setData = vi.fn();
    const native = fireEvent.copy(host, { clipboardData: { setData } });
    return { setData, native };
  };
  const expectSourceRange = (source: HTMLElement) => {
    const range = document.getSelection()!.getRangeAt(0);
    expect(range.startContainer).toBe(source);
    expect(range.startOffset).toBe(0);
    expect(range.endContainer).toBe(source);
    expect(range.endOffset).toBe(source.childNodes.length);
  };

  beforeEach(() => document.getSelection()!.removeAllRanges());
  afterEach(() => document.getSelection()!.removeAllRanges());

  it.each(["metaKey", "ctrlKey"])(
    "selects only the presented source with %s and copies its literal content",
    async (modifier) => {
      readFile.mockResolvedValue(captured());
      openWorkspaceFile(testClient.workspace.access, path);
      mount(
        <>
          <textarea aria-label="Message" defaultValue="Retained draft" />
          <FileWorkspace />
        </>,
      );
      await waitFor(() => expect(row(10)).toBeTruthy());
      const host = screen.getByRole("region", { name: path });
      const source = sourceView();
      act(() => host.focus());

      expect(fireEvent.keyDown(host, { key: "a", [modifier]: true })).toBe(false);
      expectSourceRange(source);
      expect(source.contains(host.querySelector('[data-slot="file-path"]'))).toBe(false);
      expect(document.activeElement).toBe(host);
      const copied = clipboard(host);
      expect(copied.native).toBe(false);
      expect(copied.setData.mock.calls).toEqual([["text/plain", content]]);
      expect(screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message" }).value).toBe(
        "Retained draft",
      );
    },
  );

  it.each(["metaKey", "ctrlKey"])(
    "scopes rendered Markdown with %s, keeps its native copy, and scopes its selected or fallback source",
    async (modifier) => {
      const text = "# Guide\n\nA  paragraph\n";
      readFile.mockResolvedValue(captured(text));
      openWorkspaceFile(testClient.workspace.access, "docs/guide.md");
      mount(
        <>
          <p>Conversation outside the file</p>
          <FileWorkspace />
        </>,
      );
      const rendered = await screen.findByTestId("rendered-markdown");
      const host = screen.getByRole("region", { name: "docs/guide.md" });
      act(() => host.focus());

      expect(document.activeElement).toBe(host);
      expect(fireEvent.keyDown(host, { key: "a", [modifier]: true })).toBe(false);
      const selected = document.getSelection()!;
      expect(selected.toString()).toBe(text);
      const scope = selected.getRangeAt(0).commonAncestorContainer;
      expect(scope.contains(rendered)).toBe(true);
      for (const outside of [
        within(host).getByText("docs/guide.md"),
        screen.getByRole("button", { name: "Back to files" }),
        screen.getByRole("radio", { name: "Preview" }),
        screen.getByText("Conversation outside the file"),
      ]) {
        expect(scope.contains(outside)).toBe(false);
      }
      expect(document.activeElement).toBe(host);
      const native = clipboard(host);
      expect(native.native).toBe(true);
      expect(native.setData).not.toHaveBeenCalled();

      fireEvent.click(screen.getByRole("radio", { name: "Source" }));
      expect(fireEvent.keyDown(host, { key: "a", [modifier]: true })).toBe(false);
      expectSourceRange(sourceView());
      expect(clipboard(host).setData.mock.calls).toEqual([["text/plain", text]]);

      fireEvent.click(screen.getByRole("radio", { name: "Preview" }));
      await screen.findByTestId("rendered-markdown");
      await resetKernelForTest();
      expect(screen.queryByTestId("rendered-markdown")).toBeNull();
      expect(fireEvent.keyDown(host, { key: "a", [modifier]: true })).toBe(false);
      expectSourceRange(sourceView());
      expect(clipboard(host).setData.mock.calls).toEqual([["text/plain", text]]);
    },
  );

  it("keeps a rendered field's native selection inside the file reader", async () => {
    renderedExtra = <input aria-label="Rendered note" defaultValue="Field draft" />;
    readFile.mockResolvedValue(captured("# Guide\n"));
    openWorkspaceFile(testClient.workspace.access, "docs/guide.md");
    mount(<FileWorkspace />);
    await screen.findByTestId("rendered-markdown");
    const field = screen.getByRole<HTMLInputElement>("textbox", { name: "Rendered note" });
    act(() => field.focus());

    expect(fireEvent.keyDown(field, { key: "a", metaKey: true })).toBe(true);
    expect(document.getSelection()!.toString()).toBe("");
    expect(field.value).toBe("Field draft");
    const copied = clipboard(field);
    expect(copied.native).toBe(true);
    expect(copied.setData).not.toHaveBeenCalled();
  });

  it("keeps a rendered code block's own selection inside the file reader", async () => {
    renderedExtra = <CodeBlock label="Example code">{"fmt.Println(1)"}</CodeBlock>;
    readFile.mockResolvedValue(captured("# Guide\n"));
    openWorkspaceFile(testClient.workspace.access, "docs/guide.md");
    mount(<FileWorkspace />);
    await screen.findByTestId("rendered-markdown");
    const code = screen.getByRole("region", { name: "Example code" });
    act(() => code.focus());

    expect(fireEvent.keyDown(code, { key: "a", ctrlKey: true })).toBe(false);
    const selected = document.getSelection()!;
    expect(selected.toString()).toBe("fmt.Println(1)");
    expect(selected.getRangeAt(0).commonAncestorContainer).toBe(code);
    expect(document.activeElement).toBe(code);
    const copied = clipboard(code);
    expect(copied.native).toBe(true);
    expect(copied.setData).not.toHaveBeenCalled();
  });

  it("copies accepted content through a pending page change and a shorter refresh", async () => {
    readFile.mockResolvedValueOnce(captured(content, 30));
    openWorkspaceFile(testClient.workspace.access, path);
    mount(<FileWorkspace />);
    await waitFor(() => expect(row(10)).toBeTruthy());
    const host = screen.getByRole("region", { name: path });
    const source = sourceView();
    const pending = Promise.withResolvers<WorkspaceFileContent>();
    readFile.mockReturnValueOnce(pending.promise);
    fireEvent.click(screen.getByRole("button", { name: /more/i }));
    fireEvent.keyDown(host, { key: "a", metaKey: true });
    expectSourceRange(source);
    expect(clipboard(host).setData.mock.calls).toEqual([["text/plain", content]]);

    const nextPage = "func extra() {}\n\n";
    await act(async () =>
      pending.resolve({ content: nextPage, startLine: 11, endLine: 13, totalLines: 30 }),
    );
    await waitFor(() => expect(row(13)).toBeTruthy());
    expect(sourceView()).toBe(source);
    expectSourceRange(source);
    expect(clipboard(host).setData.mock.calls).toEqual([["text/plain", nextPage]]);

    const shortened = "func shorter() {}";
    readFile.mockResolvedValueOnce({
      content: shortened,
      startLine: 11,
      endLine: 11,
      totalLines: 30,
    });
    await act(() => client.refetchQueries({ type: "active" }));
    await waitFor(() => expect(row(13)).toBeNull());
    expect(sourceView()).toBe(source);
    expectSourceRange(source);
    expect(clipboard(host).setData.mock.calls).toEqual([["text/plain", shortened]]);
  });
});

describe("byte-bounded file pages", () => {
  it("reaches both ends of a large source and copies only the accepted page", async () => {
    const lines = Array.from({ length: 1200 }, (_, index) => `${index + 1}:${"x".repeat(1000)}`);
    lines.push("");
    const original = lines.join("\n");
    expect(new TextEncoder().encode(original).length).toBeGreaterThan(1 << 20);
    let served: WorkspaceFileContent | undefined;
    readFile.mockImplementation(async ({ startLine = 1, endLine = lines.length, fromEnd }) => {
      let start = startLine - 1;
      let end = Math.min(endLine, lines.length);
      let bytes = 0;
      let count = 0;
      for (
        let index = fromEnd ? end - 1 : start;
        index >= start && index < end;
        index += fromEnd ? -1 : 1
      ) {
        const added = new TextEncoder().encode(lines[index]!).length + (count > 0 ? 1 : 0);
        if (bytes + added > 1 << 20) break;
        bytes += added;
        count++;
      }
      if (fromEnd) start = end - count;
      else end = start + count;
      served = {
        content: lines.slice(start, end).join("\n"),
        startLine: start + 1,
        endLine: end,
        totalLines: lines.length,
        truncated: start > 0 || end < lines.length,
      };
      return served;
    });
    openWorkspaceFile(testClient.workspace.access, "large.log");
    mount(<FileWorkspace />);
    await waitFor(() => expect(row(1)).toBeTruthy());
    const host = screen.getByRole("region", { name: "large.log" });
    const forward: string[] = [served!.content];
    while (served!.endLine < lines.length) {
      const previousEnd = served!.endLine;
      fireEvent.click(screen.getByRole("button", { name: /more/i }));
      await waitFor(() => expect(row(previousEnd + 1)).toBeTruthy());
      expect(readFile).toHaveBeenLastCalledWith({
        cwd: undefined,
        path: "large.log",
        startLine: previousEnd + 1,
        endLine: previousEnd + 200,
      });
      expect(row(previousEnd)).toBeNull();
      forward.push(served!.content);
    }
    expect(forward.join("\n")).toBe(original);
    expect(row(lines.length)).toBeTruthy();
    fireEvent.keyDown(host, { key: "a", metaKey: true });
    const source = document.querySelector('[data-quote-source="file"]')!;
    expect(document.getSelection()!.getRangeAt(0).commonAncestorContainer).toBe(source);
    const setData = vi.fn();
    fireEvent.copy(host, { clipboardData: { setData } });
    expect(setData.mock.calls).toEqual([["text/plain", served!.content]]);
    expect(served!.content).not.toBe(original);
    const backward: string[] = [served!.content];
    while (served!.startLine > 1) {
      const previousStart = served!.startLine;
      fireEvent.click(screen.getByRole("button", { name: /earlier/i }));
      await waitFor(() => expect(row(previousStart - 1)).toBeTruthy());
      expect(readFile).toHaveBeenLastCalledWith({
        cwd: undefined,
        path: "large.log",
        startLine: Math.max(1, previousStart - 200),
        endLine: previousStart - 1,
        fromEnd: true,
      });
      expect(row(previousStart)).toBeNull();
      backward.unshift(served!.content);
    }
    expect(backward.join("\n")).toBe(original);
    expect(screen.queryByRole("button", { name: /earlier/i })).toBeNull();
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
      endLine: 2,
      totalLines: 2,
    });
    openWorkspaceFile(testClient.workspace.access, guide);
    mount(
      <>
        <textarea aria-label="Message" defaultValue="Retained message draft" />
        <FileWorkspace />
      </>,
    );
    await screen.findByTestId("rendered-markdown");
    fireEvent.click(screen.getByRole("radio", { name: "Source" }));
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
    expect(screen.getByRole("radio", { name: "Source", checked: true })).toBeTruthy();
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
          read.resolve({
            content: "# Guide\nUpdated line",
            startLine: 1,
            endLine: 2,
            totalLines: 2,
          }),
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
    expect(screen.getByRole("radio", { name: "Source", checked: true })).toBeTruthy();
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
      readFile.mockResolvedValueOnce({ content, startLine: 1, endLine: 1, totalLines: 1 });
      openWorkspaceFile(testClient.workspace.access, path);
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
      expect(screen.queryByRole("radio", { checked: true })?.textContent).toBe(mode);

      readFile.mockResolvedValueOnce({ content: updated, startLine: 1, endLine: 1, totalLines: 1 });
      fireEvent.click(screen.getByRole("button", { name: "Retry" }));
      await waitFor(() => expect(node.textContent).toContain(updated));
      expect(shown()).toBe(node);
      expect(screen.getByRole("region", { name: path })).toBe(host);
      expect(screen.queryByText("Couldn't refresh file")).toBeNull();
      expect(screen.queryByRole("radio", { checked: true })?.textContent).toBe(mode);
    },
  );

  it("explains an unsupported refresh beside the accepted source without a retry", async () => {
    openWorkspaceFile(testClient.workspace.access, "src/main.go");
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
      readFile.mockResolvedValueOnce({
        content: "cached pixels",
        startLine: 1,
        endLine: 1,
        totalLines: 1,
      });
      openWorkspaceFile(testClient.workspace.access, path);
      mount(<FileWorkspace />);
      await screen.findByText("cached pixels");
      expect(screen.getByText("1 line")).toBeTruthy();
      if (hasMode) fireEvent.click(screen.getByRole("radio", { name: "Source" }));
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
      expect(screen.queryByRole("radio", { name: "Source" })).toBeNull();
      expect(screen.queryByRole("radio", { name: "Preview" })).toBeNull();
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
      openWorkspaceFile(testClient.workspace.access, "src/main.go");
      const view = mount(<FileWorkspace />);
      await screen.findByText("package main");
      readFile.mockRejectedValueOnce(unavailable());
      await act(() => client.refetchQueries(readQuery("src/main.go", "/work/one")));
      await screen.findByText("Couldn't refresh file");

      const read = Promise.withResolvers<WorkspaceFileContent>();
      readFile.mockReturnValueOnce(read.promise);
      selection.current = { status: "ready", cwd: next.cwd };
      if (change === "path") {
        act(() => openWorkspaceFile(testClient.workspace.access, next.path));
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

      await act(async () =>
        read.resolve({ content: "package next", startLine: 1, endLine: 1, totalLines: 1 }),
      );
      expect(await screen.findByText("package next")).toBeTruthy();
      expect(readFile).toHaveBeenLastCalledWith(next);
    },
  );

  it("keeps the ordinary read failure for a new line window without accepted data", async () => {
    readFile.mockResolvedValueOnce(lineWindow(100, 500, 600));
    openWorkspaceFile(testClient.workspace.access, "src/big.go");
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
      startLine: 501,
      endLine: 700,
    });
  });
});

describe("Go to line target validation", () => {
  afterEach(() => vi.restoreAllMocks());

  async function openLineEditor() {
    readFile.mockResolvedValue({
      content: "one\ntwo\nthree\nfour\nfive",
      startLine: 1,
      endLine: 5,
      totalLines: 5,
    });
    const viewer = testClient.renderHook(useWorkspaceFileViewer);
    act(() => openWorkspaceFile(testClient.workspace.access, "src/main.go", 2));
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

    expect(navigate).toHaveBeenCalledExactlyOnceWith(testClient.workspace.access, "src/main.go", 4);
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
      expect(navigate).toHaveBeenCalledExactlyOnceWith(
        testClient.workspace.access,
        "src/main.go",
        4,
      );
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
    vi.mocked(testClient.workspace.access.local.open).mockResolvedValue(false);
    vi.mocked(testClient.workspace.access.local.reveal).mockResolvedValue(false);
    return vi.spyOn(testClient.messages, "record");
  }

  it("reports a refused header reveal without replacing the reader or moving focus", async () => {
    const record = prepareLocalActions();
    openWorkspaceFile(testClient.workspace.access, file.path);
    mount(<FileWorkspace />);
    const reader = await screen.findByRole("region", { name: file.path });
    await screen.findByText("package main");
    const reveal = screen.getByRole("button", { name: "Show in Finder" });
    act(() => reveal.focus());
    const before = testClient.workspace.dock.store.getState().fileViewer;

    fireEvent.click(reveal);

    const feedback = await screen.findByRole("alert");
    expect(feedback.textContent).toContain(file.path);
    expect(feedback.textContent).toContain("Finder");
    expect(record).toHaveBeenCalledOnce();
    expect(record.mock.calls[0]![0].message).toContain(file.path);
    expect(testClient.workspace.access.local.reveal).toHaveBeenCalledExactlyOnceWith(
      "/work/project",
      file.path,
    );
    expect(screen.getByRole("region", { name: file.path })).toBe(reader);
    expect(testClient.workspace.dock.store.getState().fileViewer).toBe(before);
    expect(document.activeElement).toBe(reveal);
  });

  it("reports a refused tree reveal after the menu returns to its original row", async () => {
    const record = prepareLocalActions();
    listFiles.mockResolvedValue([file]);
    mount(<FileWorkspace />);
    const item = await screen.findByRole("row", { name: file.name });
    act(() => item.focus());
    fireEvent.contextMenu(item);
    const menu = await screen.findByRole("menu", { name: file.path });

    fireEvent.click(within(menu).getByRole("menuitem", { name: "Show in Finder" }));

    const feedback = await screen.findByRole("alert");
    expect(feedback.textContent).toContain(file.path);
    expect(record).toHaveBeenCalledOnce();
    expect(testClient.workspace.access.local.reveal).toHaveBeenCalledExactlyOnceWith(
      "/work/project",
      file.path,
    );
    await waitFor(() => expect(menu.isConnected).toBe(false));
    await waitFor(() => expect(document.activeElement).toBe(item));
    expect(screen.getByRole("row", { name: file.name })).toBe(item);
    expect(readFile).not.toHaveBeenCalled();
  });

  async function unsupportedFile() {
    const path = "assets/logo.png";
    readFile.mockRejectedValue(
      new RpcError({ code: -32602, message: "binary", data: { type: "unsupported_mime" } }),
    );
    openWorkspaceFile(testClient.workspace.access, path);
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
    const before = testClient.workspace.dock.store.getState().fileViewer;
    act(() => open.focus());

    fireEvent.click(open);

    expect((await screen.findByRole("alert")).textContent).toContain(path);
    expect(record).toHaveBeenCalledOnce();
    expect(testClient.workspace.access.local.open).toHaveBeenCalledExactlyOnceWith(
      "/work/project",
      path,
    );
    expect(screen.getByRole("button", { name: "Open" })).toBe(open);
    expect(screen.getByText("No image preview here yet")).toBeTruthy();
    expect(testClient.workspace.dock.store.getState().fileViewer).toBe(before);
    expect(document.activeElement).toBe(open);
  });

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
    vi.mocked(testClient.workspace.access.local.reveal).mockReturnValue(pending.promise);
    openWorkspaceFile(testClient.workspace.access, file.path);
    mount(
      <>
        <FileWorkspace />
        <button type="button">Elsewhere</button>
      </>,
    );
    await screen.findByText("package main");
    fireEvent.click(screen.getByRole("button", { name: "Show in Finder" }));
    act(() => openWorkspaceFile(testClient.workspace.access, "src/other.go"));
    await screen.findByRole("region", { name: "src/other.go" });
    const outside = screen.getByRole("button", { name: "Elsewhere" });
    act(() => outside.focus());

    await act(async () => pending.reject(new Error("Original file is unavailable")));

    const feedback = await screen.findByRole("alert");
    expect(feedback.textContent).toContain(file.path);
    expect(feedback.textContent).not.toContain("src/other.go");
    expect(testClient.workspace.dock.store.getState().fileViewer?.path).toBe("src/other.go");
    expect(document.activeElement).toBe(outside);
  });
});

describe("retained file workspace popups", () => {
  it("starts each workspace's file at its requested window across a round trip", async () => {
    selection.current = { status: "ready", cwd: "/work/one" };
    readFile.mockImplementation(async (query) => {
      const start = query.startLine ?? 1;
      return lineWindow(start, start + 9, 1000);
    });
    openWorkspaceFile(testClient.workspace.access, "src/big.go");
    const view = mount(<FileWorkspace />);
    await waitFor(() => expect(row(1)).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: /more/i }));
    await waitFor(() => expect(row(11)).toBeTruthy());

    for (const cwd of ["/work/two", "/work/one"]) {
      selection.current = { status: "ready", cwd };
      view.rerender(
        <QueryClientProvider client={client}>
          <FileWorkspace />
        </QueryClientProvider>,
      );
      await waitFor(() => expect(row(1)).toBeTruthy());
      expect(row(11)).toBeNull();
    }
    expect(readFile).toHaveBeenCalledWith({ cwd: "/work/two", path: "src/big.go" });
  });

  it("retires a file's presentation and popup drafts across workspace round trips", async () => {
    selection.current = { status: "ready", cwd: "/work/one" };
    readFile.mockResolvedValue({ content: "# Guide", startLine: 1, endLine: 1, totalLines: 1 });
    openWorkspaceFile(testClient.workspace.access, "docs/guide.md");
    const view = mount(<FileWorkspace />);
    await screen.findByTestId("rendered-markdown");
    fireEvent.click(screen.getByRole("radio", { name: "Source" }));
    fireEvent.click(screen.getByRole("button", { name: "Go to line" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Go to line" }), {
      target: { value: "37" },
    });

    for (const cwd of ["/work/two", "/work/one"]) {
      selection.current = { status: "ready", cwd };
      view.rerender(
        <QueryClientProvider client={client}>
          <FileWorkspace />
        </QueryClientProvider>,
      );
      await screen.findByTestId("rendered-markdown");
      expect(screen.queryByRole("dialog")).toBeNull();
      fireEvent.click(screen.getByRole("button", { name: "Go to line" }));
      expect(screen.getByRole<HTMLInputElement>("textbox", { name: "Go to line" }).value).toBe("");
      fireEvent.keyDown(screen.getByRole("textbox", { name: "Go to line" }), { key: "Escape" });
      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    }
  });

  it("retires an open row menu when another workspace has the same cached paths", async () => {
    selection.current = { status: "ready", cwd: "/work/one" };
    client.setQueryData(directoryKey("/work/two"), {
      pages: [page([directory])],
      pageParams: [undefined],
    });
    const view = mount(<FileWorkspace />);
    const original = await screen.findByRole("row", { name: "src" });
    fireEvent.contextMenu(original);
    const menu = await screen.findByRole("menu");
    await vi.waitFor(() => expect(menu.contains(document.activeElement)).toBe(true));

    selection.current = { status: "ready", cwd: "/work/two" };
    view.rerender(
      <QueryClientProvider client={client}>
        <FileWorkspace />
      </QueryClientProvider>,
    );

    expect(screen.getByRole("row", { name: "src" })).toBeTruthy();
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it.each([
    { label: "file", cwd: "/work/one", path: "src/other.go" },
    { label: "workspace", cwd: "/work/two", path: file.path },
  ])(
    "retires Go to line and its draft when another $label is opened from cache",
    async ({ cwd, path }) => {
      selection.current = { status: "ready", cwd: "/work/one" };
      client.setQueryData([WORKSPACE_READ_FILE_KEY, { cwd, path }], {
        content: "another file",
        startLine: 1,
        endLine: 1,
        totalLines: 1,
      });
      act(() => openWorkspaceFile(testClient.workspace.access, file.path));
      const view = mount(<FileWorkspace />);
      await screen.findByText("package main");
      fireEvent.click(screen.getByRole("button", { name: "Go to line" }));
      const dialog = await screen.findByRole("dialog");
      fireEvent.change(within(dialog).getByRole("textbox", { name: "Go to line" }), {
        target: { value: "37" },
      });

      selection.current = { status: "ready", cwd };
      act(() => openWorkspaceFile(testClient.workspace.access, path));
      view.rerender(
        <QueryClientProvider client={client}>
          <FileWorkspace />
        </QueryClientProvider>,
      );

      await screen.findByText("another file");
      expect(screen.queryByRole("dialog")).toBeNull();
      fireEvent.click(screen.getByRole("button", { name: "Go to line" }));
      const reopened = await screen.findByRole("dialog");
      expect(
        within(reopened).getByRole<HTMLInputElement>("textbox", { name: "Go to line" }).value,
      ).toBe("");
    },
  );

  it("subscribes only to the current workspace after the retained browser changes source while hidden", async () => {
    selection.current = { status: "ready", cwd: "/work/one" };
    testClient.workspace.dock.store.setState({
      memory: { expandedDirs: ["src"], lastFilePath: file.path },
    });
    const scene = (visible: boolean) => (
      <QueryClientProvider client={client}>
        <ContentVisibility visible={visible}>
          <FileWorkspace />
        </ContentVisibility>
      </QueryClientProvider>
    );
    const view = testClient.render(scene(true));
    await screen.findByRole("row", { name: "main.go" });
    view.rerender(scene(false));
    await waitFor(() => expect(client.isFetching()).toBe(0));
    listFiles.mockClear();

    selection.current = { status: "ready", cwd: "/work/two" };
    view.rerender(scene(false));
    await act(async () => client.invalidateQueries({ queryKey: [WORKSPACE_LIST_FILES_KEY] }));
    expect(listFiles).not.toHaveBeenCalled();

    view.rerender(scene(true));
    await screen.findByRole("row", { name: "main.go" });
    await waitFor(() => expect(client.isFetching()).toBe(0));
    expect(listFiles).toHaveBeenCalledTimes(2);
    expect(listFiles).toHaveBeenCalledWith({
      cwd: "/work/two",
      path: "",
      limit: 100,
      cursor: undefined,
    });
    expect(listFiles).toHaveBeenCalledWith({
      cwd: "/work/two",
      path: "src",
      limit: 100,
      cursor: undefined,
    });
  });

  it("retires Go to line when hidden and keeps its draft for an explicit reopen", async () => {
    readFile.mockResolvedValue({
      content: "one\ntwo\nthree\nfour\nfive",
      startLine: 1,
      endLine: 5,
      totalLines: 5,
    });
    const viewer = testClient.renderHook(useWorkspaceFileViewer);
    act(() => openWorkspaceFile(testClient.workspace.access, "src/main.go"));
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
    testClient.workspace.dock.store.setState({
      memory: { expandedDirs: ["src"], lastFilePath: "src/main.go" },
    });
    const host = mountRetained();
    const row = await screen.findByRole("row", { name: "main.go" });
    act(() => row.focus());
    fireEvent.contextMenu(row);
    const menu = await screen.findByRole("menu");
    await vi.waitFor(() => expect(menu.contains(document.activeElement)).toBe(true));

    host.hide();
    await vi.waitFor(() => expect(menu.isConnected).toBe(false));
    host.reveal();

    expect(screen.queryByRole("menu")).toBeNull();
    expect(screen.getByRole("row", { name: "main.go" })).toBe(row);
    expect(row.tabIndex).toBe(0);
    expect(row.querySelector("[aria-current]")?.getAttribute("aria-current")).toBe("true");
    expect(screen.getByRole("row", { name: "src" }).getAttribute("aria-expanded")).toBe("true");
  });

  it("retires the tree menu when a file preview hides the retained browser", async () => {
    testClient.workspace.dock.store.setState({
      memory: { expandedDirs: ["src"], lastFilePath: "src/main.go" },
    });
    mount(<FileWorkspace />);
    const row = await screen.findByRole("row", { name: "main.go" });
    act(() => row.focus());
    fireEvent.contextMenu(row);
    const menu = await screen.findByRole("menu");
    await vi.waitFor(() => expect(menu.contains(document.activeElement)).toBe(true));

    act(() => openWorkspaceFile(testClient.workspace.access, file.path));
    await screen.findByRole("button", { name: "Back to files" });
    expect(screen.queryByRole("menu")).toBeNull();
    act(() => closeWorkspaceFile(testClient.workspace.access));
    expect(screen.queryByRole("menu")).toBeNull();
    await screen.findByRole("row", { name: "main.go" });
    expect(screen.getByRole("row", { name: "main.go" })).toBe(row);
  });

  it("pauses root and expanded directory reads behind a preview and resumes them on return", async () => {
    testClient.workspace.dock.store.setState({
      memory: { expandedDirs: ["src"], lastFilePath: "src/main.go" },
    });
    mount(<FileWorkspace />);
    const row = await screen.findByRole("row", { name: "main.go" });
    act(() => openWorkspaceFile(testClient.workspace.access, file.path));
    await screen.findByText("package main");
    await waitFor(() => expect(client.isFetching()).toBe(0));
    listFiles.mockClear();

    await act(async () => client.invalidateQueries({ queryKey: [WORKSPACE_LIST_FILES_KEY] }));
    expect(listFiles).not.toHaveBeenCalled();

    act(() => closeWorkspaceFile(testClient.workspace.access));
    await waitFor(() => expect(listFiles).toHaveBeenCalledTimes(2));
    expect(listFiles).toHaveBeenCalledWith({
      cwd: undefined,
      path: "",
      limit: 100,
      cursor: undefined,
    });
    expect(listFiles).toHaveBeenCalledWith({
      cwd: undefined,
      path: "src",
      limit: 100,
      cursor: undefined,
    });
    await waitFor(() => expect(client.isFetching()).toBe(0));
    expect(screen.getByRole("row", { name: "main.go" })).toBe(row);
  });
});
