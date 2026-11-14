import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, onTestFinished, vi } from "vitest";
import type { ReactNode } from "react";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { type NotificationFeed } from "@/features/notifications/public/notifications";
import { WindowNotifications } from "@/features/notifications/toaster/window";
import { AppToaster } from "@/features/notifications/toaster/index";
import { bindClientPageLifecycle } from "@/app/pageLifecycle";

beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  cleanup();
  vi.clearAllTimers();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

async function settle(milliseconds = 50) {
  await act(() => vi.advanceTimersByTimeAsync(milliseconds));
}

function mountWindow(ready?: (messages: NotificationFeed) => void) {
  let client!: TestClient;
  let messages!: NotificationFeed;
  let renderContent!: (content: ReactNode) => void;
  const presentation = (children?: ReactNode) => (
    <client.Wrapper>
      {children}
      <AppToaster />
    </client.Wrapper>
  );
  const view = render(
    <WindowNotifications
      start={(feed, update) => {
        messages = feed;
        client = createTestClient({ messages });
        onTestFinished(() => client.dispose());
        renderContent = (children) => update(presentation(children));
        ready?.(feed);
        renderContent(undefined);
        return () => {};
      }}
    />,
  );
  return { ...view, messages, update: renderContent };
}

function pagehide(persisted: boolean) {
  const event = new Event("pagehide");
  Object.defineProperty(event, "persisted", { value: persisted });
  window.dispatchEvent(event);
}

it("publishes through the ready provider and keeps all six live messages readable", async () => {
  mountWindow((messages) => {
    for (let index = 0; index < 6; index++)
      messages.show({
        level: "info",
        message: `Long feedback ${index}`,
        description: "Full details and the recovery action remain available. ".repeat(10),
        lifetime: "persistent",
      });
  });
  await settle();
  expect(screen.getAllByRole("listitem")).toHaveLength(6);
  expect(screen.getAllByRole("dialog")).toHaveLength(6);
  expect(screen.getAllByRole("button", { name: "Dismiss" })).toHaveLength(6);
  for (const message of screen.getAllByRole("dialog")) {
    expect(message.closest("[inert]")).toBeNull();
    expect(message.textContent).toContain("Full details and the recovery action remain available.");
  }
});

it("keeps the four-second timer and input through layout changes", async () => {
  const view = mountWindow();
  act(() => view.update(<textarea aria-label="Draft" defaultValue="Keep this draft." />));
  const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Draft" });
  act(() => {
    draft.focus();
    draft.setSelectionRange(5, 10);
    view.messages.show({ level: "success", message: "Saved" });
  });
  await settle();
  const list = screen.getByRole("list");
  act(() =>
    view.update(
      <textarea style={{ height: 360 }} aria-label="Draft" defaultValue="Keep this draft." />,
    ),
  );
  expect(screen.getByRole("list")).toBe(list);
  expect(screen.getByRole("textbox", { name: "Draft" })).toBe(draft);
  expect(document.activeElement).toBe(draft);
  expect([draft.selectionStart, draft.selectionEnd]).toEqual([5, 10]);
  await settle(3800);
  expect(screen.getByText("Saved")).toBeTruthy();
  await settle(600);
  expect(screen.queryByText("Saved")).toBeNull();
  expect(document.activeElement).toBe(draft);
});

it("keeps history independent of explicit action and Toast dismissal", async () => {
  const run = vi.fn();
  const view = mountWindow();
  act(() => {
    view.messages.notify("info", "Recorded history");
    view.messages.show({
      level: "info",
      message: "Read result",
      lifetime: "persistent",
      action: { label: "Open result", run },
    });
  });
  await settle();
  const open = screen.getByRole("button", { name: "Open result" });
  expect(open.tabIndex).toBe(0);
  fireEvent.click(open);
  await settle(250);
  expect(run).toHaveBeenCalledOnce();
  expect(screen.queryByText("Read result")).toBeNull();
  const dismiss = screen.getByRole("button", { name: "Dismiss" });
  expect(dismiss.tabIndex).toBe(0);
  fireEvent.click(dismiss);
  await settle(250);
  expect(screen.queryByText("Recorded history")).toBeNull();
  expect(view.messages.snapshot()).toMatchObject([{ message: "Recorded history" }]);
  expect(view.messages.snapshot()[0]?.dismissed).not.toBe(true);
});

it("keeps persistent errors actionable past the ordinary timeout", async () => {
  const view = mountWindow();
  act(() =>
    view.messages.show({
      level: "error",
      message: "Could not open link",
      description: "https://example.test",
      lifetime: "persistent",
    }),
  );
  await settle(10_000);
  expect(screen.getByRole("dialog", { name: "Could not open link" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
  await settle(250);
  expect(screen.queryByText("Could not open link")).toBeNull();
});

it("reveals idle feedback and preserves a focused reader's scroll", async () => {
  const view = mountWindow();
  act(() =>
    view.messages.show({ level: "info", message: "First message", lifetime: "persistent" }),
  );
  await settle();
  const list = screen.getByRole("list");
  list.scrollTop = 120;
  act(() =>
    view.messages.show({ level: "info", message: "New idle message", lifetime: "persistent" }),
  );
  await settle();
  expect(list.scrollTop).toBe(0);
  act(() => list.focus());
  list.scrollTop = 80;
  act(() =>
    view.messages.show({
      level: "info",
      message: "New message while reading",
      lifetime: "persistent",
    }),
  );
  await settle();
  expect(screen.getByRole("list")).toBe(list);
  expect(list.scrollTop).toBe(80);
  expect(document.activeElement).toBe(list);
});

it("preserves capability until the page actually retires", async () => {
  const view = mountWindow();
  const unbind = bindClientPageLifecycle(window, view.unmount);
  onTestFinished(unbind);
  act(() => {
    window.dispatchEvent(new Event("beforeunload", { cancelable: true }));
    pagehide(true);
    document.dispatchEvent(new Event("visibilitychange"));
    view.messages.notify("info", "Still active");
  });
  await settle();
  expect(screen.getByText("Still active")).toBeTruthy();
  act(() => pagehide(false));
  view.messages.notify("error", "Late feedback");
  expect(view.messages.snapshot()).toHaveLength(1);
  expect(screen.queryByText("Still active")).toBeNull();
  expect(screen.queryByText("Late feedback")).toBeNull();
});
