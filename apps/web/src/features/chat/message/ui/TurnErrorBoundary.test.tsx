import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { fireEvent, screen } from "@testing-library/react";
import { useLayoutEffect, useRef, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TurnErrorBoundary } from "@/features/chat/message/ui/TurnErrorBoundary";

beforeEach(() => {
  vi.spyOn(console, "error").mockImplementation(() => {});
});

afterEach(() => {
  vi.restoreAllMocks();
});

function focusRetry() {
  const retry = screen.getByRole("button", { name: /retry/i });
  retry.focus();
  expect(document.activeElement).toBe(retry);
  return retry;
}

describe("turn retry focus", () => {
  it("returns focus to the nearest retained reader when the turn recovers", () => {
    let fail = true;
    function Turn(): ReactNode {
      if (fail) throw new Error("turn failed");
      return <p>Recovered message</p>;
    }
    testClient.render(
      <div role="region" aria-label="Workbench" tabIndex={-1}>
        <div role="region" aria-label="Conversation" tabIndex={-1}>
          <TurnErrorBoundary resetKey="message">
            <Turn />
          </TurnErrorBoundary>
        </div>
      </div>,
    );

    expect(console.error).toHaveBeenCalledWith(
      "[turn-error-boundary]:",
      expect.objectContaining({ message: "turn failed" }),
      expect.stringContaining("Turn"),
    );
    const retry = focusRetry();
    fail = false;
    fireEvent.click(retry);

    expect(screen.getByText("Recovered message")).toBeTruthy();
    expect(document.activeElement).toBe(screen.getByRole("region", { name: "Conversation" }));
  });

  it("keeps the reader focused when the turn fails again", () => {
    function Turn(): ReactNode {
      throw new Error("turn failed");
    }
    testClient.render(
      <div role="region" aria-label="Child task" tabIndex={-1}>
        <TurnErrorBoundary resetKey="message">
          <Turn />
        </TurnErrorBoundary>
      </div>,
    );

    fireEvent.click(focusRetry());

    expect(screen.getByRole("alert")).toBeTruthy();
    expect(screen.getByRole("button", { name: /retry/i })).toBeTruthy();
    expect(document.activeElement).toBe(screen.getByRole("region", { name: "Child task" }));
  });

  it("preserves focus already moved outside the retiring failure", () => {
    let fail = true;
    function Turn(): ReactNode {
      if (fail) throw new Error("turn failed");
      return <p>Recovered message</p>;
    }
    testClient.render(
      <>
        <div role="region" aria-label="Conversation" tabIndex={-1}>
          <TurnErrorBoundary resetKey="message">
            <Turn />
          </TurnErrorBoundary>
        </div>
        <button type="button">Elsewhere</button>
      </>,
    );

    const retry = focusRetry();
    const elsewhere = screen.getByRole("button", { name: "Elsewhere" });
    elsewhere.focus();
    fail = false;
    fireEvent.click(retry);

    expect(screen.getByText("Recovered message")).toBeTruthy();
    expect(document.activeElement).toBe(elsewhere);
  });

  it("lets recovered content choose its own focus", () => {
    let fail = true;
    function Turn(): ReactNode {
      const action = useRef<HTMLButtonElement>(null);
      useLayoutEffect(() => action.current?.focus(), []);
      if (fail) throw new Error("turn failed");
      return (
        <button ref={action} type="button">
          Recovered action
        </button>
      );
    }
    testClient.render(
      <div role="region" aria-label="Conversation" tabIndex={-1}>
        <TurnErrorBoundary resetKey="message">
          <Turn />
        </TurnErrorBoundary>
      </div>,
    );

    const retry = focusRetry();
    fail = false;
    fireEvent.click(retry);

    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Recovered action" }));
  });
});

it("keeps a failure until the message identity changes", () => {
  function Turn({ fail }: { fail: boolean }): ReactNode {
    if (fail) throw new Error("turn failed");
    return <p>Recovered message</p>;
  }
  const view = (resetKey: string, fail: boolean) => (
    <TurnErrorBoundary resetKey={resetKey}>
      <Turn fail={fail} />
    </TurnErrorBoundary>
  );
  const { rerender } = testClient.render(view("failed-message", true));

  rerender(view("failed-message", false));
  expect(screen.getByRole("alert")).toBeTruthy();
  expect(screen.queryByText("Recovered message")).toBeNull();

  rerender(view("next-message", false));
  expect(screen.queryByRole("alert")).toBeNull();
  expect(screen.getByText("Recovered message")).toBeTruthy();
});
