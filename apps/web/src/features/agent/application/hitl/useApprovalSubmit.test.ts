import { createInterruptResponseFixture } from "@/test/agentOwners";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act } from "@testing-library/react";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { waitingTree } from "@/features/agent/application/hitl/hitl.fixtures";
import { type PendingInterruptGroup } from "@/features/agent/public/sessionView";
import { useApprovalSubmit } from "@/features/agent/application/hitl/useApprovalSubmit";
const SID = "ses_1";

let disposeCoordinator: () => void = () => undefined;

function bindResume(impl?: (...args: unknown[]) => void) {
  const resume = vi.fn((...args: unknown[]) => {
    impl?.(...args);
    return true;
  });
  testClient.navigation.go({ session: SID });
  testClient.agent.projection.store.getState().ensureSession(SID);
  testClient.agent.projection.store.getState().setResume(SID, resume);
  return resume;
}

function seedPending(itemId: string): void {
  const store = testClient.agent.projection.store.getState();
  store.ensureSession(SID);
  const token = store.beginViewRefresh(SID, false)!;
  const view = testClient.agent.projection.store.getState().sessions[SID]!.view;
  const groups: PendingInterruptGroup[] = [
    {
      runId: "run_1",
      interrupts: [{ itemId, kind: "approval" }],
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
  testClient.agent.projection.store.getState().dropSession(SID);
});

beforeEach(() => {
  testClient.agent.projection.store.getState().dropSession(SID);
  disposeCoordinator = createInterruptResponseFixture(testClient);
});

describe("useApprovalSubmit", () => {
  it("maps approved → approve and latches pending", () => {
    const resume = bindResume();
    seedPending("item_1");
    const { result } = testClient.renderHook(() => useApprovalSubmit(SID, "run_1", "item_1"));
    act(() => result.current.submit("approve"));
    expect(resume).toHaveBeenCalledWith(
      "run_1",
      [{ itemId: "item_1", response: { type: "approval", decision: "approve" } }],
      expect.any(Function),
      expect.any(Function),
    );
    expect(result.current.pending).toBe("approve");
  });

  it("maps declined → deny", () => {
    const resume = bindResume();
    seedPending("item_2");
    const { result } = testClient.renderHook(() => useApprovalSubmit(SID, "run_1", "item_2"));
    act(() => result.current.submit("deny"));
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
    const { result } = testClient.renderHook(() => useApprovalSubmit(SID, "run_1", "item_e"));
    act(() => result.current.submit("approve", { editedArgs: { path: "/safe" } }));
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
    const { result } = testClient.renderHook(() => useApprovalSubmit(SID, "run_1", "item_r"));
    act(() => result.current.submit("deny", { rememberScope: "project" }));
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
    const { result } = testClient.renderHook(() => useApprovalSubmit(SID, undefined, undefined));
    act(() => result.current.submit("approve"));
    expect(resume).not.toHaveBeenCalled();

    seedPending("item_3");
    const { result: r2 } = testClient.renderHook(() => useApprovalSubmit(SID, "run_1", "item_3"));
    act(() => r2.current.submit("approve"));
    act(() => r2.current.submit("deny"));
    expect(r2.current.pending).toBe("approve");
    expect(resume).toHaveBeenCalledTimes(1);
  });

  it("acknowledges submission without inventing an approval fact", () => {
    bindResume((_run, _resp, onSettled) => (onSettled as () => void)());
    seedPending("item_ok");
    const before = testClient.agent.projection.store.getState().sessions[SID]!.view;
    const { result } = testClient.renderHook(() => useApprovalSubmit(SID, "run_1", "item_ok"));
    act(() => result.current.submit("approve"));
    expect(testClient.agent.projection.store.getState().sessions[SID]!.view).toBe(before);
    expect(result.current.pending).toBeNull();
  });

  it("rolls back pending and does NOT resolve when the resume rejects (channel-a)", () => {
    bindResume((_run, _resp, _onSettled, onStartError) => (onStartError as () => void)());
    seedPending("item_fail");
    const before = testClient.agent.projection.store.getState().sessions[SID]!.view;
    const { result } = testClient.renderHook(() => useApprovalSubmit(SID, "run_1", "item_fail"));
    act(() => result.current.submit("approve"));
    expect(testClient.agent.projection.store.getState().sessions[SID]!.view).toBe(before);
    expect(result.current.pending).toBeNull();
  });
});
