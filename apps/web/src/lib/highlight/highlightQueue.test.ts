import { expect, it, vi } from "vitest";
import { createHighlightQueue } from "./highlightQueue";
import type { HighlightRequest, HighlightResponse } from "./shiki.worker";

function transport() {
  const worker = {
    postMessage: vi.fn<(request: HighlightRequest) => void>(),
    terminate: vi.fn(),
    onmessage: null as ((event: MessageEvent<HighlightResponse>) => void) | null,
    onerror: null as ((event: ErrorEvent) => void) | null,
  };
  return {
    worker: worker as unknown as Worker,
    sent: worker.postMessage,
    terminate: worker.terminate,
    complete(index: number, failure = false) {
      const request = worker.postMessage.mock.calls[index]![0];
      const data: HighlightResponse = failure
        ? {
            id: request.id,
            status: "failed",
            error: { message: "grammar failed", details: "original stack and cause" },
          }
        : { id: request.id, status: "ready", html: request.code, lang: request.lang, duration: 1 };
      worker.onmessage?.(new MessageEvent("message", { data }));
    },
    fail(error: Error) {
      worker.onerror?.(new ErrorEvent("error", { error }));
    },
  };
}
const input = (code: string) => ({ code, lang: "typescript", theme: "github-dark" });
const signal = () => new AbortController().signal;
const loadLanguage = async () => [];

it("keeps one in-flight job and only the latest waiting material per reader", async () => {
  const wire = transport();
  const create = vi.fn(async () => wire.worker);
  const queue = createHighlightQueue(create, loadLanguage);
  const first = queue.request(Symbol(), input("first"), signal());
  await vi.waitFor(() => expect(wire.sent).toHaveBeenCalledTimes(1));
  const reader = Symbol();
  const old = queue.request(reader, input("outdated"), signal());
  const retiring = (async () => {
    await expect(old).rejects.toMatchObject({ name: "AbortError" });
  })();
  const latest = queue.request(reader, input("latest"), signal());
  await retiring;
  expect(wire.sent).toHaveBeenCalledTimes(1);
  wire.complete(0);
  expect(await first).toMatchObject({ html: "first" });
  await vi.waitFor(() => expect(wire.sent).toHaveBeenCalledTimes(2));
  expect(wire.sent.mock.calls[1]![0].code).toBe("latest");
  wire.complete(1);
  expect(await latest).toMatchObject({ html: "latest" });
  expect(create).toHaveBeenCalledOnce();
  queue.dispose();
});

it("retires an active result without starting another CPU job until its reply", async () => {
  const wire = transport();
  const queue = createHighlightQueue(async () => wire.worker, loadLanguage);
  const reader = Symbol();
  const owner = new AbortController();
  const old = queue.request(reader, input("old"), owner.signal);
  const retiring = (async () => {
    await expect(old).rejects.toMatchObject({ name: "AbortError" });
  })();
  await vi.waitFor(() => expect(wire.sent).toHaveBeenCalledOnce());
  owner.abort();
  await retiring;
  const latest = queue.request(reader, input("new"), signal());
  expect(wire.sent).toHaveBeenCalledOnce();
  wire.complete(0);
  await vi.waitFor(() => expect(wire.sent).toHaveBeenCalledTimes(2));
  wire.complete(1);
  expect(await latest).toMatchObject({ html: "new" });
  queue.dispose();
});

it("removes a retired waiting reader instead of sending its material", async () => {
  const wire = transport();
  const queue = createHighlightQueue(async () => wire.worker, loadLanguage);
  const first = queue.request(Symbol(), input("first"), signal());
  await vi.waitFor(() => expect(wire.sent).toHaveBeenCalledOnce());
  const owner = new AbortController();
  const waiting = queue.request(Symbol(), input("retired"), owner.signal);
  const retiring = (async () => {
    await expect(waiting).rejects.toMatchObject({ name: "AbortError" });
  })();
  owner.abort();
  await retiring;
  wire.complete(0);
  await first;
  expect(wire.sent).toHaveBeenCalledOnce();
  queue.dispose();
});

