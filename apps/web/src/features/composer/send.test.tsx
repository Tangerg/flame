import { act, fireEvent, screen } from "@testing-library/react";
import { beforeEach, expect, it, onTestFinished, vi } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { loadPluginsForTest } from "@/test/kernel";
import { EMPTY_AGENT_SESSION_VIEW, type AgentRunView } from "@/features/agent/public/sessionView";
import {
  captureComposerSubmission,
  useComposerText,
  useSetComposerText,
} from "@/features/composer/public/draft";
import { composerSend } from "@/features/composer/send";
import { Composer } from "@/features/composer/ui/Composer";

let testClient: TestClient;
beforeEach(() => {
  testClient = createTestClient();
  onTestFinished(() => testClient.dispose());
  testClient.projection.store.setState({
    service: {
      phase: "ready",
      observation: {
        server: { name: "flame", version: "test" },
        protocolVersion: "2",
        health: "ready",
        checks: {},
      },
      failure: null,
    },
  });
});

const root: AgentRunView = {
  id: "root",
  sessionId: "session",
  parentRunId: null,
  rootRunId: "root",
  spawnedByItemId: null,
  status: "running",
  activeSegmentId: "segment",
  outcome: null,
  metrics: { steps: 1, activeDurationMillis: 10 },
  progress: null,
  contextTokens: null,
  createdAt: "2026-01-01T00:00:00Z",
  finishedAt: null,
};

function publish(running: boolean) {
  const store = testClient.agent.projection.store.getState();
  store.ensureSession("session");
  const refresh = store.beginViewRefresh("session", true);
  if (!refresh) throw new Error("The Composer test requires a refresh owner");
  expect(
    store.commitViewRefresh("session", refresh, {
      ...EMPTY_AGENT_SESSION_VIEW,
      runsById: {
        root: running
          ? root
          : {
              ...root,
              status: "finished",
              activeSegmentId: null,
              outcome: { type: "canceled" },
              finishedAt: "2026-01-01T00:00:10Z",
            },
      },
    }),
  ).toBe(true);
}

function Authoring() {
  const value = useComposerText();
  const onChange = useSetComposerText();
  return (
    <>
      <Composer
        value={value}
        onChange={onChange}
        captureSubmission={() => captureComposerSubmission(testClient.composer.access)}
        onSend={() => false}
        images={[]}
        onRemoveImage={() => {}}
        onAddImages={() => {}}
        pastes={[]}
        onRemovePaste={() => {}}
        onEditPaste={() => false}
        onRestorePaste={() => {}}
        onAddPaste={() => {}}
        acceptsImages={false}
      />
      <input aria-label="Later reading" />
    </>
  );
}

async function authoring({
  text = "Keep this draft.",
  running = true,
  stop = vi.fn(() => true),
} = {}) {
  await loadPluginsForTest(composerSend);
  testClient.navigation.go({ session: "session" });
  publish(running);
  const store = testClient.agent.projection.store.getState();
  store.setStop("session", stop);
  const send = vi.fn(async () => ({
    status: "accepted" as const,
    sessionId: "session",
    runId: "root",
    userItemId: "submitted",
  }));
  store.setSend("session", send);
  testClient.composer.access.replaceDraft({ text });
  testClient.render(<Authoring />);
  return { stop, send };
}

it.each(["", "Keep this draft."])(
  "returns accepted Stop to the same editable draft (%j)",
  async (text) => {
    const stop = vi.fn(() => {
      publish(false);
      return true;
    });
    await authoring({ text, stop });
    const input = screen.getByRole("textbox", { name: "Message composer" }) as HTMLTextAreaElement;
    input.setSelectionRange(Math.min(5, text.length), Math.min(5, text.length));
    const action = screen.getByRole("button", { name: "Stop" });
    act(() => action.focus());
    fireEvent.click(action);
    expect(stop).toHaveBeenCalledOnce();
    expect(screen.queryByRole("button", { name: "Stop" })).toBeNull();
    expect(document.activeElement).toBe(input);
    expect(input.value).toBe(text);
    expect(input.selectionStart).toBe(Math.min(5, text.length));
    expect(captureComposerSubmission(testClient.composer.access).value).toBe(text);
    fireEvent.change(input, { target: { value: `${text} Continue.` } });
    expect(captureComposerSubmission(testClient.composer.access).value).toBe(`${text} Continue.`);
  },
);

it("leaves a refused Stop at its current action without consuming the draft", async () => {
  const { stop } = await authoring({ stop: vi.fn(() => false) });
  const action = screen.getByRole("button", { name: "Stop" });
  act(() => action.focus());
  fireEvent.click(action);
  expect(stop).toHaveBeenCalledOnce();
  expect(document.activeElement).toBe(action);
  expect(captureComposerSubmission(testClient.composer.access).value).toBe("Keep this draft.");
});

it("returns accepted Stop before settlement without reclaiming later reading focus", async () => {
  await authoring();
  const action = screen.getByRole("button", { name: "Stop" });
  const input = screen.getByRole("textbox", { name: "Message composer" });
  act(() => action.focus());
  fireEvent.click(action);
  expect(action.isConnected).toBe(true);
  expect(document.activeElement).toBe(input);
  const later = screen.getByRole("textbox", { name: "Later reading" });
  act(() => later.focus());
  act(() => publish(false));
  expect(document.activeElement).toBe(later);
  expect(captureComposerSubmission(testClient.composer.access).value).toBe("Keep this draft.");
});

it("preserves later focus chosen by the Stop owner", async () => {
  const stop = vi.fn(() => {
    screen.getByRole("textbox", { name: "Later reading" }).focus();
    publish(false);
    return true;
  });
  await authoring({ stop });
  const action = screen.getByRole("button", { name: "Stop" });
  act(() => action.focus());
  fireEvent.click(action);
  expect(document.activeElement).toBe(screen.getByRole("textbox", { name: "Later reading" }));
  expect(captureComposerSubmission(testClient.composer.access).value).toBe("Keep this draft.");
});

it.each(["Stop", "Steer the running turn", "Send"])(
  "retains focused %s while unavailable and refuses activation",
  async (label) => {
    const { stop, send } = await authoring({ running: label !== "Send" });
    const action = screen.getByRole("button", { name: label });
    act(() => action.focus());
    act(() =>
      testClient.projection.store.setState({
        service: { phase: "unavailable", observation: null, failure: null },
      }),
    );
    expect(action.getAttribute("aria-disabled")).toBe("true");
    expect(action.hasAttribute("disabled")).toBe(false);
    expect(document.activeElement).toBe(action);
    fireEvent.click(action);
    fireEvent.keyDown(action, { key: "Enter" });
    fireEvent.keyUp(action, { key: "Enter" });
    expect(stop).not.toHaveBeenCalled();
    expect(send).not.toHaveBeenCalled();
    expect(captureComposerSubmission(testClient.composer.access).value).toBe("Keep this draft.");
  },
);

it("retains focused Stop when its captured capability retires", async () => {
  await authoring();
  const action = screen.getByRole("button", { name: "Stop" });
  act(() => action.focus());
  act(() => testClient.agent.projection.store.getState().setStop("session", null));
  expect(action.getAttribute("aria-disabled")).toBe("true");
  expect(action.hasAttribute("disabled")).toBe(false);
  expect(document.activeElement).toBe(action);
});
