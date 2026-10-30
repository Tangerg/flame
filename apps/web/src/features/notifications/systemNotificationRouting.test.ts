import { afterEach, expect, it, vi } from "vitest";
import { createTestClient } from "@/test/renderClient";
import { startSystemNotificationRouting } from "./systemNotificationRouting";

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

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

it("routes an OS notification to its captured Session independently of optional policy", async () => {
  vi.stubGlobal("Notification", FakeNotification);
  const client = createTestClient();
  const reveal = vi.fn().mockResolvedValue(undefined);
  const stop = startSystemNotificationRouting(
    client.notifications.access.notifier,
    client.agent.access,
    client.workspace.access,
    reveal,
  );
  try {
    client.navigation.go({ session: "other", view: "settings" });
    await client.notifications.access.notifier.sendSystemNotification({
      id: "installed-channel:captured",
      title: "Plugin title",
      body: "Plugin body",
      target: "captured",
    });
    expect(FakeNotification.last!.title).toBe("Plugin title");
    FakeNotification.last!.onclick!();
    expect(reveal).toHaveBeenCalledOnce();
    expect(client.agent.access.getActiveSessionId()).toBe("captured");
    expect(client.navigation.get().view).toBeNull();
    stop();
    client.navigation.go({ session: "later" });
    FakeNotification.last!.onclick!();
    expect(reveal).toHaveBeenCalledOnce();
    expect(client.agent.access.getActiveSessionId()).toBe("later");
  } finally {
    stop();
    await client.dispose();
  }
});
