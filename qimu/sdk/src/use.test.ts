import { describe, expect, it } from "vitest";
import { Lifetime } from "./lifetime";
import { acquire, withResource } from "./use";

describe("acquired resource", () => {
  it("owns a late acquisition and rejects new borrowing atomically", async () => {
    const owner = new Lifetime("owner");
    let resolve!: (resource: { value: number; close(): void }) => void;
    let closes = 0;
    const pending = acquire(
      owner,
      "late",
      () =>
        new Promise<{ value: number; close(): void }>((yes) => {
          resolve = yes;
        }),
    );
    const closed = owner.close();
    resolve({
      value: 42,
      close() {
        closes++;
      },
    });
    await expect(pending).rejects.toThrow("arrived after");
    expect((await closed).status).toBe("released");
    expect(closes).toBe(1);
  });

  it("joins current borrows before releasing and shares one exit", async () => {
    const owner = new Lifetime("owner");
    let finish!: () => void;
    const order: string[] = [];
    const acquired = await acquire(owner, "value", () => ({
      value: 7,
      close() {
        order.push("closed");
      },
    }));
    const borrowed = acquired.with(
      (value) =>
        new Promise<number>((resolve) => {
          finish = () => resolve(value);
        }),
    );
    const first = acquired.close();
    const second = acquired.close();
    expect(() => acquired.with(() => 0)).toThrow("no longer accepts");
    expect(order).toEqual([]);
    finish();
    expect(await borrowed).toBe(7);
    expect((await first).closeId).toBe((await second).closeId);
    expect(order).toEqual(["closed"]);
    await owner.close();
    expect(order).toEqual(["closed"]);
  });

  it("borrows ordinary results and closes resource before returning", async () => {
    const owner = new Lifetime("owner");
    let closed = false;
    const result = await withResource(
      owner,
      "value",
      () => ({
        value: 8,
        close() {
          closed = true;
        },
      }),
      (value) => value * 2,
    );
    expect(result).toBe(16);
    expect(closed).toBe(true);
    await owner.close();
  });
});