it("preserves grammar failure diagnostics and then continues another reader", async () => {
  const wire = transport();
  const queue = createHighlightQueue(async () => wire.worker, loadLanguage);
  const first = queue.request(Symbol(), input("invalid"), signal());
  const rejected = (async () => {
    await expect(first).rejects.toMatchObject({
      message: "grammar failed",
      cause: "original stack and cause",
    });
  })();
  const next = queue.request(Symbol(), input("valid"), signal());
  await vi.waitFor(() => expect(wire.sent).toHaveBeenCalledOnce());
  wire.complete(0, true);
  await rejected;
  await vi.waitFor(() => expect(wire.sent).toHaveBeenCalledTimes(2));
  wire.complete(1);
  await next;
  queue.dispose();
});

it("fails active and waiting requests on a crash and creates a fresh worker", async () => {
  const firstWire = transport();
  const nextWire = transport();
  const create = vi
    .fn()
    .mockResolvedValueOnce(firstWire.worker)
    .mockResolvedValueOnce(nextWire.worker);
  const queue = createHighlightQueue(create, loadLanguage);
  const cause = new Error("worker crashed");
  const first = queue.request(Symbol(), input("first"), signal());
  const waiting = queue.request(Symbol(), input("waiting"), signal());
  const failures = [first, waiting].map(async (request) => {
    await expect(request).rejects.toBe(cause);
  });
  await vi.waitFor(() => expect(firstWire.sent).toHaveBeenCalledOnce());
  firstWire.fail(cause);
  await Promise.all(failures);
  expect(firstWire.terminate).toHaveBeenCalledOnce();
  const next = queue.request(Symbol(), input("fresh"), signal());
  await vi.waitFor(() => expect(nextWire.sent).toHaveBeenCalledOnce());
  firstWire.complete(0);
  nextWire.complete(0);
  expect(await next).toMatchObject({ html: "fresh" });
  queue.dispose();
});

it("terminates a late disposed worker without retiring its successor", async () => {
  const oldCreation = Promise.withResolvers<Worker>();
  const oldWire = transport();
  const nextWire = transport();
  const create = vi
    .fn()
    .mockReturnValueOnce(oldCreation.promise)
    .mockResolvedValueOnce(nextWire.worker);
  const queue = createHighlightQueue(create, loadLanguage);
  const old = queue.request(Symbol(), input("old"), signal());
  const retiring = (async () => {
    await expect(old).rejects.toMatchObject({ name: "AbortError" });
  })();
  await vi.waitFor(() => expect(create).toHaveBeenCalledOnce());
  queue.dispose();
  await retiring;
  const next = queue.request(Symbol(), input("next"), signal());
  await vi.waitFor(() => expect(nextWire.sent).toHaveBeenCalledOnce());
  oldCreation.resolve(oldWire.worker);
  await vi.waitFor(() => expect(oldWire.terminate).toHaveBeenCalledOnce());
  expect(oldWire.sent).not.toHaveBeenCalled();
  nextWire.complete(0);
  expect(await next).toMatchObject({ html: "next" });
  queue.dispose();
});

it("keeps an initialization refusal explicit for every waiting reader", async () => {
  const cause = new Error("worker module could not load");
  const queue = createHighlightQueue(async () => {
    throw cause;
  }, loadLanguage);
  const first = queue.request(Symbol(), input("first"), signal());
  const second = queue.request(Symbol(), input("second"), signal());
  await Promise.all([expect(first).rejects.toBe(cause), expect(second).rejects.toBe(cause)]);
  queue.dispose();
});

it("keeps a grammar data import failure local to its request", async () => {
  const wire = transport();
  const cause = new Error("grammar bytes unavailable");
  const load = vi.fn().mockRejectedValueOnce(cause).mockResolvedValueOnce([]);
  const create = vi.fn(async () => wire.worker);
  const queue = createHighlightQueue(create, load);
  const first = queue.request(Symbol(), input("bad grammar"), signal());
  const rejected = (async () => {
    await expect(first).rejects.toBe(cause);
  })();
  const next = queue.request(Symbol(), input("other grammar"), signal());
  await rejected;
  await vi.waitFor(() => expect(wire.sent).toHaveBeenCalledOnce());
  expect(wire.sent.mock.calls[0]![0]).toMatchObject({ code: "other grammar", grammars: [] });
  wire.complete(0);
  await next;
  expect(create).toHaveBeenCalledOnce();
  queue.dispose();
});
