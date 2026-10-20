import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { configureNotificationCentre } from "@/features/notifications/application/ports/notificationCentre";
import { useNotificationAuthorization } from "@/features/notifications/public/notifications";
import { SystemNotificationsSection } from "@/features/notifications/public/PreferencesSections";
describe("SystemNotificationsSection", () => {
  let uninstall: (() => void) | undefined;
  afterEach(() => {
    uninstall?.();
    cleanup();
  });

  it("asks the platform what it allows when the row is shown", async () => {
    const refresh = vi.fn(async () => "denied" as const);
    useNotificationAuthorization.setState({ authorization: "denied" });
    uninstall = configureNotificationCentre({
      authorization: refresh,
      requestAuthorization: vi.fn(async () => "denied" as const),
      send: vi.fn(async () => false),
      onOpened: () => () => {},
    });
    render(<SystemNotificationsSection />);
    await waitFor(() => expect(refresh).toHaveBeenCalledOnce());
    expect(screen.getByRole("switch").getAttribute("aria-disabled") ?? "").not.toBe("");
    expect(screen.getByText(/System Settings/)).toBeTruthy();
  });
});
