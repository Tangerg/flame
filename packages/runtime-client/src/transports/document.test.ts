import { expect, it, vi } from "vitest";
import { readResponseDocument } from "./document";
it("admits exact-budget documents and observes cancellation while a read waits", async () => {
  await expect(readResponseDocument(new Response("世界"), 6, "test", undefined)).resolves.toBe(
    "世界",
  );
  const cancel = vi.fn();
  const response = new Response(new ReadableStream({ cancel }));
  const owner = new AbortController();
  const waiting = readResponseDocument(response, 6, "test", undefined, owner.signal);
  owner.abort();
  await expect(waiting).rejects.toBe(owner.signal.reason);
  expect(cancel).toHaveBeenCalledOnce();
});
