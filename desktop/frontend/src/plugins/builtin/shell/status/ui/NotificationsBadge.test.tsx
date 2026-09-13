import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { drainBrowserTasks } from "@/test/browserTasks";
import { useNotificationStore } from "@/plugins/sdk";
import { NotificationsBadge } from "./NotificationsBadge";

afterEach(async () => {
  cleanup();
  useNotificationStore.setState({ log: [] });
  await drainBrowserTasks();
});

it("reads and dismisses global notifications without navigating the conversation", async () => {
  const first = useNotificationStore
    .getState()
    .push({ plugin: "test", level: "info", message: "Saved" });
  useNotificationStore
    .getState()
    .push({ plugin: "test", level: "error", message: "Could not save" });
  const location = window.location.href;
  render(<NotificationsBadge />);
  fireEvent.click(screen.getByRole("button", { name: "Notifications" }));
  await screen.findByText("Could not save");
  const dismiss = screen.getAllByRole("button", { name: "Dismiss" });
  fireEvent.click(dismiss[0]!);
  expect(useNotificationStore.getState().log[1]?.dismissed).toBe(true);
  expect(useNotificationStore.getState().log[0]?.id).toBe(first.id);
  expect(useNotificationStore.getState().log[0]?.dismissed).not.toBe(true);

  act(() => {
    useNotificationStore
      .getState()
      .push({ plugin: "test", level: "warn", message: "Still running" });
  });
  expect(screen.getByText("Still running")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Clear all" }));
  expect(screen.getByText("No notifications")).toBeTruthy();
  expect(useNotificationStore.getState().log).toEqual([]);
  expect(window.location.href).toBe(location);
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});
