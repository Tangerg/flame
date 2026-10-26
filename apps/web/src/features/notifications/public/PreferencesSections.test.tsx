import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createSystemNotifier } from "@/features/notifications/systemNotifier";
import { NotificationProvider } from "@/features/notifications/public/notifier";

import { SystemNotificationsSection } from "@/features/notifications/public/PreferencesSections";
describe("SystemNotificationsSection", () => {
  let uninstall: (() => void) | undefined;
  afterEach(() => {
    uninstall?.();
    cleanup();
  });

  it("asks the platform what it allows when the row is shown", async () => {
    const refresh = vi.fn(async () => "denied" as const);
    const notifier = createSystemNotifier({
      authorization: refresh,
      requestAuthorization: vi.fn(async () => "denied" as const),
      send: vi.fn(async () => false),
      onOpened: () => () => {},
    });
    uninstall = notifier.dispose;
    notifier.useNotificationAuthorization.setState({ authorization: "denied" });
    testClient.render(
      <NotificationProvider value={{ ...testClient.notifications.access, notifier }}>
        <SystemNotificationsSection />
      </NotificationProvider>,
    );
    await waitFor(() => expect(refresh).toHaveBeenCalledOnce());
    expect(screen.getByRole("switch").getAttribute("aria-disabled") ?? "").not.toBe("");
    expect(screen.getByText(/System Settings/)).toBeTruthy();
  });
});
