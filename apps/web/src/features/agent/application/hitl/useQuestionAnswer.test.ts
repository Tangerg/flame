import { act, renderHook } from "@testing-library/react";
import { navigator } from "@/lib/navigation";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useAgentStore } from "@/features/agent/adapters/agentStore";
import { waitingTree } from "@/features/agent/application/hitl/hitl.fixtures";
import { type PendingInterruptGroup } from "@/features/agent/public/sessionView";
import { useQuestionAnswer } from "@/features/agent/application/hitl/useQuestionAnswer";
import { installInterruptResponseCoordinator } from "@/features/agent/application/hitl/interruptResponseCoordinator";
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
  const groups: PendingInterruptGroup[] = [
    {
      runId: "run_1",
      interrupts: [{ itemId, kind: "question" }],
    },
  ];
  store.commitViewRefresh(SID, token, {
    ...view,
    runsById: waitingTree(SID, "run_1", groups),
    pendingInterrupts: groups,
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
    const { result } = renderHook(() => useQuestionAnswer(SID, "run_1", "item_q"));
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
    const { result } = renderHook(() => useQuestionAnswer(SID, undefined, undefined));
    act(() => result.current.submit([["x"]]));
    expect(resume).not.toHaveBeenCalled();

    seedPending("item_q2");
    const { result: r2 } = renderHook(() => useQuestionAnswer(SID, "run_1", "item_q2"));
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

  it("settles only local submission state after acceptance and rolls back on rejection", () => {
    const onStarted = bindResume((_r, _resp, onSettled) => (onSettled as () => void)());
    seedPending("q_ok");
    const before = useAgentStore.getState().sessions[SID]!.view;
    const { result } = renderHook(() => useQuestionAnswer(SID, "run_1", "q_ok"));
    act(() => result.current.submit([["x"]]));
    expect(useAgentStore.getState().sessions[SID]!.view).toBe(before);
    expect(onStarted).toHaveBeenCalledTimes(1);
    expect(result.current.pending).toBe(false);
    act(() => result.current.submit([["again"]]));
    expect(onStarted).toHaveBeenCalledTimes(1);

    bindResume((_r, _resp, _s, onStartError) => (onStartError as () => void)());
    seedPending("q_fail");
    const { result: r2 } = renderHook(() => useQuestionAnswer(SID, "run_1", "q_fail"));
    act(() => r2.current.submit([["x"]]));
    expect(r2.current.pending).toBe(false);
  });

  it("targets the passed session identity, not the active navigation", () => {
    const resume = bindResume();
    seedPending("item_explicit");
    navigator().go({ session: "ses_other" });
    const { result } = renderHook(() => useQuestionAnswer(SID, "run_1", "item_explicit"));
    act(() => result.current.submit([["x"]]));
    expect(resume).toHaveBeenCalledOnce();
  });
});
