import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { afterEach, describe, expect, it, vi } from "vitest";

import { type RunRef } from "@flame/runtime-client";

import { canAcceptChatInput, useChatSend } from "@/features/agent/application/input/chatSend";
import { agentTextInput } from "@/features/agent/domain/input";
describe("chat input admission", () => {
  it("requires an explicitly selected project before accepting welcome input", () => {
    expect(canAcceptChatInput("", false, "idle")).toBe(false);
  });

  it("requires the existing Session lifecycle owner to be mounted", () => {
    expect(canAcceptChatInput("ses_1", false, "idle")).toBe(false);
    expect(canAcceptChatInput("ses_1", true, "idle")).toBe(true);
  });

  it("rejects a competing turn while the root is parked for HITL", () => {
    expect(canAcceptChatInput("ses_1", true, "waiting")).toBe(false);
  });
});

const SID = "ses_chat_send";

function waitingRoot(): RunRef {
  return {
    id: "run_1",
    sessionId: SID,
    status: "waiting",
    provider: "openai",
    model: "gpt-5",
    createdAt: "2026-10-14T00:00:00Z",
    metrics: { steps: 0, activeDurationMillis: 0 },
    protocolProfile: { interruptTypes: [], requiredFeatures: [] },
  };
}

function seedSession(root?: RunRef) {
  const send = vi.fn(() => true);
  testClient.navigation.go({ session: SID });
  const store = testClient.agent.projection.store.getState();
  store.ensureSession(SID);
  store.setSend(SID, send);
  if (root) store.applyRunSnapshot(SID, root);
  return send;
}

afterEach(() => {
  testClient.agent.projection.store.getState().dropSession(SID);
  vi.restoreAllMocks();
});

describe("useChatSend admission", () => {
  it("refuses a fresh turn while the root run is waiting", () => {
    const send = seedSession(waitingRoot());
    const { result } = testClient.renderHook(() => useChatSend());
    expect(result.current(agentTextInput("hi"))).toBe(false);
    expect(send).not.toHaveBeenCalled();
  });

  it("sends a fresh turn when no run is parked", () => {
    const send = seedSession();
    const { result } = testClient.renderHook(() => useChatSend());
    expect(result.current(agentTextInput("hi"))).toBe(true);
    expect(send).toHaveBeenCalledOnce();
  });
});
