import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { RpcError, RpcTransportError } from "@/rpc/errors";
import { DataView } from "./data-view";

const rows = (items: string[]) => <div>{items.join(",")}</div>;

const broke = new Error("the runtime answered badly");
const notImplemented = new RpcError({
  message: "method not found",
  data: { type: "method_not_found" },
});

describe("DataView", () => {
  it("keeps recovery available when the HTTP endpoint is unavailable", () => {
    const onRetry = vi.fn();
    render(
      <DataView
        items={undefined}
        isLoading={false}
        failure={new RpcTransportError("not found", 404)}
        unsupported={{ title: "Not supported" }}
        onRetry={onRetry}
      >
        {rows}
      </DataView>,
    );

    expect(screen.queryByText("Not supported")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it("recovers from an error rather than dead-ending on it", () => {
    const onRetry = vi.fn();
    render(
      <DataView items={undefined} isLoading={false} failure={broke} onRetry={onRetry}>
        {rows}
      </DataView>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it("keeps the failure glyph even when the caller renames the failure", () => {
    const { container } = render(
      <DataView
        items={[]}
        isLoading={false}
        failure={broke}
        error={{ title: "Couldn't load the diff" }}
      >
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
        failure={notImplemented}
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
      <DataView items={undefined} isLoading={false} failure={broke}>
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

  it("tells a Runtime that broke from one that never had the call", () => {
    const onRetry = vi.fn();
    const view = render(
      <DataView items={undefined} isLoading={false} failure={broke} onRetry={onRetry}>
        {rows}
      </DataView>,
    );
    expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy();

    view.rerender(
      <DataView items={undefined} isLoading={false} failure={notImplemented} onRetry={onRetry}>
        {rows}
      </DataView>,
    );
    expect(
      screen.queryByRole("button", { name: "Retry" }),
      "a call the Runtime does not have cannot be tried again",
    ).toBeNull();
  });
});
