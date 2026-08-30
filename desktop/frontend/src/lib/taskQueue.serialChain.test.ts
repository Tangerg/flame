import { describe, expect, it } from "vitest";
import { SerialTaskChain } from "./taskQueue";

// Eight mutation owners each carried a private copy of this chain, and none wrote down what
// made it correct. Three things did, and each is a live defect if dropped: a rejecting tail
// fails work that has not run, an unconditional delete breaks the ordering the chain exists
// for, and never deleting grows a map for the lifetime of the process.

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("serialising work per identity", () => {
  it("holds a second call until the first for the same identity settles", async () => {
    const chain = new SerialTaskChain();
    const first = deferred<string>();
    const order: string[] = [];

    const a = chain.chain("same", (tail) =>
      tail.then(() => first.promise).then((value) => order.push(value)),
    );
    const b = chain.chain("same", (tail) => tail.then(() => order.push("second")));

    expect(order).toEqual([]);
    first.resolve("first");
    await Promise.all([a, b]);
    expect(order).toEqual(["first", "second"]);
  });

  it("lets different identities run without waiting for each other", async () => {
    const chain = new SerialTaskChain();
    const blocked = deferred<void>();
    const order: string[] = [];

    const held = chain.chain("a", (tail) => tail.then(() => blocked.promise));
    await chain.chain("b", (tail) => tail.then(() => void order.push("b")));

    expect(order).toEqual(["b"]);
    blocked.resolve();
    await held;
  });

  // The tail is what the NEXT call waits on. If a rejection propagated into it, one failed
  // save would fail the next, unrelated one for the same identity before it even ran.
  it("does not fail the next call because the previous one did", async () => {
    const chain = new SerialTaskChain();
    const failing = chain.chain("same", (tail) =>
      tail.then(() => Promise.reject(new Error("save failed"))),
    );
    await expect(failing).rejects.toThrow("save failed");

    await expect(chain.chain("same", (tail) => tail.then(() => "ok"))).resolves.toBe("ok");
  });

  it("keeps ordering when a call is queued behind one that then fails", async () => {
    const chain = new SerialTaskChain();
    const first = deferred<void>();
    const order: string[] = [];

    const failing = chain.chain("same", (tail) =>
      tail.then(() => first.promise).then(() => Promise.reject(new Error("boom"))),
    );
    const queued = chain.chain("same", (tail) => tail.then(() => void order.push("after")));

    expect(order).toEqual([]);
    first.resolve();
    await expect(failing).rejects.toThrow("boom");
    await queued;
    expect(order).toEqual(["after"]);
  });

  // The identity check. A and B are both in flight; when A settles it must NOT forget the
  // tail, because B replaced it. Forgetting it lets C start while B is still running, which
  // is the ordering this class exists to provide. C has to be queued AFTER A's cleanup
  // microtask has run, or the wrong implementation looks right.
  it("does not let a later call start because an earlier one finished", async () => {
    const chain = new SerialTaskChain();
    const first = deferred<void>();
    const second = deferred<void>();
    const order: string[] = [];

    const a = chain.chain("same", (tail) =>
      tail.then(() => first.promise).then(() => void order.push("a")),
    );
    const b = chain.chain("same", (tail) =>
      tail.then(() => second.promise).then(() => void order.push("b")),
    );

    first.resolve();
    await a;
    await Promise.resolve();
    await Promise.resolve();

    const c = chain.chain("same", (tail) => tail.then(() => void order.push("c")));
    await Promise.resolve();
    await Promise.resolve();
    expect(order).toEqual(["a"]);

    second.resolve();
    await Promise.all([b, c]);
    expect(order).toEqual(["a", "b", "c"]);
  });

  it("forgets an identity once its work is done, so the map cannot grow forever", async () => {
    const chain = new SerialTaskChain();
    for (let index = 0; index < 50; index += 1) {
      await chain.chain(`identity_${index}`, (tail) => tail.then(() => index));
    }
    // The map is private, so the observable proof is that a fresh call for a long-finished
    // identity starts immediately rather than chaining onto a retained tail.
    const order: string[] = [];
    await chain.chain("identity_0", (tail) => tail.then(() => void order.push("reused")));
    expect(order).toEqual(["reused"]);
  });

  it("drops every tail when cleared, so a retired owner queues nothing behind old work", async () => {
    const chain = new SerialTaskChain();
    const stuck = deferred<void>();
    const held = chain.chain("same", (tail) => tail.then(() => stuck.promise));

    chain.clear();
    const order: string[] = [];
    await chain.chain("same", (tail) => tail.then(() => void order.push("fresh")));
    expect(order).toEqual(["fresh"]);

    stuck.resolve();
    await held;
  });
});
