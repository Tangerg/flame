import { type RuntimeAgentDriver } from "@/features/agent/adapters/runtimeDriver";
import { describe, expect, it, vi } from "vitest";
import { rpcAgentSource } from "@/features/agent/adapters/rpcAgentSource";
import { type RpcRunsGateway } from "@/features/agent/adapters/rpcAgentDriver";
const input: Parameters<RuntimeAgentDriver["start"]>[0] = [{ type: "text", text: "hello" }];

describe("rpcAgentSource", () => {
  it("projects the RPC driver factory into a stable agent source spec", () => {
    const gateway: RpcRunsGateway = {
      start: vi.fn(),
      resume: vi.fn(),
    };

    const source = rpcAgentSource(
      (key) => `t:${key}`,
      () => "ses_1",
      () => gateway,
    );

    expect(source).toMatchObject({
      id: "rpc",
      label: "t:agentSource.rpc",
      priority: 1,
    });
  });

  it("builds drivers for the active session when the source factory runs", async () => {
    const result = {} as Awaited<ReturnType<RuntimeAgentDriver["start"]>>;
    const gateway: RpcRunsGateway = {
      start: vi.fn().mockResolvedValue(result),
      resume: vi.fn(),
    };
    let activeSessionId = "ses_1";
    const source = rpcAgentSource(
      (key) => key,
      () => activeSessionId,
      () => gateway,
    );
    activeSessionId = "ses_2";

    await expect(source.factory().start(input, {}, undefined)).resolves.toBe(result);

    expect(gateway.start).toHaveBeenCalledWith({ sessionId: "ses_2", input }, undefined);
  });
});
