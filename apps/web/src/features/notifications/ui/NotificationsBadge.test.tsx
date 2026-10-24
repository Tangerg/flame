import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { drainBrowserTasks } from "@/test/browserTasks";
import { useNotificationStore } from "@/features/notifications/public/notifications";
import { NotificationsBadge } from "@/features/notifications/ui/NotificationsBadge";
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
  fireEvent.click(screen.getByRole("button", { name: /unread notifications/i }));
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

function pushInfo(message: string) {
  return useNotificationStore.getState().push({ plugin: "test", level: "info", message });
}

async function openHistory() {
  render(<NotificationsBadge />);
  const bell = screen.getByRole("button", { name: /unread notification/i });
  fireEvent.click(bell);
  await screen.findByRole("dialog");
  return bell;
}

function describedMessage(control: HTMLElement) {
  const id = control.getAttribute("aria-describedby");
  expect(id).toBeTruthy();
  return document.getElementById(id!);
}

it("keeps a dismissed entry's control focused in place and refuses repeated activation", async () => {
  const entries = Array.from({ length: 12 }, (_, index) => pushInfo(`Event ${index + 1}`));
  await openHistory();
  const controls = screen.getAllByRole("button", { name: "Dismiss" });
  expect(controls).toHaveLength(12);
  const target = controls[6]!;

  const logs: unknown[] = [];
  const unsubscribe = useNotificationStore.subscribe((state) => logs.push(state.log));
  try {
    target.focus();
    fireEvent.click(target);

    expect(logs).toHaveLength(1);
    const log = useNotificationStore.getState().log;
    expect(log.find((entry) => entry.dismissed)?.id).toBe(entries[5]!.id);
    expect(log.filter((entry) => entry.dismissed)).toHaveLength(1);
    expect(screen.getByRole("button", { name: /11 unread notifications/i })).toBeTruthy();

    expect(target.isConnected).toBe(true);
    expect(document.activeElement).toBe(target);
    expect(screen.getAllByRole("button", { name: "Dismiss" })[6]).toBe(target);
    expect(target.getAttribute("aria-disabled")).toBe("true");
    expect((target as HTMLButtonElement).disabled).toBe(false);
    expect(describedMessage(target)?.textContent).toBe("Event 6");

    fireEvent.click(target);
    expect(logs).toHaveLength(1);
    expect(useNotificationStore.getState().log).toBe(log);
    expect(document.activeElement).toBe(target);
  } finally {
    unsubscribe();
  }
});

it("describes each control by its own message even when messages repeat", async () => {
  const older = pushInfo("Saved");
  pushInfo("Saved");
  await openHistory();
  const [newerControl, olderControl] = screen.getAllByRole("button", { name: "Dismiss" });
  const newerMessage = describedMessage(newerControl!);
  const olderMessage = describedMessage(olderControl!);

  expect(newerMessage?.textContent).toBe("Saved");
  expect(olderMessage?.textContent).toBe("Saved");
  expect(newerMessage).not.toBe(olderMessage);
  expect(screen.getAllByRole("button", { name: "Dismiss", description: "Saved" })).toHaveLength(2);

  fireEvent.click(olderControl!);
  const log = useNotificationStore.getState().log;
  expect(log.find((entry) => entry.id === older.id)?.dismissed).toBe(true);
  expect(log.filter((entry) => entry.dismissed)).toHaveLength(1);
  expect(olderControl!.getAttribute("aria-disabled")).toBe("true");
  expect(newerControl!.getAttribute("aria-disabled")).toBe("false");
});

it("keeps current focus and state while other store transitions arrive", async () => {
  const first = pushInfo("First");
  const second = pushInfo("Second");
  await openHistory();
  const [secondControl, firstControl] = screen.getAllByRole("button", { name: "Dismiss" });

  secondControl!.focus();
  fireEvent.click(secondControl!);
  act(() => {
    pushInfo("Third");
  });
  expect(screen.getByText("Third")).toBeTruthy();
  const controls = screen.getAllByRole("button", { name: "Dismiss" });
  expect(describedMessage(controls[0]!)?.textContent).toBe("Third");
  expect(controls[1]).toBe(secondControl);
  expect(document.activeElement).toBe(secondControl);
  expect(secondControl!.getAttribute("aria-disabled")).toBe("true");

  firstControl!.focus();
  act(() => {
    useNotificationStore.getState().dismiss(second.id);
    useNotificationStore.getState().dismiss(first.id);
  });
  expect(document.activeElement).toBe(firstControl);
  expect(firstControl!.getAttribute("aria-disabled")).toBe("true");
  expect(screen.getByText("First")).toBeTruthy();
  expect(useNotificationStore.getState().log.map((entry) => entry.id)).toEqual([
    first.id,
    second.id,
    expect.any(String),
  ]);
});

it("keeps Clear all focused on the empty state and returns to the bell on Escape", async () => {
  pushInfo("Saved");
  const bell = await openHistory();
  const clearAll = screen.getByRole("button", { name: "Clear all" });

  clearAll.focus();
  fireEvent.click(clearAll);
  expect(useNotificationStore.getState().log).toEqual([]);
  expect(screen.getByText("No notifications")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Dismiss" })).toBeNull();
  expect(document.activeElement).toBe(clearAll);
  expect(clearAll.getAttribute("aria-disabled")).toBe("true");

  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(bell));
});

it("preserves later outside focus when another source dismisses a notification", async () => {
  const entry = pushInfo("Saved");
  render(
    <>
      <NotificationsBadge />
      <textarea aria-label="Draft" defaultValue="Keep this draft." />
    </>,
  );
  fireEvent.click(screen.getByRole("button", { name: /unread notification/i }));
  await screen.findByRole("dialog");
  const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Draft" });
  draft.focus();
  draft.setSelectionRange(5, 10);
  act(() => useNotificationStore.getState().dismiss(entry.id));
  expect(document.activeElement).toBe(draft);
  expect(draft.value).toBe("Keep this draft.");
  expect([draft.selectionStart, draft.selectionEnd]).toEqual([5, 10]);
  expect(useNotificationStore.getState().log[0]?.dismissed).toBe(true);
});
