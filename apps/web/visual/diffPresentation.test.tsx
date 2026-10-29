import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { mockSelectLayout, mockSelectViewport } from "@/test/selectLayout";
let testClient: TestClient;
prepareTestClient(() => {
  mockSelectLayout();
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { type ComponentContext } from "@flame/component-sdk";
import { createFlameClient } from "@flame/runtime-client";
import { type Diff, type FileStatus, type GetDiffRequest } from "@flame/runtime-client/wire";
import { WorkspaceDiff } from "../../../plugins/diff/component/component";
import { RUNTIME_AGENT_SESSION_SNAPSHOTS } from "./agentSessionSnapshots";
import { VISUAL_CAPABILITIES } from "./agentFixtureFacts";
import { createVisualRuntimeTransport } from "./createVisualRuntimeTransport";
import { ACTIVE_DIFF_FILE, REVIEW_DIFF, scaledReview } from "./diffSnapshot";
const releases: Array<() => Promise<void>> = [];

afterEach(async () => {
  cleanup();
  await Promise.all(releases.splice(0).map((release) => release()));
  vi.restoreAllMocks();
  vi.useRealTimers();
});

function mountDiff(
  path = "",
  readDiff: (signal?: AbortSignal, request?: GetDiffRequest) => Promise<Diff> = async (
    _signal,
    request,
  ) => ({
    ...REVIEW_DIFF,
    files: REVIEW_DIFF.files?.filter((file) => !request?.path || file.path === request.path),
  }),
  locale = "en",
) {
  const owner = new AbortController();
  const runtime = createFlameClient(
    createVisualRuntimeTransport(
      structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
      VISUAL_CAPABILITIES,
      readDiff,
    ),
    { capabilities: () => VISUAL_CAPABILITIES },
  );
  const workspace = runtime.workspace({ path: "/Users/visual/scope" });
  const get = vi.spyOn(workspace.diff, "get");
  vi.spyOn(runtime, "workspace").mockReturnValue(workspace);
  const context: ComponentContext = {
    component: { installationId: "diff-review", digest: "a".repeat(64), componentId: "diff" },
    runtime,
    workspace: { ref: { path: "/Users/visual/scope" }, focus: { path, revision: 1n } },
    telemetry: { read: () => ({ spans: [], metrics: [], logs: [] }), clear() {} },
    locale,
    signal: owner.signal,
    track: (operation) => operation(owner.signal),
    compose: () => null,
  };
  const outsideEscape = vi.fn();
  const returnHandler = (event: KeyboardEvent) => {
    if (event.key === "Escape" && !event.defaultPrevented) outsideEscape();
  };
  window.addEventListener("keydown", returnHandler);
  releases.push(async () => {
    window.removeEventListener("keydown", returnHandler);
    owner.abort();
    await runtime.close();
  });
  const view = testClient.render(
    <>
      <button>Outside action</button>
      <div data-testid="viewport" style={{ overflowY: "auto", height: 400 }}>
        <WorkspaceDiff context={context} />
      </div>
    </>,
  );
  mockSelectViewport(view.getByTestId("viewport"));
  return { ...view, get, owner, outsideEscape };
}

async function loaded() {
  await screen.findByText("Showing uncommitted changes.");
}

async function comparison(label: string) {
  fireEvent.click(screen.getByRole("combobox", { name: "Comparison" }));
  const option = await screen.findByRole("option", { name: label });
  fireEvent.pointerDown(option, { pointerType: "mouse" });
  fireEvent.click(option);
}

it("keeps an accepted read when the current comparison is selected again", async () => {
  const { get } = mountDiff();
  await loaded();
  const at = document.querySelector("time")!.dateTime;
  await comparison("Uncommitted changes");
  expect(get).toHaveBeenCalledTimes(1);
  expect(document.querySelector("time")!.dateTime).toBe(at);
});

it("reads a source-targeted file outside the workspace page and explicitly requests the overview", async () => {
  const read = vi.fn(async (_signal?: AbortSignal, request?: GetDiffRequest): Promise<Diff> => ({
    ...REVIEW_DIFF,
    files: request?.path === ACTIVE_DIFF_FILE ? [REVIEW_DIFF.files![0]!] : [REVIEW_DIFF.files![1]!],
    truncated: request?.path === undefined,
  }));
  const { get } = mountDiff(`/Users/visual/scope/${ACTIVE_DIFF_FILE}`, read);
  await loaded();

  expect(get).toHaveBeenCalledWith(
    { mode: "worktree", format: "rows", path: ACTIVE_DIFF_FILE },
    expect.any(AbortSignal),
  );
  expect(screen.getByRole("heading", { name: "DockResizer.tsx" })).toBeTruthy();
  expect(screen.getAllByRole("table")).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: "Show changed files" }));

  await waitFor(() => expect(get).toHaveBeenCalledTimes(2));
  expect(get).toHaveBeenLastCalledWith(
    { mode: "worktree", format: "rows" },
    expect.any(AbortSignal),
  );
  await screen.findByRole("heading", { name: "1 included file" });
  expect(screen.getByRole("button", { name: /^session.go / })).toBeTruthy();
  expect(screen.getByRole("note").textContent).toContain("Partial diff");
});

const BOUNDED_PATCH = `diff --git a/${ACTIVE_DIFF_FILE} b/${ACTIVE_DIFF_FILE}\n--- a/${ACTIVE_DIFF_FILE}\n+++ b/${ACTIVE_DIFF_FILE}\n@@ -1 +1 @@\n-old value\n+accepted patch value\n`;

