import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { describe, expect, it, vi } from "vitest";

describe("AgentCommandOwner", () => {
  it("retires predecessor effects before the successor accepts commands", () => {
    const rollback = vi.fn();
    const retired = testClient.agent.access.commands.replace();
    retired.trackEffect(rollback);

    const successor = testClient.agent.access.commands.replace();

    expect(rollback).toHaveBeenCalledOnce();
    expect(retired.isCurrent()).toBe(false);
    expect(successor.isCurrent()).toBe(true);
    successor.dispose();
  });

  it("does not let a stale disposer retire the successor", () => {
    const retired = testClient.agent.access.commands.replace();
    const successor = testClient.agent.access.commands.replace();

    retired.dispose();

    expect(successor.isCurrent()).toBe(true);
    expect(() => successor.assertCurrent()).not.toThrow();
    successor.dispose();
  });

  it("settles admitted work immediately when its generation retires", async () => {
    const retired = testClient.agent.access.commands.replace();
    let settleOperation!: () => void;
    const pending = new Promise<void>((resolve) => {
      settleOperation = resolve;
    });
    const operation = retired.settle(pending);

    const successor = testClient.agent.access.commands.replace();

    await expect(operation).rejects.toMatchObject({ message: "agent_command_owner_retired" });
    settleOperation();
    await pending;
    successor.dispose();
  });

  it("retires the real Session summary RPC instead of only its continuation", async () => {
    const retired = testClient.agent.access.commands.replace();
    let settleRPC!: (value: { revision: number }) => void;
    const rpc = new Promise<{ revision: number }>((resolve) => {
      settleRPC = resolve;
    });
    const execute = vi.fn(() => retired.settle(rpc));
    const operation = retired.serializeSessionSummary("session_1", execute);
    await Promise.resolve();
    expect(execute).toHaveBeenCalledOnce();

    const successor = testClient.agent.access.commands.replace();
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
});

async function flushMicrotasks(): Promise<void> {
  for (let index = 0; index < 8; index += 1) await Promise.resolve();
}
