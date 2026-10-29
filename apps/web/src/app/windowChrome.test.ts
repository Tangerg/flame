import { afterEach, expect, it, vi } from "vitest";
import { applyWindowChrome, watchWindowChrome } from "@/app/windowChrome";
import { type ClientHost } from "@/platform/host";

type Chrome = Awaited<ReturnType<ClientHost["windowChrome"]>>;
const disposals: (() => void)[] = [];
afterEach(async () => {
  for (const dispose of disposals.splice(0)) dispose();
  await applyWindowChrome({ windowChrome: () => Promise.resolve(null) });
  vi.restoreAllMocks();
});

it("publishes only the latest window observation and retires pending reads", async () => {
  const first = Promise.withResolvers<Chrome>();
  const second = Promise.withResolvers<Chrome>();
  const third = Promise.withResolvers<Chrome>();
  const windowChrome = vi
    .fn<() => Promise<Chrome>>()
    .mockReturnValueOnce(first.promise)
    .mockReturnValueOnce(second.promise)
    .mockReturnValueOnce(third.promise);
  const dispose = watchWindowChrome({ windowChrome });
  disposals.push(dispose);
  dispatchEvent(new Event("resize"));
  dispatchEvent(new Event("resize"));
  second.resolve({ controlsInlineEnd: 0, controlsCentreY: 0 });
  await second.promise;
  first.resolve({ controlsInlineEnd: 80, controlsCentreY: 12 });
  await first.promise;
  expect(document.documentElement.style.getPropertyValue("--window-controls-gutter")).toBe("0px");
  dispatchEvent(new Event("resize"));
  dispose();
  third.resolve({ controlsInlineEnd: 120, controlsCentreY: 14 });
  await third.promise;
  expect(document.documentElement.style.getPropertyValue("--window-controls-gutter")).toBe("0px");
});

it("observes failures from a native resize query", async () => {
  const failure = new Error("native channel closed");
  const report = vi.spyOn(console, "error").mockImplementation(() => {});
  disposals.push(watchWindowChrome({ windowChrome: () => Promise.reject(failure) }));
  dispatchEvent(new Event("resize"));
  await vi.waitFor(() =>
    expect(report).toHaveBeenCalledWith("[window] chrome refresh failed:", failure),
  );
});