it("continues a row-bounded target through the existing raw read and returns to workspace files", async () => {
  const read = vi.fn(async (_signal?: AbortSignal, request?: GetDiffRequest): Promise<Diff> => {
    if (request?.format === "raw") return { baseline: REVIEW_DIFF.baseline, patch: BOUNDED_PATCH };
    if (request?.path) return { baseline: REVIEW_DIFF.baseline, truncated: true };
    return { ...REVIEW_DIFF, files: [REVIEW_DIFF.files![1]!], truncated: true };
  });
  const { get, outsideEscape } = mountDiff(ACTIVE_DIFF_FILE, read);
  await loaded();
  expect(screen.queryByText("No changes in this comparison.")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Read patch" }));

  const patch = await screen.findByRole("region", { name: `Patch for ${ACTIVE_DIFF_FILE}` });
  expect(patch.textContent).toContain("accepted patch value");
  expect(get).toHaveBeenLastCalledWith(
    { mode: "worktree", format: "raw", path: ACTIVE_DIFF_FILE },
    expect.any(AbortSignal),
  );
  const heading = screen.getByRole<HTMLElement>("heading", { name: "DockResizer.tsx" });
  expect(document.activeElement).toBe(heading);
  expect(screen.queryByText("Modified")).toBeNull();
  fireEvent.keyDown(heading, { key: "Escape" });
  expect(outsideEscape).toHaveBeenCalledOnce();
  fireEvent.click(screen.getByRole("button", { name: "Show changed files" }));

  await screen.findByRole("heading", { name: "1 included file" });
  expect(get).toHaveBeenLastCalledWith(
    { mode: "worktree", format: "rows" },
    expect.any(AbortSignal),
  );
  expect(screen.queryByRole("region", { name: `Patch for ${ACTIVE_DIFF_FILE}` })).toBeNull();
});

it("keeps the accepted scope, baseline and read time beside a reader after a failed comparison", async () => {
  const first = "2026-10-28T03:00:01.000Z";
  const next = "2026-10-28T03:05:02.000Z";
  vi.setSystemTime(new Date(first));
  const read = vi.fn(async () => REVIEW_DIFF);
  mountDiff("", read);
  await loaded();
  read.mockRejectedValueOnce(new Error("No common ancestor"));
  vi.setSystemTime(new Date(next));
  await comparison("Branch changes");
  await screen.findByRole("alert");
  fireEvent.click(screen.getByRole("button", { name: /^DockResizer.tsx / }));

  const controls = document.querySelector<HTMLElement>('[data-slot="panel-controls"]')!;
  expect(controls.textContent).toContain("Uncommitted changes");
  expect(controls.textContent).toContain("HEAD");
  expect(controls.textContent).toContain("12345678");
  expect(controls.querySelector("time")?.dateTime).toBe(first);
  expect(screen.getByRole("region", { name: `Changes in ${ACTIVE_DIFF_FILE}` })).toBeTruthy();
  read.mockResolvedValueOnce({
    ...REVIEW_DIFF,
    baseline: { type: "mergeBase", commit: "b".repeat(40) },
  });
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));

  await screen.findByText("Showing branch changes.");
  expect(controls.textContent).toContain("Branch changes");
  expect(controls.textContent).toContain("merge base");
  expect(controls.textContent).toContain("bbbbbbbb");
  expect(controls.querySelector("time")?.dateTime).toBe(next);
});

it("retains a bounded target after a byte-bounded patch failure and retries the same scope", async () => {
  const read = vi.fn(async (_signal?: AbortSignal, request?: GetDiffRequest): Promise<Diff> => {
    if (request?.format === "raw") throw new Error("Workspace diff result exceeds the byte limit");
    return { baseline: REVIEW_DIFF.baseline, truncated: true };
  });
  const { get } = mountDiff(ACTIVE_DIFF_FILE, read);
  await loaded();
  const at = document.querySelector("time")!.dateTime;
  fireEvent.click(screen.getByRole("button", { name: "Read patch" }));
  const error = await screen.findByRole("alert");
  expect(error.textContent).toContain("byte limit");
  expect(document.querySelector("time")!.dateTime).toBe(at);
  expect(screen.getByRole("note").textContent).toContain("Partial diff");
  expect(screen.queryByText("No changes in this comparison.")).toBeNull();
  read.mockResolvedValueOnce({ baseline: REVIEW_DIFF.baseline, patch: BOUNDED_PATCH });
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await screen.findByRole("region", { name: `Patch for ${ACTIVE_DIFF_FILE}` });
  expect(get).toHaveBeenLastCalledWith(
    { mode: "worktree", format: "raw", path: ACTIVE_DIFF_FILE },
    expect.any(AbortSignal),
  );
  fireEvent.click(screen.getByRole("button", { name: "Show structured diff" }));
  await screen.findByRole("button", { name: "Read patch" });
  expect(get).toHaveBeenLastCalledWith(
    { mode: "worktree", format: "rows", path: ACTIVE_DIFF_FILE },
    expect.any(AbortSignal),
  );
});

