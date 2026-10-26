import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Activity } from "react";
import { type TranscriptRow } from "@/features/agent/public/conversation";
import { type AgentRunView } from "@/features/agent/public/sessionView";

import {
  openWorkspaceSubagentRun,
  returnFromWorkspaceInspection,
  useWorkspaceInspection,
} from "@/features/workspace/public/navigation";
import { ContentVisibility } from "@/ui/index";
import { PluginBoundary } from "@/extensions/host/PluginBoundary";
import { SubagentsPanel } from "@/features/chat/message/ui/SubagentsPanel";
const material = vi.hoisted(() => ({
  rows: [] as readonly TranscriptRow[],
  available: true,
  cancel: vi.fn(),
}));

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

prepareTestClient(() => {
  Object.assign(testClient.agent.access, { cancelSessionRun: material.cancel });
});

vi.mock("@/features/agent/public/conversation", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agent/public/conversation")>()),
  useActiveConversationRows: () => material.rows,
}));

vi.mock("@/features/connection/public/serviceStatus", () => ({
  useRuntimeCommandsAvailable: () => material.available,
}));

vi.mock("@/features/chat/message/ui/MessageTurn", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/features/chat/message/ui/MessageTurn")>();
  return {
    MessageTurn: (props: Parameters<typeof actual.MessageTurn>[0]) => {
      if (props.row.message.id === "testClient.render-boom")
        throw new Error("testClient.render-boom");
      return <actual.MessageTurn {...props} />;
    },
  };
});

const child: AgentRunView = {
  id: "child",
  sessionId: "session",
  parentRunId: "root",
  rootRunId: "root",
  spawnedByItemId: "delegate",
  status: "running",
  activeSegmentId: "segment",
  outcome: null,
  progress: { step: 1, activity: "Inspecting axios" },
  contextTokens: null,
  metrics: {
    steps: 1,
    activeDurationMillis: 10,
    usage: { inputTokens: 1, outputTokens: 1, cacheReadTokens: 0 },
  },
  createdAt: "2026-01-01T00:00:00Z",
  finishedAt: null,
};

function rows(run = child, text = "Child streamed output"): TranscriptRow[] {
  return [
    {
      message: { id: "parent", role: "assistant", runId: "root", blocks: [] },
      runOwner: { kind: "owned", runId: "root", status: "running" },
      facts: {
        toolCalls: {},
        delegatedRuns: {
          delegate: [
            {
              run,
              messages: [
                {
                  id: "child-output",
                  role: "assistant",
                  runId: run.id,
                  phase: "commentary",
                  blocks: [{ kind: "text", text, status: "complete" }],
                },
              ],
            },
          ],
          nested: [
            {
              run: {
                ...child,
                id: "nested",
                parentRunId: "child",
                spawnedByItemId: "nested",
                status: "finished",
                activeSegmentId: null,
                outcome: { type: "completed" },
              },
              messages: [],
            },
          ],
        },
        awaiting: new Map<string, string>(),
      },
    },
  ];
}

it("contains a failed child turn without losing the selected identity or healthy siblings", async () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  material.rows = rows();
  material.rows[0]!.facts.delegatedRuns.delegate![0]!.messages.unshift({
    id: "testClient.render-boom",
    role: "assistant",
    runId: "child",
    blocks: [],
  });
  testClient.navigation.go({ session: "session" });
  openWorkspaceSubagentRun(testClient.workspace.access, "child");
  const { container } = testClient.render(
    <PluginBoundary plugin="workspace:subagents" label="Subagents">
      <SubagentsPanel />
    </PluginBoundary>,
  );
  await act(async () => vi.dynamicImportSettled());

  expect(screen.getByRole("button", { name: "All subagents" })).toBeTruthy();
  expect(within(container.querySelector("header")!).getByText("Running")).toBeTruthy();
  expect(screen.getByRole("region", { name: "Sub-agent" })).toBeTruthy();
  expect(screen.getByText("Child streamed output")).toBeTruthy();
  expect(screen.getAllByRole("alert")).toHaveLength(1);
  expect(screen.queryByText(/failed to testClient.render/i)).toBeNull();
});

