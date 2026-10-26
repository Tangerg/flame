import { createInterruptResponseFixture } from "@/test/agentOwners";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { waitingTree } from "@/features/agent/application/hitl/hitl.fixtures";
import { type PendingInterruptGroup } from "@/features/agent/public/sessionView";
import { registerApprovalActions } from "@/features/agent/application/hitl/useApprovalSubmit";
import { submitPendingApproval } from "@/features/agent/application/hitl/submitPendingApproval";
const SESSION_ID = "ses_keyboard_barrier";

const ROOT_RUN_ID = "run_root";

let disposeCoordinator: () => void = () => undefined;

function seedPending(groups: PendingInterruptGroup[]): void {
  const store = testClient.agent.projection.store.getState();
  store.ensureSession(SESSION_ID);
  const token = store.beginViewRefresh(SESSION_ID, false)!;
  const view = testClient.agent.projection.store.getState().sessions[SESSION_ID]!.view;
  store.commitViewRefresh(SESSION_ID, token, {
    ...view,
    runsById: waitingTree(SESSION_ID, ROOT_RUN_ID, groups),
    pendingInterrupts: groups,
  });
}

beforeEach(() => {
  testClient.agent.projection.store.getState().dropSession(SESSION_ID);
  testClient.navigation.go({ session: SESSION_ID });
  disposeCoordinator = createInterruptResponseFixture(testClient);
});

afterEach(() => {
  disposeCoordinator();
  testClient.agent.projection.store.getState().dropSession(SESSION_ID);
});

describe("submitPendingApproval", () => {
  it("walks an atomic child-run barrier and resumes its root once", () => {
    seedPending([
      {
        runId: "run_child_a",
        interrupts: [{ itemId: "approval_a", kind: "approval" }],
      },
      {
        runId: "run_child_b",
        interrupts: [{ itemId: "approval_b", kind: "approval" }],
      },
    ]);
    let accept: (() => void) | undefined;
    const resume = vi.fn((_runId, _responses, onSettled?: () => void) => {
      accept = onSettled;
      return true;
    });
    testClient.agent.projection.store.getState().setResume(SESSION_ID, resume);

    expect(submitPendingApproval(testClient.agent.dependencies, "approve")).toBe(true);
    expect(resume).not.toHaveBeenCalled();
    expect(submitPendingApproval(testClient.agent.dependencies, "deny")).toBe(true);
    expect(resume).toHaveBeenCalledWith(
      ROOT_RUN_ID,
      [
        {
          itemId: "approval_a",
          response: { type: "approval", decision: "approve" },
        },
        {
          itemId: "approval_b",
          response: { type: "approval", decision: "deny" },
        },
      ],
      expect.any(Function),
      expect.any(Function),
    );
    expect(submitPendingApproval(testClient.agent.dependencies, "approve")).toBe(true);

    accept?.();
    expect(
      testClient.agent.projection.store.getState().sessions[SESSION_ID]!.view.pendingInterrupts,
    ).toHaveLength(2);
    expect(submitPendingApproval(testClient.agent.dependencies, "approve")).toBe(true);
    expect(resume).toHaveBeenCalledOnce();
  });

  it("does not consume the approval shortcut for a question-only barrier", () => {
    seedPending([
      {
        runId: ROOT_RUN_ID,
        interrupts: [{ itemId: "question_a", kind: "question" }],
      },
    ]);
    testClient.agent.projection.store.getState().setResume(
      SESSION_ID,
      vi.fn(() => true),
    );

    expect(submitPendingApproval(testClient.agent.dependencies, "approve")).toBe(false);
  });

  it("does not borrow a mounted approval card from another Session", () => {
    seedPending([
      {
        runId: ROOT_RUN_ID,
        interrupts: [{ itemId: "approval_same", kind: "approval" }],
      },
    ]);
    const resume = vi.fn(() => true);
    testClient.agent.projection.store.getState().setResume(SESSION_ID, resume);
    const retiredApprove = vi.fn();
    const unregister = registerApprovalActions(
      { sessionId: "ses_retired", rootRunId: ROOT_RUN_ID, itemId: "approval_same" },
      {
        approve: retiredApprove,
        decline: vi.fn(),
      },
    );

    try {
      expect(submitPendingApproval(testClient.agent.dependencies, "approve")).toBe(true);
      expect(retiredApprove).not.toHaveBeenCalled();
      expect(resume).toHaveBeenCalledOnce();
    } finally {
      unregister();
    }
  });

  it("does not borrow a mounted approval card from another root Run", () => {
    seedPending([
      {
        runId: ROOT_RUN_ID,
        interrupts: [{ itemId: "approval_same", kind: "approval" }],
      },
    ]);
    const resume = vi.fn(() => true);
    testClient.agent.projection.store.getState().setResume(SESSION_ID, resume);
    const retiredApprove = vi.fn();
    const unregister = registerApprovalActions(
      { sessionId: SESSION_ID, rootRunId: "run_retired", itemId: "approval_same" },
      {
        approve: retiredApprove,
        decline: vi.fn(),
      },
    );

    try {
      expect(submitPendingApproval(testClient.agent.dependencies, "approve")).toBe(true);
      expect(retiredApprove).not.toHaveBeenCalled();
      expect(resume).toHaveBeenCalledOnce();
    } finally {
      unregister();
    }
  });

  it("uses the mounted card only for the exact approval owner", () => {
    seedPending([
      {
        runId: ROOT_RUN_ID,
        interrupts: [{ itemId: "approval_exact", kind: "approval" }],
      },
    ]);
    const resume = vi.fn(() => true);
    testClient.agent.projection.store.getState().setResume(SESSION_ID, resume);
    const approve = vi.fn();
    const unregister = registerApprovalActions(
      { sessionId: SESSION_ID, rootRunId: ROOT_RUN_ID, itemId: "approval_exact" },
      {
        approve,
        decline: vi.fn(),
      },
    );

    try {
      expect(submitPendingApproval(testClient.agent.dependencies, "approve")).toBe(true);
      expect(approve).toHaveBeenCalledOnce();
      expect(resume).not.toHaveBeenCalled();
    } finally {
      unregister();
    }
  });
});
