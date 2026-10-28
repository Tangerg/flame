import { createRef } from "react";
import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { type RuntimeServiceSnapshot } from "@/features/connection/public/serviceStatus";
import { type AgentSessionSummary } from "@/features/agent/public/session";
import { ChatPanel } from "@/features/shell/workbench/panel/ChatPanel";

const model = vi.hoisted(() => ({
  loading: true,
  sessions: undefined as AgentSessionSummary[] | undefined,
  service: { phase: "checking", observation: null, failure: null } as RuntimeServiceSnapshot,
  openSettings: vi.fn(),
}));

vi.mock("@/features/agent/public/session", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agent/public/session")>()),
  useAgentSessions: () => ({ isLoading: model.loading, data: model.sessions }),
}));

vi.mock("@/features/connection/public/serviceStatus", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/connection/public/serviceStatus")>()),
  useRuntimeServiceStatus: () => model.service,
}));

vi.mock("@/features/workspace/public/navigation", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/workspace/public/navigation")>()),
  openWorkspaceSettingsPane: model.openSettings,
}));

vi.mock("@/features/shell/workbench/panel/ChatStream", async () => {
  const { FloatingComposer } = await import("@/features/shell/workbench/panel/FloatingComposer");
  return {
    ChatStream: () => (
      <FloatingComposer overlayRef={createRef<HTMLDivElement>()} floating={false}>
        <textarea aria-label="Message" defaultValue="Continue the conversation" />
      </FloatingComposer>
    ),
  };
});

let client: TestClient;

beforeEach((context) => {
  const owned = createTestClient();
  client = owned;
  context.onTestFinished(() => owned.dispose());
  model.loading = true;
  model.sessions = undefined;
  model.service = { phase: "checking", observation: null, failure: null };
  model.openSettings.mockClear();
});

function unavailable() {
  model.service = {
    phase: "unavailable",
    observation: null,
    failure: { reason: "failed", detail: "connection refused" },
  };
}

function ready() {
  model.service = {
    phase: "ready",
    observation: {
      server: { name: "flame", version: "test" },
      protocolVersion: "2",
      health: "ready",
      checks: {},
    },
    failure: null,
  };
}

it("keeps cold-start inspection quiet without mounting an unowned conversation", () => {
  client.render(<ChatPanel onSend={() => false} />);
  expect(screen.queryByRole("alert")).toBeNull();
  expect(screen.queryByRole("status")).toBeNull();
  expect(screen.queryByRole("textbox", { name: "Message" })).toBeNull();
});

it("makes the existing connection recovery reachable before the first Session read settles", () => {
  unavailable();
  client.render(<ChatPanel onSend={() => false} />);
  expect(screen.getByRole("alert").textContent).toContain(
    "Runtime is unavailable. Flame will keep trying.",
  );
  fireEvent.click(screen.getByRole("button", { name: "Connection settings" }));
  expect(model.openSettings).toHaveBeenCalledExactlyOnceWith(client.workspace.access, "connection");
  expect(screen.queryByRole("textbox", { name: "Message" })).toBeNull();
});

it("shows the existing reconnecting status while the Session read remains pending", () => {
  model.service = { phase: "reconnecting", observation: null, failure: null };
  client.render(<ChatPanel onSend={() => false} />);
  expect(screen.getByRole("status").textContent).toContain(
    "Runtime connection was lost. Reconnecting…",
  );
  expect(screen.queryByRole("textbox", { name: "Message" })).toBeNull();
});

it("hands recovery to the loaded Composer once and retains its later draft and focus", async () => {
  unavailable();
  const view = client.render(<ChatPanel onSend={() => false} />);
  expect(screen.getAllByRole("alert")).toHaveLength(1);
  model.loading = false;
  model.sessions = [];
  view.rerender(<ChatPanel onSend={() => false} />);
  expect(screen.getAllByRole("alert")).toHaveLength(1);
  const input = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message" });
  fireEvent.change(input, { target: { value: "Continue the unfinished draft" } });
  act(() => input.focus());
  ready();
  view.rerender(<ChatPanel onSend={() => false} />);
  await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
  expect(screen.getByRole("textbox", { name: "Message" })).toBe(input);
  expect(input.value).toBe("Continue the unfinished draft");
  expect(document.activeElement).toBe(input);
});

it("keeps one focused recovery action while request activity changes without a Session snapshot", () => {
  unavailable();
  const view = client.render(<ChatPanel onSend={() => false} />);
  const action = screen.getByRole("button", { name: "Connection settings" });
  act(() => action.focus());
  model.loading = false;
  view.rerender(<ChatPanel onSend={() => false} />);
  expect(screen.getByRole("button", { name: "Connection settings" })).toBe(action);
  expect(screen.queryByRole("textbox", { name: "Message" })).toBeNull();
  model.loading = true;
  view.rerender(<ChatPanel onSend={() => false} />);
  expect(screen.getByRole("button", { name: "Connection settings" })).toBe(action);
  expect(document.activeElement).toBe(action);
});

it("preserves later outside focus when the pending failure observation changes", () => {
  unavailable();
  const content = () => (
    <>
      <button>Outside recovery</button>
      <ChatPanel onSend={() => false} />
    </>
  );
  const view = client.render(content());
  const outside = screen.getByRole("button", { name: "Outside recovery" });
  act(() => outside.focus());
  model.service = { ...model.service, failure: { reason: "timeout" } };
  view.rerender(content());
  expect(document.activeElement).toBe(outside);
});

it("retires the startup notice after recovery while still waiting for Session data", async () => {
  unavailable();
  const view = client.render(<ChatPanel onSend={() => false} />);
  expect(screen.getByRole("alert")).toBeTruthy();
  ready();
  view.rerender(<ChatPanel onSend={() => false} />);
  await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
  expect(screen.queryByRole("status")).toBeNull();
  expect(screen.queryByRole("textbox", { name: "Message" })).toBeNull();
});