it.each([
  ["en", "Read patch", "Copy patch", "Enable word wrap", "Disable word wrap"],
  ["zh", "阅读补丁", "复制补丁", "启用自动换行", "关闭自动换行"],
  ["zh-TW", "閱讀修補檔", "複製修補檔", "啟用自動換行", "關閉自動換行"],
  ["ja", "パッチを読む", "パッチをコピー", "折り返しを有効にする", "折り返しを無効にする"],
  ["ko", "패치 읽기", "패치 복사", "자동 줄 바꿈 사용", "자동 줄 바꿈 사용 안 함"],
  ["es", "Leer parche", "Copiar parche", "Activar ajuste de línea", "Desactivar ajuste de línea"],
  [
    "fr",
    "Lire le patch",
    "Copier le patch",
    "Activer le retour à la ligne",
    "Désactiver le retour à la ligne",
  ],
  ["de", "Patch lesen", "Patch kopieren", "Zeilenumbruch aktivieren", "Zeilenumbruch deaktivieren"],
])(
  "names patch actions in %s and keeps wrapping state observable",
  async (locale, open, copy, wrap, unwrap) => {
    mountDiff(
      ACTIVE_DIFF_FILE,
      async (_signal, request) =>
        request?.format === "raw"
          ? { baseline: REVIEW_DIFF.baseline, patch: BOUNDED_PATCH }
          : { baseline: REVIEW_DIFF.baseline, truncated: true },
      locale,
    );
    fireEvent.click(await screen.findByRole("button", { name: open }));
    await screen.findByRole("button", { name: copy });
    const enable = screen.queryByRole("button", { name: wrap });
    const toggle = enable ?? screen.getByRole("button", { name: unwrap });
    const next = enable ? unwrap : wrap;
    expect(toggle.getAttribute("aria-pressed")).toBeNull();
    fireEvent.click(toggle);
    expect(screen.getByRole("button", { name: next })).toBe(toggle);
    fireEvent.click(toggle);
  },
);

it("leaves later focus alone while the source reader requests the workspace overview", async () => {
  let release!: (value: Diff) => void;
  const read = vi.fn(async (_signal?: AbortSignal, request?: GetDiffRequest): Promise<Diff> => {
    if (!request?.path) return new Promise((resolve) => (release = resolve));
    return { ...REVIEW_DIFF, files: [REVIEW_DIFF.files![0]!] };
  });
  mountDiff(ACTIVE_DIFF_FILE, read);
  await loaded();
  fireEvent.click(screen.getByRole("button", { name: "Show changed files" }));
  const outside = screen.getByRole<HTMLButtonElement>("button", { name: "Outside action" });
  outside.focus();
  await act(async () => release(REVIEW_DIFF));
  expect(screen.getByRole("heading", { name: "2 changed files" })).toBeTruthy();
  expect(document.activeElement).toBe(outside);
});

it("returns keyboard focus to the accepted overview after a failed comparison without another read", async () => {
  const read = vi.fn(async () => REVIEW_DIFF);
  const { get } = mountDiff("", read);
  await loaded();
  read.mockRejectedValueOnce(new Error("No common ancestor"));
  await comparison("Branch changes");
  await screen.findByRole("alert");
  fireEvent.click(screen.getByRole("button", { name: /^DockResizer.tsx / }));
  fireEvent.keyDown(screen.getByRole("heading", { name: "DockResizer.tsx" }), { key: "Escape" });
  const row = screen.getByRole("button", { name: /^DockResizer.tsx / });
  expect(document.activeElement).toBe(row);
  expect(get).toHaveBeenCalledTimes(2);
});

it("keeps full read feedback in the scrolling content with a compact persistent failure cue", async () => {
  const read = vi.fn(async () => REVIEW_DIFF);
  mountDiff("", read);
  await loaded();
  read.mockRejectedValueOnce(new Error("No common ancestor"));
  await comparison("Branch changes");
  const error = await screen.findByRole("alert");
  fireEvent.click(screen.getByRole("button", { name: /^DockResizer.tsx / }));
  const controls = document.querySelector<HTMLElement>('[data-slot="panel-controls"]')!;
  expect(controls.contains(error)).toBe(false);
  expect(within(controls).getByText("Read failed")).toBeTruthy();
  expect(controls.textContent).toContain("Uncommitted changes");
  expect(controls.textContent).not.toContain("last successful read");
  expect(error.textContent).toContain("No common ancestor");
  expect(within(controls).getByRole("button", { name: "Refresh" })).toBeTruthy();
});

it("retries an explicit workspace read after it fails while retaining an accepted patch", async () => {
  let workspaceReads = 0;
  const read = vi.fn(async (_signal?: AbortSignal, request?: GetDiffRequest): Promise<Diff> => {
    if (request?.format === "raw") return { baseline: REVIEW_DIFF.baseline, patch: BOUNDED_PATCH };
    if (request?.path) return { baseline: REVIEW_DIFF.baseline, truncated: true };
    if (++workspaceReads === 1) throw new Error("Workspace read unavailable");
    return REVIEW_DIFF;
  });
  const { get } = mountDiff(ACTIVE_DIFF_FILE, read);
  await loaded();
  fireEvent.click(screen.getByRole("button", { name: "Read patch" }));
  await screen.findByRole("region", { name: `Patch for ${ACTIVE_DIFF_FILE}` });
  const at = document.querySelector("time")!.dateTime;
  fireEvent.click(screen.getByRole("button", { name: "Show changed files" }));
  await screen.findByRole("alert");
  expect(screen.getByRole("region", { name: `Patch for ${ACTIVE_DIFF_FILE}` })).toBeTruthy();
  expect(document.querySelector("time")!.dateTime).toBe(at);
  fireEvent.click(screen.getByRole("button", { name: "Show changed files" }));
  await screen.findByRole("heading", { name: "2 changed files" });
  expect(get).toHaveBeenLastCalledWith(
    { mode: "worktree", format: "rows" },
    expect.any(AbortSignal),
  );
  expect(workspaceReads).toBe(2);
  expect(document.activeElement).toBe(screen.getByRole("button", { name: /^DockResizer.tsx / }));
});

