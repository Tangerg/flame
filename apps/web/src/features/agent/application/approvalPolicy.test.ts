import { createAgentGatewayFixture } from "@/test/agentOwners";
import { stubInstance } from "@/test/stubInstance";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient({ runtimeClient: getRuntimeClient });
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { type ApprovalRuleSummary } from "@/features/agent/application/approvalPolicyQueries";
import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { type FlameClient } from "@flame/runtime-client";
import { type AgentRuntimeGateway } from "@/features/agent/application/runtimeGateway";
import {
  forgetRules,
  setApprovalMode,
  allowMCPTool,
} from "@/features/agent/application/approvalPolicy";
import {
  APPROVAL_MODE_KEY,
  APPROVAL_RULES_KEY,
} from "@/features/agent/application/approvalPolicyQueries";
import { type ApprovalMode, type ApprovalModeResult } from "@/features/agent/domain/hitl";
import { rejected } from "@/test/rejected";
let runtimeClient: () => FlameClient = () => {
  throw new Error("Runtime test client is not configured");
};

const getRuntimeClient = () => runtimeClient();

let uninstall: (() => void) | undefined;

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  queryClient.removeQueries({ queryKey: [APPROVAL_MODE_KEY] });
  queryClient.removeQueries({ queryKey: [APPROVAL_RULES_KEY] });
  vi.restoreAllMocks();
});

