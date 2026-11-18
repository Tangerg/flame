import { type RuntimeAgentDriver } from "@/features/agent/adapters/runtimeDriver";
import { describe, expect, it } from "vitest";
import {
  DEFAULT_RPC_SESSION_ID,
  activeRpcSessionId,
  rpcRunStartParams,
} from "@/features/agent/adapters/rpcAgentDriver";
const input: Parameters<RuntimeAgentDriver["start"]>[0] = [{ type: "text", text: "hello" }];

describe("activeRpcSessionId", () => {
  it("falls back to the runtime default session id when no session is active", () => {
    expect(activeRpcSessionId(null)).toBe(DEFAULT_RPC_SESSION_ID);
    expect(activeRpcSessionId("")).toBe(DEFAULT_RPC_SESSION_ID);
    expect(activeRpcSessionId("ses_123")).toBe("ses_123");
  });
});

describe("rpcRunStartParams", () => {
  it("sends provider, model, and reasoning effort as one exact selection", () => {
    expect(
      rpcRunStartParams("ses_1", input, {
        provider: "openai",
        model: "gpt",
        reasoningEffort: "high",
      }),
    ).toEqual({
      sessionId: "ses_1",
      input,
      provider: "openai",
      model: "gpt",
      reasoningEffort: "high",
    });
    expect(rpcRunStartParams("ses_1", input, { provider: "openai" })).toEqual({
      sessionId: "ses_1",
      input,
    });
    expect(rpcRunStartParams("ses_1", input, { model: "gpt" })).toEqual({
      sessionId: "ses_1",
      input,
    });
  });
});
