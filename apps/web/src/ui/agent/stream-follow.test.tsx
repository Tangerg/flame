import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import {
  StreamFollowProvider,
  usePublishStreamFollow,
  useStreamFollow,
  type StreamFollow,
} from "./stream-follow";

function controls(): StreamFollow {
  return {
    isAtBottom: true,
    state: { isAtBottom: true, targetScrollTop: 100 },
    scrollRef: Object.assign(() => {}, { current: null as HTMLElement | null }),
    contentRef: Object.assign(() => {}, { current: null as HTMLElement | null }),
    stopScroll: vi.fn(),
    scrollToBottom: vi.fn(() => true),
  };
}

function Stream({ follow }: { follow: StreamFollow }) {
  usePublishStreamFollow(follow);
  return null;
}

function Consumer({ name }: { name: string }) {
  const follow = useStreamFollow();
  return (
    <button disabled={!follow} onClick={() => void follow?.scrollToBottom()}>
      {name}: {follow?.isAtBottom ? "following" : "reading"}
    </button>
  );
}

describe("Conversation stream follow", () => {
  it("keeps each Conversation's controls with its own live stream", () => {
    const first = controls();
    const second = controls();
    const content = (showFirst: boolean) => (
      <>
        <StreamFollowProvider>
          {showFirst && <Stream follow={first} />}
          <Consumer name="First" />
        </StreamFollowProvider>
        <StreamFollowProvider>
          <Stream follow={second} />
          <Consumer name="Second" />
        </StreamFollowProvider>
      </>
    );
    const view = render(content(true));
    fireEvent.click(screen.getByRole("button", { name: "First: following" }));
    expect(first.scrollToBottom).toHaveBeenCalledOnce();
    expect(second.scrollToBottom).not.toHaveBeenCalled();
    view.rerender(content(false));
    expect(
      (screen.getByRole("button", { name: "First: reading" }) as HTMLButtonElement).disabled,
    ).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "Second: following" }));
    expect(second.scrollToBottom).toHaveBeenCalledOnce();
  });

  it("publishes actual controller updates and routes the next action to its replacement", () => {
    const first = controls();
    const replacement = { ...controls(), isAtBottom: false };
    const content = (follow: StreamFollow) => (
      <StreamFollowProvider>
        <Stream follow={follow} />
        <Consumer name="Conversation" />
      </StreamFollowProvider>
    );
    const view = render(content(first));
    view.rerender(content(replacement));
    fireEvent.click(screen.getByRole("button", { name: "Conversation: reading" }));
    expect(replacement.scrollToBottom).toHaveBeenCalledOnce();
    expect(first.scrollToBottom).not.toHaveBeenCalled();
  });
});