it("starts a general review at its overview and gives each navigation depth a distinct return", async () => {
  const { get, outsideEscape } = mountDiff();
  await loaded();
  expect(get).toHaveBeenCalledWith({ mode: "worktree", format: "rows" }, expect.any(AbortSignal));
  expect(screen.getByRole("heading", { name: "2 changed files" })).toBeTruthy();
  expect(screen.queryByRole("table")).toBeNull();
  expect(screen.queryByRole("combobox", { name: "Diff layout" })).toBeNull();
  expect(screen.queryByRole("button", { name: "Expand all" })).toBeNull();
  const scope = screen.getByRole("group", { name: "Comparison scope" });
  const filter = screen.getByRole("searchbox", { name: "Filter files" });
  expect(scope.compareDocumentPosition(filter) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

  fireEvent.click(screen.getByRole("button", { name: /^DockResizer.tsx / }));
  expect(screen.getAllByRole("table")).toHaveLength(1);
  expect(
    screen.getByRole("region", { name: `Scrollable diff for ${ACTIVE_DIFF_FILE}` }),
  ).toBeTruthy();
  expect(document.activeElement).toBe(screen.getByRole("heading", { name: "DockResizer.tsx" }));
  fireEvent.click(screen.getByRole("button", { name: "Next file" }));
  const heading = screen.getByRole<HTMLElement>("heading", { name: "session.go" });
  expect(document.activeElement).toBe(heading);
  expect(screen.getAllByRole("table")).toHaveLength(1);
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "Next file" }).disabled).toBe(true);
  fireEvent.keyDown(heading, { key: "Escape" });
  expect(outsideEscape).not.toHaveBeenCalled();
  expect(screen.queryByRole("table")).toBeNull();
  const current = screen.getByRole<HTMLElement>("button", { name: /^session.go / });
  expect(current.getAttribute("aria-current")).toBe("true");
  expect(document.activeElement).toBe(current);
  fireEvent.keyDown(current, { key: "Escape" });
  expect(outsideEscape).toHaveBeenCalledOnce();
});

it("retains the list filter and scroll and returns to the current file row", async () => {
  mountDiff();
  await loaded();
  const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Filter files" });
  fireEvent.change(search, { target: { value: "session" } });
  expect(
    within(screen.getByRole("region", { name: "Changed files" })).getAllByRole("button"),
  ).toHaveLength(1);
  const viewport = screen.getByTestId("viewport");
  viewport.scrollTop = 125;
  const row = screen.getByRole<HTMLElement>("button", { name: /^session.go / });
  row.focus();
  fireEvent.click(row);
  expect(viewport.scrollTop).toBe(0);
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "Previous file" }).disabled).toBe(
    true,
  );
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "Next file" }).disabled).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "Back to changed files" }));
  expect(screen.getByRole<HTMLInputElement>("searchbox", { name: "Filter files" }).value).toBe(
    "session",
  );
  expect(viewport.scrollTop).toBe(125);
  expect(document.activeElement).toBe(row);
});

it("reveals the beginning of a changed filter while preserving unchanged reading and refresh", async () => {
  const report = scaledReview(20);
  const read = vi.fn(async () => report);
  const { get } = mountDiff("", read);
  await loaded();
  const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Filter files" });
  const viewport = screen.getByTestId("viewport");
  search.focus();
  viewport.scrollTop = 1200;

  fireEvent.change(search, { target: { value: "module-01" } });
  expect(viewport.scrollTop).toBe(0);
  const rows = within(screen.getByRole("region", { name: "Changed files" })).getAllByRole("button");
  expect(rows).toHaveLength(10);
  expect(rows[0]!.getAttribute("data-diff-file")).toBe("src/generated/module-010.ts");
  expect(document.activeElement).toBe(search);
  expect(get).toHaveBeenCalledOnce();

  viewport.scrollTop = 300;
  fireEvent.change(search, { target: { value: "module-01" } });
  expect(viewport.scrollTop).toBe(300);
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await waitFor(() => expect(get).toHaveBeenCalledTimes(2));
  await loaded();
  expect(viewport.scrollTop).toBe(300);
  expect(search.value).toBe("module-01");
  expect(document.activeElement).toBe(search);
});

it.each(["clear button", "Escape"])(
  "reveals the unfiltered beginning through %s",
  async (action) => {
    mountDiff("", async () => scaledReview(20));
    await loaded();
    const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Filter files" });
    const viewport = screen.getByTestId("viewport");
    search.focus();
    fireEvent.change(search, { target: { value: "module-01" } });
    viewport.scrollTop = 600;

    if (action === "Escape") fireEvent.keyDown(search, { key: "Escape" });
    else fireEvent.click(screen.getByRole("button", { name: "Clear filter" }));

    expect(viewport.scrollTop).toBe(0);
    expect(search.value).toBe("");
    expect(document.activeElement).toBe(search);
    const rows = within(screen.getByRole("region", { name: "Changed files" })).getAllByRole(
      "button",
    );
    expect(rows).toHaveLength(20);
    expect(rows[0]!.getAttribute("data-diff-file")).toBe("src/generated/module-000.ts");
  },
);

