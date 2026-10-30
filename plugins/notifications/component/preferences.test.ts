// @vitest-environment happy-dom
import { afterEach, expect, it, vi } from "vitest";
import { readPreferences, writePreference } from "./preferences";

afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.removeItem("flame.completion-sound");
  localStorage.removeItem("flame.system-notifications");
  vi.restoreAllMocks();
});

it("retains the existing client preference records and changes only the chosen fact", () => {
  localStorage.setItem(
    "flame.completion-sound",
    '{"state":{"completionSound":true},"version":0}',
  );
  localStorage.setItem(
    "flame.system-notifications",
    '{"state":{"systemNotifications":false},"version":0}',
  );
  expect(readPreferences()).toEqual({
    completionSound: true,
    systemNotifications: false,
  });
  const sound = localStorage.getItem("flame.completion-sound");
  writePreference("systemNotifications", true);
  expect(readPreferences()).toEqual({
    completionSound: true,
    systemNotifications: true,
  });
  expect(localStorage.getItem("flame.completion-sound")).toBe(sound);
  expect(
    JSON.parse(localStorage.getItem("flame.system-notifications")!),
  ).toEqual({ state: { systemNotifications: true }, version: 0 });
});

it("uses the current defaults for absent or malformed saved values", () => {
  expect(readPreferences()).toEqual({
    completionSound: false,
    systemNotifications: true,
  });
  const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
  localStorage.setItem(
    "flame.completion-sound",
    '{"state":{"completionSound":"true"},"version":0}',
  );
  localStorage.setItem("flame.system-notifications", "invalid json");
  expect(readPreferences()).toEqual({
    completionSound: false,
    systemNotifications: true,
  });
  expect(warn).toHaveBeenCalledTimes(2);
});

it("preserves an unexpected storage failure instead of reporting a saved preference", () => {
  const failure = new DOMException("storage is unavailable", "SecurityError");
  const storage = localStorage;
  vi.stubGlobal("localStorage", {
    getItem: storage.getItem.bind(storage),
    removeItem: storage.removeItem.bind(storage),
    setItem() {
      throw failure;
    },
  });
  expect(() => writePreference("completionSound", true)).toThrow(failure);
});