describe("approval policy", () => {
  it("sets a global MCP rule through approval and invalidates every rule view", async () => {
    const setRule = vi.fn().mockResolvedValue(undefined);
    runtimeClient = () => ({ approval: { setRule } }) as unknown as FlameClient;
    uninstall = createAgentGatewayFixture(testClient, getRuntimeClient).dispose;
    const invalidate = vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    const docs = { origin: { type: "user" as const }, name: "docs" };
    const request = {
      tool: { type: "mcp" as const, server: docs, name: "read" },
      scope: "global" as const,
      subject: { type: "all" as const },
      decision: "allow" as const,
    };

    await allowMCPTool(testClient.agent.dependencies, docs, "read");

    expect(setRule).toHaveBeenCalledWith(request);
    expect(invalidate).toHaveBeenCalledWith({ queryKey: [APPROVAL_RULES_KEY] });
  });

  it("serializes mode changes and commits each authoritative response", async () => {
    const first = Promise.withResolvers<ApprovalModeResult>();
    const second = Promise.withResolvers<ApprovalModeResult>();
    const setMode = vi
      .fn<(mode: ApprovalMode) => Promise<ApprovalModeResult>>()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);
    uninstall = stubInstance(testClient.agent.dependencies.runtime, {
      setApprovalMode: setMode,
    } as unknown as AgentRuntimeGateway);
    queryClient.setQueryData([APPROVAL_MODE_KEY], saved("balanced"));

    const safe = setApprovalMode(testClient.agent.dependencies, "safe");
    const yolo = setApprovalMode(testClient.agent.dependencies, "yolo");
    await Promise.resolve();
    expect(setMode).toHaveBeenCalledTimes(1);

    first.resolve(saved("safe"));
    await expect(safe).resolves.toEqual(saved("safe"));
    await Promise.resolve();
    expect(setMode).toHaveBeenNthCalledWith(2, "yolo");

    second.resolve(saved("yolo"));
    await expect(yolo).resolves.toEqual(saved("yolo"));
    expect(queryClient.getQueryData([APPROVAL_MODE_KEY])).toEqual(saved("yolo"));
  });

  it("continues with the next mode after a rejected change", async () => {
    const first = Promise.withResolvers<ApprovalModeResult>();
    const setMode = vi
      .fn<(mode: ApprovalMode) => Promise<ApprovalModeResult>>()
      .mockReturnValueOnce(first.promise)
      .mockResolvedValueOnce(saved("yolo"));
    uninstall = stubInstance(testClient.agent.dependencies.runtime, {
      setApprovalMode: setMode,
    } as unknown as AgentRuntimeGateway);

    const rejected = setApprovalMode(testClient.agent.dependencies, "safe");
    const accepted = setApprovalMode(testClient.agent.dependencies, "yolo");
    first.reject(new Error("not saved"));

    await expect(rejected).rejects.toThrow("not saved");
    await expect(accepted).resolves.toEqual(saved("yolo"));
    expect(setMode).toHaveBeenNthCalledWith(2, "yolo");
  });

  it("does not serialize the successor behind an old Plugin Host mode change", async () => {
    const retiredMode = Promise.withResolvers<ApprovalModeResult>();
    uninstall = stubInstance(testClient.agent.dependencies.runtime, {
      setApprovalMode: vi.fn(() => retiredMode.promise),
    } as unknown as AgentRuntimeGateway);
    const retired = setApprovalMode(testClient.agent.dependencies, "safe");

    const successorSetMode = vi.fn().mockResolvedValue(saved("yolo"));
    runtimeClient = () => ({ approval: { setMode: successorSetMode } }) as unknown as FlameClient;
    const disposeSuccessor = createAgentGatewayFixture(testClient, getRuntimeClient);
    const successor = setApprovalMode(testClient.agent.dependencies, "yolo");
    await Promise.resolve();
    const successorStartedBeforeRetiredSettlement = successorSetMode.mock.calls.length;
    retiredMode.resolve(saved("safe"));
    try {
      await Promise.allSettled([retired, successor]);
      expect(successorStartedBeforeRetiredSettlement).toBe(1);
      expect(successorSetMode).toHaveBeenCalledTimes(1);
      expect(successorSetMode).toHaveBeenCalledWith("yolo");
      expect(queryClient.getQueryData([APPROVAL_MODE_KEY])).toEqual(saved("yolo"));
    } finally {
      disposeSuccessor.dispose();
    }
  });

  it("keeps accepted rule deletions when a later item in the batch fails", async () => {
    const failure = new Error("second delete failed");
    const forgetApprovalRule = vi
      .fn()
      .mockResolvedValueOnce(undefined)
      .mockRejectedValueOnce(failure);
    uninstall = stubInstance(testClient.agent.dependencies.runtime, {
      forgetApprovalRule,
    } as unknown as AgentRuntimeGateway);
    const key = [APPROVAL_RULES_KEY, { sessionId: "ses_1" }];
    queryClient.setQueryData(key, [rule("rule-1"), rule("rule-2")]);
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();

    await expect(forgetRules(testClient.agent.dependencies, ["rule-1", "rule-2"])).rejects.toBe(
      failure,
    );
    expect(queryClient.getQueryData(key)).toEqual([rule("rule-2")]);
  });

  it("keeps an accepted rule deletion successful when cache repair fails", async () => {
    uninstall = stubInstance(testClient.agent.dependencies.runtime, {
      forgetApprovalRule: vi.fn().mockResolvedValue(undefined),
    } as unknown as AgentRuntimeGateway);
    const key = [APPROVAL_RULES_KEY, { sessionId: "ses_1" }];
    queryClient.setQueryData(key, [rule("rule-1")]);
    vi.spyOn(queryClient, "invalidateQueries").mockRejectedValue(new Error("cache unavailable"));

    await expect(forgetRules(testClient.agent.dependencies, ["rule-1"])).resolves.toBeUndefined();
    expect(queryClient.getQueryData(key)).toEqual([]);
  });

  it("does not continue a rule batch through a successor Plugin Host", async () => {
    const retiredWrite = Promise.withResolvers<void>();
    const forgetRetired = vi.fn(() => retiredWrite.promise);
    const forgetSuccessor = vi.fn().mockResolvedValue(undefined);
    runtimeClient = () => ({ approval: { forgetRule: forgetRetired } }) as unknown as FlameClient;
    const retiredInstallation = createAgentGatewayFixture(testClient, getRuntimeClient);
    const command = rejected(forgetRules(testClient.agent.dependencies, ["rule-1", "rule-2"]));
    await vi.waitFor(() => expect(forgetRetired).toHaveBeenCalledOnce());

    runtimeClient = () => ({ approval: { forgetRule: forgetSuccessor } }) as unknown as FlameClient;
    const successorInstallation = createAgentGatewayFixture(testClient, getRuntimeClient);
    try {
      await expect(command).resolves.toMatchObject({ message: "agent_command_owner_retired" });
      expect(forgetSuccessor).not.toHaveBeenCalled();
    } finally {
      retiredWrite.resolve();
      successorInstallation.dispose();
      retiredInstallation.dispose();
    }
  });
});

function rule(id: string): ApprovalRuleSummary {
  return {
    id,
    scope: "global",
    subject: { type: "all" },
    tool: { type: "builtIn", name: "shell" },
    modelName: "shell",
    stale: false,
    decision: "allow",
  };
}

function saved(mode: ApprovalMode): ApprovalModeResult {
  return {
    mode,
    modes: [
      { mode: "safe", write: "prompt", exec: "prompt", network: "prompt" },
      { mode: "balanced", write: "pass", exec: "prompt", network: "pass" },
      { mode: "yolo", write: "pass", exec: "pass", network: "pass" },
    ],
  };
}
