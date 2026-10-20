import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { UserMessageFold } from "@/features/chat/message/ui/UserMessageFold";
function stubHeights(scrollHeight: number, clientHeight: number) {
  vi.spyOn(HTMLElement.prototype, "scrollHeight", "get").mockReturnValue(scrollHeight);
  vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockReturnValue(clientHeight);
}

afterEach(() => vi.restoreAllMocks());

describe("UserMessageFold", () => {
  it("leaves a message that fits exactly as it was", () => {
    stubHeights(100, 100);
    render(<UserMessageFold>short ask</UserMessageFold>);
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("folds a message taller than its budget and opens it on request", () => {
    stubHeights(900, 160);
    const { container } = render(<UserMessageFold>a very long paste</UserMessageFold>);
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
    const { container } = render(
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
});
