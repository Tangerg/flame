import { describe, expect, it, vi } from "vitest";
import { AgentCommandOwner } from "./agentCommandOwner";

describe("AgentCommandOwner", () => {
  it("retires predecessor effects before the successor accepts commands", () => {
    const rollback = vi.fn();
    const retired = AgentCommandOwner.install();
    retired.trackEffect(rollback);

    const successor = AgentCommandOwner.install();

    expect(rollback).toHaveBeenCalledOnce();
    expect(retired.isCurrent()).toBe(false);
    expect(successor.isCurrent()).toBe(true);
    successor.dispose();
  });

  it("does not let a stale disposer retire the successor", () => {
    const retired = AgentCommandOwner.install();
    const successor = AgentCommandOwner.install();

    retired.dispose();

    expect(successor.isCurrent()).toBe(true);
    expect(() => successor.assertCurrent()).not.toThrow();
    successor.dispose();
  });

  it("settles admitted work immediately when its generation retires", async () => {
    const retired = AgentCommandOwner.install();
    let settleOperation!: () => void;
    const pending = new Promise<void>((resolve) => {
      settleOperation = resolve;
    });
    const operation = retired.settle(pending);

    const successor = AgentCommandOwner.install();

    await expect(operation).rejects.toMatchObject({ message: "agent_command_owner_retired" });
    settleOperation();
    await pending;
    successor.dispose();
  });

  it("retires the real Session summary RPC instead of only its continuation", async () => {
    const retired = AgentCommandOwner.install();
    let settleRPC!: (value: { revision: number }) => void;
    const rpc = new Promise<{ revision: number }>((resolve) => {
      settleRPC = resolve;
    });
    const execute = vi.fn(() => rpc);
    const operation = retired.settleSessionSummary("session_1", 3, execute);
    await Promise.resolve();
    expect(execute).toHaveBeenCalledOnce();

    const successor = AgentCommandOwner.install();
    let outcome = "pending";
    void operation.then(
      () => {
        outcome = "resolved";
      },
      () => {
        outcome = "retired";
      },
    );
    await flushMicrotasks();
    const outcomeBeforeOldRPC = outcome;

    settleRPC({ revision: 4 });
    await rpc;
    await Promise.resolve();
    expect(outcomeBeforeOldRPC).toBe("retired");
    successor.dispose();
  });

  it("uses the accepted summary revision for the next queued edit", async () => {
    const owner = AgentCommandOwner.install();
    const first = Promise.withResolvers<{ revision: number }>();
    const rename = vi.fn(() => first.promise);
    const favorite = vi.fn(async (revision: number) => ({ revision: revision + 1 }));

    const renamed = owner.settleSessionSummary("session_1", 3, rename);
    const favorited = owner.settleSessionSummary("session_1", 3, favorite);
    await Promise.resolve();
    expect(rename).toHaveBeenCalledWith(3);
    expect(favorite).not.toHaveBeenCalled();

    first.resolve({ revision: 4 });

    await renamed;
    await expect(favorited).resolves.toEqual({ revision: 5 });
    expect(favorite).toHaveBeenCalledWith(4);
    owner.dispose();
  });
});

async function flushMicrotasks(): Promise<void> {
  for (let index = 0; index < 8; index += 1) await Promise.resolve();
}
