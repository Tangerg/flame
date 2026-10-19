import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { ComponentContext } from "@flame/plugin-sdk";
import { createFlameClient } from "@flame/runtime-contract/client";
import type { Diff } from "@flame/runtime-contract/wire";
import { WorkspaceDiff } from "../../../plugins/diff/src/component";
import { RUNTIME_AGENT_SESSION_SNAPSHOTS } from "./agentSessionSnapshots";
import { VISUAL_CAPABILITIES } from "./agentFixtureFacts";
import { createVisualRuntimeTransport } from "./createVisualRuntimeTransport";
import { ACTIVE_DIFF_FILE, REVIEW_DIFF } from "./diffSnapshot";

const releases: Array<() => Promise<void>> = [];
afterEach(async () => {
  cleanup();
  await Promise.all(releases.splice(0).map((release) => release()));
  vi.restoreAllMocks();
});

function mountDiff(
  path = "",
  readDiff: (signal?: AbortSignal) => Promise<Diff> = async () => REVIEW_DIFF,
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
    signal: owner.signal,
    track: (operation) => operation,
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
  const view = render(
    <>
      <button>Outside action</button>
      <div data-testid="viewport" style={{ overflowY: "auto", height: 400 }}>
        <WorkspaceDiff context={context} />
      </div>
    </>,
  );
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
  expect(document.activeElement).toBe(row);
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

it("retains the accepted comparison and selected reader on failure, then retries the requested scope", async () => {
  const read = vi.fn(async () => REVIEW_DIFF);
  mountDiff(ACTIVE_DIFF_FILE, read);
  await loaded();
  read.mockRejectedValueOnce(new Error("No common ancestor"));
  await comparison("Branch changes");
  await screen.findByRole("alert");
  expect(screen.getByRole("alert").textContent).toContain("Could not read branch changes");
  expect(screen.getByRole("status").textContent).toBe(
    "Showing uncommitted changes from the last successful read.",
  );
  expect(screen.getAllByRole("table")).toHaveLength(1);
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
