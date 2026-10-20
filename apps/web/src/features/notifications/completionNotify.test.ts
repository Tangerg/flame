import { createBrowserHost } from "@/platform/browserHost";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  announceSettlement,
  startCompletionNotifications,
} from "@/features/notifications/completionNotify";
import { installNotificationCentre } from "@/features/notifications/adapters/systemNotifier";
import { useSystemNotificationsStore } from "@/features/notifications/systemNotifications";
const calls = vi.hoisted(() => ({
  select: vi.fn(),
  chat: vi.fn(),
  reveal: vi.fn(() => Promise.resolve()),
}));

vi.mock("@/features/agent/public/session", () => ({ selectAgentSession: calls.select }));

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
  let uninstall: () => void;
  beforeEach(() => {
    uninstall = installNotificationCentre(createBrowserHost());
    stop = startCompletionNotifications(calls.reveal, "Flame");
    vi.stubGlobal("Notification", FakeNotification);
    vi.spyOn(document, "hasFocus").mockReturnValue(false);
    FakeNotification.permission = "granted";
    FakeNotification.last = null;
    useSystemNotificationsStore.setState({ systemNotifications: true });
    for (const call of Object.values(calls)) call.mockClear();
  });
  afterEach(() => {
    stop();
    uninstall();
    vi.unstubAllGlobals();
  });

  it("opens the session the notification is about when it is clicked", async () => {
    await announceSettlement(
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
    useSystemNotificationsStore.setState({ systemNotifications: false });
    await announceSettlement({ sessionId: "s-a", status: "finished", errorMessage: null }, "Flame");
    expect(FakeNotification.last).toBeNull();
  });

  it("does not pretend to notify when the system refused permission", async () => {
    FakeNotification.permission = "denied";
    await announceSettlement({ sessionId: "s-a", status: "finished", errorMessage: null }, "Flame");
    expect(FakeNotification.last).toBeNull();
  });
});
