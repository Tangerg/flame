import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});

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
      { sessionId: "s-b", status: "needsInput", errorMessage: null },
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
      { sessionId: "s-a", status: "finished", errorMessage: null },
      "Flame",
    );
    expect(FakeNotification.last).toBeNull();
  });

  it("does not pretend to notify when the system refused permission", async () => {
    FakeNotification.permission = "denied";
    await announceSettlement(
      testClient.notifications.access,
      { sessionId: "s-a", status: "finished", errorMessage: null },
      "Flame",
    );
    expect(FakeNotification.last).toBeNull();
  });
});
