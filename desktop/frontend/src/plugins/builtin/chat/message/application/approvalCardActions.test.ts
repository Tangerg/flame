import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  approvalSubmitOptions,
  canRegisterApprovalActions,
  useApprovalCardActions,
} from "./approvalCardActions";

type RegisteredApprovalActions = {
  approve: () => void;
  decline: () => void;
};

const hitl = vi.hoisted(() => ({
  registerActions: vi.fn<(actions: RegisteredApprovalActions) => () => void>(() => () => undefined),
  submit: vi.fn(),
}));

vi.mock("@/plugins/builtin/agent/public/hitl", () => ({
  useApprovalSubmit: () => ({
    pending: null,
    registerActions: hitl.registerActions,
    submit: hitl.submit,
  }),
}));

vi.mock("@/plugins/builtin/agent/public/messagePresentation", () => ({
  canSubmitApproval: () => true,
}));

afterEach(() => vi.clearAllMocks());

describe("approvalSubmitOptions", () => {
  it("omits the options object when approval has no extra payload", () => {
    expect(approvalSubmitOptions({})).toBeUndefined();
  });

  it("preserves edited args and remember scope", () => {
    expect(approvalSubmitOptions({ editedArgs: {}, rememberScope: "project" })).toEqual({
      editedArgs: {},
      rememberScope: "project",
    });
  });
});

describe("canRegisterApprovalActions", () => {
  it("registers shortcuts only while the approval awaits an answer", () => {
    expect(canRegisterApprovalActions({ resumeRunId: "run", itemId: "item" })).toBe(true);
    expect(canRegisterApprovalActions({ resumeRunId: undefined, itemId: "item" })).toBe(false);
    expect(
      canRegisterApprovalActions({ resumeRunId: "run", itemId: "item", runtimeAvailable: false }),
    ).toBe(false);
  });
});

describe("useApprovalCardActions", () => {
  it("attaches a remembered scope only to the explicitly scoped approval", () => {
    const argsEditor = { commit: vi.fn(() => ({ path: "/safe" })) };
    const { result } = renderHook(() =>
      useApprovalCardActions({
        resumeRunId: "run",
        itemId: "item",
        argsEditor,
        runtimeAvailable: true,
      }),
    );

    act(() => result.current.approve("project"));
    expect(hitl.submit).toHaveBeenLastCalledWith("approved", {
      editedArgs: { path: "/safe" },
      rememberScope: "project",
    });

    act(() => result.current.decline());
    expect(hitl.submit).toHaveBeenLastCalledWith("declined");
  });

  it("keeps the registered keyboard approval one-shot", () => {
    renderHook(() =>
      useApprovalCardActions({
        resumeRunId: "run",
        itemId: "item",
        runtimeAvailable: true,
      }),
    );

    const registered = hitl.registerActions.mock.calls.at(-1)?.[0];
    act(() => registered?.approve());
    expect(hitl.submit).toHaveBeenLastCalledWith("approved", undefined);
  });
});
