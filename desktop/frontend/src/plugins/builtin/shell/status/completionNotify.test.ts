import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const calls = vi.hoisted(() => ({
  select: vi.fn(),
  chat: vi.fn(),
  reveal: vi.fn(() => Promise.resolve()),
}));

vi.mock("@/plugins/builtin/agent/public/session", () => ({ selectAgentSession: calls.select }));
vi.mock("@/plugins/builtin/workspace/public/navigation", () => ({
  selectWorkspaceChat: calls.chat,
}));
vi.mock("./adapters/windowFocus", () => ({ revealClientWindow: calls.reveal }));
vi.mock("./chime", () => ({ playCompletionChime: vi.fn() }));

import { announceSettlement, startCompletionNotifications } from "./completionNotify";
import { installNotificationCentre } from "./adapters/systemNotifier";
import { useSystemNotificationsStore } from "./systemNotifications";

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
    uninstall = installNotificationCentre();
    stop = startCompletionNotifications();
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
    await announceSettlement({ sessionId: "s-b", status: "needsInput", errorMessage: null });
    expect(FakeNotification.last?.title).toMatch(/needs you/i);
    FakeNotification.last!.onclick!();
    expect(calls.reveal).toHaveBeenCalledOnce();
    expect(calls.select).toHaveBeenCalledWith("s-b");
    expect(calls.chat).toHaveBeenCalledOnce();
  });

  it("stays quiet when the user turned notifications off", async () => {
    useSystemNotificationsStore.setState({ systemNotifications: false });
    await announceSettlement({ sessionId: "s-a", status: "finished", errorMessage: null });
    expect(FakeNotification.last).toBeNull();
  });

  it("does not pretend to notify when the system refused permission", async () => {
    FakeNotification.permission = "denied";
    await announceSettlement({ sessionId: "s-a", status: "finished", errorMessage: null });
    expect(FakeNotification.last).toBeNull();
  });
});
