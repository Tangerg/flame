import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { RunErrorBanner } from "@/features/shell/workbench/panel/RunErrorBanner";
import { PROBLEM_RECOVERY, RUN_PROBLEM_RECOVERY } from "@flame/runtime-client/wire";
import { type AgentProblem } from "@/features/agent/public/sessionView";
const state = vi.hoisted(() => ({
  problem: null as AgentProblem | null,
  sent: [] as unknown[],
  showWorkspaceDock: vi.fn(),
}));

vi.mock("@/features/agent/public/run", () => ({
  useActiveSessionProblem: () => state.problem,
  dismissActiveSessionProblem: vi.fn(),
}));

vi.mock("@/features/agent/public/input", () => ({
  agentTextInput: (text: string) => ({ text }),
  useCanSendToAgent: () => true,
  useChatSend: () => (input: unknown) => {
    state.sent.push(input);
    return true;
  },
}));

vi.mock("@/features/agent/public/conversation", () => ({
  getActiveConversationMessages: () => [
    { role: "user", blocks: [{ kind: "text", text: "Run the suite" }] },
  ],
}));

vi.mock("@/features/connection/public/serviceStatus", () => ({
  useRuntimeCommandsAvailable: () => true,
}));

vi.mock("@/features/workspace/public/deeplinks", () => ({
  openDiagnosticsView: vi.fn(),
}));

vi.mock("@/features/workspace/public/navigation", () => ({
  showWorkspaceDock: state.showWorkspaceDock,
}));

afterEach(() => {
  cleanup();
  state.sent = [];
  state.showWorkspaceDock.mockClear();
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
      testClient.render(<RunErrorBanner />);

      expect(retryButton()).toBeNull();
      expect(screen.getByRole("alert").textContent).toContain(problem.code!);
    },
  );

  it("opens the contributed context catalog to inspect the failure", () => {
    state.problem = { code: "internal_error", recovery: RUN_PROBLEM_RECOVERY.internal_error };
    testClient.render(<RunErrorBanner />);
    screen.getByRole("button", { name: "Inspect context" }).click();
    expect(state.showWorkspaceDock).toHaveBeenCalledOnce();
  });

  it("offers retry for a Run an internal error ended", () => {
    state.problem = { code: "internal_error", recovery: RUN_PROBLEM_RECOVERY.internal_error };
    testClient.render(<RunErrorBanner />);

    expect(retryButton()).not.toBeNull();
    expect(retryButton()!.hasAttribute("disabled")).toBe(false);
  });

  describe("while the provider's retry-after is still running", () => {
    beforeEach(() => vi.useFakeTimers());
    afterEach(() => vi.useRealTimers());

    it("counts down and only then opens the action", () => {
      state.problem = { code: "rate_limited", retryAfterSeconds: 3 } as AgentProblem;
      testClient.render(<RunErrorBanner />);

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
      testClient.render(<RunErrorBanner />);

      retryButton()!.click();
      expect(state.sent).toEqual([]);
    });
  });
});
