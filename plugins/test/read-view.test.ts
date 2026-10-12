import assert from "node:assert/strict";
import { test } from "node:test";
import { ReadView } from "../src/read-view.ts";

test("one admitted read preserves the page and cursor on failure", async (t) => {
  const { port1, port2 } = new MessageChannel();
  t.after(() => {
    port1.close();
    port2.close();
  });
  const initial = { data: ["first"], nextCursor: "next" };
  const view = new ReadView(initial, port1);
  const requests: unknown[] = [];
  port2.onmessage = (event) => {
    requests.push(event.data);
    port2.postMessage(
      requests.length === 1
        ? { type: "data", data: { data: ["second"] } }
        : { type: "error", reason: "Read failed. Refresh to retry." },
    );
  };
  function settled() {
    return new Promise<void>((resolve) => {
      const stop = view.subscribe(() => {
        if (view.getSnapshot().pending === undefined) {
          stop();
          resolve();
        }
      });
    });
  }
  view.read(["next"]);
  view.read(["competing"]);
  await settled();
  assert.deepEqual(requests, [{ type: "read", cursor: "next" }]);
  assert.deepEqual(view.getSnapshot(), {
    data: { data: ["second"] },
    history: ["next"],
  });
  view.read([]);
  await settled();
  assert.deepEqual(view.getSnapshot(), {
    data: { data: ["second"] },
    history: ["next"],
    pending: undefined,
    error: "Read failed. Refresh to retry.",
  });
  assert.deepEqual(requests, [
    { type: "read", cursor: "next" },
    { type: "read" },
  ]);
});

test("format rejection cannot replace an accepted read", async (t) => {
  const { port1, port2 } = new MessageChannel();
  t.after(() => {
    port1.close();
    port2.close();
  });
  const validate = (value: { format: string }) => {
    if (value.format !== "fixture.v1")
      throw new Error("Unsupported fixture format.");
  };
  assert.throws(
    () => new ReadView({ format: "wrong" }, port1, validate),
    /Unsupported fixture/,
  );
  const view = new ReadView({ format: "fixture.v1" }, port1, validate);
  const rejected = new Promise<unknown>((resolve) => {
    port2.onmessage = (event) => {
      if (event.data.type === "read")
        port2.postMessage({ type: "data", data: { format: "wrong" } });
      else resolve(event.data);
    };
  });
  view.read();
  assert.deepEqual(await rejected, {
    type: "failure",
    reason: "Unsupported fixture format.",
  });
  assert.equal(view.getSnapshot().data.format, "fixture.v1");
});
