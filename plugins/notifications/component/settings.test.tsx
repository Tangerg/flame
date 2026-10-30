// @vitest-environment happy-dom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { NotificationSettings } from "./component";
import { readPreferences } from "./preferences";
import { notificationFixture } from "./testing";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.removeItem("flame.completion-sound");
  localStorage.removeItem("flame.system-notifications");
  vi.restoreAllMocks();
});

it("asks the permission owner when shown and renders its denied state", async () => {
  const { context, channel, setAuthorization } = notificationFixture();
  setAuthorization("denied");
  render(<NotificationSettings context={context} />);
  await waitFor(() =>
    expect(channel.refreshAuthorization).toHaveBeenCalledOnce(),
  );
  expect(
    screen
      .getByRole("switch", { name: "System notifications" })
      .getAttribute("aria-disabled"),
  ).toBe("true");
  expect(screen.getByText(/System Settings/)).toBeTruthy();
});

it("writes the existing preference record while keeping sound and system choices independent", async () => {
  const { context, channel } = notificationFixture();
  render(<NotificationSettings context={context} />);
  fireEvent.click(screen.getByRole("switch", { name: "Completion sound" }));
  expect(readPreferences()).toEqual({
    completionSound: true,
    systemNotifications: true,
  });
  fireEvent.click(screen.getByRole("switch", { name: "System notifications" }));
  expect(readPreferences()).toEqual({
    completionSound: true,
    systemNotifications: false,
  });
  expect(channel.requestAuthorization).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("switch", { name: "System notifications" }));
  await waitFor(() =>
    expect(channel.requestAuthorization).toHaveBeenCalledOnce(),
  );
  expect(readPreferences()).toEqual({
    completionSound: true,
    systemNotifications: true,
  });
});

it("does not request permission when saving its enabled preference fails", async () => {
  localStorage.setItem(
    "flame.system-notifications",
    '{"state":{"systemNotifications":false},"version":0}',
  );
  const { context, channel } = notificationFixture();
  render(<NotificationSettings context={context} />);
  const failure = new DOMException(
    "preference storage is unavailable",
    "SecurityError",
  );
  const storage = localStorage;
  vi.stubGlobal("localStorage", {
    getItem: storage.getItem.bind(storage),
    removeItem: storage.removeItem.bind(storage),
    setItem() {
      throw failure;
    },
  });
  fireEvent.click(screen.getByRole("switch", { name: "System notifications" }));
  expect((await screen.findByRole("alert")).textContent).toContain(
    failure.message,
  );
  expect(channel.requestAuthorization).not.toHaveBeenCalled();
});

it("refuses an old view's preference write after its captured lifetime ends", () => {
  const { context, controller } = notificationFixture();
  render(<NotificationSettings context={context} />);
  controller.abort();
  fireEvent.click(screen.getByRole("switch", { name: "Completion sound" }));
  expect(localStorage.getItem("flame.completion-sound")).toBeNull();
});
