import { act, renderHook } from "@testing-library/react";
import { navigator } from "@/lib/navigation";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useAgentStore } from "@/plugins/builtin/agent/adapters/agentStore";
import { useApprovalSubmit } from "./useApprovalSubmit";
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
        interrupts: [{ itemId, kind: "approval" }],
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

describe("useApprovalSubmit", () => {
  it("maps approved → approve and latches pending", () => {
    const resume = bindResume();
    seedPending("item_1");
    const { result } = renderHook(() => useApprovalSubmit("run_1", "item_1"));
    act(() => result.current.submit("approved"));
    expect(resume).toHaveBeenCalledWith(
      "run_1",
      [{ itemId: "item_1", response: { type: "approval", decision: "approve" } }],
      expect.any(Function),
      expect.any(Function),
    );
    expect(result.current.pending).toBe("approved");
  });

  it("maps declined → deny", () => {
    const resume = bindResume();
    seedPending("item_2");
    const { result } = renderHook(() => useApprovalSubmit("run_1", "item_2"));
    act(() => result.current.submit("declined"));
    expect(resume).toHaveBeenCalledWith(
      "run_1",
      [{ itemId: "item_2", response: { type: "approval", decision: "deny" } }],
      expect.any(Function),
      expect.any(Function),
    );
  });

  it("forwards editedArgs only when provided (approve-with-modified-args)", () => {
    const resume = bindResume();
    seedPending("item_e");
    const { result } = renderHook(() => useApprovalSubmit("run_1", "item_e"));
    act(() => result.current.submit("approved", { editedArgs: { path: "/safe" } }));
    expect(resume).toHaveBeenCalledWith(
      "run_1",
      [
        {
          itemId: "item_e",
          response: { type: "approval", decision: "approve", editedArgs: { path: "/safe" } },
        },
      ],
      expect.any(Function),
      expect.any(Function),
    );
  });

  it("forwards remember{scope} when rememberScope is set (AUX_API §6)", () => {
    const resume = bindResume();
    seedPending("item_r");
    const { result } = renderHook(() => useApprovalSubmit("run_1", "item_r"));
    act(() => result.current.submit("declined", { rememberScope: "project" }));
    expect(resume).toHaveBeenCalledWith(
      "run_1",
      [
        {
          itemId: "item_r",
          response: { type: "approval", decision: "deny", remember: { scope: "project" } },
        },
      ],
      expect.any(Function),
      expect.any(Function),
    );
  });

  it("no-ops without a runId/itemId, and never double-submits", () => {
    const resume = bindResume();
    const { result } = renderHook(() => useApprovalSubmit(undefined, undefined));
    act(() => result.current.submit("approved"));
    expect(resume).not.toHaveBeenCalled();

    seedPending("item_3");
    const { result: r2 } = renderHook(() => useApprovalSubmit("run_1", "item_3"));
    act(() => r2.current.submit("approved"));
    act(() => r2.current.submit("declined"));
    expect(r2.current.pending).toBe("approved");
    expect(resume).toHaveBeenCalledTimes(1);
  });

  it("commits resolveInterrupt only after the run starts (onSettled)", () => {
    bindResume((_run, _resp, onSettled) => (onSettled as () => void)());
    seedPending("item_ok");
    const spy = vi.spyOn(useAgentStore.getState(), "resolveInterrupt");
    const { result } = renderHook(() => useApprovalSubmit("run_1", "item_ok"));
    act(() => result.current.submit("approved"));
    expect(spy).toHaveBeenCalledWith(SID, "item_ok", { decision: "approved" }, expect.any(Number));
    spy.mockRestore();
  });

  it("rolls back pending and does NOT resolve when the resume rejects (channel-a)", () => {
    bindResume((_run, _resp, _onSettled, onStartError) => (onStartError as () => void)());
    seedPending("item_fail");
    const spy = vi.spyOn(useAgentStore.getState(), "resolveInterrupt");
    const { result } = renderHook(() => useApprovalSubmit("run_1", "item_fail"));
    act(() => result.current.submit("approved"));
    expect(spy).not.toHaveBeenCalled();
    expect(result.current.pending).toBeNull();
    spy.mockRestore();
  });
});