it("projects live child material, returns to the list, and never carries it into another session", async () => {
  material.rows = rows();
  testClient.navigation.go({ session: "session" });
  openWorkspaceSubagentRun(testClient.workspace.access, "child");
  const { rerender, container } = testClient.render(<SubagentsPanel />);
  await act(async () => vi.dynamicImportSettled());
  expect(screen.getByText("Child streamed output")).toBeTruthy();
  expect(within(container.querySelector("header")!).getByText("Running")).toBeTruthy();

  material.rows = rows(
    { ...child, status: "finished", activeSegmentId: null, outcome: { type: "completed" } },
    "Child final answer",
  );
  rerender(<SubagentsPanel />);
  expect(screen.queryByText("Child streamed output")).toBeNull();
  expect(screen.getByText("Child final answer")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Cancel this run" })).toBeNull();

  fireEvent.click(screen.getByRole("button", { name: "All subagents" }));
  expect(testClient.navigation.get()).toMatchObject({
    session: "session",
    dock: "subagents",
    subagent: null,
  });
  expect(screen.getByText("Finished · 2")).toBeTruthy();
  expect(screen.queryByText("Child final answer")).toBeNull();
  const list = screen.getByRole("region", { name: "Subagents" });
  fireEvent.click(within(list).getAllByRole("button", { name: /Sub-agent/ })[1]!);
  expect(testClient.navigation.get().subagent).toBe("nested");
  expect(screen.getByText("No narrative material yet")).toBeTruthy();

  material.rows = [];
  act(() => testClient.navigation.go({ session: "other" }));
  rerender(<SubagentsPanel />);
  expect(screen.getByText("No subagents yet")).toBeTruthy();
  expect(testClient.navigation.get().subagent).toBeNull();
});

it("marks a delegated user message as the same speech bubble the transcript draws", async () => {
  material.rows = rows();
  material.rows[0]!.facts.delegatedRuns.delegate![0]!.messages.unshift({
    id: "child-task",
    role: "user",
    runId: "child",
    steer: { runId: "child", delivery: "pending" },
    blocks: [{ kind: "text", text: "Audit axios cancellation", status: "complete" }],
  });
  testClient.navigation.go({ session: "session" });
  openWorkspaceSubagentRun(testClient.workspace.access, "child");
  const { container } = testClient.render(<SubagentsPanel />);
  await act(async () => vi.dynamicImportSettled());

  const bubbles = container.querySelectorAll("[data-user-message-bubble]");
  expect(bubbles).toHaveLength(1);
  expect(bubbles[0]!.textContent).toContain("Audit axios cancellation");
  expect(screen.getByRole("heading", { name: "You" })).toBeTruthy();
  expect(screen.getByRole("heading", { name: "Assistant" })).toBeTruthy();
  expect(screen.getByText("Entered model context")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Retry original submission" })).toBeNull();
  expect(
    screen.getByText("Child streamed output").closest("[data-user-message-bubble]"),
  ).toBeNull();
});

it("keeps list position and returns keyboard focus to the opened task", async () => {
  material.rows = rows();
  testClient.navigation.go({ session: "session" });
  openWorkspaceSubagentRun(testClient.workspace.access, null);
  testClient.render(<SubagentsPanel />);
  const group = screen.getByRole("group", { name: "Subagents" });
  const scroller = group.parentElement!;
  scroller.scrollTop = 240;
  const opener = within(group).getAllByRole("button", { name: /Sub-agent/ })[0]!;
  act(() => opener.focus());
  fireEvent.click(opener);
  const back = screen.getByRole("button", { name: "All subagents" });
  expect(document.activeElement).toBe(back);
  fireEvent.click(back);
  expect(screen.getByRole("group", { name: "Subagents" }).parentElement).toBe(scroller);
  expect(scroller.scrollTop).toBe(240);
  expect(document.activeElement).toBe(opener);
  await act(async () => vi.dynamicImportSettled());
});

it("retains only the parent readers on the explicit inspection path", async () => {
  material.rows = rows();
  const facts = material.rows[0]!.facts;
  facts.toolCalls.nested = {
    id: "nested",
    runId: "child",
    name: "delegate_task",
    fn: "Review request headers",
    args: '{"summary":"Review request headers"}',
    status: "ok",
  };
  facts.delegatedRuns.delegate![0]!.messages[0]!.blocks.push({
    kind: "tool",
    toolCallId: "nested",
  });
  testClient.navigation.go({ session: "session" });
  const origin = document.createElement("button");
  document.body.append(origin);
  openWorkspaceSubagentRun(testClient.workspace.access, "child", origin);
  const { unmount } = testClient.render(
    <div data-dock-view-id="subagents">
      <SubagentsPanel />
    </div>,
  );
  const inspection = testClient.renderHook(useWorkspaceInspection);
  await act(async () => vi.dynamicImportSettled());
  const parent = screen.getByText("Child streamed output").closest('[role="region"]')!;
  const viewport = parent.parentElement!;
  viewport.scrollTop = 240;
  const nested = within(parent as HTMLElement).getByRole("button", { name: /Sub-agent/ });
  fireEvent.click(nested);
  expect(testClient.navigation.get().subagent).toBe("nested");
  expect(screen.queryByRole("region", { name: "Sub-agent" })).toBeNull();
  expect(parent.isConnected).toBe(true);
  act(() => screen.getByRole("region", { name: "Review request headers" }).focus());
  act(() => returnFromWorkspaceInspection(testClient.workspace.access, inspection.result.current!));
  expect(testClient.navigation.get().subagent).toBe("child");
  expect(screen.getByRole("region", { name: "Sub-agent" })).toBe(parent);
  expect(parent.parentElement).toBe(viewport);
  expect(viewport.scrollTop).toBe(240);
  expect(document.activeElement).toBe(document.body);
  fireEvent.click(within(parent as HTMLElement).getByRole("button", { name: /Sub-agent/ }));
  fireEvent.click(screen.getByRole("button", { name: "All subagents" }));
  expect(testClient.navigation.get().subagent).toBeNull();
  expect(parent.isConnected).toBe(false);
  expect(inspection.result.current?.origin).toBe(origin);
  expect(document.activeElement).toBe(
    within(screen.getByRole("group", { name: "Subagents" })).getByRole("button", {
      name: /Review request headers/,
    }),
  );
  unmount();
  origin.remove();
});

it("retains the parent reader through a nested inspection in explicit main", async () => {
  material.rows = rows();
  const facts = material.rows[0]!.facts;
  facts.toolCalls.nested = {
    id: "nested",
    runId: "child",
    name: "delegate_task",
    fn: "Review request headers",
    args: '{"summary":"Review request headers"}',
    status: "ok",
  };
  facts.delegatedRuns.delegate![0]!.messages[0]!.blocks.push({
    kind: "tool",
    toolCallId: "nested",
  });
  testClient.navigation.go({ session: "session", view: null, dock: null });
  const origin = document.createElement("button");
  document.body.append(origin);
  openWorkspaceSubagentRun(testClient.workspace.access, "child", origin);
  const outer = testClient.renderHook(useWorkspaceInspection);
  const source = outer.result.current;
  act(() => testClient.navigation.go({ view: "subagents" }));
  const { unmount } = testClient.render(<SubagentsPanel />);
  const inspection = testClient.renderHook(useWorkspaceInspection);
  await act(async () => vi.dynamicImportSettled());
  const parent = screen.getByText("Child streamed output").closest('[role="region"]')!;
  const viewport = parent.parentElement!;
  viewport.scrollTop = 2554;
  const nested = within(parent as HTMLElement).getByRole("button", { name: /Sub-agent/ });
  act(() => nested.focus());
  fireEvent.click(nested);
  expect(testClient.navigation.get()).toMatchObject({ view: "subagents", subagent: "nested" });
  expect(parent.isConnected).toBe(true);
  expect(inspection.result.current?.origin).toBe(nested);
  expect(inspection.result.current?.returnViewId).toBe("subagents");
  expect(inspection.result.current?.returnSubagentRunId).toBe("child");
  expect(inspection.result.current?.previous).toBe(source);
  act(() => returnFromWorkspaceInspection(testClient.workspace.access, inspection.result.current!));
  expect(testClient.navigation.get()).toMatchObject({ view: "subagents", subagent: "child" });
  expect(screen.getByRole("region", { name: "Sub-agent" })).toBe(parent);
  expect(viewport.scrollTop).toBe(2554);
  expect(inspection.result.current).toBe(source);
  fireEvent.click(within(parent as HTMLElement).getByRole("button", { name: /Sub-agent/ }));
  fireEvent.click(screen.getByRole("button", { name: "All subagents" }));
  expect(testClient.navigation.get()).toMatchObject({ view: "subagents", subagent: null });
  expect(parent.isConnected).toBe(false);
  expect(inspection.result.current).toBe(source);
  const row = within(screen.getByRole("group", { name: "Subagents" })).getByRole("button", {
    name: /Review request headers/,
  });
  expect(row.closest<HTMLElement>('[data-slot="delegated-run-link"]')?.dataset.runId).toBe(
    "nested",
  );
  expect(document.activeElement).toBe(row);
  unmount();
  origin.remove();
});

it("measures a retained reader only while both its host and its selection show it", async () => {
  const live = new Set<Set<Element>>();
  vi.stubGlobal(
    "ResizeObserver",
    class {
      readonly targets = new Set<Element>();
      observe(target: Element) {
        this.targets.add(target);
        live.add(this.targets);
      }
      unobserve(target: Element) {
        this.targets.delete(target);
      }
      disconnect() {
        live.delete(this.targets);
      }
    },
  );
  const measured = (element: Element) => [...live].some((targets) => targets.has(element));
  material.rows = rows();
  const facts = material.rows[0]!.facts;
  facts.toolCalls.nested = {
    id: "nested",
    runId: "child",
    name: "delegate_task",
    fn: "Review request headers",
    args: '{"summary":"Review request headers"}',
    status: "ok",
  };
  facts.delegatedRuns.delegate![0]!.messages[0]!.blocks.push({
    kind: "tool",
    toolCallId: "nested",
  });
  testClient.navigation.go({ session: "session" });
  const origin = document.createElement("button");
  document.body.append(origin);
  openWorkspaceSubagentRun(testClient.workspace.access, "child", origin);
  const view = (visible: boolean) => (
    <div data-dock-view-id="subagents">
      <ContentVisibility visible={visible}>
        <SubagentsPanel />
      </ContentVisibility>
    </div>
  );
  const { rerender, unmount } = testClient.render(view(true));
  const inspection = testClient.renderHook(useWorkspaceInspection);
  await act(async () => vi.dynamicImportSettled());
  const parent = screen.getByRole("region", { name: "Sub-agent" });
  expect(measured(parent)).toBe(true);

  rerender(view(false));
  expect(parent.isConnected).toBe(true);
  expect(measured(parent)).toBe(false);
  rerender(view(true));
  expect(measured(parent)).toBe(true);

  fireEvent.click(within(parent).getByRole("button", { name: /Sub-agent/ }));
  const nested = screen.getByRole("region", { name: "Review request headers" });
  expect(measured(parent)).toBe(false);
  expect(measured(nested)).toBe(true);

  rerender(view(false));
  expect(measured(nested)).toBe(false);
  expect(measured(parent)).toBe(false);
  rerender(view(true));
  expect(measured(nested)).toBe(true);

  act(() => nested.focus());
  act(() => returnFromWorkspaceInspection(testClient.workspace.access, inspection.result.current!));
  expect(testClient.navigation.get().subagent).toBe("child");
  expect(screen.getByRole("region", { name: "Sub-agent" })).toBe(parent);
  expect(measured(parent)).toBe(true);
  unmount();
  origin.remove();
});

it("does not initiate a new focus handoff when the same selected reader resumes", async () => {
  material.rows = rows();
  testClient.navigation.go({ session: "session" });
  openWorkspaceSubagentRun(testClient.workspace.access, "child");
  const { rerender } = testClient.render(
    <Activity mode="visible">
      <SubagentsPanel />
    </Activity>,
  );
  await act(async () => vi.dynamicImportSettled());
  const back = screen.getByRole("button", { name: "All subagents" });
  expect(document.activeElement).toBe(back);
  rerender(
    <Activity mode="hidden">
      <SubagentsPanel />
    </Activity>,
  );
  act(() => back.blur());
  expect(document.activeElement).toBe(document.body);

  rerender(
    <Activity mode="visible">
      <SubagentsPanel />
    </Activity>,
  );
  expect(screen.getByRole("button", { name: "All subagents" })).toBe(back);
  expect(document.activeElement).toBe(document.body);
});

it("folds a long child input with the main message interaction", async () => {
  vi.spyOn(HTMLElement.prototype, "scrollHeight", "get").mockReturnValue(900);
  vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockReturnValue(160);
  material.rows = rows();
  material.rows[0]!.facts.delegatedRuns.delegate![0]!.messages.unshift({
    id: "child-task",
    role: "user",
    runId: "child",
    blocks: [{ kind: "text", text: "Inspect the complete module graph", status: "complete" }],
  });
  testClient.navigation.go({ session: "session" });
  openWorkspaceSubagentRun(testClient.workspace.access, "child");
  testClient.render(<SubagentsPanel />);
  await act(async () => vi.dynamicImportSettled());
  fireEvent.click(screen.getByRole("button", { name: "Show full message" }));
  expect(screen.getByRole("button", { name: "Show less" }).getAttribute("aria-expanded")).toBe(
    "true",
  );
});

it("keeps the selected run cancellation visible when no narrative was produced", () => {
  material.rows = rows({
    ...child,
    status: "finished",
    activeSegmentId: null,
    outcome: { type: "canceled", detail: "The user canceled the review." },
  });
  material.rows[0]!.facts.delegatedRuns.delegate![0]!.messages = [];
  testClient.navigation.go({ session: "session" });
  openWorkspaceSubagentRun(testClient.workspace.access, "child");
  testClient.render(<SubagentsPanel />);
  expect(
    within(screen.getByRole("region", { name: "Sub-agent" })).getByText(
      "The user canceled the review.",
    ),
  ).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Cancel this run" })).toBeNull();
});

it("explains unconfirmed effects for the selected terminal child without replacing its outcome", () => {
  material.rows = rows({
    ...child,
    status: "finished",
    activeSegmentId: null,
    finishedAt: "2026-01-01T00:00:01Z",
    outcome: {
      type: "canceled",
      unresolvedEffects: [
        { processId: "child-process", effectId: "child-effect", cause: "unknown" },
      ],
    },
  });
  material.rows[0]!.facts.delegatedRuns.delegate![0]!.messages = [];
  testClient.navigation.go({ session: "session" });
  openWorkspaceSubagentRun(testClient.workspace.access, "child");
  const { rerender, container } = testClient.render(<SubagentsPanel />);
  expect(screen.getByText(/This run has ended/)).toBeTruthy();
  expect(within(container.querySelector("header")!).getByText("Canceled")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Cancel this run" })).toBeNull();
  const operation = screen.getByRole("button", {
    name: /^Unconfirmed operation 1\s*child-effect$/,
  });
  fireEvent.click(operation);
  const evidence = within(document.getElementById(operation.getAttribute("aria-controls")!)!);
  expect(evidence.getByText("child-effect")).toBeTruthy();
  expect(evidence.getByText("child-process")).toBeTruthy();
  expect(material.rows[0]!.facts.delegatedRuns.delegate![0]!.run.outcome?.type).toBe("canceled");

  act(() => openWorkspaceSubagentRun(testClient.workspace.access, "nested"));
  rerender(<SubagentsPanel />);
  expect(screen.queryByText(/This run has ended/)).toBeNull();
  expect(screen.queryByRole("button", { name: /^Unconfirmed operation/ })).toBeNull();
  expect(screen.queryByText("child-effect")).toBeNull();
});

it("cancels the currently selected child and disables mutations while disconnected", () => {
  material.rows = rows();
  material.available = true;
  testClient.navigation.go({ session: "session" });
  openWorkspaceSubagentRun(testClient.workspace.access, "child");
  const { rerender } = testClient.render(<SubagentsPanel />);
  fireEvent.click(screen.getByRole("button", { name: "Cancel this run" }));
  expect(material.cancel).toHaveBeenLastCalledWith({
    sessionId: "session",
    runId: "child",
  });
  const nested = material.rows[0]!.facts.delegatedRuns.nested![0]!;
  nested.run = { ...child, id: "nested", parentRunId: "child", spawnedByItemId: "nested" };
  act(() => openWorkspaceSubagentRun(testClient.workspace.access, "nested"));
  fireEvent.click(screen.getByRole("button", { name: "Cancel this run" }));
  expect(material.cancel).toHaveBeenLastCalledWith({
    sessionId: "session",
    runId: "nested",
  });
  material.available = false;
  rerender(<SubagentsPanel />);
  expect(screen.getByRole("button", { name: "Cancel this run" }).hasAttribute("disabled")).toBe(
    true,
  );
  material.available = true;
});

it("names delegated tasks from their parent tool call in both list and detail", () => {
  material.rows = rows();
  material.rows[0]!.facts.toolCalls.delegate = {
    id: "delegate",
    runId: "root",
    name: "delegate_task",
    fn: "Audit axios cancellation",
    args: '{"summary":"Audit axios cancellation"}',
    status: "ok",
  };
  material.rows[0]!.facts.toolCalls.nested = {
    id: "nested",
    runId: "child",
    name: "delegate_task",
    fn: "Review request headers",
    args: '{"summary":"Review request headers"}',
    status: "ok",
  };
  testClient.navigation.go({ session: "session" });
  openWorkspaceSubagentRun(testClient.workspace.access, null);
  testClient.render(<SubagentsPanel />);
  expect(screen.getByRole("button", { name: /Review request headers/ })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: /Audit axios cancellation/ }));
  expect(screen.getByRole("region", { name: "Audit axios cancellation" })).toBeTruthy();
  expect(testClient.navigation.get().subagent).toBe("child");
});

it("counts a subagent waiting on the person as active, not running", () => {
  material.rows = rows({ ...child, status: "waiting" });
  testClient.navigation.go({ session: "session" });
  openWorkspaceSubagentRun(testClient.workspace.access, null);
  testClient.render(<SubagentsPanel />);
  expect(screen.getByText("2 subagents")).toBeTruthy();
  expect(screen.queryByText(/running/)).toBeNull();
  expect(screen.getByText("Active · 1")).toBeTruthy();
});

it("preserves reading focus after geometry changes in the selected child transcript", async () => {
  const observations: Array<{ nodes: Set<Element>; callback: ResizeObserverCallback }> = [];
  vi.stubGlobal(
    "ResizeObserver",
    class {
      nodes = new Set<Element>();
      constructor(callback: ResizeObserverCallback) {
        observations.push({ nodes: this.nodes, callback });
      }
      observe(node: Element) {
        this.nodes.add(node);
      }
      unobserve(node: Element) {
        this.nodes.delete(node);
      }
      disconnect() {
        this.nodes.clear();
      }
    },
  );
  material.rows = rows();
  const facts = material.rows[0]!.facts;
  facts.toolCalls.nested = {
    id: "nested",
    runId: "child",
    name: "delegate_task",
    fn: "Review request headers",
    args: '{"summary":"Review request headers"}',
    status: "ok",
  };
  facts.delegatedRuns.delegate![0]!.messages[0]!.blocks.push({
    kind: "tool",
    toolCallId: "nested",
  });
  testClient.navigation.go({ session: "session" });
  openWorkspaceSubagentRun(testClient.workspace.access, "child");
  testClient.render(<SubagentsPanel />);
  await act(async () => vi.dynamicImportSettled());
  const content = screen.getByRole("region", { name: "Sub-agent" });
  const viewport = content.parentElement!;
  const target = within(content).getByRole("button", { name: /Sub-agent/ });
  const shape = { height: 400 };
  viewport.style.scrollPaddingTop = "20px";
  viewport.style.scrollPaddingBottom = "80px";
  vi.spyOn(viewport, "getBoundingClientRect").mockImplementation(
    () => new DOMRect(0, 0, 600, shape.height),
  );
  const bounds = () => new DOMRect(0, 210 - viewport.scrollTop, 100, 24);
  vi.spyOn(target, "getBoundingClientRect").mockImplementation(bounds);
  vi.spyOn(target, "getClientRects").mockImplementation(() => [bounds()] as unknown as DOMRectList);
  Object.defineProperty(viewport, "scrollBy", {
    configurable: true,
    value: (options: ScrollToOptions) => {
      viewport.scrollTop += options.top ?? 0;
    },
  });
  act(() => target.focus());
  shape.height = 240;
  fireEvent.scroll(viewport);
  act(() => {
    for (const observer of observations)
      if (observer.nodes.has(viewport)) {
        observer.callback(
          [
            {
              target: viewport,
              contentRect: viewport.getBoundingClientRect(),
              borderBoxSize: [],
              contentBoxSize: [],
              devicePixelContentBoxSize: [],
            },
          ],
          {} as ResizeObserver,
        );
      }
  });
  expect(bounds().bottom).toBeLessThanOrEqual(160);
  expect(document.activeElement).toBe(target);
  expect(testClient.navigation.get().subagent).toBe("child");
  expect(screen.getByRole("region", { name: "Sub-agent" })).toBe(content);
});
