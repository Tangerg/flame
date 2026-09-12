import { act, renderHook } from "@testing-library/react";
import { navigator } from "@/lib/navigation";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useAgentStore } from "@/plugins/builtin/agent/adapters/agentStore";
import { useQuestionAnswer } from "./useQuestionAnswer";
import { installInterruptResponseCoordinator } from "./interruptResponseCoordinator";

const SID = "ses_1";
let disposeCoordinator: () => void = () => undefined;

function bindResume(impl?: (...args: unknown[]) => void) {
  const resume = vi.fn((...args: unknown[]) => {
    impl?.(...args);
    return true;
  });
  navigator().go({ session: SID });
  useAgentStore.getState().ensureSession(SID);
  useAgentStore.getState().setResume(SID, resume);
  return resume;
}

function seedPending(itemId: string): void {
  const store = useAgentStore.getState();
  store.ensureSession(SID);
  const token = store.beginViewRefresh(SID, false)!;
  const view = useAgentStore.getState().sessions[SID]!.view;
  store.commitViewRefresh(SID, token, {
    ...view,
    pendingInterrupts: [
      {
        sessionId: SID,
        runId: "run_1",
        rootRunId: "run_1",
        interrupts: [{ itemId, kind: "question" }],
      },
    ],
  });
}

afterEach(() => {
  disposeCoordinator();
  useAgentStore.getState().dropSession(SID);
});
beforeEach(() => {
  useAgentStore.getState().dropSession(SID);
  disposeCoordinator = installInterruptResponseCoordinator();
});

describe("useQuestionAnswer", () => {
  it("resumes with an answer InterruptResponse and latches pending", () => {
    const resume = bindResume();
    seedPending("item_q");
    const { result } = renderHook(() => useQuestionAnswer("run_1", "item_q"));
    const answers = [["Postgres"], ["tools", "vision"]];
    act(() => result.current.submit(answers));
    expect(resume).toHaveBeenCalledWith(
      "run_1",
      [
        {
          itemId: "item_q",
          response: { type: "answer", answers },
        },
      ],
      expect.any(Function),
      expect.any(Function),
    );
    expect(result.current.pending).toBe(true);
  });

  it("no-ops without a runId/itemId, and never double-submits", () => {
    const resume = bindResume();
    const { result } = renderHook(() => useQuestionAnswer(undefined, undefined));
    act(() => result.current.submit([["x"]]));
    expect(resume).not.toHaveBeenCalled();

    seedPending("item_q2");
    const { result: r2 } = renderHook(() => useQuestionAnswer("run_1", "item_q2"));
    act(() => r2.current.submit([["first"]]));
    act(() => r2.current.submit([["second"]]));
    expect(resume).toHaveBeenCalledTimes(1);
    expect(resume).toHaveBeenCalledWith(
      "run_1",
      [{ itemId: "item_q2", response: { type: "answer", answers: [["first"]] } }],
      expect.any(Function),
      expect.any(Function),
    );
  });

  it("commits resolveInterrupt only after the run starts; rolls back on reject", () => {
    const onStarted = bindResume((_r, _resp, onSettled) => (onSettled as () => void)());
    seedPending("q_ok");
    const spy = vi.spyOn(useAgentStore.getState(), "resolveInterrupt");
    const { result } = renderHook(() => useQuestionAnswer("run_1", "q_ok"));
    act(() => result.current.submit([["x"]]));
    expect(spy).toHaveBeenCalledWith(
      SID,
      "q_ok",
      { answered: true, answers: [["x"]] },
      expect.any(Number),
    );
    expect(onStarted).toHaveBeenCalled();

    spy.mockClear();
    bindResume((_r, _resp, _s, onStartError) => (onStartError as () => void)());
    seedPending("q_fail");
    const { result: r2 } = renderHook(() => useQuestionAnswer("run_1", "q_fail"));
    act(() => r2.current.submit([["x"]]));
    expect(spy).not.toHaveBeenCalled();
    expect(r2.current.pending).toBe(false);
    spy.mockRestore();
  });
});