it("opens a requested file directly while keeping complete paths in the list and reader", async () => {
  const { outsideEscape } = mountDiff(`/Users/visual/scope/${ACTIVE_DIFF_FILE}`);
  await loaded();
  expect(screen.queryByRole("searchbox", { name: "Filter files" })).toBeNull();
  expect(screen.getAllByRole("table")).toHaveLength(1);
  expect(
    screen.getByRole("region", { name: `Changes in ${ACTIVE_DIFF_FILE}` }).textContent,
  ).toContain(ACTIVE_DIFF_FILE);
  const heading = screen.getByRole<HTMLElement>("heading", { name: "DockResizer.tsx" });
  heading.focus();
  fireEvent.keyDown(heading, { key: "Escape" });
  expect(outsideEscape).toHaveBeenCalledOnce();
  expect(screen.getAllByRole("table")).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: "Show changed files" }));
  const row = screen.getByRole<HTMLElement>("button", { name: /^DockResizer.tsx / });
  expect(row.title).toBe(ACTIVE_DIFF_FILE);
  await waitFor(() => expect(document.activeElement).toBe(row));
});

it("shows each complete path in the scrolling reader while the filename stays in the controls", async () => {
  const name = `${"long-file-name-".repeat(12)}${"x".repeat(14)}.tsx`;
  const segments = Array.from({ length: 7 }, (_, index) => `level-${index}-${"d".repeat(39)}`);
  const directory = segments.join("/");
  const nested = `${directory}/${name}`;
  const diff: Diff = {
    baseline: { type: "head", commit: "c".repeat(40) },
    files: [
      {
        path: nested,
        status: "modified",
        added: 1,
        removed: 0,
        rows: [
          { type: "hunk", text: "@@ -1,0 +1,1 @@" },
          { type: "added", rightLine: 1, code: "export {};" },
        ],
      },
      { path: "logo.png", status: "added", binary: true, rows: [] },
      {
        path: "docs/new.md",
        previousPath: "docs/old.md",
        status: "renamed",
        added: 0,
        removed: 0,
        rows: [],
      },
    ],
  };
  mountDiff(nested, async () => diff);
  await loaded();

  function expectReader(path: string) {
    const controls = document.querySelector<HTMLElement>('[data-slot="panel-controls"]')!;
    const reader = screen.getByRole("region", { name: `Changes in ${path}` });
    expect(controls.contains(reader)).toBe(false);
    const visible = Array.from(reader.querySelectorAll("*")).filter(
      (element) =>
        element.textContent === path &&
        !element.closest(".sr-only, caption, [hidden], [aria-hidden='true']"),
    );
    expect(visible.length).toBeGreaterThan(0);
    const heading = within(controls).getByRole<HTMLElement>("heading", {
      name: path.slice(path.lastIndexOf("/") + 1),
    });
    expect(heading.textContent).toBe(path.slice(path.lastIndexOf("/") + 1));
    expect(within(controls).getByRole("button", { name: "Previous file" })).toBeTruthy();
    expect(within(controls).getByRole("button", { name: "Next file" })).toBeTruthy();
    return { controls, heading };
  }

  const long = expectReader(nested);
  expect(long.controls.textContent).not.toContain(directory);
  expect(within(long.controls).getByRole("combobox", { name: "Diff layout" })).toBeTruthy();

  fireEvent.click(screen.getByRole("button", { name: "Next file" }));
  expect(document.activeElement).toBe(expectReader("logo.png").heading);
  expect(
    screen.getByText("Binary content is not displayed. Line counts are unavailable."),
  ).toBeTruthy();

  fireEvent.click(screen.getByRole("button", { name: "Next file" }));
  expect(document.activeElement).toBe(expectReader("docs/new.md").heading);
  expect(screen.getByText(/Renamed from/).textContent).toContain("docs/old.md");
  expect(screen.getByText("No line changes. Only file metadata changed.")).toBeTruthy();

  fireEvent.click(screen.getByRole("button", { name: "Previous file" }));
  fireEvent.click(screen.getByRole("button", { name: "Previous file" }));
  expect(document.activeElement).toBe(expectReader(nested).heading);

  fireEvent.click(screen.getByRole("button", { name: "Show changed files" }));
  const row = screen.getByRole<HTMLElement>("button", {
    name: (label) => label.startsWith(`${name} `),
  });
  expect(row.textContent).toContain(name);
  expect(row.textContent).toContain(directory);
});

it("preserves query and focus when a refreshed file leaves the comparison", async () => {
  let result = REVIEW_DIFF;
  mountDiff(ACTIVE_DIFF_FILE, async () => result);
  await loaded();
  screen.getByRole<HTMLElement>("heading", { name: "DockResizer.tsx" }).focus();
  result = { ...REVIEW_DIFF, files: [REVIEW_DIFF.files![1]!] };
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await screen.findByText(/The requested file is not included in this read/);
  expect(screen.queryByRole("table")).toBeNull();
  const search = screen.getByRole("searchbox", { name: "Filter files" });
  expect(document.activeElement).toBe(search);
  expect(screen.getByRole("heading", { name: "1 changed file" })).toBeTruthy();
});

