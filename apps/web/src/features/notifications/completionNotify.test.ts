import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});

import { type RunEvent, type RunRef } from "@flame/runtime-client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  announceSettlement,
  startCompletionNotifications,
} from "@/features/notifications/completionNotify";

const calls = vi.hoisted(() => ({
  select: vi.fn(),
  chat: vi.fn(),
  reveal: vi.fn(() => Promise.resolve()),
}));

prepareTestClient(() => {
  Object.assign(testClient.agent.access, { selectAgentSession: calls.select });
});

vi.mock("@/features/workspace/public/navigation", () => ({
  selectWorkspaceChat: calls.chat,
}));

vi.mock("@/features/notifications/chime", () => ({ playCompletionChime: vi.fn() }));

class FakeNotification {
  static permission: NotificationPermission = "granted";
  static requestPermission = vi.fn(async () => FakeNotification.permission);
  static last: FakeNotification | null = null;
  onclick: (() => void) | null = null;
  constructor(
    readonly title: string,
    readonly options: NotificationOptions,
  ) {
    FakeNotification.last = this;
  }
  close() {}
}

describe("announceSettlement", () => {
  let stop: () => void;

  beforeEach(() => {
    stop = startCompletionNotifications(
      testClient.notifications.access,
      testClient.agent.access,
      testClient.workspace.access,
      calls.reveal,
      "Flame",
    );
    vi.stubGlobal("Notification", FakeNotification);
    vi.spyOn(document, "hasFocus").mockReturnValue(false);
    FakeNotification.permission = "granted";
    FakeNotification.last = null;
    testClient.notifications.access.systemNotifications.setState({ systemNotifications: true });
    for (const call of Object.values(calls)) call.mockClear();
  });
  afterEach(() => {
    stop();

    vi.unstubAllGlobals();
  });

  it("opens the session the notification is about when it is clicked", async () => {
    await announceSettlement(
      testClient.notifications.access,
      {
        sessionId: "s-b",
        runId: "run-notify",
        segmentId: "segment-notify",
        status: "needsInput",
        errorMessage: null,
      },
      "Flame",
    );
    expect(FakeNotification.last?.title).toMatch(/needs you/i);
    FakeNotification.last!.onclick!();
    expect(calls.reveal).toHaveBeenCalledOnce();
    expect(calls.select).toHaveBeenCalledWith("s-b");
    expect(calls.chat).toHaveBeenCalledOnce();
  });

  it("stays quiet when the user turned notifications off", async () => {
    testClient.notifications.access.systemNotifications.setState({ systemNotifications: false });
    await announceSettlement(
      testClient.notifications.access,
      {
        sessionId: "s-a",
        runId: "run-notify",
        segmentId: "segment-notify",
        status: "finished",
        errorMessage: null,
      },
      "Flame",
    );
    expect(FakeNotification.last).toBeNull();
  });

  it("does not pretend to notify when the system refused permission", async () => {
    FakeNotification.permission = "denied";
    await announceSettlement(
      testClient.notifications.access,
      {
        sessionId: "s-a",
        runId: "run-notify",
        segmentId: "segment-notify",
        status: "finished",
        errorMessage: null,
      },
      "Flame",
    );
    expect(FakeNotification.last).toBeNull();
  });
  it.each([false, true])(
    "notifies a background Run after committed events (separate batches=$0)",
    async (separateBatches) => {
      const sent = vi.spyOn(testClient.notifications.access.notifier, "sendSystemNotification");
      const sessionId = "notification-session";
      const segmentId = "notification-segment";
      const run: RunRef = {
        id: "notification-run",
        sessionId,
        status: "running",
        activeSegmentId: segmentId,
        createdAt: "2026-06-03T00:00:00.000Z",
        metrics: { steps: 0, activeDurationMillis: 0 },
        protocolProfile: { interruptTypes: [], requiredFeatures: [] },
        provider: "openai",
        model: "gpt-5",
      };
      const store = testClient.agent.projection.store;
      store.getState().ensureSession(sessionId);
      finishTestClient(() => {
        sent.mockRestore();
      });
      const event = (id: string, value: RunEvent["event"]): RunEvent => ({
        event: value,
        eventId: id,
        runId: run.id,
        segmentId,
        timestamp: "2026-06-03T00:00:01.000Z",
      });
      const start = event("start", { type: "segment.started", run });
      if (separateBatches) store.getState().applyRunEvents(sessionId, [start]);
      const { activeSegmentId: _active, ...rest } = run;
      const finish = event("finish", {
        type: "segment.finished",
        run: {
          ...rest,
          status: "finished",
          outcome: { type: "completed" },
          finishedAt: "2026-06-03T00:00:01.000Z",
        },
      });
      store.getState().applyRunEvents(sessionId, separateBatches ? [finish] : [start, finish]);
      await vi.waitFor(() =>
        expect(sent).toHaveBeenCalledExactlyOnceWith(
          expect.objectContaining({ target: sessionId }),
        ),
      );
      expect(FakeNotification.last?.title).toMatch(/finished/i);
      store.getState().applyRunEvents(sessionId, [finish]);
      expect(sent).toHaveBeenCalledOnce();
    },
  );
});
