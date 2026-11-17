import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { type TranscriptRow } from "@/features/agent/public/conversation";
import { type AgentProblem } from "@/features/agent/public/sessionView";

import {
  beginComposerDraftReplacement,
  captureComposerSubmission,
  replaceComposerDraft,
} from "@/features/composer/public/draft";
import { ChatStream } from "@/features/shell/workbench/panel/ChatStream";
import { ContentVisibility } from "@/ui/index";
import { AgentAppShell } from "@/ui/agent/index";

const material = vi.hoisted(() => ({
  rows: [] as TranscriptRow[],
  problem: null as AgentProblem | null,
}));

vi.mock("@/features/agent/public/run", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agent/public/run")>()),
  useActiveSessionProblem: () => material.problem,
}));

vi.mock("@/features/agent/public/conversation", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agent/public/conversation")>()),
  useActiveConversationRows: () => material.rows,
}));

vi.mock("@/features/connection/public/serviceStatus", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/connection/public/serviceStatus")>()),
  useRuntimeServiceStatus: () => ({ phase: "ready", observation: null, failure: null }),
  useRuntimeCommandsAvailable: () => true,
  runtimeCommandsAvailable: () => true,
}));

vi.mock("@/features/agent/public/hitl", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agent/public/hitl")>()),
  useQuestionAnswer: () => ({ submit: () => undefined, pending: false, submitted: false }),
}));

function userRow(): TranscriptRow {
  return {
    message: {
      id: "user-1",
      runId: "run-1",
      role: "user",
      blocks: [{ kind: "text", status: "complete", text: "Earlier request" }],
    },
    runOwner: { kind: "owned", runId: "run-1", status: "finished" },
    facts: { toolCalls: {}, delegatedRuns: {}, awaiting: new Map() },
  };
}

function Harness({ visible = true }: { visible?: boolean }) {
  return (
    <AgentAppShell
      sidebarLabel="Navigation"
      sidebarResizeLabel="Resize navigation"
      sidebarOpen={false}
      sidebarLayout={null}
      onLayoutChanged={() => undefined}
      onSidebarToggle={() => undefined}
      sidebarExpandLabel="Expand navigation"
      sidebarCollapseLabel="Collapse navigation"
      main={
        <>
          <button>Later control</button>
          <ContentVisibility visible={visible}>
            <ChatStream onSend={() => true} />
          </ContentVisibility>
        </>
      }
    />
  );
}

function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity } },
  });
  const view = testClient.render(<Harness />, {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    ),
  });
  return {
    ...view,
    showRows(rows: TranscriptRow[], visible = true) {
      material.rows = rows;
      view.rerender(<Harness visible={visible} />);
    },
  };
}

beforeEach(() => {
  material.rows = [userRow()];
  material.problem = null;
  testClient.agent.access.selectAgentSession("stream-session");
  replaceComposerDraft(testClient.composer.access, { text: "Keep this authored draft" });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("retains the Lexical document and editor identity across conversation layouts", async () => {
  const view = mount();
  const draft = await screen.findByRole("textbox", { name: "Message composer" });
  act(() => draft.focus());
  view.showRows([]);
  expect(screen.getByRole("textbox", { name: "Message composer" })).toBe(draft);
  expect(draft.textContent).toBe("Keep this authored draft");
  expect(document.activeElement).toBe(draft);
  view.showRows([userRow()]);
  expect(screen.getByRole("textbox", { name: "Message composer" })).toBe(draft);
  expect(captureComposerSubmission(testClient.composer.access).value).toBe(
    "Keep this authored draft",
  );
});
it("restores captured history without claiming later outside focus", async () => {
  const view = mount();
  await screen.findByRole("textbox", { name: "Message composer" });
  let replace!: ReturnType<typeof beginComposerDraftReplacement>;
  act(() => {
    replace = beginComposerDraftReplacement(testClient.composer.access);
  });
  const outside = screen.getByRole("button", { name: "Later control" });
  outside.focus();
  view.showRows([]);
  act(() => {
    expect(replace({ text: "Restored history" })).toBe(true);
  });
  await waitFor(() =>
    expect(screen.getByRole("textbox", { name: "Message composer" }).textContent).toBe(
      "Restored history",
    ),
  );
  expect(document.activeElement).toBe(outside);
});
it("withdraws editing while the retained conversation is hidden", async () => {
  const view = mount();
  const draft = await screen.findByRole("textbox", { name: "Message composer" });
  view.showRows([userRow()], false);
  await waitFor(() => expect(draft.getAttribute("contenteditable")).toBe("false"));
  view.showRows([userRow()]);
  await waitFor(() => expect(draft.getAttribute("contenteditable")).toBe("true"));
});
