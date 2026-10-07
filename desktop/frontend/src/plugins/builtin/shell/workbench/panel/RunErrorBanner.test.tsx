import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { RunErrorBanner } from "./RunErrorBanner";
import { PROBLEM_RECOVERY, RUN_PROBLEM_RECOVERY } from "@flame/runtime-contract/wire";
import type { AgentProblem } from "@/plugins/sdk/types/agentSessionView";

const state = vi.hoisted(() => ({
  problem: null as AgentProblem | null,
  sent: [] as unknown[],
}));

vi.mock("@/plugins/builtin/agent/public/run", () => ({
  useActiveSessionProblem: () => state.problem,
  dismissActiveSessionProblem: vi.fn(),
}));
vi.mock("@/plugins/builtin/agent/public/input", () => ({
  agentTextInput: (text: string) => ({ text }),
  useCanSendToAgent: () => true,
  useChatSend: () => (input: unknown) => {
    state.sent.push(input);
    return true;
  },
}));
vi.mock("@/plugins/builtin/agent/public/conversation", () => ({
  getActiveConversationMessages: () => [
    { role: "user", blocks: [{ kind: "text", text: "Run the suite" }] },
  ],
}));
vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  useRuntimeCommandsAvailable: () => true,
}));
vi.mock("@/plugins/builtin/workspace/public/deeplinks", () => ({
  openDiagnosticsView: vi.fn(),
  openTimelineView: vi.fn(),
}));

afterEach(() => {
  cleanup();
  state.sent = [];
});

const retryButton = () => screen.queryByRole("button", { name: /Retry/ });

describe("run error banner", () => {
  it.each([
    [
      "a Run the provider rejected",
      { code: "provider_rejected", recovery: RUN_PROBLEM_RECOVERY.provider_rejected },
    ],
    [
      "a Run refused its credential",
      { code: "invalid_api_key", recovery: RUN_PROBLEM_RECOVERY.invalid_api_key },
    ],
    [
      "a request with invalid params",
      { code: "invalid_params", recovery: PROBLEM_RECOVERY.invalid_params },
    ],
  ] satisfies [string, AgentProblem][])(
    "withholds retry for %s, as the Runtime's recovery says",
    (_, problem) => {
      state.problem = problem;
      render(<RunErrorBanner />);

      expect(retryButton()).toBeNull();
      expect(screen.getByRole("alert").textContent).toContain(problem.code!);
    },
  );

  it("offers retry for a Run an internal error ended", () => {
    state.problem = { code: "internal_error", recovery: RUN_PROBLEM_RECOVERY.internal_error };
    render(<RunErrorBanner />);

    expect(retryButton()).not.toBeNull();
    expect(retryButton()!.hasAttribute("disabled")).toBe(false);
  });

  describe("while the provider's retry-after is still running", () => {
    beforeEach(() => vi.useFakeTimers());
    afterEach(() => vi.useRealTimers());

    it("counts down and only then opens the action", () => {
      state.problem = { code: "rate_limited", retryAfterSeconds: 3 } as AgentProblem;
      render(<RunErrorBanner />);

      expect(retryButton()!.textContent).toContain("3");
      expect(retryButton()!.hasAttribute("disabled")).toBe(true);

      act(() => void vi.advanceTimersByTime(1_500));
      expect(retryButton()!.textContent).toContain("2");
      expect(retryButton()!.hasAttribute("disabled")).toBe(true);

      act(() => void vi.advanceTimersByTime(2_000));
      expect(retryButton()!.hasAttribute("disabled")).toBe(false);
      expect(state.sent).toEqual([]);
    });

    it("sends nothing while the countdown is still running", () => {
      state.problem = { code: "rate_limited", retryAfterSeconds: 5 } as AgentProblem;
      render(<RunErrorBanner />);

      retryButton()!.click();
      expect(state.sent).toEqual([]);
    });
  });
});
