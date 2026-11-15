import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
} from "@testing-library/react";
import { Activity, Suspense, type ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import {
  ContentVisibility,
  useCopyAttempt,
  useCopyFeedback,
} from "../dist/index.js";

afterEach(cleanup);

function Copy({
  source,
  capture,
}: {
  source: object;
  capture: (current: () => boolean) => void;
}) {
  const begin = useCopyAttempt(source);
  return (
    <button
      onClick={() => {
        const current = begin();
        if (current) capture(current);
      }}
    >
      Copy
    </button>
  );
}

it("gives only the latest attempt permission to publish and never revives a retired source", () => {
  const first = {};
  const second = {};
  const capture = vi.fn<(current: () => boolean) => void>();
  const view = render(<Copy source={first} capture={capture} />);
  fireEvent.click(screen.getByRole("button"));
  const older = capture.mock.calls[0]![0];
  expect(older()).toBe(true);
  fireEvent.click(screen.getByRole("button"));
  const latest = capture.mock.calls[1]![0];
  expect(older()).toBe(false);
  expect(latest()).toBe(true);
  view.rerender(<Copy source={second} capture={capture} />);
  expect(latest()).toBe(false);
  view.rerender(<Copy source={first} capture={capture} />);
  expect(latest()).toBe(false);
  fireEvent.click(screen.getByRole("button"));
  const returned = capture.mock.calls[2]![0];
  expect(returned()).toBe(true);
  view.unmount();
  expect(returned()).toBe(false);
});

it("retires hidden scope attempts and admits a fresh attempt only after reveal", () => {
  const source = {};
  const capture = vi.fn<(current: () => boolean) => void>();
  const scene = (visible: boolean) => (
    <ContentVisibility visible={visible}>
      <Copy source={source} capture={capture} />
    </ContentVisibility>
  );
  const view = render(scene(true));
  fireEvent.click(screen.getByRole("button"));
  const current = capture.mock.calls[0]![0];
  act(() => view.rerender(scene(false)));
  expect(current()).toBe(false);
  fireEvent.click(screen.getByRole("button"));
  expect(capture).toHaveBeenCalledTimes(1);
  view.rerender(scene(true));
  fireEvent.click(screen.getByRole("button"));
  expect(current()).toBe(false);
  expect(capture.mock.calls[1]![0]()).toBe(true);
});

it.each(["source replacement", "hide and reveal"])(
  "does not revive a captured command after %s",
  (retirement) => {
    const first = {};
    const second = {};
    let visible = true;
    const view = renderHook(({ source }) => useCopyAttempt(source), {
      initialProps: { source: first },
      wrapper: ({ children }: { children: ReactNode }) => (
        <ContentVisibility visible={visible}>{children}</ContentVisibility>
      ),
    });
    const original = view.result.current;
    view.rerender({ source: first });
    expect(original()!()).toBe(true);
    if (retirement === "source replacement") view.rerender({ source: second });
    else {
      visible = false;
      view.rerender({ source: first });
    }
    expect(original()).toBeNull();
    visible = true;
    view.rerender({ source: first });
    const latest = view.result.current()!;
    expect(latest()).toBe(true);
    expect(original()).toBeNull();
    expect(latest()).toBe(true);
  },
);

it("does not write the clipboard through a retired copy callback after its text returns", async () => {
  const write = vi
    .fn<(text: string) => Promise<boolean>>()
    .mockResolvedValue(true);
  const view = renderHook(({ text }) => useCopyFeedback(text, 1500, write), {
    initialProps: { text: "first" },
  });
  const original = view.result.current.copy;
  view.rerender({ text: "second" });
  view.rerender({ text: "first" });
  let result: boolean | undefined;
  await act(async () => {
    result = await original();
  });
  expect(result).toBeUndefined();
  expect(write).not.toHaveBeenCalled();
  await act(async () => {
    expect(await view.result.current.copy()).toBe(true);
  });
  expect(write).toHaveBeenCalledExactlyOnceWith("first");
  expect(view.result.current.copied).toBe(true);
});

it("never revives a captured command when Activity reconnects its effects", () => {
  const source = {};
  let visible = true;
  const view = renderHook(() => useCopyAttempt(source), {
    wrapper: ({ children }: { children: ReactNode }) => (
      <Activity mode={visible ? "visible" : "hidden"}>{children}</Activity>
    ),
  });
  const original = view.result.current;
  const originalAttempt = original()!;
  visible = false;
  view.rerender();
  expect(original()).toBeNull();
  expect(originalAttempt()).toBe(false);
  visible = true;
  view.rerender();
  const latest = view.result.current()!;
  expect(latest()).toBe(true);
  expect(original()).toBeNull();
  expect(originalAttempt()).toBe(false);
  expect(latest()).toBe(true);
});

it("does not write through a copy callback captured before Activity hides", async () => {
  const write = vi
    .fn<(text: string) => Promise<boolean>>()
    .mockResolvedValue(true);
  let visible = true;
  const view = renderHook(
    () => useCopyFeedback("captured source", 1500, write),
    {
      wrapper: ({ children }: { children: ReactNode }) => (
        <Activity mode={visible ? "visible" : "hidden"}>{children}</Activity>
      ),
    },
  );
  const original = view.result.current.copy;
  visible = false;
  view.rerender();
  visible = true;
  view.rerender();
  await act(async () => {
    expect(await original()).toBeUndefined();
  });
  expect(write).not.toHaveBeenCalled();
  await act(async () => {
    expect(await view.result.current.copy()).toBe(true);
  });
  expect(write).toHaveBeenCalledExactlyOnceWith("captured source");
  expect(view.result.current.copied).toBe(true);
});

it("retires copy callbacks across Suspense fallback and reveal", async () => {
  const write = vi
    .fn<(text: string) => Promise<boolean>>()
    .mockResolvedValue(true);
  const pending = new Promise<never>(() => {});
  let suspended = false;
  function Pending(): never {
    throw pending;
  }
  const view = renderHook(
    () => useCopyFeedback("captured source", 1500, write),
    {
      wrapper: ({ children }: { children: ReactNode }) => (
        <Suspense fallback={<p>Waiting for source</p>}>
          {suspended && <Pending />}
          {children}
        </Suspense>
      ),
    },
  );
  const original = view.result.current.copy;
  suspended = true;
  view.rerender();
  expect(screen.getByText("Waiting for source")).toBeTruthy();
  suspended = false;
  view.rerender();
  await act(async () => {
    expect(await original()).toBeUndefined();
  });
  expect(write).not.toHaveBeenCalled();
  await act(async () => {
    expect(await view.result.current.copy()).toBe(true);
  });
  expect(write).toHaveBeenCalledExactlyOnceWith("captured source");
  expect(view.result.current.copied).toBe(true);
});
