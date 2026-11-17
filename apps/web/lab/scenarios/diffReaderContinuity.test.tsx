import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, onTestFinished, vi, type Mock } from "vitest";
import type { ComponentContext } from "@flame/component-sdk";
import { createFlameClient } from "@flame/runtime-client";
import type { Diff as WireDiff } from "@flame/runtime-client/wire";
import type { FileDiffMetadata } from "@pierre/diffs";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { mockSelectLayout, mockSelectViewport } from "@/test/selectLayout";
import { unavailableComponentNotifications } from "@/test/unavailableNotifications";
import { WorkspaceDiff } from "../../../../plugins/diff/component/component";
import { RUNTIME_AGENT_SESSION_SNAPSHOTS } from "./agentSessionSnapshots";
import { VISUAL_CAPABILITIES } from "./agentFixtureFacts";
import { createVisualRuntimeTransport } from "./createVisualRuntimeTransport";
import { ACTIVE_DIFF_FILE, REVIEW_DIFF, visualRead } from "./diffSnapshot";

type Diff = Extract<WireDiff, { type: "visual" }>;
type Preparation = {
  ready: PromiseWithResolvers<void>;
  primeDiffHighlightCache: Mock<(file: FileDiffMetadata) => Promise<void>>;
  terminate: Mock<() => void>;
};
const workers = vi.hoisted(() => ({ pools: [] as Preparation[] }));

vi.mock("@pierre/diffs/worker", () => ({
  WorkerPoolManager: class {
    ready = Promise.withResolvers<void>();
    primeDiffHighlightCache = vi.fn((_file: FileDiffMetadata) => this.ready.promise);
    terminate = vi.fn<() => void>();

    constructor() {
      workers.pools.push(this);
    }
  },
}));

vi.mock("@pierre/diffs/react", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@pierre/diffs/react")>();
  const { use } = await import("react");
  return {
    ...actual,
    // Keep PreparedDiff's real readiness and provider; only Pierre's native leaf is replaced.
    FileDiff({ fileDiff }: { fileDiff: FileDiffMetadata }) {
      if (!use(actual.WorkerPoolContext)) throw new Error("expected the prepared worker owner");
      return (
        <pre>
          <code>{fileDiff.additionLines.join("")}</code>
        </pre>
      );
    },
  };
});

const FIRST_AT = "2026-11-17T03:00:01.000Z";
const NEXT_AT = "2026-11-17T03:05:02.000Z";
const ORIGINAL = ["const ready = true;", 'return "Keep 保留";'];
const CHANGED = ["const ready = true;", 'return "Reviewed 保留";'];
const OTHER_PATH = REVIEW_DIFF.visual.manifest[1]!.path;

function capturedRead(current = ORIGINAL, other = "const other = 1;", path?: string): Diff {
  return visualRead(
    [
      {
        change: { path: ACTIVE_DIFF_FILE, status: "modified", added: 2, removed: 2 },
        body: [
          "@@ -1,2 +1,2 @@",
          "-const ready = false;",
          '-return "Previous";',
          ...current.map((line) => `+${line}`),
        ],
      },
      {
        change: { path: OTHER_PATH, status: "modified", added: 1, removed: 1 },
        body: ["@@ -1 +1 @@", "-const other = 0;", `+${other}`],
      },
    ],
    { type: "head", commit: "a".repeat(40) },
    path,
  );
}

const FIRST_READ = capturedRead();
let testClient: TestClient;
const releases: Array<() => Promise<void>> = [];

beforeEach(() => {
  mockSelectLayout();
  const owned = createTestClient();
  testClient = owned;
  onTestFinished(() => owned.dispose());
  vi.setSystemTime(new Date(FIRST_AT));
});

afterEach(async () => {
  document.getSelection()?.removeAllRanges();
  cleanup();
  await act(async () => {
    for (const pool of workers.pools) pool.ready.resolve();
  });
  workers.pools.length = 0;
  await Promise.all(releases.splice(0).map((release) => release()));
  vi.restoreAllMocks();
  vi.useRealTimers();
});

