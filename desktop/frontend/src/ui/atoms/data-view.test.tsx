import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { DataView } from "./data-view";

const rows = (items: string[]) => <div>{items.join(",")}</div>;

describe("DataView", () => {
  it("recovers from an error rather than dead-ending on it", () => {
    const onRetry = vi.fn();
    render(
      <DataView items={undefined} isLoading={false} isError onRetry={onRetry}>
        {rows}
      </DataView>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalledOnce();
  });

  // Every view that draws a failure with its own glyph draws the same picture as its own
  // empty result, which is how two states came to differ only in wording.
  it("keeps the failure glyph even when the caller renames the failure", () => {
    const { container } = render(
      <DataView items={[]} isLoading={false} isError error={{ title: "Couldn't load the diff" }}>
        {rows}
      </DataView>,
    );

    expect(screen.getByText("Couldn't load the diff")).toBeTruthy();
    expect(container.querySelector('[data-icon-name="alert"]')).toBeTruthy();
  });

  it("offers no retry for a call the Runtime does not implement", () => {
    const onRetry = vi.fn();
    render(
      <DataView
        items={undefined}
        isLoading={false}
        isError
        unsupported={{ icon: "shield", title: "Not supported" }}
        onRetry={onRetry}
      >
        {rows}
      </DataView>,
    );

    expect(screen.getByText("Not supported")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
  });

  it("does not invent a retry the caller has no way to serve", () => {
    render(
      <DataView items={undefined} isLoading={false} isError>
        {rows}
      </DataView>,
    );

    expect(screen.queryByRole("button")).toBeNull();
  });

  it("leaves an empty result to its own icon and no action", () => {
    const onRetry = vi.fn();
    const { container } = render(
      <DataView
        items={[]}
        isLoading={false}
        empty={{ icon: "diff", title: "Nothing to compare" }}
        onRetry={onRetry}
      >
        {rows}
      </DataView>,
    );

    expect(container.querySelector('[data-icon-name="diff"]')).toBeTruthy();
    expect(screen.queryByRole("button")).toBeNull();
  });
});