it("keeps partial, binary, rename and metadata-only states distinct from a clean comparison", async () => {
  const diff: Diff = {
    baseline: { type: "emptyTree" },
    truncated: true,
    files: [
      { path: "assets/图标.png", status: "added", binary: true, rows: [] },
      {
        path: "docs/new.md",
        previousPath: "docs/old.md",
        status: "renamed",
        added: 0,
        removed: 0,
        rows: [],
      },
    ],
  };
  mountDiff("missing.ts", async () => diff);
  await loaded();
  expect(screen.getByRole("note").textContent).toContain("Partial diff");
  expect(screen.queryByText(/No changes in this comparison/)).toBeNull();
  const binary = screen.getByRole("button", { name: /^图标.png / });
  expect(binary.querySelector('[data-slot="diff-stat"]')).toBeNull();
  fireEvent.click(binary);
  expect(
    screen.getByText("Binary content is not displayed. Line counts are unavailable."),
  ).toBeTruthy();
  expect(screen.queryByRole("table")).toBeNull();
  expect(screen.queryByRole("combobox", { name: "Diff layout" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Next file" }));
  expect(screen.getByText(/Renamed from/).textContent).toContain("docs/old.md");
  expect(screen.getByText("No line changes. Only file metadata changed.")).toBeTruthy();
  expect(screen.queryByRole("combobox", { name: "Diff layout" })).toBeNull();
});

const STATUS_DIFF: Diff = {
  baseline: { type: "head", commit: "d".repeat(40) },
  files: [
    {
      path: "src/created.ts",
      status: "added",
      added: 2,
      removed: 0,
      rows: [
        { type: "hunk", text: "@@ -0,0 +1,2 @@" },
        { type: "added", rightLine: 1, code: "export const created = true;" },
        { type: "added", rightLine: 2, code: "export default created;" },
      ],
    },
    {
      path: "src/changed.ts",
      status: "modified",
      added: 1,
      removed: 1,
      rows: [
        { type: "hunk", text: "@@ -1,1 +1,1 @@" },
        { type: "deleted", leftLine: 1, code: "export const value = 1;" },
        { type: "added", rightLine: 1, code: "export const value = 2;" },
      ],
    },
    {
      path: "src/removed.ts",
      status: "deleted",
      added: 0,
      removed: 1,
      rows: [
        { type: "hunk", text: "@@ -1,1 +0,0 @@" },
        { type: "deleted", leftLine: 1, code: "export {};" },
      ],
    },
    {
      path: "docs/new.md",
      previousPath: "docs/old.md",
      status: "renamed",
      added: 0,
      removed: 0,
      rows: [],
    },
    { path: "assets/draft.png", status: "untracked", binary: true, rows: [] },
  ],
};
const STATUS_COUNTS = ["+2 −0", "+1 −1", "+0 −1", "+0 −0", null];
const STATUS_LABELS: Record<string, Record<FileStatus, string>> = {
  en: {
    added: "Added",
    modified: "Modified",
    deleted: "Deleted",
    renamed: "Renamed",
    untracked: "Untracked",
  },
  zh: {
    added: "新增",
    modified: "已修改",
    deleted: "已删除",
    renamed: "已重命名",
    untracked: "未跟踪",
  },
  "zh-TW": {
    added: "新增",
    modified: "已修改",
    deleted: "已刪除",
    renamed: "已重新命名",
    untracked: "未追蹤",
  },
  ja: {
    added: "追加",
    modified: "変更",
    deleted: "削除",
    renamed: "名前変更",
    untracked: "未追跡",
  },
  ko: {
    added: "추가됨",
    modified: "수정됨",
    deleted: "삭제됨",
    renamed: "이름 변경됨",
    untracked: "추적되지 않음",
  },
  es: {
    added: "Añadido",
    modified: "Modificado",
    deleted: "Eliminado",
    renamed: "Renombrado",
    untracked: "Sin seguimiento",
  },
  fr: {
    added: "Ajouté",
    modified: "Modifié",
    deleted: "Supprimé",
    renamed: "Renommé",
    untracked: "Non suivi",
  },
  de: {
    added: "Hinzugefügt",
    modified: "Geändert",
    deleted: "Gelöscht",
    renamed: "Umbenannt",
    untracked: "Nicht verfolgt",
  },
};

it.each(Object.entries(STATUS_LABELS))(
  "labels every canonical file status in %s without changing file identity or focus",
  async (locale, labels) => {
    const { get } = mountDiff("", async () => STATUS_DIFF, locale);
    const rows = await waitFor(() => {
      const found = Array.from(document.querySelectorAll<HTMLElement>("[data-diff-file]"));
      expect(found).toHaveLength(STATUS_DIFF.files!.length);
      return found;
    });

    for (const [index, file] of STATUS_DIFF.files!.entries()) {
      const label = labels[file.status];
      const row = rows[index]!;
      expect(row.dataset.diffFile).toBe(file.path);
      expect(row.closest("[hidden]")).toBeNull();
      expect(within(row).getByText(label)).toBeTruthy();
      expect(within(row).queryByText(file.status)).toBeNull();
      expect(row.querySelector('[data-slot="diff-stat"]')?.textContent ?? null).toBe(
        STATUS_COUNTS[index],
      );

      fireEvent.click(row);
      const controls = document.querySelector<HTMLElement>('[data-slot="panel-controls"]')!;
      const heading = within(controls).getByRole<HTMLElement>("heading", {
        name: file.path.slice(file.path.lastIndexOf("/") + 1),
      });
      expect(document.activeElement).toBe(heading);
      const header = heading.parentElement!;
      expect(within(header).getByText(label)).toBeTruthy();
      expect(within(header).queryByText(file.status)).toBeNull();
      expect(header.querySelector('[data-slot="diff-stat"]')?.textContent ?? null).toBe(
        STATUS_COUNTS[index],
      );
      const previousPaths = screen
        .queryAllByText("docs/old.md")
        .filter((element) => !element.closest("[hidden]"));
      expect(previousPaths.map((element) => element.textContent)).toEqual(
        file.previousPath ? [file.previousPath] : [],
      );

      fireEvent.keyDown(heading, { key: "Escape" });
      expect(document.querySelector(`[data-diff-file="${file.path}"]`)).toBe(row);
      expect(row.closest("[hidden]")).toBeNull();
      expect(row.getAttribute("aria-current")).toBe("true");
      expect(document.activeElement).toBe(row);
    }

    expect(get).toHaveBeenCalledOnce();
    const read: Diff = await get.mock.results[0]!.value;
    expect(read.files!.map((file) => [file.status, file.previousPath])).toEqual([
      ["added", undefined],
      ["modified", undefined],
      ["deleted", undefined],
      ["renamed", "docs/old.md"],
      ["untracked", undefined],
    ]);
  },
);

it("retains the accepted comparison on failure, then retries the requested scope", async () => {
  const read = vi.fn(async () => REVIEW_DIFF);
  mountDiff("", read);
  await loaded();
  read.mockRejectedValueOnce(new Error("No common ancestor"));
  await comparison("Branch changes");
  await screen.findByRole("alert");
  expect(screen.getByRole("alert").textContent).toContain("Could not read branch changes");
  expect(screen.getByRole("status").textContent).toBe(
    "Showing uncommitted changes from the last successful read.",
  );
  expect(screen.getByRole("heading", { name: "2 changed files" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Comparison details" }));
  expect(screen.getByText(/Changes against HEAD/)).toBeTruthy();
  read.mockResolvedValueOnce({
    ...REVIEW_DIFF,
    baseline: { type: "mergeBase", commit: "b".repeat(40) },
  });
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await screen.findByText("Showing branch changes.");
  expect(screen.queryByRole("alert")).toBeNull();
  expect(screen.getByText(/Committed and uncommitted tracked changes/)).toBeTruthy();
});

it("retires superseded reads without replacing the newer comparison", async () => {
  const stale = Promise.withResolvers<Diff>();
  const read = vi
    .fn()
    .mockImplementationOnce(() => stale.promise)
    .mockResolvedValueOnce({
      ...REVIEW_DIFF,
      baseline: { type: "mergeBase", commit: "b".repeat(40) },
    });
  const { get } = mountDiff("", read);
  await waitFor(() => expect(get).toHaveBeenCalledOnce());
  const oldSignal = get.mock.calls[0]![1]!;
  await comparison("Branch changes");
  await screen.findByText("Showing branch changes.");
  expect(oldSignal.aborted).toBe(true);
  await act(async () => stale.resolve({ baseline: { type: "emptyTree" } }));
  expect(screen.getByRole("heading", { name: "2 changed files" })).toBeTruthy();
  expect(screen.getByText("Showing branch changes.")).toBeTruthy();
});

it("does not take focus from a later outside choice when a read completes", async () => {
  const result = Promise.withResolvers<Diff>();
  mountDiff(ACTIVE_DIFF_FILE, () => result.promise);
  const outside = screen.getByRole<HTMLElement>("button", { name: "Outside action" });
  outside.focus();
  await act(async () => result.resolve(REVIEW_DIFF));
  await loaded();
  expect(document.activeElement).toBe(outside);
});

it("keeps a pending Refresh focused and inert through completion, failure and retry", async () => {
  const pending = [
    Promise.withResolvers<Diff>(),
    Promise.withResolvers<Diff>(),
    Promise.withResolvers<Diff>(),
  ];
  const read = vi.fn<(signal?: AbortSignal) => Promise<Diff>>().mockResolvedValueOnce(REVIEW_DIFF);
  for (const deferred of pending) read.mockImplementationOnce(() => deferred.promise);
  const { get } = mountDiff(ACTIVE_DIFF_FILE, read);
  await loaded();
  const refresh = screen.getByRole<HTMLButtonElement>("button", { name: "Refresh" });

  async function startHeldRead(call: number) {
    refresh.focus();
    fireEvent.click(refresh);
    await waitFor(() => expect(get).toHaveBeenCalledTimes(call));
    const signal = get.mock.calls[call - 1]![1]!;
    expect(screen.getByText("Reading uncommitted changes…")).toBeTruthy();
    expect(document.activeElement).toBe(refresh);
    refresh.blur();
    refresh.focus();
    expect(document.activeElement).toBe(refresh);
    expect(refresh.matches(":disabled")).toBe(false);
    expect(refresh.getAttribute("aria-disabled")).toBe("true");
    fireEvent.click(refresh);
    fireEvent.keyDown(refresh, { key: "Enter" });
    fireEvent.keyDown(refresh, { key: " " });
    fireEvent.keyUp(refresh, { key: " " });
    expect(get).toHaveBeenCalledTimes(call);
    expect(signal.aborted).toBe(false);
  }

  await startHeldRead(2);
  await act(async () => pending[0]!.resolve(REVIEW_DIFF));
  await loaded();
  expect(refresh.getAttribute("aria-disabled")).not.toBe("true");
  expect(document.activeElement).toBe(refresh);
  expect(screen.getByRole("heading", { name: "DockResizer.tsx" })).toBeTruthy();

  await startHeldRead(3);
  await act(async () => pending[1]!.reject(new Error("Git is busy")));
  await screen.findByRole("alert");
  expect(document.activeElement).toBe(refresh);

  await startHeldRead(4);
  await act(async () => pending[2]!.resolve(REVIEW_DIFF));
  await loaded();
  expect(screen.queryByRole("alert")).toBeNull();
  expect(document.activeElement).toBe(refresh);
});

it("lets Escape from a pending Refresh return a list-opened file to its filtered row while the read continues", async () => {
  const held = Promise.withResolvers<Diff>();
  const read = vi
    .fn<(signal?: AbortSignal) => Promise<Diff>>()
    .mockResolvedValueOnce(REVIEW_DIFF)
    .mockImplementationOnce(() => held.promise);
  const { get, outsideEscape } = mountDiff("", read);
  await loaded();
  fireEvent.change(screen.getByRole("searchbox", { name: "Filter files" }), {
    target: { value: "session" },
  });
  const row = screen.getByRole<HTMLElement>("button", { name: /^session.go / });
  row.focus();
  fireEvent.click(row);
  expect(screen.getByRole("heading", { name: "session.go" })).toBeTruthy();
  const refresh = screen.getByRole<HTMLElement>("button", { name: "Refresh" });
  refresh.focus();
  fireEvent.click(refresh);
  await waitFor(() => expect(get).toHaveBeenCalledTimes(2));
  const signal = get.mock.calls[1]![1]!;
  expect(refresh.getAttribute("aria-disabled")).toBe("true");
  expect(document.activeElement).toBe(refresh);

  expect((await screen.findByRole("tooltip")).textContent).toBe("Refresh");
  fireEvent.keyDown(refresh, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("tooltip")).toBeNull());
  expect(screen.getAllByRole("table")).toHaveLength(1);
  expect(outsideEscape).not.toHaveBeenCalled();

  fireEvent.keyDown(refresh, { key: "Escape" });
  expect(outsideEscape).not.toHaveBeenCalled();
  expect(screen.queryByRole("table")).toBeNull();
  expect(screen.getByRole<HTMLInputElement>("searchbox", { name: "Filter files" }).value).toBe(
    "session",
  );
  expect(row.getAttribute("aria-current")).toBe("true");
  expect(document.activeElement).toBe(row);
  expect(signal.aborted).toBe(false);
  expect(screen.getByText("Reading uncommitted changes…")).toBeTruthy();

  await act(async () => held.resolve(REVIEW_DIFF));
  await loaded();
  expect(get).toHaveBeenCalledTimes(2);
  expect(screen.queryByRole("table")).toBeNull();
  expect(screen.getByRole<HTMLInputElement>("searchbox", { name: "Filter files" }).value).toBe(
    "session",
  );
  expect(screen.getByRole("button", { name: /^session.go / })).toBe(row);
  expect(row.getAttribute("aria-current")).toBe("true");
  expect(document.activeElement).toBe(row);
  expect(outsideEscape).not.toHaveBeenCalled();
});

it("leaves Escape from a pending Refresh on a directly opened file to the source return", async () => {
  const held = Promise.withResolvers<Diff>();
  const read = vi
    .fn<(signal?: AbortSignal) => Promise<Diff>>()
    .mockResolvedValueOnce(REVIEW_DIFF)
    .mockImplementationOnce(() => held.promise);
  const { get, outsideEscape } = mountDiff(ACTIVE_DIFF_FILE, read);
  await loaded();
  const refresh = screen.getByRole<HTMLElement>("button", { name: "Refresh" });
  refresh.focus();
  fireEvent.click(refresh);
  await waitFor(() => expect(get).toHaveBeenCalledTimes(2));
  const signal = get.mock.calls[1]![1]!;
  expect(refresh.getAttribute("aria-disabled")).toBe("true");

  expect((await screen.findByRole("tooltip")).textContent).toBe("Refresh");
  fireEvent.keyDown(refresh, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("tooltip")).toBeNull());
  expect(screen.getAllByRole("table")).toHaveLength(1);
  expect(outsideEscape).not.toHaveBeenCalled();

  fireEvent.keyDown(refresh, { key: "Escape" });
  expect(outsideEscape).toHaveBeenCalledOnce();
  expect(screen.getAllByRole("table")).toHaveLength(1);
  expect(screen.getByRole("button", { name: "Show changed files" })).toBeTruthy();
  expect(screen.queryByRole("searchbox", { name: "Filter files" })).toBeNull();
  expect(document.activeElement).toBe(refresh);
  expect(signal.aborted).toBe(false);

  await act(async () => held.resolve(REVIEW_DIFF));
  await loaded();
  expect(get).toHaveBeenCalledTimes(2);
  expect(screen.getAllByRole("table")).toHaveLength(1);
  expect(document.activeElement).toBe(refresh);
  expect(outsideEscape).toHaveBeenCalledOnce();
});

it("leaves a later outside focus choice when a pending refresh drops the selected file", async () => {
  const held = Promise.withResolvers<Diff>();
  const read = vi
    .fn<(signal?: AbortSignal) => Promise<Diff>>()
    .mockResolvedValueOnce(REVIEW_DIFF)
    .mockImplementationOnce(() => held.promise);
  const { get } = mountDiff(ACTIVE_DIFF_FILE, read);
  await loaded();
  const refresh = screen.getByRole<HTMLElement>("button", { name: "Refresh" });
  refresh.focus();
  fireEvent.click(refresh);
  await waitFor(() => expect(get).toHaveBeenCalledTimes(2));
  const outside = screen.getByRole<HTMLElement>("button", { name: "Outside action" });
  outside.focus();
  await act(async () => held.resolve({ ...REVIEW_DIFF, files: [REVIEW_DIFF.files![1]!] }));
  await screen.findByText(/The requested file is not included in this read/);
  expect(screen.queryByRole("table")).toBeNull();
  expect(document.activeElement).toBe(outside);
});

it("dismisses a layout popup before consuming a file-list return", async () => {
  const { outsideEscape } = mountDiff();
  await loaded();
  fireEvent.click(screen.getByRole("button", { name: /^DockResizer.tsx / }));
  const layout = screen.getByRole<HTMLElement>("combobox", { name: "Diff layout" });
  fireEvent.click(layout);
  const options = await screen.findByRole("listbox");
  fireEvent.keyDown(options, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  expect(screen.getByRole("heading", { name: "DockResizer.tsx" })).toBeTruthy();
  expect(screen.getAllByRole("table")).toHaveLength(1);
  expect(outsideEscape).not.toHaveBeenCalled();
});