function mountDiff(read: () => Promise<Diff>) {
  const owner = new AbortController();
  const runtime = createFlameClient(
    createVisualRuntimeTransport(
      structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
      VISUAL_CAPABILITIES,
      read,
    ),
    { capabilities: () => VISUAL_CAPABILITIES },
  );
  const context: ComponentContext = {
    component: { installationId: "diff-review", digest: "a".repeat(64), componentId: "diff" },
    runtime,
    workspace: { ref: { path: "/Users/visual/scope" }, focus: { path: "", revision: 1n } },
    telemetry: { read: () => ({ spans: [], metrics: [], logs: [] }), clear() {} },
    locale: "en",
    signal: owner.signal,
    track: (operation) => operation(owner.signal),
    saveFile: async () => "requested",
    notifications: unavailableComponentNotifications,
    subscribeRootRunSettlements() {
      throw new Error("this fixture must not observe run settlements");
    },
    registerCommand() {
      throw new Error("this fixture must not register commands");
    },
    compose: () => null,
  };
  releases.push(async () => {
    owner.abort();
    await runtime.close();
  });
  const view = testClient.render(
    <div data-testid="viewport" style={{ overflowY: "auto", height: 400 }}>
      <WorkspaceDiff context={context} />
    </div>,
  );
  mockSelectViewport(view.getByTestId("viewport"));
}

async function preparedReader(lines = ORIGINAL) {
  fireEvent.click(await screen.findByRole("button", { name: /^DockResizer\.tsx / }));
  const reader = screen.getByRole("region", { name: `Scrollable diff for ${ACTIVE_DIFF_FILE}` });
  expect(within(reader).getByRole("status").textContent).toBe("Loading diff…");
  await waitFor(() => expect(workers.pools).toHaveLength(1));
  const pool = workers.pools[0]!;
  await act(async () => pool.ready.resolve());
  const code = reader.querySelector("code")!;
  expect(code.textContent).toBe(`${lines.join("\n")}\n`);
  const text = code.firstChild as Text;
  const range = document.createRange();
  range.setStart(text, 6);
  range.setEnd(text, 11);
  const selection = document.getSelection()!;
  selection.removeAllRanges();
  selection.addRange(range);
  expect(range.toString()).toBe("ready");
  return { reader, code, text, range, pool };
}

function refresh() {
  vi.setSystemTime(new Date(NEXT_AT));
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
}

async function acceptedNextRead() {
  await waitFor(() => expect(document.querySelector("time")!.dateTime).toBe(NEXT_AT));
}

it("keeps prepared source and its native range when the same patch is refreshed", async () => {
  const next: Diff = { ...FIRST_READ, baseline: { type: "head", commit: "b".repeat(40) } };
  const read = vi.fn().mockResolvedValueOnce(FIRST_READ).mockResolvedValueOnce(next);
  mountDiff(read);
  const original = await preparedReader();

  refresh();
  await acceptedNextRead();

  expect(screen.getByText("Baseline: HEAD · bbbbbbbb")).toBeTruthy();
  expect(screen.getByRole("region", { name: `Scrollable diff for ${ACTIVE_DIFF_FILE}` })).toBe(
    original.reader,
  );
  expect(original.reader.querySelector("code")).toBe(original.code);
  expect(original.code.firstChild).toBe(original.text);
  expect(original.text.isConnected).toBe(true);
  expect(original.range.toString()).toBe("ready");
  expect(within(original.reader).queryByText("Loading diff…")).toBeNull();
  expect(workers.pools).toHaveLength(1);
  expect(original.pool.primeDiffHighlightCache).toHaveBeenCalledOnce();
  expect(original.pool.terminate).not.toHaveBeenCalled();
  expect(read).toHaveBeenCalledTimes(2);
});

