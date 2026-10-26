import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, fireEvent, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { UserMessageFold } from "@/features/chat/message/ui/UserMessageFold";
function stubHeights(scrollHeight: number, clientHeight: number) {
  return {
    scrollHeight: vi
      .spyOn(HTMLElement.prototype, "scrollHeight", "get")
      .mockReturnValue(scrollHeight),
    clientHeight: vi
      .spyOn(HTMLElement.prototype, "clientHeight", "get")
      .mockReturnValue(clientHeight),
  };
}

function stubResizeObserver() {
  const callbacks = new Set<ResizeObserverCallback>();
  vi.stubGlobal(
    "ResizeObserver",
    class {
      readonly callback: ResizeObserverCallback;
      constructor(callback: ResizeObserverCallback) {
        this.callback = callback;
      }
      observe() {
        callbacks.add(this.callback);
      }
      unobserve() {}
      disconnect() {
        callbacks.delete(this.callback);
      }
    },
  );
  return () => {
    for (const callback of [...callbacks]) callback([], {} as ResizeObserver);
  };
}

function renderRetainedMessage() {
  return testClient.render(
    <div role="region" aria-label="Conversation" tabIndex={-1}>
      <div role="group" aria-label="User message" tabIndex={-1}>
        <UserMessageFold>
          <p>A retained paragraph</p>
          <a href="https://example.com/guide">Read the guide</a>
        </UserMessageFold>
      </div>
      <button type="button">Elsewhere</button>
    </div>,
  );
}

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("UserMessageFold", () => {
  it("leaves a message that fits exactly as it was", () => {
    stubHeights(100, 100);
    testClient.render(<UserMessageFold>short ask</UserMessageFold>);
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("folds a message taller than its budget and opens it on request", () => {
    stubHeights(900, 160);
    const { container } = testClient.render(<UserMessageFold>a very long paste</UserMessageFold>);
    const toggle = screen.getByRole("button", { name: /show full message/i });
    expect(container.querySelector("[data-folded]")).not.toBeNull();

    fireEvent.click(toggle);
    expect(screen.getByRole("button", { name: /show less/i }).getAttribute("aria-expanded")).toBe(
      "true",
    );
    expect(screen.getByText("a very long paste")).toBeTruthy();
  });

  it("reveals focused content without replacing it and folds back to the beginning", () => {
    stubHeights(900, 160);
    const { container } = testClient.render(
      <UserMessageFold>
        <p>A very long paste</p>
        <a href="https://example.com/guide">Read the guide</a>
      </UserMessageFold>,
    );
    const link = screen.getByRole("link", { name: "Read the guide" });
    const scrollIntoView = vi.spyOn(link, "scrollIntoView").mockImplementation(() => undefined);
    expect(container.querySelector("[data-folded]")).not.toBeNull();

    act(() => link.focus());

    expect(screen.getByRole("button", { name: /show less/i }).getAttribute("aria-expanded")).toBe(
      "true",
    );
    expect(container.querySelector("[data-folded]")).toBeNull();
    expect(screen.getByRole("link", { name: "Read the guide" })).toBe(link);
    expect(document.activeElement).toBe(link);
    expect(scrollIntoView).toHaveBeenCalledWith({ block: "nearest" });

    const frame = container.querySelector<HTMLDivElement>('[data-slot="user-message-fold"]')!;
    frame.scrollTop = 817;
    const toggle = screen.getByRole("button", { name: /show less/i });
    act(() => toggle.focus());
    fireEvent.click(toggle);

    expect(frame.hasAttribute("data-folded")).toBe(true);
    expect(screen.getByRole("button", { name: /show full message/i })).toBe(toggle);
    expect(document.activeElement).toBe(toggle);
    expect(frame.scrollTop).toBe(0);
  });

  it.each(["folded resize", "expanded collapse"] as const)(
    "returns disappearing toggle focus to the nearest message container after %s",
    (transition) => {
      const heights = stubHeights(198, 182);
      const resize = stubResizeObserver();
      const { container } = renderRetainedMessage();
      const message = screen.getByRole("group", { name: "User message" });
      const focus = vi.spyOn(message, "focus");
      const frame = container.querySelector<HTMLDivElement>('[data-slot="user-message-fold"]')!;
      const link = screen.getByRole("link", { name: "Read the guide" });
      const toggle = screen.getByRole("button", { name: "Show full message" });
      if (transition === "expanded collapse") {
        fireEvent.click(toggle);
        frame.scrollTop = 75;
      }
      const toggleName = transition === "expanded collapse" ? "Show less" : "Show full message";
      expect(screen.getByRole("button", { name: toggleName })).toBe(toggle);
      act(() => toggle.focus());
      heights.scrollHeight.mockReturnValue(120);
      heights.clientHeight.mockReturnValue(120);

      if (transition === "expanded collapse") fireEvent.click(toggle);
      else act(resize);

      expect(screen.queryByRole("button", { name: /show full message|show less/i })).toBeNull();
      expect(document.activeElement).toBe(message);
      expect(focus).toHaveBeenCalledWith({ preventScroll: true });
      expect(container.querySelector('[data-slot="user-message-fold"]')).toBe(frame);
      expect(screen.getByRole("link", { name: "Read the guide" })).toBe(link);
      expect(frame.hasAttribute("data-folded")).toBe(false);
      expect(frame.scrollTop).toBe(0);
    },
  );

  it.each(["folded resize", "expanded collapse"] as const)(
    "preserves later outside focus when the toggle disappears after %s",
    (transition) => {
      const heights = stubHeights(198, 182);
      const resize = stubResizeObserver();
      renderRetainedMessage();
      const toggle = screen.getByRole("button", { name: "Show full message" });
      if (transition === "expanded collapse") fireEvent.click(toggle);
      act(() => toggle.focus());
      const elsewhere = screen.getByRole("button", { name: "Elsewhere" });
      act(() => elsewhere.focus());
      heights.scrollHeight.mockReturnValue(120);
      heights.clientHeight.mockReturnValue(120);

      if (transition === "expanded collapse") fireEvent.click(toggle);
      else act(resize);

      expect(screen.queryByRole("button", { name: /show full message|show less/i })).toBeNull();
      expect(document.activeElement).toBe(elsewhere);
    },
  );
});
