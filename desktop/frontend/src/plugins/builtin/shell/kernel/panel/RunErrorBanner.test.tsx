import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { RunErrorBanner } from "./RunErrorBanner";
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
  getActiveConversationSnapshot: () => ({
    messages: [{ role: "user", blocks: [{ kind: "text", text: "Run the suite" }] }],
  }),
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
  it("withholds retry for the codes retrying cannot fix", () => {
    state.problem = { code: "provider_rejected" } as AgentProblem;
    render(<RunErrorBanner />);

    expect(retryButton()).toBeNull();
    expect(screen.getByRole("alert").textContent).toContain("provider_rejected");
  });

  it("offers retry for a code that a second attempt can fix", () => {
    state.problem = { code: "provider_error" } as AgentProblem;
    render(<RunErrorBanner />);

    expect(retryButton()).not.toBeNull();
    expect(retryButton()!.hasAttribute("disabled")).toBe(false);
  });

  // The one banner state no golden can hold: the label changes once a second, so a
  // screenshot photographs whichever number it landed on. What must hold is that the
  // action stays SHUT until the provider's own retry-after has elapsed — an enabled
  // button during a rate limit sends the request straight back into the same refusal.
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