it("keeps the current prepared file while accepting and copying another file's changed patch", async () => {
  const next = capturedRead(ORIGINAL, "const other = 2;");
  const read = vi.fn().mockResolvedValueOnce(FIRST_READ).mockResolvedValueOnce(next);
  const writeText = vi.fn().mockResolvedValue(undefined);
  const descriptor = Object.getOwnPropertyDescriptor(navigator, "clipboard");
  Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
  onTestFinished(() => {
    if (descriptor) Object.defineProperty(navigator, "clipboard", descriptor);
    else Reflect.deleteProperty(navigator, "clipboard");
  });
  mountDiff(read);
  const original = await preparedReader();

  refresh();
  await acceptedNextRead();

  expect(original.reader.querySelector("code")).toBe(original.code);
  expect(original.code.firstChild).toBe(original.text);
  expect(original.range.toString()).toBe("ready");
  expect(within(original.reader).queryByText("Loading diff…")).toBeNull();
  expect(workers.pools).toHaveLength(1);
  expect(original.pool.primeDiffHighlightCache).toHaveBeenCalledOnce();
  expect(original.pool.terminate).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Copy included patch" }));
  await waitFor(() => expect(writeText).toHaveBeenCalledExactlyOnceWith(next.visual.patch));
  expect(next.visual.patch).toContain("+const other = 2;");
  expect(read).toHaveBeenCalledTimes(2);
});

it("retires preparation and publishes the exact new source when the current file changes", async () => {
  const next = capturedRead(CHANGED);
  const read = vi.fn().mockResolvedValueOnce(FIRST_READ).mockResolvedValueOnce(next);
  mountDiff(read);
  const original = await preparedReader();

  refresh();
  await acceptedNextRead();
  await waitFor(() => expect(workers.pools).toHaveLength(2));

  expect(original.pool.terminate).toHaveBeenCalledOnce();
  expect(original.text.isConnected).toBe(false);
  expect(within(original.reader).getByRole("status").textContent).toBe("Loading diff…");
  const current = workers.pools[1]!;
  await act(async () => current.ready.resolve());
  const code = original.reader.querySelector("code")!;
  expect(code).not.toBe(original.code);
  expect(code.textContent).toBe(`${CHANGED.join("\n")}\n`);
  expect(within(original.reader).queryByText("Loading diff…")).toBeNull();
  expect(current.primeDiffHighlightCache).toHaveBeenCalledOnce();
  expect(current.terminate).not.toHaveBeenCalled();
  expect(read).toHaveBeenCalledTimes(2);
});

it("retains prepared evidence and accepted facts when a later patch disagrees with its manifest", async () => {
  const malformed: Diff = {
    ...FIRST_READ,
    baseline: { type: "head", commit: "b".repeat(40) },
    visual: { ...FIRST_READ.visual, manifest: FIRST_READ.visual.manifest.slice(0, 1) },
  };
  const read = vi.fn().mockResolvedValueOnce(FIRST_READ).mockResolvedValueOnce(malformed);
  mountDiff(read);
  const original = await preparedReader();

  refresh();
  const error = await screen.findByRole("alert");

  expect(error.textContent).toContain("workspace diff: patch and manifest cover different files");
  expect(document.querySelector("time")!.dateTime).toBe(FIRST_AT);
  expect(screen.getByText("Baseline: HEAD · aaaaaaaa")).toBeTruthy();
  expect(
    screen.getByText("Showing uncommitted changes from the last successful read."),
  ).toBeTruthy();
  expect(original.reader.querySelector("code")).toBe(original.code);
  expect(original.text.isConnected).toBe(true);
  expect(original.range.toString()).toBe("ready");
  expect(workers.pools).toHaveLength(1);
  expect(original.pool.primeDiffHighlightCache).toHaveBeenCalledOnce();
  expect(original.pool.terminate).not.toHaveBeenCalled();
  expect(read).toHaveBeenCalledTimes(2);
});

it.each([
  { kind: "carriage return", separator: "\r" },
  { kind: "line separator", separator: "\u2028" },
  { kind: "paragraph separator", separator: "\u2029" },
])("keeps a literal $kind before diff --git inside one file's source", async ({ separator }) => {
  const lines = ["const ready = true;", `const proof = "Keep${separator}diff --git literal";`];
  const read = vi.fn().mockResolvedValue(capturedRead(lines, undefined, ACTIVE_DIFF_FILE));
  mountDiff(read);

  await screen.findByRole("heading", { name: "1 changed file" });
  const original = await preparedReader(lines);

  expect(original.text.isConnected).toBe(true);
  expect(screen.queryByRole("alert")).toBeNull();
  expect(workers.pools).toHaveLength(1);
  expect(original.pool.primeDiffHighlightCache).toHaveBeenCalledOnce();
  expect(read).toHaveBeenCalledOnce();
});
