import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { DATA_PROVIDER, definePlugin } from "@/plugins/sdk";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import { navigator } from "@/lib/navigation";
import { openWorkspaceFile } from "../application/navigation";
import { useContextDockStore } from "../adapters/contextDockStore";
import { installWorkspaceErrorClassifier } from "../adapters/runtimeWorkspaceErrorClassifier";
import {
  WORKSPACE_LIST_FILES_KEY,
  WORKSPACE_READ_FILE_KEY,
  type WorkspaceFileContent,
  type WorkspaceFileEntry,
  type WorkspaceListFilesQuery,
  type WorkspaceReadFileQuery,
} from "../application/workspaceQueries";
import { FileViewTab } from "./file";
import { FileTree } from "./views/FileTree";
import { RpcError } from "@/rpc";

const selection = vi.hoisted(() => ({
  current: { status: "ready" } as
    { status: "ready"; cwd?: string } | { status: "resolving"; sessionId: string },
}));

vi.mock("@/plugins/builtin/agent/public/session", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/plugins/builtin/agent/public/session")>()),
  useActiveSessionWorkspace: () => selection.current,
}));

let client: QueryClient;
const listFiles = vi.fn<(query: WorkspaceListFilesQuery) => Promise<WorkspaceFileEntry[]>>();
const readFile = vi.fn<(query: WorkspaceReadFileQuery) => Promise<WorkspaceFileContent>>();
const directory: WorkspaceFileEntry = { name: "src", path: "src", type: "dir" };
const file: WorkspaceFileEntry = { name: "main.go", path: "src/main.go", type: "file" };

let uninstallClassifier: () => void;

beforeEach(async () => {
  uninstallClassifier = installWorkspaceErrorClassifier();
  selection.current = { status: "ready" };
  useContextDockStore.setState({
    fileViewer: null,
    memory: {
      expandedDirs: [],
      lastFilePath: null,
      searchQuery: "",
      searchPath: "",
      diffMode: "worktree",
      diffLayout: "unified",
      collapsedDiffFiles: [],
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
      },
    }),
  );
});

afterEach(() => {
  uninstallClassifier();
  cleanup();
  client.clear();
});

function mount(children: React.ReactNode) {
  return render(<QueryClientProvider client={client}>{children}</QueryClientProvider>);
}

describe("workspace files panel", () => {
  it("reads a file in the Runtime default workspace", async () => {
    openWorkspaceFile("src/main.go");
    mount(<FileViewTab />);

    await screen.findByText("package main");
    expect(readFile).toHaveBeenCalledWith({ cwd: undefined, path: "src/main.go" });
  });

  it("waits for an unresolved Session workspace before following a line reference", async () => {
    selection.current = { status: "resolving", sessionId: "session-loading" };
    openWorkspaceFile("src/main.go", 450);
    const view = mount(<FileViewTab />);
    selection.current = { status: "ready", cwd: "/work/project" };
    view.rerender(
      <QueryClientProvider client={client}>
        <FileViewTab />
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

  it("browses and reads files in one panel, preserving expanded directories on return", async () => {
    mount(<FileViewTab />);
    fireEvent.click(await screen.findByRole("treeitem", { name: "src" }));
    fireEvent.click(await screen.findByRole("treeitem", { name: "main.go" }));
    await screen.findByText("package main");
    expect(navigator().get().dock).toBe("file");
    expect(useContextDockStore.getState().dockViewIds).toEqual(["file"]);

    fireEvent.click(screen.getByRole("button", { name: "Back to files" }));
    expect(await screen.findByRole("treeitem", { name: "main.go" })).toBeTruthy();
    expect(screen.queryByText("package main")).toBeNull();
    expect(useContextDockStore.getState().dockViewIds).toEqual(["file"]);
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
    openWorkspaceFile("src/main.go");
    mount(<FileViewTab />);
    await screen.findByText("package main");

    fireEvent.click(screen.getByRole("button", { name: "Back to files" }));
    const row = await screen.findByRole("treeitem", { name: "main.go" });
    expect(row.getAttribute("aria-current")).toBe("true");
    expect(row.getAttribute("title")).toBe("src/main.go");
  });

  it("names a file it cannot read as text and offers the platform's own exits", async () => {
    selection.current = { status: "ready", cwd: "/work/project" };
    readFile.mockRejectedValue(
      new RpcError({ code: -32602, message: "binary", data: { type: "unsupported_mime" } }),
    );
    openWorkspaceFile("assets/logo.png");
    mount(<FileViewTab />);

    expect(await screen.findByText("No image preview here yet")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Open" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Go to line" })).toBeNull();
  });
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
});
