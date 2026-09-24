import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { UserMessageFold } from "./UserMessageFold";

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
});
