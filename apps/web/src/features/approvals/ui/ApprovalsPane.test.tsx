import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { RpcError, RpcTransportError } from "@flame/runtime-client";
import { type ApprovalMode, type ApprovalModeResult } from "@/features/agent/public/approvalPolicy";
import { ApprovalsPane } from "@/features/approvals/ui/ApprovalsPane";
const model = vi.hoisted(() => ({
  query: {} as Record<string, unknown>,
  refetch: vi.fn(),
  setApprovalMode: vi.fn(),
}));

vi.mock("@/features/approvals/application/approvalConfig", () => ({
  useApprovalModeConfig: () => model.query,
}));

vi.mock("@/features/approvals/ui/RulesRow", () => ({
  RulesRow: () => <div>Remembered rules fixture</div>,
}));

vi.mock("@/features/agent/public/approvalPolicy", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agent/public/approvalPolicy")>()),
  setApprovalMode: model.setApprovalMode,
}));

const LOAD_FAILED = "Couldn't load approval mode";
const UNAVAILABLE = "Approval controls unavailable";
const UNAVAILABLE_SUB = "This runtime doesn't expose approval-mode control.";

beforeEach(() => {
  model.refetch.mockReset();
  model.refetch.mockResolvedValue(undefined);
  model.setApprovalMode.mockReset();
});

afterEach(cleanup);

describe("ApprovalsPane", () => {
  it("reports a Runtime internal error as a failed read that can be retried", () => {
    model.query = failed(new RpcError({ message: "internal", data: { type: "internal_error" } }));
    testClient.render(<ApprovalsPane />);

    expect(screen.getByText(LOAD_FAILED)).toBeTruthy();
    expect(screen.queryByText(UNAVAILABLE)).toBeNull();
    expect(screen.queryByText(UNAVAILABLE_SUB)).toBeNull();
    expect(screen.queryAllByRole("radio")).toHaveLength(0);

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(model.refetch).toHaveBeenCalledOnce();
  });

  it("keeps an HTTP 404 retryable instead of claiming the method is missing", () => {
    model.query = failed(new RpcTransportError("not found", 404));
    testClient.render(<ApprovalsPane />);

    expect(screen.getByText(LOAD_FAILED)).toBeTruthy();
    expect(screen.queryByText(UNAVAILABLE)).toBeNull();
    expect(screen.queryByText(UNAVAILABLE_SUB)).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(model.refetch).toHaveBeenCalledOnce();
  });

  it("explains a missing approval-mode method without offering retry or choices", () => {
    model.query = failed(
      new RpcError({ message: "method not found", data: { type: "method_not_found" } }),
    );
    testClient.render(<ApprovalsPane />);

    expect(screen.getByText(UNAVAILABLE)).toBeTruthy();
    expect(screen.getByText(UNAVAILABLE_SUB)).toBeTruthy();
    expect(screen.queryByText(LOAD_FAILED)).toBeNull();
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
    expect(screen.queryAllByRole("radio")).toHaveLength(0);
    expect(screen.getByText("Remembered rules fixture")).toBeTruthy();
  });

  it("replaces the failure with the accepted mode once a retried read succeeds", () => {
    model.query = failed(new RpcError({ message: "internal", data: { type: "internal_error" } }));
    const view = testClient.render(<ApprovalsPane />);
    expect(screen.getByText(LOAD_FAILED)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    model.query = accepted(approval("balanced"));
    view.rerender(<ApprovalsPane />);

    expect(screen.queryByText(LOAD_FAILED)).toBeNull();
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
    expect(screen.getByRole("radio", { name: "Safe" }).getAttribute("aria-checked")).toBe("false");
    expect(screen.getByRole("radio", { name: "Balanced" }).getAttribute("aria-checked")).toBe(
      "true",
    );
    expect(screen.getByRole("radio", { name: "Auto" }).getAttribute("aria-checked")).toBe("false");
    expect(model.refetch).toHaveBeenCalledOnce();
    expect(model.setApprovalMode).not.toHaveBeenCalled();
  });

  it("shows the mode row loading while no read has settled", () => {
    model.query = {
      data: undefined,
      error: null,
      isError: false,
      isLoading: true,
      isPending: true,
      isSuccess: false,
      status: "pending",
      refetch: model.refetch,
    };
    testClient.render(<ApprovalsPane />);

    expect(screen.getByText("Mode")).toBeTruthy();
    expect(screen.getByText("Loading")).toBeTruthy();
    expect(screen.queryByText(UNAVAILABLE)).toBeNull();
    expect(screen.queryByText(LOAD_FAILED)).toBeNull();
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
    expect(screen.queryAllByRole("radio")).toHaveLength(0);
  });
});

function failed(error: unknown): Record<string, unknown> {
  return {
    data: undefined,
    error,
    isError: true,
    isLoading: false,
    isPending: false,
    isSuccess: false,
    status: "error",
    refetch: model.refetch,
  };
}

function accepted(data: ApprovalModeResult): Record<string, unknown> {
  return {
    data,
    error: null,
    isError: false,
    isLoading: false,
    isPending: false,
    isSuccess: true,
    status: "success",
    refetch: model.refetch,
  };
}

function approval(mode: ApprovalMode): ApprovalModeResult {
  return {
    mode,
    modes: [
      { mode: "safe", write: "prompt", exec: "prompt", network: "deny" },
      { mode: "balanced", write: "pass", exec: "prompt", network: "prompt" },
      { mode: "yolo", write: "pass", exec: "pass", network: "pass" },
    ],
  };
}
