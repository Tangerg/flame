import { act, fireEvent, render, screen } from "@testing-library/react";
import { createRef } from "react";
import { describe, expect, it, vi } from "vitest";
import { RpcError, RpcTransportError } from "@flame/runtime-client";
import { DataError } from "@/ui/atoms/data-error";
import { DataView } from "@/ui/atoms/data-view";
import { TreePrimitive } from "@/ui/primitives/tree";
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

  it("hands a focused Retry to its nearest host before the error gives way", () => {
    const focusAtRetry: (Element | null)[] = [];
    const onRetry = vi.fn(() => {
      focusAtRetry.push(document.activeElement);
    });
    const panel = (state: { isLoading: boolean; failure?: unknown; items?: string[] }) => (
      <div role="group" tabIndex={-1} aria-label="Panel">
        <div role="group" tabIndex={-1} aria-label="Reader">
          <div>
            <DataView
              items={state.items}
              isLoading={state.isLoading}
              failure={state.failure}
              onRetry={onRetry}
            >
              {() => <input aria-label="Recovered filter" ref={(node) => node?.focus()} />}
            </DataView>
          </div>
        </div>
      </div>
    );
    const view = render(panel({ isLoading: false, failure: broke }));
    const outer = screen.getByLabelText("Panel");
    const reader = screen.getByLabelText("Reader");
    const focusPanel = vi.spyOn(outer, "focus");
    const focusReader = vi.spyOn(reader, "focus");
    const retry = screen.getByRole("button", { name: "Retry" });
    act(() => retry.focus());

    fireEvent.click(retry);
    expect(onRetry).toHaveBeenCalledOnce();
    expect(focusAtRetry).toEqual([reader]);
    expect(focusReader).toHaveBeenCalledExactlyOnceWith({ preventScroll: true });
    expect(focusPanel).not.toHaveBeenCalled();

    view.rerender(panel({ isLoading: true }));
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
    expect(document.activeElement).toBe(reader);

    view.rerender(panel({ isLoading: false, items: ["recovered"] }));
    expect(document.activeElement).toBe(screen.getByLabelText("Recovered filter"));
  });

  it("leaves focus Retry does not own with its owner", () => {
    const onRetry = vi.fn();
    render(
      <>
        <button>Another control</button>
        <div role="group" tabIndex={-1} aria-label="Reader">
          <DataView items={undefined} isLoading={false} failure={broke} onRetry={onRetry}>
            {rows}
          </DataView>
        </div>
      </>,
    );
    const focusReader = vi.spyOn(screen.getByLabelText("Reader"), "focus");
    const other = screen.getByRole("button", { name: "Another control" });
    act(() => other.focus());

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalledOnce();
    expect(document.activeElement).toBe(other);
    expect(focusReader).not.toHaveBeenCalled();
  });

  it("retries without a surviving host and keeps the focus its callback claims", () => {
    const onRetry = vi.fn(() => screen.getByRole("textbox", { name: "Message" }).focus());
    render(
      <>
        <textarea aria-label="Message" />
        <DataView items={undefined} isLoading={false} failure={broke} onRetry={onRetry}>
          {rows}
        </DataView>
      </>,
    );
    const retry = screen.getByRole("button", { name: "Retry" });
    act(() => retry.focus());

    fireEvent.click(retry);
    expect(onRetry).toHaveBeenCalledOnce();
    expect(document.activeElement).toBe(screen.getByRole("textbox", { name: "Message" }));
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

describe("DataError", () => {
  it("hands owned Retry focus to its caller's surviving sibling composite", () => {
    const target = createRef<HTMLDivElement>();
    const callbackFocus: (Element | null)[] = [];
    render(
      <>
        <TreePrimitive.Root ref={target} aria-label="Recovered files">
          <TreePrimitive.Item level={1} selected={false} focusable onActivate={vi.fn()}>
            kept.go
          </TreePrimitive.Item>
        </TreePrimitive.Root>
        <div role="region" aria-label="Read error" tabIndex={-1}>
          <DataError
            focusTarget={target}
            onRetry={() => callbackFocus.push(document.activeElement)}
          />
        </div>
        <button>Another task</button>
      </>,
    );
    const retry = screen.getByRole("button", { name: "Retry" });
    const row = screen.getByRole("treeitem", { name: "kept.go" });
    act(() => retry.focus());
    fireEvent.click(retry);
    expect(document.activeElement).toBe(row);

    const outside = screen.getByRole("button", { name: "Another task" });
    act(() => outside.focus());
    fireEvent.click(retry);
    expect(document.activeElement).toBe(outside);
    expect(callbackFocus).toEqual([row, outside]);
  });

  it("classifies an unsupported call with its caller's copy and offers no retry", () => {
    const { container } = render(
      <DataError
        failure={notImplemented}
        unsupported={{ icon: "folder", sub: "The last files remain.", size: "compact" }}
        onRetry={vi.fn()}
        title="Couldn't refresh"
        size="compact"
      />,
    );

    expect(screen.getByText("Not supported by this runtime")).toBeTruthy();
    expect(screen.getByText("The last files remain.")).toBeTruthy();
    expect(screen.queryByText("Couldn't refresh")).toBeNull();
    expect(container.querySelector('[data-icon-name="folder"]')).toBeTruthy();
    expect(screen.queryByRole("button")).toBeNull();
  });

  it.each([
    { name: "no failure", failure: undefined },
    { name: "a transport failure", failure: new RpcTransportError("not found", 404) },
  ])("keeps generic recovery for $name", ({ failure }) => {
    const onRetry = vi.fn();
    const { container } = render(
      <DataError failure={failure} unsupported={{ title: "Not supported" }} onRetry={onRetry} />,
    );

    expect(screen.getByText("Couldn't load")).toBeTruthy();
    expect(screen.queryByText("Not supported")).toBeNull();
    expect(container.querySelector('[data-icon-name="alert"]')).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalledOnce();
  });
});
