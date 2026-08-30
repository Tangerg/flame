import { describe, expect, it } from "vitest";
import { createPushPullChannel } from "./channel";

const unboundedChannel = <T>() => createPushPullChannel<T>({ capacity: "unbounded" });

describe("createPushPullChannel", () => {
  it("tryPush reports bounded saturation without retaining the rejected value", async () => {
    const ch = createPushPullChannel<number>({ capacity: 1 });

    expect(ch.tryPush(1)).toBe(true);
    expect(ch.tryPush(2)).toBe(false);
    await expect(ch.iterator().next()).resolves.toEqual({ value: 1, done: false });
    expect(ch.tryPush(2)).toBe(true);
    await expect(ch.iterator().next()).resolves.toEqual({ value: 2, done: false });
  });

  it("rendezvous send waits until the sole consumer accepts the value", async () => {
    const ch = createPushPullChannel<number>({ capacity: 0 });
    let settled = false;
    const sent = ch.send(42).then((accepted) => {
      settled = true;
      return accepted;
    });

    await Promise.resolve();
    expect(settled).toBe(false);

    const next = ch.iterator().next();
    await expect(next).resolves.toEqual({ value: 42, done: false });
    await expect(sent).resolves.toBe(true);
  });

  it("close releases a rendezvous producer whose value was never accepted", async () => {
    const ch = createPushPullChannel<number>({ capacity: 0 });
    const sent = ch.send(42);

    ch.close();

    await expect(sent).resolves.toBe(false);
    await expect(ch.iterator().next()).resolves.toEqual({ value: undefined, done: true });
  });

  it("fail rejects a rendezvous producer with the channel failure", async () => {
    const ch = createPushPullChannel<number>({ capacity: 0 });
    const sent = ch.send(42);
    const failure = new Error("upstream failed");

    ch.fail(failure);

    await expect(sent).rejects.toBe(failure);
  });

  it("yields pushed values in FIFO order", async () => {
    const ch = unboundedChannel<number>();
    ch.push(1);
    ch.push(2);
    ch.push(3);
    const it = ch.iterator();
    expect(await it.next()).toEqual({ value: 1, done: false });
    expect(await it.next()).toEqual({ value: 2, done: false });
    expect(await it.next()).toEqual({ value: 3, done: false });
  });

  it("blocks next() until a push arrives", async () => {
    const ch = unboundedChannel<string>();
    const it = ch.iterator();
    const pending = it.next();
    let resolved = false;
    void pending.then(() => {
      resolved = true;
    });
    await Promise.resolve();
    expect(resolved).toBe(false);
    ch.push("delayed");
    expect(await pending).toEqual({ value: "delayed", done: false });
  });

  it("resolves concurrent next() calls in FIFO order", async () => {
    const ch = unboundedChannel<string>();
    const it = ch.iterator();
    const first = it.next();
    const second = it.next();

    ch.push("first");
    ch.push("second");

    expect(await first).toEqual({ value: "first", done: false });
    expect(await second).toEqual({ value: "second", done: false });
  });

  it("close() resolves waiting next() with done=true", async () => {
    const ch = unboundedChannel<number>();
    const it = ch.iterator();
    const pending = it.next();
    ch.close();
    expect(await pending).toEqual({ value: undefined, done: true });
  });

  it("close() resolves all waiting next() calls", async () => {
    const ch = unboundedChannel<number>();
    const it = ch.iterator();
    const first = it.next();
    const second = it.next();

    ch.close();

    expect(await first).toEqual({ value: undefined, done: true });
    expect(await second).toEqual({ value: undefined, done: true });
  });

  it("buffered values drain before close-driven done", async () => {
    const ch = unboundedChannel<number>();
    ch.push(10);
    ch.push(20);
    ch.close();
    const it = ch.iterator();
    expect(await it.next()).toEqual({ value: 10, done: false });
    expect(await it.next()).toEqual({ value: 20, done: false });
    expect(await it.next()).toEqual({ value: undefined, done: true });
  });

  it("push after close is silently dropped", async () => {
    const ch = unboundedChannel<number>();
    ch.close();
    ch.push(99);
    const it = ch.iterator();
    expect(await it.next()).toEqual({ value: undefined, done: true });
  });

  it("close() is idempotent", () => {
    const ch = unboundedChannel<number>();
    ch.close();
    ch.close();
    expect(ch.closed).toBe(true);
  });

  it("fail() drains buffered values and then rejects iteration", async () => {
    const ch = unboundedChannel<number>();
    const failure = new Error("upstream stream failed");
    ch.push(10);
    ch.push(20);
    ch.fail(failure);

    const it = ch.iterator();
    expect(await it.next()).toEqual({ value: 10, done: false });
    expect(await it.next()).toEqual({ value: 20, done: false });
    await expect(it.next()).rejects.toBe(failure);
  });

  it("fail() rejects every waiting next() immediately", async () => {
    const ch = unboundedChannel<number>();
    const it = ch.iterator();
    const first = it.next();
    const second = it.next();
    const failure = new Error("connection lost");

    ch.fail(failure);

    await expect(first).rejects.toBe(failure);
    await expect(second).rejects.toBe(failure);
  });

  it("keeps the first terminal state", async () => {
    const failed = unboundedChannel<number>();
    const firstFailure = new Error("first failure");
    failed.fail(firstFailure);
    failed.fail(new Error("second failure"));
    failed.close();
    await expect(failed.iterator().next()).rejects.toBe(firstFailure);

    const closed = unboundedChannel<number>();
    closed.close();
    closed.fail(new Error("too late"));
    expect(await closed.iterator().next()).toEqual({ value: undefined, done: true });
  });

  it("iterator.return() closes the channel", async () => {
    const ch = unboundedChannel<number>();
    const it = ch.iterator();
    expect(ch.closed).toBe(false);
    await it.return!();
    expect(ch.closed).toBe(true);
  });

  it("for-await drains then exits on close", async () => {
    const ch = unboundedChannel<string>();
    ch.push("a");
    ch.push("b");
    setTimeout(() => {
      ch.push("c");
      ch.close();
    }, 0);
    const collected: string[] = [];
    for await (const v of ch.iterator()) collected.push(v);
    expect(collected).toEqual(["a", "b", "c"]);
  });
});
