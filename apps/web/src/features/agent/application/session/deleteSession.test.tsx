import { stubInstance } from "@/test/stubInstance";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { afterEach, describe, expect, it, vi } from "vitest";
import { act } from "@testing-library/react";
import { queryClient } from "@/lib/queryClient";
import { type AgentRuntimeGateway } from "@/features/agent/application/runtimeGateway";
import { type AgentSessionStatePort } from "@/features/agent/application/session/sessionAccess";
import {
  AGENT_SESSIONS_KEY,
  type AgentSessionSummary,
} from "@/features/agent/application/session/sessionQueries";
import { useDeleteSession } from "@/features/agent/application/session/deleteSession";

let restoreRuntime: (() => void) | undefined;

let restoreState: (() => void) | undefined;

function session(id: string): AgentSessionSummary {
  return {
    id,
    revision: 1,
    title: id,
    status: "idle",
    provider: "openai",
    model: "gpt-5",
    workspace: { path: "/repo", availability: "available" },
    time: "2026-08-12T00:00:00Z",
  };
}

afterEach(() => {
  restoreRuntime?.();
  restoreState?.();
  restoreRuntime = undefined;
  restoreState = undefined;
  queryClient.removeQueries({ queryKey: [AGENT_SESSIONS_KEY] });
});

describe("useDeleteSession", () => {
  it("keeps Session identity authoritative until the delete command commits", async () => {
    let commit!: () => void;
    const deleteSession = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          commit = resolve;
        }),
    );
    const localDeletion = vi.fn();
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      deleteSession,
    } as unknown as AgentRuntimeGateway);
    restoreState = stubInstance(testClient.agent.dependencies.state, {
      deleteSession: localDeletion,
    } as unknown as AgentSessionStatePort);
    queryClient.setQueryData([AGENT_SESSIONS_KEY], [session("ses_a"), session("ses_b")]);
    const { result } = testClient.renderHook(() => useDeleteSession());

    let deleting!: Promise<void>;
    await act(async () => {
      deleting = result.current("ses_a");
      await Promise.resolve();
    });

    expect(queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY])).toEqual([
      session("ses_a"),
      session("ses_b"),
    ]);
    expect(localDeletion).not.toHaveBeenCalled();

    commit();
    await act(async () => deleting);
    expect(localDeletion).toHaveBeenCalledWith("ses_a");
  });

  it("retires delete loading before an old non-cooperative RPC responds", async () => {
    let commit!: () => void;
    const deleteSession = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          commit = resolve;
        }),
    );
    const localDeletion = vi.fn();
    restoreRuntime = stubInstance(testClient.agent.dependencies.runtime, {
      deleteSession,
    } as unknown as AgentRuntimeGateway);
    restoreState = stubInstance(testClient.agent.dependencies.state, {
      deleteSession: localDeletion,
    } as unknown as AgentSessionStatePort);
    const retiredOwner = testClient.agent.dependencies.commands.replace();
    const { result } = testClient.renderHook(() => useDeleteSession());
    const deleting = result.current("ses_a");
    await Promise.resolve();
    expect(deleteSession).toHaveBeenCalledOnce();

    const successor = testClient.agent.dependencies.commands.replace();
    let settled = false;
    void deleting.then(() => {
      settled = true;
    });
    await Promise.resolve();
    await Promise.resolve();
    const settledBeforeOldRPC = settled;

    commit();
    await deleting;
    expect(settledBeforeOldRPC).toBe(true);
    expect(localDeletion).not.toHaveBeenCalled();
    retiredOwner.dispose();
    successor.dispose();
  });
});
