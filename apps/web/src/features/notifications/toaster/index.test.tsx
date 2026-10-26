import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { AppToaster } from "@/features/notifications/toaster/index";

beforeEach(() => vi.useFakeTimers());

afterEach(() => {
  cleanup();
  toast.dismiss();
  vi.clearAllTimers();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

async function settle(milliseconds = 50) {
  await act(() => vi.advanceTimersByTimeAsync(milliseconds));
}

it("keeps every live message readable in one bounded normal-flow list", async () => {
  testClient.render(<AppToaster />);
  act(() => {
    for (let index = 0; index < 6; index++) {
      toast(`Long feedback ${index}`, {
        description: "Full details and the recovery action remain available. ".repeat(10),
        duration: Infinity,
      });
    }
  });
  await settle();

  const list = screen.getByRole("list");
  expect(list.style.position).toBe("relative");
  expect(list.style.overflowY).toBe("auto");
  expect(list.style.maxHeight).not.toBe("");
  const messages = screen.getAllByRole("listitem");
  expect(messages).toHaveLength(6);
  for (const message of messages) {
    expect(message.getAttribute("data-visible")).toBe("true");
    expect(message.getAttribute("data-expanded")).toBe("true");
    expect(message.style.position).toBe("relative");
    expect(message.style.flexShrink).toBe("0");
  }
});

it("retains the original input and notification timer through layout changes", async () => {
  const autoClose = vi.fn();
  const view = testClient.render(
    <div>
      <textarea aria-label="Draft" defaultValue="Keep this draft." />
      <AppToaster />
    </div>,
  );
  const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Draft" });
  act(() => {
    draft.focus();
    draft.setSelectionRange(5, 10);
    toast.success("Saved", { onAutoClose: autoClose });
  });
  await settle();
  const list = screen.getByRole("list");
  view.rerender(
    <div style={{ height: 360 }}>
      <textarea aria-label="Draft" defaultValue="Keep this draft." />
      <AppToaster />
    </div>,
  );
  expect(screen.getByRole("list")).toBe(list);
  expect(screen.getByRole("textbox", { name: "Draft" })).toBe(draft);
  expect(document.activeElement).toBe(draft);
  expect([draft.selectionStart, draft.selectionEnd]).toEqual([5, 10]);
  await settle(3800);
  expect(screen.getByText("Saved")).toBeTruthy();
  expect(autoClose).not.toHaveBeenCalled();
  await settle(600);
  expect(screen.queryByText("Saved")).toBeNull();
  expect(autoClose).toHaveBeenCalledOnce();
  expect(document.activeElement).toBe(draft);
});

it("keeps Sonner action and explicit dismissal outcomes", async () => {
  const action = vi.fn();
  const dismissed = vi.fn();
  testClient.render(<AppToaster />);
  act(() => {
    toast("Read the result", {
      duration: Infinity,
      action: { label: "Open result", onClick: action },
    });
  });
  await settle();
  const open = screen.getByRole("button", { name: "Open result" });
  expect(open.getAttribute("tabindex")).toBe("0");
  fireEvent.click(open);
  await settle(250);
  expect(action).toHaveBeenCalledOnce();
  expect(screen.queryByText("Read the result")).toBeNull();

  act(() => {
    toast("Dismiss this result", { duration: Infinity, closeButton: true, onDismiss: dismissed });
  });
  await settle();
  const dismiss = screen.getByRole("button", { name: "Dismiss" });
  expect(dismiss.getAttribute("tabindex")).toBe("0");
  fireEvent.click(dismiss);
  await settle(250);
  expect(dismissed).toHaveBeenCalledOnce();
  expect(screen.queryByText("Dismiss this result")).toBeNull();
});

it("reveals new feedback while idle and preserves a focused reader's scroll", async () => {
  testClient.render(<AppToaster />);
  act(() => toast("First message", { duration: Infinity }));
  await settle();
  const list = screen.getByRole("list");
  list.scrollTop = 120;
  act(() => toast("New idle message", { duration: Infinity }));
  await settle();
  expect(list.scrollTop).toBe(0);

  act(() => list.focus());
  list.scrollTop = 80;
  act(() => toast("New message while reading", { duration: Infinity }));
  await settle();
  expect(screen.getByRole("list")).toBe(list);
  expect(list.scrollTop).toBe(80);
  expect(document.activeElement).toBe(list);
});

it("keeps updated actions tabbable without replacing their message or custom focus choices", async () => {
  testClient.render(<AppToaster />);
  act(() => toast.loading("Working", { id: "updating-result" }));
  await settle();
  const message = screen.getByRole("listitem");

  act(() => {
    toast.success("Ready", {
      id: "updating-result",
      action: { label: "Read updated result", onClick: () => undefined },
    });
  });
  await settle();
  expect(screen.getByRole("listitem")).toBe(message);
  expect(screen.getByRole("button", { name: "Read updated result" }).getAttribute("tabindex")).toBe(
    "0",
  );

  act(() => {
    toast("Custom feedback", {
      duration: Infinity,
      action: (
        <button data-button tabIndex={-1}>
          Custom action
        </button>
      ),
    });
  });
  await settle();
  expect(screen.getByRole("button", { name: "Custom action" }).getAttribute("tabindex")).toBe("-1");
});
