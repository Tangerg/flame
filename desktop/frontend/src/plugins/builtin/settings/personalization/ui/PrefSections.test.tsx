import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { configurePersonalizationPreferencesPort } from "../application/ports/preferences";
import { SystemNotificationsSection } from "./PrefSections";

describe("SystemNotificationsSection", () => {
  let uninstall: (() => void) | undefined;
  afterEach(() => {
    uninstall?.();
    cleanup();
  });

  it("asks the platform what it allows when the row is shown", async () => {
    const refresh = vi.fn(async () => "denied" as const);
    uninstall = configurePersonalizationPreferencesPort({
      useCompletionSound: () => false,
      useSetCompletionSound: () => () => {},
      useSystemNotifications: () => true,
      useSetSystemNotifications: () => () => {},
      useNotificationAuthorization: () => "denied",
      refreshNotificationAuthorization: refresh,
      requestNotificationAuthorization: vi.fn(),
      useStreamReveal: () => "smooth",
      useSetStreamReveal: () => () => {},
    });
    render(<SystemNotificationsSection />);
    await waitFor(() => expect(refresh).toHaveBeenCalledOnce());
    expect(screen.getByRole("switch").getAttribute("aria-disabled") ?? "").not.toBe("");
    expect(screen.getByText(/System Settings/)).toBeTruthy();
  });
});
